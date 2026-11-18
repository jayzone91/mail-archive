package archive

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestStorePreservesBytesAndRejectsPartialWrites(t *testing.T) {
	store := Store{Root: t.TempDir()}
	const raw = "Subject: Example\r\n\r\nOriginal content\r\n"
	size := int64(len(raw))
	if _, err := store.Put(strings.NewReader(raw), size+1); err == nil {
		t.Fatal("Teilweise Nachricht gespeichert")
	}
	entries, err := os.ReadDir(store.Root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("Artefakte nach Fehler: %v, %v", entries, err)
	}
	one, err := store.Put(strings.NewReader(raw), size)
	if err != nil {
		t.Fatal(err)
	}
	filepath, err := store.path(one.Digest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath)
	if err != nil || string(data) != raw {
		t.Fatalf("Originaldaten: %q, %v", data, err)
	}
	two, err := store.Put(strings.NewReader(raw), size)
	if err != nil || two != one {
		t.Fatalf("Doppeltes Objekt: %v, %v", two, err)
	}
	if err := os.WriteFile(filepath, []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(one); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("Defekt nicht erkannt: %v", err)
	}
	if _, err := store.Put(strings.NewReader(raw), size); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify(one); err != nil {
		t.Fatal(err)
	}
}
