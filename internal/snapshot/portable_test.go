package snapshot_test

import (
	"bytes"
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/database"
	"github.com/jayzone91/mail-archive/internal/snapshot"
	"github.com/jayzone91/mail-archive/internal/web"
)

func TestPortableInstallationRoundTrip(t *testing.T) {
	ctx := context.Background()
	original, second, replacement := t.TempDir(), t.TempDir(), t.TempDir()
	path := filepath.Join(original, "mailarchive.sqlite")
	db, err := database.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(statement string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	const adminPassword = "Synthetic admin password"
	originalSalt := []byte("original-salt")
	originalHash, err := pbkdf2.Key(sha256.New, adminPassword, originalSalt, 600000, 32)
	if err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO administrator(id,salt,password_hash) VALUES (1,?,?)", originalSalt, originalHash)
	exec(`INSERT INTO accounts(id,name,email,username,host,port,tls_mode,enabled,auto_backup,credential)
 VALUES ('id','Test','test@example.test','test','localhost',993,'implicit',1,1,x'dead');
 INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next)
 VALUES ('id','INBOX','/','[]',1,2);
 INSERT INTO jobs(account_id,status,mode) VALUES ('id','running','full');`)
	source := archive.Store{Root: filepath.Join(original, "archive")}
	const raw = "Subject: Portable\r\n\r\nSynthetic test message\r\n"
	object, err := source.Put(strings.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
 VALUES (1,1,1,?,?,'2020-01-01T00:00:00Z','[]','null')`, object.Digest, object.Size)

	const snapshotID = "portable-test"
	if _, err := snapshot.Create(ctx, db, source, second, snapshotID); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO backup_snapshots(id,target,status,finished_at) VALUES (?,?,'verified',CURRENT_TIMESTAMP)", snapshotID, second)
	// Settings modified after the snapshot must still be included in the export.
	exec("UPDATE settings SET backup_hours=12,automatic_backup=1,snapshot_target=?,archive_path=?", second, source.Root)

	var encrypted bytes.Buffer
	const password = "Synthetic transfer phrase"
	if err := snapshot.ExportPortable(ctx, db, original, snapshotID, password, &encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted.Bytes(), []byte("SQLite format 3")) {
		t.Fatal("SQLite wurde im Klartext exportiert")
	}
	snapshotDir := filepath.Join(second, snapshotID)
	target := filepath.Join(replacement, "new-data")
	if err := snapshot.ImportPortable(ctx, bytes.NewReader(encrypted.Bytes()), "wrong password", snapshotDir, target); !errors.Is(err, snapshot.ErrPortableDecrypt) {
		t.Fatalf("Falsches Passwort: %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Falsches Passwort erzeugte Ziel: %v", err)
	}
	damaged := bytes.Clone(encrypted.Bytes())
	damaged[len(damaged)-1] ^= 1
	if err := snapshot.ImportPortable(ctx, bytes.NewReader(damaged), password, snapshotDir, target); err == nil {
		t.Fatal("Manipuliertes Paket wurde angenommen")
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Defektes Paket erzeugte Ziel: %v", err)
	}
	if err := snapshot.ImportPortable(ctx, bytes.NewReader(encrypted.Bytes()), password, snapshotDir, target); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.ImportPortable(ctx, bytes.NewReader(encrypted.Bytes()), password, snapshotDir, target); !errors.Is(err, snapshot.ErrExistingInstallation) {
		t.Fatalf("Vorhandene Installation: %v", err)
	}
	imported, err := database.Open(ctx, filepath.Join(target, "mailarchive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer imported.Close()
	var salt, hash []byte
	if err := imported.QueryRow("SELECT salt,password_hash FROM administrator WHERE id=1").Scan(&salt, &hash); err != nil ||
		!bytes.Equal(salt, originalSalt) || !bytes.Equal(hash, originalHash) {
		t.Fatalf("Adminzugang wurde nicht erhalten: %v", err)
	}
	var hours int
	var automatic bool
	var snapshotTarget, archivePath, status string
	if err := imported.QueryRow("SELECT backup_hours,automatic_backup,snapshot_target,archive_path,(SELECT status FROM jobs WHERE id=1) FROM settings").
		Scan(&hours, &automatic, &snapshotTarget, &archivePath, &status); err != nil || hours != 12 || automatic || snapshotTarget != "" || archivePath != "" || status != "failed" {
		t.Fatalf("Einstellungen oder Jobstatus nach Import: %d %v %q %q %q %v", hours, automatic, snapshotTarget, archivePath, status, err)
	}
	var credential []byte
	if err := imported.QueryRow("SELECT credential FROM accounts WHERE id='id'").Scan(&credential); err != nil || credential != nil {
		t.Fatalf("Gerätegebundene Zugangsdaten wurden übernommen: %v", err)
	}
	if err := (archive.Store{Root: filepath.Join(target, "archive")}).Verify(object); err != nil {
		t.Fatal(err)
	}
	location, err := archive.OpenLocation(ctx, imported, target)
	if err != nil {
		t.Fatal(err)
	}
	const host = "127.0.0.1:8080"
	handler, err := web.New(ctx, imported, target, host, slog.New(slog.NewTextHandler(io.Discard, nil)), location)
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"password": {adminPassword}}
	request := httptest.NewRequest(http.MethodPost, "http://"+host+"/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://"+host)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || len(response.Result().Cookies()) != 1 {
		t.Fatalf("Anmeldung mit bisherigem Adminpasswort fehlgeschlagen: %d", response.Code)
	}
	var originalCredential []byte
	if err := db.QueryRow("SELECT credential FROM accounts WHERE id='id'").Scan(&originalCredential); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(originalCredential, []byte{0xde, 0xad}) {
		t.Fatal("Quellinstallation wurde verändert")
	}
}
