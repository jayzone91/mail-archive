package archive

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jayzone91/mail-archive/internal/database"
)

func TestArchiveLocationMigration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dataDir := t.TempDir()
	db, err := database.Open(ctx, filepath.Join(dataDir, "database.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	location, err := OpenLocation(ctx, db, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	original := location.Current()
	store := location.Store()
	const raw = "Subject: Archive move\r\n\r\noriginal\r\n"
	object, err := store.Put(strings.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,email,username,host,port,tls_mode,enabled,auto_backup)
 VALUES ('id','Test','test@example.test','test','localhost',993,'implicit',1,1);
 INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next)
 VALUES ('id','INBOX','/','[]',1,2)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
 VALUES (1,1,1,?,?,'2020-01-01T00:00:00Z','[]','null')`, object.Digest, object.Size); err != nil {
		t.Fatal(err)
	}
	if err := location.Queue(ctx, dataDir); !errors.Is(err, ErrMoveTarget) {
		t.Fatalf("Datenverzeichnis akzeptiert: %v", err)
	}
	root := t.TempDir()
	occupied := filepath.Join(root, "occupied")
	if err := os.Mkdir(occupied, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(occupied, "untouched.txt"), []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := location.Queue(ctx, occupied); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); location.Work(ctx, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	defer func() { cancel(); <-done }()
	sameDirectory := func(first, second string) bool {
		a, aErr := os.Stat(first)
		b, bErr := os.Stat(second)
		return aErr == nil && bErr == nil && os.SameFile(a, b)
	}
	waitStatus := func(status, target string) {
		t.Helper()
		for {
			move, err := location.LastMove(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if move.Status == status && (target == "" || sameDirectory(location.Current(), target)) {
				return
			}
			if move.Status == "failed" && status != "failed" {
				t.Fatal(move.Error)
			}
			select {
			case <-ctx.Done():
				t.Fatalf("Archivwechsel %s, aktiver Pfad %q, erwartet %q: %v", move.Status, location.Current(), target, ctx.Err())
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	waitStatus("failed", "")
	if location.Current() != original {
		t.Fatal("Fehler hat aktiven Pfad geändert")
	}
	if data, err := os.ReadFile(filepath.Join(occupied, "untouched.txt")); err != nil || string(data) != "untouched" {
		t.Fatal("Fremde Datei verändert")
	}
	target := filepath.Join(root, "eml")
	if err := location.Queue(ctx, target); err != nil {
		t.Fatal(err)
	}
	waitStatus("succeeded", target)
	if err := store.Verify(object); err != nil {
		t.Fatal(err)
	}
	if err := (Store{Root: original}).Verify(object); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenLocation(ctx, db, dataDir)
	if err != nil || !sameDirectory(reopened.Current(), target) {
		t.Fatalf("Pfad nach Neustart: %v", err)
	}
	if err := location.Queue(ctx, original); err != nil {
		t.Fatal(err)
	}
	waitStatus("succeeded", original)
}
