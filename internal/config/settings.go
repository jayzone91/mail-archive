package config

import (
	"context"
	"database/sql"
	"errors"
)

type Settings struct {
	BackupHours, RetentionMonths, CleanupHours, Workers, Retries int
	AutomaticBackup                                              bool
}

func Load(ctx context.Context, db *sql.DB) (Settings, error) {
	var s Settings
	err := db.QueryRowContext(ctx, "SELECT backup_hours, retention_months, cleanup_hours, workers, retries, automatic_backup FROM settings WHERE id = 1").
		Scan(&s.BackupHours, &s.RetentionMonths, &s.CleanupHours, &s.Workers, &s.Retries, &s.AutomaticBackup)
	return s, err
}

func (s Settings) Validate() error {
	if s.BackupHours < 1 || s.BackupHours > 8760 || s.CleanupHours < 1 || s.CleanupHours > 8760 ||
		s.RetentionMonths < 1 || s.RetentionMonths > 1200 || s.Workers < 1 || s.Workers > 64 ||
		s.Retries < 0 || s.Retries > 30 {
		return errors.New("Mindestens ein Wert liegt außerhalb des zulässigen Bereichs.")
	}
	return nil
}

func (s Settings) Save(ctx context.Context, db *sql.DB) error {
	if err := s.Validate(); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE settings SET backup_hours = ?, retention_months = ?, cleanup_hours = ?,
  workers = ?, retries = ?, automatic_backup=?, updated_at = CURRENT_TIMESTAMP WHERE id = 1`,
		s.BackupHours, s.RetentionMonths, s.CleanupHours, s.Workers, s.Retries, s.AutomaticBackup); err != nil {
		return err
	}
	if !s.AutomaticBackup {
		if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status='failed',error='Automatische Sicherung deaktiviert.'
		 WHERE scheduled=1 AND status='queued'`); err != nil {
			return err
		}
	}
	return tx.Commit()
}
