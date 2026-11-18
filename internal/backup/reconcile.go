package backup

import (
	"context"
	"encoding/json"

	protocol "github.com/emersion/go-imap/v2"
)

func (s Service) reconcileMessages(ctx context.Context, folder int64, validity uint32, uids []protocol.UID) error {
	data, err := json.Marshal(uids)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE messages SET present=(uid_validity=? AND uid IN (SELECT value FROM json_each(?)))
 WHERE folder_id=?`, validity, string(data), folder)
	return err
}

func (s Service) reconcileFolders(ctx context.Context, account string, active []int64) error {
	data, err := json.Marshal(active)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE folders SET present=(id IN (SELECT value FROM json_each(?))) WHERE account_id=?`, string(data), account); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE messages SET present=0 WHERE folder_id IN
 (SELECT id FROM folders WHERE account_id=? AND present=0)`, account); err != nil {
		return err
	}
	return tx.Commit()
}
