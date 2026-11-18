package restore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	protocol "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/jayzone91/mail-archive/internal/archive"
)

type item struct {
	ID                  int64
	Folder, Date, Flags string
	Object              archive.Object
}

var errReview = errors.New("Der letzte Upload oder die Zielzuordnung ist unklar. MailArchive lädt die Nachricht erst nach eindeutiger Zuordnung erneut hoch.")
var errMissing = errors.New("Die Zielnachricht konnte nicht bytegenau verifiziert werden.")
var errMetadata = errors.New("Das Ziel enthält die Originalnachricht, aber das Datum oder die unterstützten Flags weichen ab.")

func (s Service) restore(ctx context.Context, client *imapclient.Client, job int64, target string, keepDate, keepFlags bool) error {
	folders, err := client.List("", "*", nil).Collect()
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(folders))
	var delimiter rune
	for _, folder := range folders {
		known[folder.Mailbox] = true
		if strings.EqualFold(folder.Mailbox, "INBOX") {
			known["INBOX"] = true
			delimiter = folder.Delim
		}
	}
	if !known["INBOX"] {
		root, err := client.List("", "", nil).Collect()
		if err != nil {
			return err
		}
		if len(root) != 0 {
			delimiter = root[0].Delim
		}
	}
	var last int64
	var current string
	var selected *protocol.SelectData
	var incomplete bool
	for {
		var value item
		var original bool
		var sourceDelimiter string
		var mapped sql.NullString
		err := s.DB.QueryRowContext(ctx, `SELECT m.id,i.folder,m.internal_date,m.flags,m.digest,m.size,i.original_path,f.delimiter,i.mapped_folder
   FROM restore_items i JOIN messages m ON m.id=i.message_id JOIN folders f ON f.id=m.folder_id
   WHERE i.job_id=? AND i.status='queued' AND m.id>? ORDER BY m.id LIMIT 1`, job, last).
			Scan(&value.ID, &value.Folder, &value.Date, &value.Flags, &value.Object.Digest, &value.Object.Size, &original, &sourceDelimiter, &mapped)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		last = value.ID
		if mapped.Valid {
			value.Folder = mapped.String
		} else {
			value.Folder, err = mapFolder(value.Folder, sourceDelimiter, delimiter, original)
			if err == nil {
				if _, err := s.DB.ExecContext(ctx, "UPDATE restore_items SET mapped_folder=? WHERE job_id=? AND message_id=?", value.Folder, job, value.ID); err != nil {
					return err
				}
			}
		}
		if err == nil && (current != value.Folder || selected == nil) {
			current = value.Folder
			selected, err = ensureFolder(client, known, current)
		}
		if err == nil {
			err = s.transfer(ctx, client, target, value, selected, keepDate, keepFlags)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		status, detail := "verified", ""
		if err != nil {
			incomplete = true
			status, detail = "failed", err.Error()
			if errors.Is(err, errReview) || errors.Is(err, errMissing) || errors.Is(err, errMetadata) {
				status = "review"
			}
		}
		if _, err := s.DB.ExecContext(ctx, "UPDATE restore_items SET status=?,error=? WHERE job_id=? AND message_id=?", status, detail, job, value.ID); err != nil {
			return err
		}
	}
	if incomplete {
		return errors.New("Einzelne Nachrichten konnten nicht wiederhergestellt oder verifiziert werden.")
	}
	var pending int
	if err := s.DB.QueryRowContext(ctx, "SELECT count(*) FROM restore_items WHERE job_id=? AND status<>'verified'", job).Scan(&pending); err != nil {
		return err
	}
	if pending > 0 {
		return errors.New("Der Auftrag enthält offene Nachrichten.")
	}
	return nil
}

func mapFolder(name, sourceDelimiter string, targetDelimiter rune, original bool) (string, error) {
	if strings.EqualFold(name, "INBOX") {
		return "INBOX", nil
	}
	if !original || sourceDelimiter == string(targetDelimiter) {
		return name, nil
	}
	if targetDelimiter != 0 && strings.ContainsRune(name, targetDelimiter) {
		return name, errors.New("Der Originalordner enthält das Hierarchietrennzeichen des Ziels. Alternativen Zielordner wählen.")
	}
	if sourceDelimiter == "" || sourceDelimiter == "\x00" || !strings.Contains(name, sourceDelimiter) {
		return name, nil
	}
	if targetDelimiter == 0 {
		return name, errors.New("Das Ziel unterstützt keine Ordnerhierarchie. Alternativen Zielordner wählen.")
	}
	if sourceDelimiter != string(targetDelimiter) {
		return strings.ReplaceAll(name, sourceDelimiter, string(targetDelimiter)), nil
	}
	return name, nil
}

func ensureFolder(client *imapclient.Client, known map[string]bool, name string) (*protocol.SelectData, error) {
	if !known[name] {
		if err := client.Create(name, nil).Wait(); err != nil {
			return nil, fmt.Errorf("Zielordner konnte nicht angelegt werden: %w", err)
		}
		known[name] = true
	}
	return client.Select(name, nil).Wait()
}

func (s Service) transfer(ctx context.Context, client *imapclient.Client, target string, value item, selected *protocol.SelectData, keepDate, keepFlags bool) error {
	if selected.UIDValidity == 0 {
		return errors.New("Zielordner hat keine gültige UIDVALIDITY.")
	}
	if err := s.Archive.Verify(value.Object); err != nil {
		return fmt.Errorf("Archivintegration prüfen: %w", err)
	}
	validity := selected.UIDValidity
	expected, err := appendOptions(value, selected, keepDate, keepFlags)
	if err != nil {
		return err
	}
	var journalValidity uint32
	var first protocol.UID
	var assigned sql.NullInt64
	err = s.DB.QueryRowContext(ctx, `SELECT validity,first_uid,target_uid FROM restore_history
  WHERE target=? AND folder=? AND message_id=?`, target, value.Folder, value.ID).Scan(&journalValidity, &first, &assigned)
	pending := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if pending && journalValidity != validity {
		return errReview
	}
	if assigned.Valid {
		matches, err := targetMatches(client, protocol.UID(assigned.Int64), value.Object)
		if err != nil {
			return err
		}
		if !matches {
			return errReview
		}
		return verifyMetadata(client, protocol.UID(assigned.Int64), expected)
	}
	matches, err := s.findMatches(ctx, client, target, value, validity, first)
	if err != nil {
		return err
	}
	if pending {
		if len(matches) != 1 {
			return errReview
		}
		return s.markAndVerify(ctx, client, target, value, matches[0], expected)
	}
	if len(matches) > 0 {
		_, err := s.DB.ExecContext(ctx, `INSERT INTO restore_history(target,folder,message_id,validity,first_uid,target_uid,restored_at)
   VALUES (?,?,?,?,0,?,CURRENT_TIMESTAMP)`, target, value.Folder, value.ID, validity, matches[0])
		if err != nil {
			return err
		}
		return verifyMetadata(client, matches[0], expected)
	}
	// Refresh UIDNEXT for every upload. A journal entry must precede any APPEND.
	current, err := client.Select(value.Folder, nil).Wait()
	if err != nil {
		return err
	}
	if current.UIDValidity != validity {
		return errReview
	}
	first = current.UIDNext
	if first == 0 {
		return errors.New("Zielordner meldet keinen gültigen UID-Bereich.")
	}
	options, err := appendOptions(value, current, keepDate, keepFlags)
	if err != nil {
		return err
	}
	source, err := s.Archive.Open(value.Object.Digest)
	if err != nil {
		return err
	}
	defer source.Close()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO restore_history(target,folder,message_id,validity,first_uid) VALUES(?,?,?,?,?)`,
		target, value.Folder, value.ID, validity, first); err != nil {
		return err
	}
	command := client.Append(value.Folder, value.Object.Size, options)
	_, copyErr := io.CopyN(command, source, value.Object.Size)
	closeErr := command.Close()
	reply, resultErr := command.Wait()
	var rejected *protocol.Error
	if errors.As(resultErr, &rejected) && (rejected.Type == protocol.StatusResponseTypeNo || rejected.Type == protocol.StatusResponseTypeBad) {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM restore_history WHERE target=? AND folder=? AND message_id=? AND target_uid IS NULL`, target, value.Folder, value.ID); err != nil {
			return err
		}
		return resultErr
	}
	if copyErr != nil || closeErr != nil || resultErr != nil {
		return errReview
	}
	if reply.UID != 0 {
		if reply.UIDValidity != validity {
			return errReview
		}
		matches, err := targetMatches(client, reply.UID, value.Object)
		if err != nil {
			return err
		}
		if !matches {
			return errMissing
		}
		return s.markAndVerify(ctx, client, target, value, reply.UID, options)
	}
	candidates, err := s.findMatches(ctx, client, target, value, validity, first)
	if err != nil {
		return err
	}
	if len(candidates) != 1 {
		return errReview
	}
	return s.markAndVerify(ctx, client, target, value, candidates[0], options)
}

func verifyMetadata(client *imapclient.Client, uid protocol.UID, expected *protocol.AppendOptions) error {
	if expected.Time.IsZero() && len(expected.Flags) == 0 {
		return nil
	}
	result, err := client.Fetch(protocol.UIDSetNum(uid), &protocol.FetchOptions{UID: true, InternalDate: !expected.Time.IsZero(), Flags: len(expected.Flags) != 0}).Collect()
	if err != nil {
		return err
	}
	if len(result) != 1 || result[0].UID != uid {
		return errMissing
	}
	if !expected.Time.IsZero() && !expected.Time.Truncate(time.Second).Equal(result[0].InternalDate.Truncate(time.Second)) {
		return errMetadata
	}
	for _, flag := range expected.Flags {
		if !slices.ContainsFunc(result[0].Flags, func(got protocol.Flag) bool { return strings.EqualFold(string(got), string(flag)) }) {
			return errMetadata
		}
	}
	return nil
}

func appendOptions(value item, selected *protocol.SelectData, keepDate, keepFlags bool) (*protocol.AppendOptions, error) {
	options := &protocol.AppendOptions{}
	if keepDate {
		date, err := time.Parse(time.RFC3339Nano, value.Date)
		if err != nil {
			return nil, err
		}
		options.Time = date
	}
	if keepFlags {
		var flags []protocol.Flag
		if err := json.Unmarshal([]byte(value.Flags), &flags); err != nil {
			return nil, err
		}
		for _, flag := range flags {
			if strings.EqualFold(string(flag), string(protocol.FlagDeleted)) || strings.EqualFold(string(flag), "\\Recent") {
				continue
			}
			supported := slices.ContainsFunc(selected.PermanentFlags, func(allowed protocol.Flag) bool {
				return strings.EqualFold(string(flag), string(allowed))
			})
			if supported || (!strings.HasPrefix(string(flag), "\\") && slices.Contains(selected.PermanentFlags, protocol.FlagWildcard)) {
				options.Flags = append(options.Flags, flag)
			}
		}
	}
	return options, nil
}

func (s Service) markAndVerify(ctx context.Context, client *imapclient.Client, target string, value item, uid protocol.UID, options *protocol.AppendOptions) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE restore_history SET target_uid=?,restored_at=CURRENT_TIMESTAMP
  WHERE target=? AND folder=? AND message_id=? AND target_uid IS NULL`, uid, target, value.Folder, value.ID)
	if err != nil {
		return err
	}
	return verifyMetadata(client, uid, options)
}

func (s Service) findMatches(ctx context.Context, client *imapclient.Client, target string, value item, validity uint32, first protocol.UID) ([]protocol.UID, error) {
	result, err := client.UIDSearch(&protocol.SearchCriteria{Larger: value.Object.Size - 1, Smaller: value.Object.Size + 1}, nil).Wait()
	if err != nil {
		return nil, err
	}
	var matches []protocol.UID
	for _, uid := range result.AllUIDs() {
		if uid < first {
			continue
		}
		var used bool
		if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM restore_history
    WHERE target=? AND folder=? AND validity=? AND target_uid=?)`, target, value.Folder, validity, uid).Scan(&used); err != nil {
			return nil, err
		}
		if used {
			continue
		}
		match, err := targetMatches(client, uid, value.Object)
		if err != nil {
			return nil, err
		}
		if match {
			matches = append(matches, uid)
		}
	}
	return matches, nil
}

func targetMatches(client *imapclient.Client, uid protocol.UID, expected archive.Object) (bool, error) {
	command := client.Fetch(protocol.UIDSetNum(uid), &protocol.FetchOptions{UID: true, BodySection: []*protocol.FetchItemBodySection{{Peek: true}}})
	defer command.Close()
	msg := command.Next()
	if msg == nil {
		return false, command.Close()
	}
	var received protocol.UID
	var match bool
	for part := msg.Next(); part != nil; part = msg.Next() {
		switch part := part.(type) {
		case imapclient.FetchItemDataUID:
			received = part.UID
		case imapclient.FetchItemDataBodySection:
			if part.Literal == nil || part.Literal.Size() != expected.Size || part.Section == nil ||
				part.Section.Specifier != protocol.PartSpecifierNone || len(part.Section.Part) != 0 || part.Section.Partial != nil {
				continue
			}
			digest := sha256.New()
			size, err := io.Copy(digest, part.Literal)
			if err != nil {
				return false, err
			}
			match = size == expected.Size && hex.EncodeToString(digest.Sum(nil)) == expected.Digest
		}
	}
	if err := command.Close(); err != nil {
		return false, err
	}
	return received == uid && match, nil
}
