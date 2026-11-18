package backup

import (
	"bytes"
	"context"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	protocol "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/database"
)

func TestBackupPreservesOriginalAndMailboxGenerations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	dir := t.TempDir()
	db, err := database.Open(ctx, filepath.Join(dir, "backup.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `INSERT INTO accounts(id,name,email,username,host,port,tls_mode,enabled,auto_backup)
  VALUES ('test','Test','mail@example.test','test','localhost',993,'implicit',1,1)`); err != nil {
		t.Fatal(err)
	}
	added, err := Enqueue(ctx, db, "test", "full")
	if err != nil || !added {
		t.Fatalf("Job: %v, %v", added, err)
	}
	added, err = Enqueue(ctx, db, "test", "incremental")
	if err != nil || added {
		t.Fatalf("Paralleler Job: %v, %v", added, err)
	}

	backend := imapmemserver.New()
	user := imapmemserver.NewUser("test", "synthetic-credential")
	backend.AddUser(user)
	const name = "Archiv/Grüße"
	const raw = "Subject: Synthetic\r\nMessage-ID: <synthetic@example.test>\r\n\r\nOriginal\r\n"
	appendMessage := func() {
		t.Helper()
		if err := user.Create(name, nil); err != nil {
			t.Fatal(err)
		}
		_, err := user.Append(name, bytes.NewReader([]byte(raw)), &protocol.AppendOptions{Time: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)})
		if err != nil {
			t.Fatal(err)
		}
	}
	appendMessage()
	server := imapserver.New(&imapserver.Options{InsecureAuth: true, NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
		return backend.NewSession(), nil, nil
	}})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.Serve(listener)
	client, err := imapclient.DialInsecure(listener.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	stop := context.AfterFunc(ctx, func() { client.Close() })
	defer stop()
	if err := client.Login("test", "synthetic-credential").Wait(); err != nil {
		t.Fatal(err)
	}
	folders, err := client.List("", "*", nil).Collect()
	if err != nil || len(folders) != 1 || folders[0].Mailbox != name {
		t.Fatalf("Ordner: %v, %v", folders, err)
	}

	service := Service{DB: db, Archive: archive.Store{Root: filepath.Join(dir, "archive")}}
	if _, err := service.folder(ctx, client, 1, "test", "full", folders[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := service.folder(ctx, client, 1, "test", "full", folders[0]); err != nil {
		t.Fatal(err)
	}
	var digest string
	var count int
	if err := db.QueryRow("SELECT MIN(digest),count(*) FROM messages").Scan(&digest, &count); err != nil || count != 1 {
		t.Fatalf("Duplikate: %d, %v", count, err)
	}
	file, err := service.Archive.Open(digest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	file.Close()
	if err != nil || string(data) != raw {
		t.Fatalf("Originaldaten: %q, %v", data, err)
	}

	if err := client.Unselect().Wait(); err != nil {
		t.Fatal(err)
	}
	if err := user.Delete(name); err != nil {
		t.Fatal(err)
	}
	appendMessage()
	if _, err := service.folder(ctx, client, 1, "test", "incremental", folders[0]); err != nil {
		t.Fatal(err)
	}
	var present, contents int
	if err := db.QueryRow("SELECT count(*),sum(present),count(DISTINCT digest) FROM messages").Scan(&count, &present, &contents); err != nil || count != 2 || present != 1 || contents != 1 {
		t.Fatalf("UIDVALIDITY-Wechsel: Einträge=%d, vorhanden=%d, EML-Objekte=%d, Fehler=%v", count, present, contents, err)
	}
	if _, err := db.Exec("UPDATE jobs SET status='failed' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if queued, err := Retry(ctx, db, 1); err != nil || !queued {
		t.Fatalf("Wiederaufnahme: %v, %v", queued, err)
	}
	if queued, err := Enqueue(ctx, db, "", "full"); err != nil || queued {
		t.Fatalf("Sicherung aller Postfächer hat aktiven Auftrag dupliziert: %v, %v", queued, err)
	}
}
