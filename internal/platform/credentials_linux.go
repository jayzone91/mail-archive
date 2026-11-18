package platform

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type Credentials struct {
	identity string
	mu       sync.Mutex
}

func NewCredentials(dir string) *Credentials {
	digest := sha256.Sum256([]byte(filepath.Clean(dir)))
	return &Credentials{identity: hex.EncodeToString(digest[:])}
}

func (c *Credentials) Seal(ctx context.Context, id string, plaintext []byte) ([]byte, error) {
	aead, err := c.aead(ctx, true)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nil, plaintext, []byte(id)), nil
}

func (c *Credentials) Open(ctx context.Context, id string, ciphertext []byte) ([]byte, error) {
	aead, err := c.aead(ctx, false)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, nil, ciphertext, []byte(id))
	if err != nil {
		return nil, errors.New("Zugangsdaten konnten nicht entschlüsselt werden. Linux-Schlüssel prüfen.")
	}
	return plaintext, nil
}

func (c *Credentials) aead(ctx context.Context, create bool) (cipher.AEAD, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var key []byte
	if dir := os.Getenv("CREDENTIALS_DIRECTORY"); dir != "" {
		var err error
		key, err = os.ReadFile(filepath.Join(dir, "mailarchive.key"))
		if err != nil {
			return nil, errors.New("systemd-Credential mailarchive.key fehlt oder ist nicht lesbar.")
		}
	} else {
		var err error
		key, err = c.desktopKey(ctx, create)
		if err != nil {
			return nil, err
		}
	}
	defer clear(key)
	if len(key) != 32 {
		return nil, errors.New("Der MailArchive-Schlüssel muss genau 32 Bytes lang sein.")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func (c *Credentials) desktopKey(ctx context.Context, create bool) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "secret-tool", "lookup", "application", "mailarchive", "database", c.identity).Output()
	if err == nil {
		defer clear(data)
		return base64.StdEncoding.AppendDecode(nil, bytes.TrimSpace(data))
	}
	var status *exec.ExitError
	if !create || !errors.As(err, &status) || status.ExitCode() != 1 || len(status.Stderr) != 0 {
		return nil, errors.New("Linux Secret Service ist nicht verfügbar oder der MailArchive-Schlüssel fehlt.")
	}
	key := make([]byte, 32)
	rand.Read(key)
	encoded := base64.StdEncoding.AppendEncode(nil, key)
	defer clear(encoded)
	cmd := exec.CommandContext(ctx, "secret-tool", "store", "--label=MailArchive", "application", "mailarchive", "database", c.identity)
	cmd.Stdin = bytes.NewReader(encoded)
	if err := cmd.Run(); err != nil {
		clear(key)
		return nil, errors.New("MailArchive-Schlüssel konnte nicht im Linux Secret Service gespeichert werden.")
	}
	return key, nil
}
