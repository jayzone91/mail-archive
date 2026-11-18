package web

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/a-h/templ"
	"github.com/jayzone91/mail-archive/internal/accounts"
	"github.com/jayzone91/mail-archive/internal/archive"
	"github.com/jayzone91/mail-archive/internal/config"
	"github.com/jayzone91/mail-archive/internal/platform"
)

//go:embed assets/app.css assets/app.js
var assets embed.FS

const webPolicy = "default-src 'none'; style-src 'self'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; form-action 'self'; base-uri 'none'"

type site struct {
	db              *sql.DB
	log             *slog.Logger
	host            string
	auth            *authentication
	accounts        accounts.Store
	dataDir         string
	archiveLocation *archive.Location
	exportMu        sync.Mutex
}

func New(ctx context.Context, db *sql.DB, dataDir, host string, log *slog.Logger, location *archive.Location) (http.Handler, error) {
	auth, err := newAuthentication(ctx, db, dataDir, log)
	if err != nil {
		return nil, err
	}
	s := &site{db: db, log: log, host: host, auth: auth, dataDir: dataDir, archiveLocation: location, accounts: accounts.Store{DB: db, Vault: platform.NewCredentials(dataDir)}}
	mux := http.NewServeMux()
	s.accountRoutes(mux)
	s.backupRoutes(mux)
	s.snapshotRoutes(mux)
	s.archiveRoutes(mux)
	s.restoreRoutes(mux)
	s.migrationRoutes(mux)
	mux.HandleFunc("GET /retention", s.protect(s.retentionPreview))
	mux.HandleFunc("GET /{$}", s.protect(s.dashboard))
	mux.HandleFunc("GET /settings", s.protect(s.settings))
	mux.HandleFunc("POST /settings", s.protect(s.saveSettings))
	mux.HandleFunc("POST /settings/archive", s.protect(s.moveArchive))
	mux.HandleFunc("GET /login", s.login)
	mux.HandleFunc("POST /login", s.signIn)
	mux.HandleFunc("POST /logout", s.protect(s.logout))
	mux.HandleFunc("GET /assets/app.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, assets, "assets/app.css")
	})
	mux.HandleFunc("GET /assets/app.js", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, assets, "assets/app.js")
	})
	return s.guard(mux), nil
}

func (s *site) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", webPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// A no-referrer policy can make a browser's form POST send Origin: null.
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Cache-Control", "no-store")
		if r.Host != s.host {
			http.Error(w, "Unzulässiger Host.", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "http://"+s.host {
				reason := "mismatch"
				switch origin {
				case "":
					reason = "missing"
				case "null":
					reason = "null"
				}
				s.log.Warn("Anfrage mit unzulässigem Ursprung abgewiesen", "method", r.Method, "reason", reason)
				http.Error(w, "Unzulässiger Ursprung.", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 8192)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *site) protect(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.validSession(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		handler(w, r)
	}
}

func (s *site) render(w http.ResponseWriter, r *http.Request, status int, component templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := component.Render(context.WithValue(r.Context(), navigationPathKey{}, r.URL.Path), w); err != nil {
		s.log.Error("Seite rendern", "error", err)
	}
}

func (s *site) failure(w http.ResponseWriter, err error) {
	s.log.Error("Anfrage fehlgeschlagen", "error", err)
	http.Error(w, "Die Anfrage konnte nicht verarbeitet werden.", http.StatusInternalServerError)
}

func (s *site) dashboard(w http.ResponseWriter, r *http.Request) {
	var counts dashboardCounts
	if err := s.db.QueryRowContext(r.Context(), `SELECT
		(SELECT count(*) FROM accounts WHERE removed_at IS NULL),
		(SELECT count(*) FROM messages),
		(SELECT count(*) FROM jobs WHERE status IN ('queued','running')) +
		(SELECT count(*) FROM restore_jobs WHERE status IN ('queued','running'))`).
		Scan(&counts.Accounts, &counts.Messages, &counts.Jobs); err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, layout("Übersicht", true, dashboardView(counts)))
}

func (s *site) settings(w http.ResponseWriter, r *http.Request) {
	settings, err := config.Load(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	move, err := s.archiveLocation.LastMove(r.Context())
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, layout("Einstellungen", true, settingsPage(settings, s.archiveLocation.Current(), move, r.URL.Query().Get("saved") == "1")))
}

func (s *site) moveArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	err := s.archiveLocation.Queue(r.Context(), strings.TrimSpace(r.PostForm.Get("archive_path")))
	switch {
	case err == nil:
		http.Redirect(w, r, "/settings#archive-storage", http.StatusSeeOther)
	case err == archive.ErrMoveTarget:
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err == archive.ErrMovePending:
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		s.failure(w, err)
	}
}

func (s *site) saveSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	value := config.Settings{AutomaticBackup: r.PostForm.Get("automatic_backup") == "on"}
	fields := []struct {
		name        string
		destination *int
	}{
		{"backup_hours", &value.BackupHours}, {"retention_months", &value.RetentionMonths},
		{"cleanup_hours", &value.CleanupHours}, {"workers", &value.Workers}, {"retries", &value.Retries},
	}
	for _, field := range fields {
		number, err := strconv.Atoi(strings.TrimSpace(r.PostForm.Get(field.name)))
		if err != nil {
			s.render(w, r, http.StatusBadRequest, layout("Einstellungen", true, settingsView(value, "Bitte ganze Zahlen eingeben.", false)))
			return
		}
		*field.destination = number
	}
	if err := value.Validate(); err != nil {
		s.render(w, r, http.StatusBadRequest, layout("Einstellungen", true, settingsView(value, err.Error(), false)))
		return
	}
	if err := value.Save(r.Context(), s.db); err != nil {
		s.failure(w, fmt.Errorf("Einstellungen speichern: %w", err))
		return
	}
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}
