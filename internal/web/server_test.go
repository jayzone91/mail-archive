package web

import (
	"context"
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
)

func TestSetupAndSettings(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CREDENTIALS_DIRECTORY", dir)
	if err := os.WriteFile(filepath.Join(dir, "mailarchive.key"), []byte("synthetic-test-key-32-bytes-long"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mail archive #.sqlite")
	db, err := database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	const origin = "http://127.0.0.1:8080"
	location, err := archive.OpenLocation(context.Background(), db, dir)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(context.Background(), db, dir, "127.0.0.1:8080", log, location)
	if err != nil {
		t.Fatal(err)
	}

	send := func(method, path, requestOrigin string, form url.Values, cookie *http.Cookie) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, origin+path, strings.NewReader(form.Encode()))
		if method == http.MethodPost {
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", requestOrigin)
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	check := func(response *httptest.ResponseRecorder, status int) {
		t.Helper()
		if response.Code != status {
			t.Fatalf("Status %d, erwartet %d: %s", response.Code, status, response.Body)
		}
	}

	check(send(http.MethodGet, "/settings", "", nil, nil), http.StatusSeeOther)
	loginPage := send(http.MethodGet, "/login", "", nil, nil)
	check(loginPage, http.StatusOK)
	if got := loginPage.Header().Get("Referrer-Policy"); got != "same-origin" {
		t.Fatalf("Referrer-Policy der Anmeldeseite: %q", got)
	}
	token, err := os.ReadFile(filepath.Join(dir, "setup-token"))
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"password": {"Test-1234"}, "setup_token": {strings.TrimSpace(string(token))}}
	check(send(http.MethodPost, "/login", "http://127.0.0.1:9999", form, nil), http.StatusForbidden)
	check(send(http.MethodPost, "/login", "null", form, nil), http.StatusForbidden)
	response := send(http.MethodPost, "/login", origin, form, nil)
	check(response, http.StatusSeeOther)
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("Sitzungscookie fehlt oder ist unsicher")
	}
	if _, err := os.Stat(filepath.Join(dir, "setup-token")); !os.IsNotExist(err) {
		t.Fatalf("Einrichtungsdatei: %v", err)
	}

	settings := url.Values{"backup_hours": {"12"}, "retention_months": {"2"}, "cleanup_hours": {"24"}, "workers": {"2"}, "retries": {"3"}}
	check(send(http.MethodPost, "/settings", origin, settings, cookies[0]), http.StatusSeeOther)
	settingsPage := send(http.MethodGet, "/settings", "", nil, cookies[0])
	check(settingsPage, http.StatusOK)
	if !strings.Contains(settingsPage.Body.String(), location.Current()) {
		t.Fatal("Aktiver Archivpfad fehlt in den Einstellungen")
	}
	check(send(http.MethodPost, "/settings/archive", origin, url.Values{"archive_path": {"relative/archive"}}, cookies[0]), http.StatusBadRequest)
	settings.Set("workers", "0")
	check(send(http.MethodPost, "/settings", origin, settings, cookies[0]), http.StatusBadRequest)
	account := url.Values{
		"name": {"Quelle"}, "email": {"migration@example.test"}, "username": {"test"},
		"host": {"imap.example.test"}, "port": {"993"}, "tls": {"implicit"},
		"password": {"synthetic-imap-secret"}, "enabled": {"on"},
	}
	check(send(http.MethodPost, "/accounts/new", origin, account, cookies[0]), http.StatusSeeOther)
	account.Set("name", "Ziel")
	check(send(http.MethodPost, "/accounts/new", origin, account, cookies[0]), http.StatusSeeOther)
	var id string
	var encrypted []byte
	if err := db.QueryRow("SELECT id, credential FROM accounts WHERE name = 'Quelle'").Scan(&id, &encrypted); err != nil {
		t.Fatal(err)
	}
	if len(encrypted) == 0 || strings.Contains(string(encrypted), account.Get("password")) {
		t.Fatal("Postfachpasswort fehlt oder ist im Klartext gespeichert")
	}
	edit := send(http.MethodGet, "/accounts/"+id, "", nil, cookies[0])
	check(edit, http.StatusOK)
	if strings.Contains(edit.Body.String(), account.Get("password")) {
		t.Fatal("Postfachpasswort in HTML-Antwort")
	}
	account.Del("password")
	account.Del("enabled")
	check(send(http.MethodPost, "/accounts/"+id, origin, account, cookies[0]), http.StatusSeeOther)
	var enabled bool
	var retained []byte
	if err := db.QueryRow("SELECT enabled, credential FROM accounts WHERE id = ?", id).Scan(&enabled, &retained); err != nil {
		t.Fatal(err)
	}
	if enabled || string(retained) != string(encrypted) {
		t.Fatal("Deaktivierung hat das gespeicherte Passwort verändert")
	}
	const original = "Subject: Fixture\r\n\r\n<untrusted>offline</untrusted>\r\n"
	store := archive.Store{Root: filepath.Join(dir, "archive")}
	object, err := store.Put(strings.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next) VALUES (?,'INBOX','/','[]',1,2)", id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
		VALUES (1,1,1,?,?,'2020-01-01T00:00:00Z','[]','{"Subject":"Fixture"}')`, object.Digest, object.Size); err != nil {
		t.Fatal(err)
	}
	check(send(http.MethodGet, "/accounts/"+id+"/remove", "", nil, cookies[0]), http.StatusOK)
	check(send(http.MethodPost, "/accounts/"+id+"/remove", origin, nil, cookies[0]), http.StatusSeeOther)
	check(send(http.MethodGet, "/accounts/"+id, "", nil, cookies[0]), http.StatusNotFound)
	var retainedHistory bool
	if err := db.QueryRow("SELECT removed_at IS NOT NULL AND credential IS NULL FROM accounts WHERE id = ?", id).Scan(&retainedHistory); err != nil || !retainedHistory {
		t.Fatalf("Entfernte Kontokonfiguration: %v, %v", retainedHistory, err)
	}
	check(send(http.MethodGet, "/archive", "", nil, cookies[0]), http.StatusOK)
	preview := send(http.MethodGet, "/archive/1", "", nil, cookies[0])
	check(preview, http.StatusOK)
	if !strings.Contains(preview.Body.String(), "&lt;untrusted&gt;offline") {
		t.Fatal("Archivvorschau fehlt oder ist nicht HTML-escaped")
	}
	eml := send(http.MethodGet, "/archive/1/eml", "", nil, cookies[0])
	check(eml, http.StatusOK)
	if eml.Body.String() != original {
		t.Fatal("Bearbeitung oder Entfernen des Postfachs hat das EML-Original verändert")
	}
	var target string
	if err := db.QueryRow("SELECT id FROM accounts WHERE name='Ziel' AND enabled=1 AND removed_at IS NULL").Scan(&target); err != nil {
		t.Fatal(err)
	}
	mapping := url.Values{"source": {id}, "target": {target}}
	check(send(http.MethodPost, "/migration", origin, mapping, cookies[0]), http.StatusSeeOther)
	plan := url.Values{"source": {"account:" + id}, "target": {target}, "action": {"preview"}, "date": {"on"}}
	check(send(http.MethodPost, "/restore", origin, plan, cookies[0]), http.StatusOK)
	var jobs int
	if err := db.QueryRow("SELECT count(*) FROM restore_jobs").Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("Vorschau hat den Auftragszustand verändert: %d, %v", jobs, err)
	}
	plan.Set("action", "queue")
	check(send(http.MethodPost, "/restore", origin, plan, cookies[0]), http.StatusSeeOther)
	migrationPage := send(http.MethodGet, "/migration", "", nil, cookies[0])
	check(migrationPage, http.StatusOK)
	if !strings.Contains(migrationPage.Body.String(), "0 von 1 Nachrichten bytegenau geprüft") {
		t.Fatal("Migrationsseite zeigt den Restore-Auftrag nicht an")
	}
	legacyRestore := send(http.MethodGet, "/restore/status", "", nil, cookies[0])
	check(legacyRestore, http.StatusOK)
	if body := legacyRestore.Body.String(); !strings.Contains(body, "Angelegt") || strings.Contains(body, "Beendet") {
		t.Fatal("Fehlende Restore-Zeitstempel dürfen nicht als Abschluss erscheinen")
	}
	if _, err := db.Exec(`UPDATE restore_jobs SET status='succeeded',started_at='2026-11-18 20:01:00',finished_at='2026-11-18 20:02:00';
		INSERT INTO jobs(account_id,mode,status,started_at,finished_at)
		VALUES (?,'full','succeeded','2026-11-18 20:01:00','2026-11-18 20:02:00')`, target); err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/backups/status", "/restore/status"} {
		fragment := send(http.MethodGet, route, "", nil, cookies[0])
		check(fragment, http.StatusOK)
		if body := fragment.Body.String(); !strings.Contains(body, "Gestartet") || !strings.Contains(body, "Beendet") ||
			!strings.Contains(body, `datetime="2026-11-18T20:02:00Z"`) {
			t.Fatalf("Zeitstempel fehlen in %s", route)
		}
	}
	if _, err := db.Exec("INSERT INTO folders(account_id,name,delimiter,attributes,uid_validity,uid_next) VALUES (?,'Archiv/Projekte','/','[]',1,2), (?,'INBOX','/','[]',1,2)", id, target); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO messages(folder_id,uid_validity,uid,digest,size,internal_date,flags,envelope)
		VALUES (2,1,1,?,?,'2020-01-02T00:00:00Z','[]','{"Subject":"Projektpost"}'),
		(3,1,1,?,?,'2020-01-02T00:00:00Z','[]','{"Subject":"Anderes Zielpostfach"}');
		INSERT INTO archive_search(rowid,subject) VALUES (1,'Fixture Suchprobe'),(2,'Projektpost Suchprobe'),(3,'Anderes Zielpostfach Suchprobe')`,
		object.Digest, object.Size, object.Digest, object.Size); err != nil {
		t.Fatal(err)
	}
	assertArchive := func(query url.Values, expected, excluded string) {
		t.Helper()
		page := send(http.MethodGet, "/archive?"+query.Encode(), "", nil, cookies[0])
		check(page, http.StatusOK)
		if html := page.Body.String(); !strings.Contains(html, expected) || (excluded != "" && strings.Contains(html, excluded)) {
			t.Fatalf("Archivfilter %s zeigt unerwartete Nachrichten", query.Encode())
		}
	}
	assertArchive(url.Values{"account": {id}, "message": {"1"}}, "&lt;untrusted&gt;offline", "Anderes Zielpostfach")
	assertArchive(url.Values{"account": {id}, "folder": {"2"}}, "Projektpost", "Fixture")
	assertArchive(url.Values{"account": {id}, "q": {"Suchprobe"}, "scope": {"account"}}, "Projektpost", "Anderes Zielpostfach")
	assertArchive(url.Values{"account": {id}, "q": {"Suchprobe"}, "scope": {"folders"}, "in": {"2", "3"}}, "Projektpost", "Anderes Zielpostfach")
	assertArchive(url.Values{"account": {id}, "q": {"Suchprobe"}, "scope": {"all"}}, "Anderes Zielpostfach", "")
	if _, err := db.Exec(`INSERT INTO jobs(id,account_id,mode,status,source_checked,finished_at)
		VALUES (99,?,'full','succeeded',1,'2026-11-18 20:00:00');
		INSERT INTO backup_folder_checks(job_id,folder_id,seen,covered) VALUES (99,1,3,2);
		UPDATE restore_items SET status='verified' WHERE job_id=1;
		UPDATE restore_jobs SET status='succeeded' WHERE id=1;
		INSERT INTO backup_snapshots(id,target,status,coverage_recorded,finished_at)
		VALUES ('proof','snapshot-target','verified',1,'2026-11-18 20:02:00');
		INSERT INTO snapshot_message_coverage(snapshot_id,message_id,digest,size)
		SELECT 'proof',id,digest,size FROM messages WHERE id=1`, id); err != nil {
		t.Fatal(err)
	}
	migrationPage = send(http.MethodGet, "/migration", "", nil, cookies[0])
	check(migrationPage, http.StatusOK)
	for _, evidence := range []string{"2 von 3 Nachrichten im Archiv bestätigt", "1 von 2 Archiveinträgen", "1 von 2 Nachrichten bytegenau geprüft"} {
		if !strings.Contains(migrationPage.Body.String(), evidence) {
			t.Fatalf("Migrationsabgleich fehlt: %s", evidence)
		}
	}
	check(send(http.MethodPost, "/logout", origin, nil, cookies[0]), http.StatusSeeOther)
	check(send(http.MethodGet, "/settings", "", nil, cookies[0]), http.StatusSeeOther)

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = database.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var interval int
	if err := db.QueryRow("SELECT backup_hours FROM settings").Scan(&interval); err != nil || interval != 12 {
		t.Fatalf("Einstellung nach Neustart: %d, %v", interval, err)
	}
	location, err = archive.OpenLocation(context.Background(), db, dir)
	if err != nil {
		t.Fatal(err)
	}
	handler, err = New(context.Background(), db, dir, "127.0.0.1:8080", log, location)
	if err != nil {
		t.Fatal(err)
	}
	check(send(http.MethodPost, "/login", origin, form, nil), http.StatusSeeOther)
}
