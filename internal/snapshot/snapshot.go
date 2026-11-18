package snapshot

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/database"
	"github.com/jayzone91/mail-archive/internal/platform"
)

type Manifest struct {
	Database archive.Object   `json:"database"`
	Objects  []archive.Object `json:"objects"`
}

// Create writes a consistent SQLite image, then copies exactly the immutable EML
// objects named by that image. An interrupted copy can reuse verified files.
func Create(ctx context.Context, db *sql.DB, source archive.Store, target, id string) (Manifest, error) {
	if filepath.Base(id) != id || id == "." || id == "" || id == ".." || strings.ContainsAny(id, "/\\:") {
		return Manifest{}, errors.New("Ungültige Snapshot-ID.")
	}
	working := filepath.Join(target, ".pending-"+id)
	published := filepath.Join(target, id)
	if _, err := os.Stat(published); err == nil {
		return Verify(ctx, published)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Manifest{}, err
	}
	if err := platform.PrivateDir(working); err != nil {
		return Manifest{}, err
	}
	image := filepath.Join(working, "mailarchive.sqlite")
	if _, err := os.Stat(image); errors.Is(err, os.ErrNotExist) {
		if err := copyDatabase(ctx, db, image); err != nil {
			return Manifest{}, err
		}
	} else if err != nil {
		return Manifest{}, err
	}
	manifest, err := describe(ctx, image)
	if err != nil {
		return Manifest{}, err
	}
	output := archive.Store{Root: filepath.Join(working, "archive")}
	for _, object := range manifest.Objects {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		if output.Verify(object) == nil {
			continue
		}
		if err := copyObject(source, output, object); err != nil {
			return Manifest{}, err
		}
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := writeManifest(working, payload); err != nil {
		return Manifest{}, err
	}
	if _, err := Verify(ctx, working); err != nil {
		return Manifest{}, err
	}
	if err := platform.PublishFile(working, published); err != nil {
		return Manifest{}, err
	}
	return Verify(ctx, published)
}

func copyDatabase(ctx context.Context, db *sql.DB, target string) error {
	temp, err := os.CreateTemp(filepath.Dir(target), ".sqlite-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err := temp.Close(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", name); err != nil {
		return err
	}
	file, err := os.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return err
	}
	return platform.PublishFile(name, target)
}

func copyObject(source, destination archive.Store, expected archive.Object) error {
	file, err := source.Open(expected.Digest)
	if err != nil {
		return err
	}
	defer file.Close()
	actual, err := destination.Put(file, expected.Size)
	if err != nil {
		return err
	}
	if actual != expected {
		return archive.ErrIntegrity
	}
	return nil
}

func writeManifest(directory string, data []byte) error {
	temp, err := os.CreateTemp(directory, ".manifest-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return platform.PublishFile(temp.Name(), filepath.Join(directory, "manifest.json"))
}

func Verify(ctx context.Context, directory string) (Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return Manifest{}, err
	}
	var expected Manifest
	if err := json.Unmarshal(raw, &expected); err != nil {
		return Manifest{}, err
	}
	actual, err := describe(ctx, filepath.Join(directory, "mailarchive.sqlite"))
	if err != nil {
		return Manifest{}, err
	}
	if actual.Database != expected.Database || len(actual.Objects) != len(expected.Objects) {
		return Manifest{}, archive.ErrIntegrity
	}
	store := archive.Store{Root: filepath.Join(directory, "archive")}
	for i, object := range actual.Objects {
		if err := ctx.Err(); err != nil {
			return Manifest{}, err
		}
		if object != expected.Objects[i] {
			return Manifest{}, archive.ErrIntegrity
		}
		if err := store.Verify(object); err != nil {
			return Manifest{}, fmt.Errorf("Snapshot-Objekt %s: %w", object.Digest, err)
		}
	}
	return actual, nil
}

func describe(ctx context.Context, path string) (Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	digest := sha256.New()
	size, err := io.Copy(digest, file)
	file.Close()
	if err != nil {
		return Manifest{}, err
	}
	manifest := Manifest{Database: archive.Object{Digest: hex.EncodeToString(digest.Sum(nil)), Size: size}, Objects: []archive.Object{}}
	db, err := database.OpenSnapshot(path)
	if err != nil {
		return Manifest{}, err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return Manifest{}, err
	}
	if integrity != "ok" {
		return Manifest{}, errors.New("SQLite-Integritätsprüfung des Snapshots fehlgeschlagen.")
	}
	rows, err := db.QueryContext(ctx, "SELECT DISTINCT digest,size FROM messages ORDER BY digest,size")
	if err != nil {
		return Manifest{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var object archive.Object
		if err := rows.Scan(&object.Digest, &object.Size); err != nil {
			return Manifest{}, err
		}
		manifest.Objects = append(manifest.Objects, object)
	}
	return manifest, rows.Err()
}
