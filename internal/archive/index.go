package archive

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	protocol "github.com/emersion/go-imap/v2"
	"github.com/jayzone91/mail-archive/internal/database"
)

func (s Store) Index(ctx context.Context, db *sql.DB, log *slog.Logger) {
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	busyFailures := 0
	for ctx.Err() == nil {
		store, release := s.Pin()
		count, err := store.indexBatch(ctx, db)
		release()
		if database.IsBusy(err) {
			busyFailures++
			if busyFailures == 3 && ctx.Err() == nil {
				log.Warn("Archivindex wartet weiterhin auf SQLite. Offene Einträge werden erneut versucht.", "error", err)
			}
		} else {
			busyFailures = 0
		}
		if err != nil && busyFailures == 0 && ctx.Err() == nil {
			log.Error("Archivindex aktualisieren", "error", err)
		}
		if err == nil && count > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}

func (s Store) indexBatch(ctx context.Context, db *sql.DB) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT messages.id,digest,envelope FROM archive_search_pending
 JOIN messages ON messages.id=archive_search_pending.message_id
 ORDER BY archive_search_pending.message_id LIMIT 25`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type candidate struct {
		id           int64
		digest, data string
	}
	var batch []candidate
	for rows.Next() {
		var entry candidate
		if err := rows.Scan(&entry.id, &entry.digest, &entry.data); err != nil {
			return 0, err
		}
		batch = append(batch, entry)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()
	for _, entry := range batch {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		var envelope protocol.Envelope
		if err := json.Unmarshal([]byte(entry.data), &envelope); err != nil {
			return 0, err
		}
		body := ""
		file, err := s.Open(entry.digest)
		if err == nil {
			body = SearchText(file)
			file.Close()
		}
		sender := Addresses(envelope.From)
		recipient := Addresses(envelope.To) + " " + Addresses(envelope.Cc)
		if err := indexMessage(ctx, db, entry.id, sender, recipient, envelope.Subject, body); err != nil {
			return 0, err
		}
	}
	return len(batch), nil
}

func indexMessage(ctx context.Context, db *sql.DB, id int64, sender, recipient, subject, body string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO archive_search(rowid,sender,recipient,subject,body)
	 VALUES (?,?,?,?,?)`, id, sender, recipient, subject, body); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM archive_search_pending WHERE message_id=?", id); err != nil {
		return err
	}
	return tx.Commit()
}

func Reindex(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM archive_search"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO archive_search_pending SELECT id FROM messages"); err != nil {
		return err
	}
	return tx.Commit()
}

func Addresses(addresses []protocol.Address) string {
	parts := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.Host != "" {
			part := address.Mailbox + "@" + address.Host
			if address.Name != "" {
				part = address.Name + " <" + part + ">"
			}
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}
