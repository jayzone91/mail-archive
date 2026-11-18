package archive

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jayzone91/mail-archive/internal/database"
)

func TestReindexFromOriginalMessage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := database.Open(ctx, filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := Store{Root: filepath.Join(dir, "archive")}
	const raw = "Content-Type: text/html; charset=utf-8\r\n\r\n<html><body><p>Archivsuchwort</p><script>Trackingwort</script></body></html>"
	object, err := store.Put(strings.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
 INSERT INTO accounts(id,name,email,username,host,port,tls_mode,enabled,auto_backup)
 VALUES ('id','Test','test@example.test','test','localhost',993,'implicit',1,1);
 INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next)
 VALUES ('id','INBOX','/','[]',1,2);
 `); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
 VALUES (1,1,1,?,?,'2020-01-01T00:00:00Z','[]','{"Subject":"Betreff"}')`, object.Digest, object.Size); err != nil {
		t.Fatal(err)
	}
	writer, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer writer.ExecContext(ctx, "ROLLBACK")
	// Open another pooled connection while the writer holds the lock.
	if _, err := db.ExecContext(ctx, "PRAGMA busy_timeout=0"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.indexBatch(ctx, db); !database.IsBusy(err) {
		t.Fatalf("Schreibkonflikt wurde nicht erkannt: %v", err)
	}
	var pending int
	if err := db.QueryRow("SELECT count(*) FROM archive_search_pending").Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("Indexauftrag nach Schreibkonflikt: %d, %v", pending, err)
	}
	if _, err := writer.ExecContext(ctx, "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	countMatches := func(query string) int {
		t.Helper()
		var count int
		if err := db.QueryRow("SELECT count(*) FROM archive_search WHERE archive_search MATCH ?", query).Scan(&count); err != nil {
			t.Fatal(err)
		}
		return count
	}
	for range 2 {
		if count, err := store.indexBatch(ctx, db); err != nil || count != 1 {
			t.Fatalf("Index: %d, %v", count, err)
		}
		if countMatches("Archivsuchwort") != 1 || countMatches("Trackingwort") != 0 {
			t.Fatal("HTML-Inhalt falsch indexiert")
		}
		if err := Reindex(ctx, db); err != nil {
			t.Fatal(err)
		}
		if countMatches("Archivsuchwort") != 0 {
			t.Fatal("Vorheriger Index blieb erhalten")
		}
	}
}
