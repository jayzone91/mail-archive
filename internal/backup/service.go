package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	protocol "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/jayzone91/mail-archive/internal/accounts"
	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/imap"
)

type Service struct {
	DB       *sql.DB
	Accounts accounts.Store
	Archive  archive.Store
}

func (s Service) run(ctx context.Context, job int64, id, mode string) error {
	store, release := s.Archive.Pin()
	defer release()
	s.Archive = store
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM backup_folder_checks WHERE job_id=?", job); err != nil {
		return err
	}
	account, err := s.Accounts.Get(ctx, id)
	if err != nil {
		return err
	}
	if !account.Enabled {
		return errors.New("Postfach ist deaktiviert.")
	}
	password, err := s.Accounts.Password(ctx, id)
	if err != nil {
		return err
	}
	defer clear(password)
	client, closeClient, err := imap.Connect(ctx, account.Endpoint, password)
	if err != nil {
		return err
	}
	defer closeClient()
	folders, err := client.List("", "*", nil).Collect()
	if err != nil {
		return fmt.Errorf("IMAP-LIST: %w", err)
	}
	var active []int64
	var failures []error
	for _, folder := range folders {
		if slices.Contains(folder.Attrs, protocol.MailboxAttrNoSelect) {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		folderID, err := s.folder(ctx, client, job, id, mode, folder)
		if err != nil {
			failures = append(failures, fmt.Errorf("Ordner %q: %w", folder.Mailbox, err))
			continue
		}
		active = append(active, folderID)
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	if err := s.reconcileFolders(ctx, id, active); err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, "UPDATE jobs SET source_checked=1 WHERE id=?", job)
	return err
}

func (s Service) folder(ctx context.Context, client *imapclient.Client, job int64, account, mode string, folder *protocol.ListData) (int64, error) {
	selected, err := client.Select(folder.Mailbox, &protocol.SelectOptions{ReadOnly: true}).Wait()
	if err != nil {
		return 0, err
	}
	if selected.UIDValidity == 0 {
		return 0, errors.New("IMAP-Ordner hat keine gültige UIDVALIDITY.")
	}
	attributes, err := json.Marshal(folder.Attrs)
	if err != nil {
		return 0, err
	}
	var folderID int64
	err = s.DB.QueryRowContext(ctx, `INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next)
  VALUES (?,?,?,?,?,?) ON CONFLICT(account_id,name) DO UPDATE SET delimiter=excluded.delimiter,
  attributes=excluded.attributes,uid_validity=excluded.uid_validity,uid_next=excluded.uid_next,present=1
  RETURNING id`, account, folder.Mailbox, string(folder.Delim), string(attributes), selected.UIDValidity, selected.UIDNext).Scan(&folderID)
	if err != nil {
		return 0, err
	}
	result, err := client.UIDSearch(&protocol.SearchCriteria{}, nil).Wait()
	if err != nil {
		return 0, err
	}
	uids := result.AllUIDs()
	var missing []protocol.UID
	for _, uid := range uids {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		meta, err := messageMetadata(client, uid)
		if err != nil {
			return 0, err
		}
		if meta == nil {
			missing = append(missing, uid)
			continue
		}
		if err := s.message(ctx, client, job, folderID, selected.UIDValidity, mode, meta); errors.Is(err, errMessageGone) {
			missing = append(missing, uid)
			continue
		} else if err != nil {
			return 0, fmt.Errorf("UID %d: %w", uid, err)
		}
	}
	// Search again after downloads to account for messages removed while the folder was read.
	current, err := client.UIDSearch(&protocol.SearchCriteria{}, nil).Wait()
	if err != nil {
		return 0, err
	}
	currentUIDs := current.AllUIDs()
	for _, uid := range missing {
		if slices.Contains(currentUIDs, uid) {
			return 0, fmt.Errorf("UID %d ist vorhanden, konnte aber nicht vollständig abgerufen werden", uid)
		}
	}
	if err := s.reconcileMessages(ctx, folderID, selected.UIDValidity, currentUIDs); err != nil {
		return 0, err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO backup_folder_checks(job_id,folder_id,seen,covered)
	 SELECT ?,?,?,count(*) FROM messages m JOIN backup_checkpoints c ON c.message_id=m.id
	 WHERE c.job_id=? AND m.folder_id=? AND m.uid_validity=? AND m.present=1
	 ON CONFLICT(job_id,folder_id) DO UPDATE SET seen=excluded.seen,covered=excluded.covered`,
		job, folderID, len(currentUIDs), job, folderID, selected.UIDValidity)
	return folderID, err
}

func messageMetadata(client *imapclient.Client, uid protocol.UID) (*imapclient.FetchMessageBuffer, error) {
	cmd := client.Fetch(protocol.UIDSetNum(uid), &protocol.FetchOptions{UID: true, Flags: true, InternalDate: true, RFC822Size: true, Envelope: true})
	values, err := cmd.Collect()
	if err != nil || len(values) == 0 {
		return nil, err
	}
	if len(values) != 1 || values[0].UID != uid || values[0].RFC822Size < 1 || values[0].InternalDate.IsZero() {
		return nil, errors.New("IMAP-Nachrichtenmetadaten fehlen oder sind unvollständig.")
	}
	return values[0], nil
}

var errMessageGone = errors.New("IMAP-Nachricht ist während der Sicherung verschwunden.")

func (s Service) message(ctx context.Context, client *imapclient.Client, job, folderID int64, validity uint32, mode string, meta *imapclient.FetchMessageBuffer) error {
	var object archive.Object
	var checkpoint bool
	err := s.DB.QueryRowContext(ctx, `SELECT digest,size,EXISTS(SELECT 1 FROM backup_checkpoints
	 WHERE job_id=? AND message_id=messages.id) FROM messages WHERE folder_id=? AND uid_validity=? AND uid=?`,
		job, folderID, validity, meta.UID).Scan(&object.Digest, &object.Size, &checkpoint)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if (mode == "full" && !checkpoint) || err != nil || object.Size != meta.RFC822Size || s.Archive.Verify(object) != nil {
		previous := object.Digest
		object, err = s.fetch(client, meta.UID, meta.RFC822Size)
		if err != nil {
			return err
		}
		if object.Digest == "" {
			return errMessageGone
		}
		if previous != "" && previous != object.Digest {
			return errors.New("Der Server liefert anderen Inhalt für eine bereits archivierte UID.")
		}
	}
	slices.Sort(meta.Flags)
	flags, err := json.Marshal(meta.Flags)
	if err != nil {
		return err
	}
	envelope, err := json.Marshal(meta.Envelope)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var messageID int64
	err = tx.QueryRowContext(ctx, `INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
 VALUES (?,?,?,?,?,?,?,?)
 ON CONFLICT(folder_id,uid_validity,uid) DO UPDATE SET flags=excluded.flags,present=1
 RETURNING id`, folderID, validity, meta.UID, object.Digest, object.Size, meta.InternalDate.UTC().Format(time.RFC3339Nano), string(flags), string(envelope)).Scan(&messageID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO message_flag_history(message_id,flags)
  SELECT ?,? WHERE ? IS NOT (SELECT flags FROM message_flag_history WHERE message_id=? ORDER BY id DESC LIMIT 1)`,
		messageID, string(flags), string(flags), messageID)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO backup_checkpoints VALUES (?,?) ON CONFLICT DO NOTHING", job, messageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE jobs SET processed=processed+1 WHERE id=?", job); err != nil {
		return err
	}
	return tx.Commit()
}

func (s Service) fetch(client *imapclient.Client, uid protocol.UID, size int64) (archive.Object, error) {
	command := client.Fetch(protocol.UIDSetNum(uid), &protocol.FetchOptions{UID: true, BodySection: []*protocol.FetchItemBodySection{{Peek: true}}})
	defer command.Close()
	message := command.Next()
	if message == nil {
		return archive.Object{}, command.Close()
	}
	var object archive.Object
	var received protocol.UID
	for item := message.Next(); item != nil; item = message.Next() {
		switch item := item.(type) {
		case imapclient.FetchItemDataUID:
			received = item.UID
		case imapclient.FetchItemDataBodySection:
			if item.Literal == nil {
				continue
			}
			if item.Section == nil || item.Section.Specifier != protocol.PartSpecifierNone ||
				len(item.Section.Part) != 0 || item.Section.Partial != nil || item.Literal.Size() != size {
				return archive.Object{}, errors.New("IMAP lieferte einen unerwarteten Nachrichtenausschnitt oder eine abweichende Größe.")
			}
			var err error
			object, err = s.Archive.Put(item.Literal, size)
			if err != nil {
				return archive.Object{}, err
			}
		}
	}
	if err := command.Close(); err != nil {
		return archive.Object{}, err
	}
	if received != uid || object.Digest == "" {
		return archive.Object{}, errors.New("IMAP lieferte keine vollständige Originalnachricht.")
	}
	return object, nil
}
