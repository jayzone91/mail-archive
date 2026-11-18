package archive

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jayzone91/mail-archive/internal/platform"
)

type Store struct {
	Root            string
	location        *Location
	requireExisting bool
}

// Pin keeps the selected storage location stable until release.
func (s Store) Pin() (Store, func()) {
	if s.location == nil {
		return s, func() {}
	}
	s.location.mu.RLock()
	root := s.location.Current()
	return Store{Root: root, requireExisting: !equalPath(root, filepath.Join(s.location.dataDir, "archive"))}, s.location.mu.RUnlock
}

type Object struct {
	Digest string
	Size   int64
}

var ErrIntegrity = errors.New("Archivdatei stimmt nicht mit Größe oder SHA-256-Prüfsumme überein.")

func (s Store) path(digest string) (string, error) {
	if len(digest) != sha256.Size*2 {
		return "", ErrIntegrity
	}
	raw, err := hex.DecodeString(digest)
	if err != nil || digest != hex.EncodeToString(raw) {
		return "", ErrIntegrity
	}
	return filepath.Join(s.Root, digest+".eml"), nil
}

func (s Store) Open(digest string) (*os.File, error) {
	if s.location != nil {
		s.Root = s.location.Current()
	}
	path, err := s.path(digest)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s Store) Verify(object Object) error {
	file, err := s.Open(object.Digest)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return err
	}
	if size != object.Size || hex.EncodeToString(hash.Sum(nil)) != object.Digest {
		return ErrIntegrity
	}
	return nil
}

// Put publishes the original bytes only after the complete expected literal has arrived.
func (s Store) Put(source io.Reader, expected int64) (Object, error) {
	return s.put(source, expected, "")
}

func (s Store) put(source io.Reader, expected int64, digest string) (Object, error) {
	s, release := s.Pin()
	defer release()
	if expected < 1 {
		return Object{}, errors.New("IMAP-Nachrichtengröße fehlt oder ist ungültig.")
	}
	if s.requireExisting {
		info, err := os.Stat(s.Root)
		if err != nil {
			return Object{}, err
		}
		if !info.IsDir() {
			return Object{}, ErrMoveTarget
		}
	} else {
		if err := os.MkdirAll(s.Root, 0700); err != nil {
			return Object{}, err
		}
	}
	temp, err := os.CreateTemp(s.Root, ".incoming-*")
	if err != nil {
		return Object{}, err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(temp, hash), source)
	if err != nil {
		return Object{}, err
	}
	if size != expected {
		return Object{}, fmt.Errorf("Unvollständige EML-Datei: %d von %d Bytes", size, expected)
	}
	if err := temp.Sync(); err != nil {
		return Object{}, err
	}
	if err := temp.Close(); err != nil {
		return Object{}, err
	}

	object := Object{Digest: hex.EncodeToString(hash.Sum(nil)), Size: size}
	if digest != "" && object.Digest != digest {
		return Object{}, ErrIntegrity
	}
	if err := s.Verify(object); err == nil {
		return object, nil
	}
	path, err := s.path(object.Digest)
	if err != nil {
		return Object{}, err
	}
	if err := platform.PublishFile(temp.Name(), path); err != nil {
		if s.Verify(object) == nil {
			return object, nil
		}
		return Object{}, err
	}
	if err := s.Verify(object); err != nil {
		return Object{}, err
	}
	return object, nil
}
