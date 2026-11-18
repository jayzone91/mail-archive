package snapshot

import (
	"archive/tar"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"filippo.io/age"
	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/database"
	"github.com/jayzone91/mail-archive/internal/platform"
)

var ErrExistingInstallation = errors.New("Das Datenverzeichnis existiert bereits. Für den Import ein neues, noch nicht vorhandenes Verzeichnis angeben.")
var ErrPortableDecrypt = errors.New("Das Sicherungspaket konnte nicht entschlüsselt werden. Sicherungspasswort und Datei prüfen.")

// ImportPortable verifies and stages a complete installation before publishing
// the new data directory. The destination must not exist.
func ImportPortable(ctx context.Context, input io.Reader, password, snapshotDir, dataDir string) error {
	if !filepath.IsAbs(dataDir) {
		return errors.New("Für den Import ist ein absoluter Datenpfad erforderlich.")
	}
	dataDir = filepath.Clean(dataDir)
	parent, err := filepath.EvalSymlinks(filepath.Dir(dataDir))
	if err != nil {
		return fmt.Errorf("Übergeordnetes Datenverzeichnis: %w", err)
	}
	dataDir = filepath.Join(parent, filepath.Base(dataDir))
	if _, err := os.Lstat(dataDir); err == nil {
		return ErrExistingInstallation
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".mailarchive-import-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := platform.PrivateDir(stage); err != nil {
		return err
	}

	imagePath := filepath.Join(stage, "mailarchive.sqlite")
	header, err := unpackPortable(ctx, input, password, imagePath)
	if err != nil {
		return err
	}
	image, err := describe(ctx, imagePath)
	if err != nil {
		return err
	}
	if image.Database != header.Database {
		return ErrPortableFormat
	}
	second, err := Verify(ctx, snapshotDir)
	if err != nil {
		return fmt.Errorf("Zweitsicherung prüfen: %w", err)
	}
	if second.Database != header.Snapshot || !slices.Equal(image.Objects, second.Objects) {
		return ErrPortableSnapshot
	}

	source := archive.Store{Root: filepath.Join(snapshotDir, "archive")}
	destination := archive.Store{Root: filepath.Join(stage, "archive")}
	for _, object := range image.Objects {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyObject(source, destination, object); err != nil {
			return fmt.Errorf("EML-Original übernehmen: %w", err)
		}
	}
	if err := resetImportedInstallation(ctx, imagePath); err != nil {
		return err
	}
	return platform.PublishNewDirectory(stage, dataDir)
}

func unpackPortable(ctx context.Context, input io.Reader, password, target string) (portableManifest, error) {
	var manifest portableManifest
	identity, err := age.NewScryptIdentity(password)
	if err != nil {
		return manifest, ErrPortableDecrypt
	}
	cleartext, err := age.Decrypt(portableReader{ctx, input}, identity)
	if err != nil {
		return manifest, ErrPortableDecrypt
	}
	pack := tar.NewReader(cleartext)
	header, err := pack.Next()
	if err != nil || header.Typeflag != tar.TypeReg || header.Name != "manifest.json" || header.Size > 4096 {
		return manifest, ErrPortableFormat
	}
	payload, err := io.ReadAll(pack)
	if err != nil || json.Unmarshal(payload, &manifest) != nil || manifest.Version != 1 || manifest.Database.Size < 1 {
		return manifest, ErrPortableFormat
	}
	header, err = pack.Next()
	if err != nil || header.Typeflag != tar.TypeReg || header.Name != "mailarchive.sqlite" || header.Size != manifest.Database.Size {
		return manifest, ErrPortableFormat
	}
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return manifest, err
	}
	_, copyErr := io.Copy(file, portableReader{ctx, pack})
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return manifest, err
	}
	if _, err := pack.Next(); !errors.Is(err, io.EOF) {
		return manifest, ErrPortableFormat
	}
	// Consume to authenticated EOF, including anything after the tar end marker.
	if n, err := io.Copy(io.Discard, cleartext); err != nil || n != 0 {
		return manifest, ErrPortableFormat
	}
	return manifest, nil
}

func resetImportedInstallation(ctx context.Context, path string) error {
	db, err := database.Open(ctx, path)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var admin bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM administrator WHERE id=1)").Scan(&admin); err != nil {
		return err
	}
	if !admin {
		return errors.New("Das Sicherungspaket enthält kein Administratorkonto.")
	}
	_, err = tx.ExecContext(ctx, `UPDATE settings SET archive_path='',automatic_backup=0,snapshot_target='',snapshot_requested=0;
 UPDATE accounts SET credential=NULL;
 UPDATE jobs SET status='failed',error='Auftrag durch Hardwarewechsel angehalten.',finished_at=CURRENT_TIMESTAMP WHERE status IN ('queued','running');
 UPDATE restore_jobs SET status='failed',finished_at=CURRENT_TIMESTAMP WHERE status IN ('queued','running');
 UPDATE backup_snapshots SET status='failed',error='Auftrag durch Hardwarewechsel angehalten.',finished_at=CURRENT_TIMESTAMP WHERE status IN ('queued','running');
 UPDATE backup_snapshots SET coverage_recorded=0,retry_at=NULL;
 DELETE FROM snapshot_message_coverage;
 UPDATE archive_moves SET status='failed',error='Auftrag durch Hardwarewechsel angehalten.' WHERE status IN ('queued','running');`)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	var busy, frames, checkpointed int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &checkpointed); err != nil {
		return err
	}
	if busy != 0 {
		return errors.New("Die importierte Datenbank konnte nicht vollständig synchronisiert werden.")
	}
	return db.Close()
}
