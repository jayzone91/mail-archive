package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/mail"
	"strings"

	"github.com/jayzone91/mail-archive/internal/imap"
)

type Vault interface {
	Seal(context.Context, string, []byte) ([]byte, error)
	Open(context.Context, string, []byte) ([]byte, error)
}

type Store struct {
	DB    *sql.DB
	Vault Vault
}

type Account struct {
	imap.Endpoint
	ID, Name, Email     string
	Enabled, AutoBackup bool
	NeedsPassword       bool
	RetentionMonths     int
}

var ErrInvalid = errors.New("Bitte Postfachname, E-Mail-Adresse, Benutzer, Server, Port und TLS-Modus prüfen.")
var ErrPassword = errors.New("Ein Passwort ist für dieses Postfach erforderlich.")
var ErrBusy = errors.New("Für dieses Postfach läuft oder wartet eine Sicherung oder Wiederherstellung.")
var ErrSource = errors.New("Für einen anderen IMAP-Server oder Benutzer bitte ein neues Postfach anlegen. Die Archivherkunft bleibt dem ursprünglichen Postfach zugeordnet.")

type CredentialError struct{ Err error }

func (e *CredentialError) Error() string { return e.Err.Error() }

func (a Account) Validate() error {
	address, err := mail.ParseAddress(a.Email)
	if err != nil || address.Address != a.Email || a.Name == "" || a.Username == "" ||
		a.Host == "" || strings.ContainsAny(a.Host, "/\\[] \t\r\n") ||
		a.Port < 1 || a.Port > 65535 || (a.TLSMode != "implicit" && a.TLSMode != "starttls") ||
		a.RetentionMonths < 0 || a.RetentionMonths > 1200 {
		return ErrInvalid
	}
	return nil
}

const columns = "id, name, email, username, host, port, tls_mode, enabled, auto_backup, COALESCE(retention_months, 0), credential IS NULL"

type scanner interface{ Scan(...any) error }

func read(row scanner) (Account, error) {
	var a Account
	err := row.Scan(&a.ID, &a.Name, &a.Email, &a.Username, &a.Host, &a.Port, &a.TLSMode, &a.Enabled, &a.AutoBackup, &a.RetentionMonths, &a.NeedsPassword)
	return a, err
}

func (s Store) List(ctx context.Context) ([]Account, error) {
	rows, err := s.DB.QueryContext(ctx, "SELECT "+columns+" FROM accounts WHERE removed_at IS NULL ORDER BY name, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Account
	for rows.Next() {
		account, err := read(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, account)
	}
	return result, rows.Err()
}

func (s Store) Get(ctx context.Context, id string) (Account, error) {
	return read(s.DB.QueryRowContext(ctx, "SELECT "+columns+" FROM accounts WHERE id = ? AND removed_at IS NULL", id))
}

func (s Store) Password(ctx context.Context, id string) ([]byte, error) {
	var encrypted []byte
	if err := s.DB.QueryRowContext(ctx, "SELECT credential FROM accounts WHERE id = ? AND removed_at IS NULL", id).Scan(&encrypted); err != nil {
		return nil, err
	}
	if len(encrypted) == 0 {
		return nil, ErrPassword
	}
	password, err := s.Vault.Open(ctx, id, encrypted)
	if err != nil {
		return nil, &CredentialError{err}
	}
	return password, nil
}

func (s Store) Save(ctx context.Context, a Account, password []byte) error {
	if err := a.Validate(); err != nil {
		return err
	}
	creating := a.ID == ""
	if creating {
		if len(password) == 0 {
			return ErrPassword
		}
		a.ID = rand.Text()
	}
	var encrypted []byte
	if len(password) != 0 {
		var err error
		encrypted, err = s.Vault.Seal(ctx, a.ID, password)
		if err != nil {
			return &CredentialError{err}
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	values := []any{a.Name, a.Email, a.Username, a.Host, a.Port, a.TLSMode, a.Enabled, a.AutoBackup, a.RetentionMonths, encrypted, a.ID}
	if creating {
		_, err = tx.ExecContext(ctx, "INSERT INTO accounts(name,email,username,host,port,tls_mode,enabled,auto_backup,retention_months,credential,id) VALUES (?,?,?,?,?,?,?,?,NULLIF(?,0),?,?)", values...)
	} else {
		if err := checkIdle(ctx, tx, a.ID); err != nil {
			return err
		}
		var differentSource bool
		if err := tx.QueryRowContext(ctx, `SELECT (host COLLATE NOCASE <> ? OR username <> ? OR port <> ?) AND
			(EXISTS(SELECT 1 FROM folders WHERE account_id=accounts.id) OR EXISTS(SELECT 1 FROM restore_history WHERE target=accounts.id))
			FROM accounts WHERE id=? AND removed_at IS NULL`,
			a.Host, a.Username, a.Port, a.ID).Scan(&differentSource); err != nil {
			return err
		}
		if differentSource {
			return ErrSource
		}
		var result sql.Result
		result, err = tx.ExecContext(ctx, "UPDATE accounts SET name=?,email=?,username=?,host=?,port=?,tls_mode=?,enabled=?,auto_backup=?,retention_months=NULLIF(?,0),credential=COALESCE(?,credential),updated_at=CURRENT_TIMESTAMP WHERE id=? AND removed_at IS NULL", values...)
		if err == nil {
			err = requireRow(result)
		}
	}
	if err != nil {
		return err
	}
	action := "account_updated"
	if len(encrypted) != 0 {
		action = "account_credentials_updated"
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit_events(account_id,action) VALUES (?,?)", a.ID, action); err != nil {
		return err
	}
	return tx.Commit()
}

func (s Store) Remove(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := checkIdle(ctx, tx, id); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "UPDATE accounts SET removed_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP, enabled=0, auto_backup=0, credential=NULL WHERE id=? AND removed_at IS NULL", id)
	if err != nil {
		return err
	}
	if err := requireRow(result); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO audit_events(account_id,action) VALUES (?,'account_removed')", id); err != nil {
		return err
	}
	return tx.Commit()
}

func requireRow(result sql.Result) error {
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

func checkIdle(ctx context.Context, tx *sql.Tx, id string) error {
	var active bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE account_id=? AND status IN ('queued','running'))
		OR EXISTS(SELECT 1 FROM restore_jobs WHERE target=? AND status IN ('queued','running'))`, id, id).Scan(&active)
	if err == nil && active {
		return ErrBusy
	}
	return err
}
