package backup

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jayzone91/mail-archive/internal/database"
)

func TestScheduleDueAndRetry(t *testing.T) {
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "schedule.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO accounts(id,name,email,username,host,port,tls_mode,enabled,auto_backup)
 VALUES ('on','On','on@example.test','test','localhost',993,'implicit',1,1),
 ('off','Off','off@example.test','test','localhost',993,'implicit',1,0)`)
	now := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	schedule := func(at time.Time) {
		t.Helper()
		if err := ScheduleDue(ctx, db, at); err != nil {
			t.Fatal(err)
		}
	}
	expectEmpty := func(at time.Time) {
		t.Helper()
		if job, err := claim(ctx, db, at); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("Unerwarteter Auftrag: %+v, %v", job, err)
		}
	}
	schedule(now)
	expectEmpty(now)
	exec("UPDATE settings SET automatic_backup=1,retries=1")
	schedule(now)
	schedule(now)
	exec("INSERT INTO restore_jobs(target,status,preserve_date,preserve_flags) VALUES ('on','running',1,1)")
	expectEmpty(now)
	exec("UPDATE restore_jobs SET status='succeeded'")
	job, err := claim(ctx, db, now)
	if err != nil || job.account != "on" || job.mode != "full" || job.attempts != 1 {
		t.Fatalf("Erster Auftrag: %+v, %v", job, err)
	}
	expectEmpty(now)
	if err := finish(ctx, db, job, errors.New("synthetic"), now); err != nil {
		t.Fatal(err)
	}
	expectEmpty(now.Add(59 * time.Second))
	retry, err := claim(ctx, db, now.Add(time.Minute))
	if err != nil || retry.id != job.id || retry.attempts != 2 {
		t.Fatalf("Persistenter Retry: %+v, %v", retry, err)
	}
	done := now.Add(time.Minute)
	if err := finish(ctx, db, retry, nil, done); err != nil {
		t.Fatal(err)
	}
	schedule(done.Add(6*time.Hour - time.Second))
	expectEmpty(done.Add(6*time.Hour - time.Second))
	due := done.Add(6 * time.Hour)
	schedule(due)
	incremental, err := claim(ctx, db, due)
	if err != nil || incremental.mode != "incremental" || incremental.id == job.id {
		t.Fatalf("Inkrementeller Folgeauftrag: %+v, %v", incremental, err)
	}
}
