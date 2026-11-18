//go:build windows || linux

package platform

import (
	"path/filepath"
	"testing"
)

func TestLockData(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := PrivateDir(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "instance.lock")
	lock, err := LockData(path)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := LockData(path); err == nil {
		second.Close()
		lock.Close()
		t.Fatal("Zweite Instanz konnte das Datenverzeichnis sperren")
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	lock, err = LockData(path)
	if err != nil {
		t.Fatal(err)
	}
	lock.Close()
}

func TestSharesStorage(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	same, err := SharesStorage(first, second)
	if err != nil || !same {
		t.Fatalf("Zwei Verzeichnisse auf demselben Volume: %v, %v", same, err)
	}
}
