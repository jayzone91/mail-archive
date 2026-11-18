package restore

import (
	"bytes"
	"context"
	"database/sql"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	protocol "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/database"
)

func TestRestoreRecoversJournalAndPreservesMultiplicity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	dir := t.TempDir()
	db, err := database.Open(ctx, filepath.Join(dir, "restore.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := archive.Store{Root: filepath.Join(dir, "archive")}
	const raw = "Subject: Synthetic\r\n\r\nOriginal\r\n"
	object, err := source.Put(strings.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO accounts(id,name,email,username,host,port,tls_mode,enabled,auto_backup)
 VALUES ('source','Source','mail@example.test','test','localhost',993,'implicit',1,1),
 ('target','Target','mail@example.test','test','localhost',993,'implicit',1,1);
 INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next)
 VALUES ('source','INBOX','/','[]',1,4);`)
	for uid := 1; uid <= 3; uid++ {
		exec(`INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
  VALUES (1,1,?,?,?,'2020-01-02T03:04:05Z','["\\Seen"]','null')`, uid, object.Digest, object.Size)
	}
	exec(`INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
	 VALUES (1,2,1,?,?,'2020-01-02T03:04:05Z','[]','null')`, object.Digest, object.Size)
	backend := imapmemserver.New()
	user := imapmemserver.NewUser("test", "synthetic-secret")
	backend.AddUser(user)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
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
	if err := client.Login("test", "synthetic-secret").Wait(); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	existing, err := user.Append("INBOX", bytes.NewReader([]byte(raw)), &protocol.AppendOptions{Time: date, Flags: []protocol.Flag{protocol.FlagSeen}})
	if err != nil {
		t.Fatal(err)
	}

	req := Request{Source: Source{Kind: "account", ID: "source"}, Target: "target", PreserveDate: true, PreserveFlags: true}
	if count, err := Preview(ctx, db, req); err != nil || count != 3 {
		t.Fatalf("Vorschau: %d, %v", count, err)
	}
	explicit := req
	explicit.Source = Source{Kind: "messages", ID: "1,4"}
	if count, err := Preview(ctx, db, explicit); err != nil || count != 2 {
		t.Fatalf("Explizite Mehrfachauswahl: %d, %v", count, err)
	}
	if err := Queue(ctx, db, req); err != nil {
		t.Fatal(err)
	}
	interrupted, err := user.Append("INBOX", bytes.NewReader([]byte(raw)), &protocol.AppendOptions{Time: date, Flags: []protocol.Flag{protocol.FlagSeen}})
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO restore_history(target,folder,message_id,validity,first_uid,target_uid,restored_at)
 VALUES ('target','INBOX',1,?,0,?,CURRENT_TIMESTAMP)`, existing.UIDValidity, existing.UID)
	exec(`INSERT INTO restore_history(target,folder,message_id,validity,first_uid)
 VALUES ('target','INBOX',2,?,?)`, interrupted.UIDValidity, interrupted.UID)

	service := Service{DB: db, Archive: source}
	if err := service.restore(ctx, client, 1, "target", true, true); err != nil {
		t.Fatal(err)
	}
	var count int
	var addedUID protocol.UID
	if err := db.QueryRow("SELECT count(*),max(target_uid) FROM restore_history WHERE target_uid IS NOT NULL").Scan(&count, &addedUID); err != nil || count != 3 {
		t.Fatalf("Restore-Zuordnungen: %d, %v", count, err)
	}
	meta, err := client.Fetch(protocol.UIDSetNum(addedUID), &protocol.FetchOptions{Flags: true, InternalDate: true}).Collect()
	if err != nil || len(meta) != 1 || !meta[0].InternalDate.Equal(date) {
		t.Fatalf("Zieldatum: %v, %v", meta, err)
	}
	exec("UPDATE restore_jobs SET status='succeeded' WHERE id=1")
	if err := Queue(ctx, db, req); err != nil {
		t.Fatal(err)
	}
	if err := service.restore(ctx, client, 2, "target", true, true); err != nil {
		t.Fatal(err)
	}
	dest, err := client.UIDSearch(&protocol.SearchCriteria{}, nil).Wait()
	if err != nil || len(dest.AllUIDs()) != 3 {
		t.Fatalf("Unbeabsichtigte Duplikate: %v, %v", dest, err)
	}

	exec("UPDATE restore_history SET target_uid=NULL,first_uid=100 WHERE message_id=3")
	exec("UPDATE restore_jobs SET status='failed' WHERE id=2")
	exec("UPDATE restore_items SET status='review' WHERE job_id=2 AND message_id=3")
	if err := Retry(ctx, db, 2); err != nil {
		t.Fatal(err)
	}
	if err := service.restore(ctx, client, 2, "target", true, true); err == nil {
		t.Fatal("Ungeklärter Upload wurde akzeptiert")
	}
	var targetUID sql.NullInt64
	if err := db.QueryRow("SELECT target_uid FROM restore_history WHERE message_id=3").Scan(&targetUID); err != nil || targetUID.Valid {
		t.Fatalf("Ungeklärter Upload wurde wiederholt: %v, %v", targetUID, err)
	}
}

func TestMapFolder(t *testing.T) {
	if name, err := mapFolder("Archiv/Grüße", "/", '.', true); err != nil || name != "Archiv.Grüße" {
		t.Fatalf("Ordnerabbildung: %q, %v", name, err)
	}
	if _, err := mapFolder("Archiv/Datei.txt", "/", '.', true); err == nil {
		t.Fatal("Mehrdeutige Ordnerabbildung wurde akzeptiert")
	}
}
