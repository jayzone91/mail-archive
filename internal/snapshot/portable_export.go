package snapshot

import (
	"archive/tar"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"filippo.io/age"
	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/database"
)

const PortablePasswordMin = 12

var ErrPortablePassword = errors.New("Das Sicherungspasswort muss mindestens 12 Zeichen lang sein und darf höchstens 1024 Bytes enthalten.")
var ErrPortableSnapshot = errors.New("Der gewählte Snapshot entspricht nicht dem aktuellen EML-Bestand. Bitte zuerst einen neuen Snapshot erstellen.")
var ErrPortableFormat = errors.New("Das Sicherungspaket ist beschädigt oder hat ein unbekanntes Format.")

type portableManifest struct {
	Version  int            `json:"version"`
	Database archive.Object `json:"database"`
	Snapshot archive.Object `json:"snapshot_database"`
}

func ExportPortable(ctx context.Context, db *sql.DB, dataDir, snapshotID, password string, out io.Writer) error {
	if utf8.RuneCountInString(password) < PortablePasswordMin || len(password) > 1024 {
		return ErrPortablePassword
	}
	var target string
	if err := db.QueryRowContext(ctx, "SELECT target FROM backup_snapshots WHERE id=? AND status='verified'", snapshotID).Scan(&target); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(dataDir, ".portable-export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	working := filepath.Join(stage, "working.sqlite")
	if err := copyDatabase(ctx, db, working); err != nil {
		return err
	}
	clean, err := database.Open(ctx, working)
	if err != nil {
		return err
	}
	defer clean.Close()
	if _, err := clean.ExecContext(ctx, "UPDATE accounts SET credential=NULL"); err != nil {
		return err
	}
	imagePath := filepath.Join(stage, "mailarchive.sqlite")
	if err := copyDatabase(ctx, clean, imagePath); err != nil {
		return err
	}
	image, err := describe(ctx, imagePath)
	if err != nil {
		return err
	}
	second, err := Verify(ctx, filepath.Join(target, snapshotID))
	if err != nil {
		return fmt.Errorf("Zweitsicherung prüfen: %w", err)
	}
	if !slices.Equal(image.Objects, second.Objects) {
		return ErrPortableSnapshot
	}
	manifest, err := json.Marshal(portableManifest{Version: 1, Database: image.Database, Snapshot: second.Database})
	if err != nil {
		return err
	}
	recipient, err := age.NewScryptRecipient(password)
	if err != nil {
		return err
	}
	encrypted, err := age.Encrypt(out, recipient)
	if err != nil {
		return err
	}
	pack := tar.NewWriter(encrypted)
	if err := pack.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest))}); err != nil {
		return err
	}
	if _, err := pack.Write(manifest); err != nil {
		return err
	}
	file, err := os.Open(imagePath)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := pack.WriteHeader(&tar.Header{Name: "mailarchive.sqlite", Mode: 0600, Size: image.Database.Size}); err != nil {
		return err
	}
	if _, err := io.Copy(pack, portableReader{ctx, file}); err != nil {
		return err
	}
	if err := pack.Close(); err != nil {
		return err
	}
	return encrypted.Close()
}

type portableReader struct {
	ctx context.Context
	io.Reader
}

func (r portableReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}
