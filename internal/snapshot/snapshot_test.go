package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/database"
)

func TestSnapshotCopiesConsistentImageAndDetectsDamage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := database.Open(ctx, filepath.Join(dir, "primary.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	source := archive.Store{Root: filepath.Join(dir, "source")}
	const content = "Subject: Synthetic\r\n\r\noriginal\r\n"
	object, err := source.Put(strings.NewReader(content), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `
 INSERT INTO accounts(id,name,email,username,host,port,tls_mode,enabled,auto_backup)
 VALUES ('id','Test','test@example.test','test','localhost',993,'implicit',1,1);
 INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next)
 VALUES ('id','INBOX','/','[]',1,2);
 `); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
 VALUES (1,1,1,?,?,'2020-01-01T00:00:00Z','[]','null')`, object.Digest, object.Size); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	// Set the test destination directly. Production configuration rejects two paths on the same disk.
	if _, err := db.ExecContext(ctx, "UPDATE settings SET snapshot_target=?,snapshot_requested=1", target); err != nil {
		t.Fatal(err)
	}
	if err := scheduleAutomatic(ctx, db); err != nil {
		t.Fatal(err)
	}
	id, destination, err := claimSnapshot(ctx, db)
	if err != nil || destination != target {
		t.Fatalf("Erster Auftrag: %q, %v", destination, err)
	}
	captured, err := Create(ctx, db, source, target, id)
	if err != nil || len(captured.Objects) != 1 || captured.Objects[0] != object {
		t.Fatalf("Snapshot: %v, %v", captured, err)
	}
	published := filepath.Join(target, id)
	if _, err := Verify(ctx, published); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(ctx, db, source, target, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
 VALUES (1,1,2,?,?,'2020-01-02T00:00:00Z','[]','null')`, object.Digest, object.Size); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO jobs(account_id,status,mode) VALUES ('id','running','incremental');
 UPDATE jobs SET status='succeeded' WHERE account_id='id';`); err != nil {
		t.Fatal(err)
	}
	if err := scheduleAutomatic(ctx, db); err != nil {
		t.Fatal(err)
	}
	if again, _, err := claimSnapshot(ctx, db); err != nil || again != id {
		t.Fatalf("Laufender Snapshot: %q, %v", again, err)
	}
	if err := recordVerified(ctx, db, id, target, captured); err != nil {
		t.Fatal(err)
	}
	checkCoverage := func(covered int64) {
		t.Helper()
		coverage, err := CurrentCoverage(ctx, db, target)
		if err != nil || len(coverage.Mailboxes) != 1 || coverage.Mailboxes[0].Total != 2 || coverage.Mailboxes[0].Covered != covered {
			t.Fatalf("Archivabdeckung: %+v, %v", coverage, err)
		}
	}
	checkCoverage(1)
	if err := scheduleAutomatic(ctx, db); err != nil {
		t.Fatal(err)
	}
	next, _, err := claimSnapshot(ctx, db)
	if err != nil || next == id {
		t.Fatalf("Folgesnapshot: %q, %v", next, err)
	}
	if err := recordFailure(ctx, db, next, ErrTarget); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE backup_snapshots SET retry_at='2000-01-01 00:00:00' WHERE id=?", next); err != nil {
		t.Fatal(err)
	}
	if retry, _, err := claimSnapshot(ctx, db); err != nil || retry != next {
		t.Fatalf("Wiederholung: %q, %v", retry, err)
	}
	second, err := Create(ctx, db, source, target, next)
	if err != nil {
		t.Fatal(err)
	}
	if err := recordVerified(ctx, db, next, target, second); err != nil {
		t.Fatal(err)
	}
	checkCoverage(2)
	if err := os.WriteFile(filepath.Join(published, "archive", object.Digest+".eml"), []byte("damaged"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(ctx, published); err == nil {
		t.Fatal("Beschädigter Snapshot wurde akzeptiert")
	}
}
