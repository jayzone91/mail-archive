package web

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

const sessionName = "mailarchive_session"
const sessionAge = 8 * time.Hour
const minAdminPasswordLength = 9

type authentication struct {
	mu          sync.Mutex
	sessions    map[[32]byte]time.Time
	setupToken  string
	setupFile   string
	nextAttempt time.Time
}

func newAuthentication(ctx context.Context, db *sql.DB, dir string, log *slog.Logger) (*authentication, error) {
	auth := &authentication{sessions: make(map[[32]byte]time.Time), setupFile: filepath.Join(dir, "setup-token")}
	var configured bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM administrator)").Scan(&configured); err != nil {
		return nil, err
	}
	if configured {
		auth.removeSetupFile(log)
		return auth, nil
	}
	auth.setupToken = rand.Text()
	if err := os.WriteFile(auth.setupFile, []byte(auth.setupToken+"\n"), 0600); err != nil {
		return nil, err
	}
	log.Info("Administrator-Ersteinrichtung verfügbar", "datei", auth.setupFile)
	return auth, nil
}

func (a *authentication) removeSetupFile(log *slog.Logger) {
	if err := os.Remove(a.setupFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Warn("Einrichtungsdatei konnte nicht entfernt werden", "error", err)
	}
}

func (a *authentication) validSession(r *http.Request) bool {
	cookie, err := r.Cookie(sessionName)
	if err != nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return time.Now().Before(a.sessions[sha256.Sum256([]byte(cookie.Value))])
}

func (s *site) login(w http.ResponseWriter, r *http.Request) {
	if s.auth.validSession(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.auth.mu.Lock()
	setup := s.auth.setupToken != ""
	s.auth.mu.Unlock()
	s.render(w, r, http.StatusOK, layout("Anmeldung", false, loginView(setup, "")))
}

func passwordHash(password string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, 600000, 32)
}

func (s *site) signIn(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	a := s.auth
	a.mu.Lock()
	defer a.mu.Unlock()
	setup := a.setupToken != ""
	now := time.Now()
	if now.Before(a.nextAttempt) {
		w.Header().Set("Retry-After", "1")
		s.render(w, r, http.StatusTooManyRequests, layout("Anmeldung", false, loginView(setup, "Bitte versuche es in einer Sekunde erneut.")))
		return
	}
	a.nextAttempt = now.Add(time.Second)
	password := r.PostForm.Get("password")
	if utf8.RuneCountInString(password) < minAdminPasswordLength || len(password) > 1024 {
		s.render(w, r, http.StatusBadRequest, layout("Anmeldung", false, loginView(setup, "Das Passwort muss mindestens 9 Zeichen und darf höchstens 1024 Bytes lang sein.")))
		return
	}
	var salt, expected []byte
	if setup {
		if subtle.ConstantTimeCompare([]byte(a.setupToken), []byte(r.PostForm.Get("setup_token"))) != 1 {
			s.render(w, r, http.StatusUnauthorized, layout("Anmeldung", false, loginView(true, "Der Einrichtungscode ist ungültig.")))
			return
		}
		salt = make([]byte, 32)
		rand.Read(salt)
	} else if err := s.db.QueryRowContext(r.Context(), "SELECT salt, password_hash FROM administrator WHERE id = 1").Scan(&salt, &expected); err != nil {
		s.failure(w, err)
		return
	}
	hash, err := passwordHash(password, salt)
	if err != nil {
		s.failure(w, err)
		return
	}
	if setup {
		if _, err := s.db.ExecContext(r.Context(), "INSERT INTO administrator VALUES (1, ?, ?)", salt, hash); err != nil {
			s.failure(w, err)
			return
		}
		a.setupToken = ""
		a.removeSetupFile(s.log)
	} else if subtle.ConstantTimeCompare(hash, expected) != 1 {
		s.render(w, r, http.StatusUnauthorized, layout("Anmeldung", false, loginView(false, "Das Passwort ist ungültig.")))
		return
	}

	for key, expiry := range a.sessions {
		if !now.Before(expiry) {
			delete(a.sessions, key)
		}
	}
	token := rand.Text()
	a.sessions[sha256.Sum256([]byte(token))] = now.Add(sessionAge)
	http.SetCookie(w, sessionCookie(token, int(sessionAge.Seconds())))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func sessionCookie(token string, age int) *http.Cookie {
	return &http.Cookie{Name: sessionName, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: age}
}

func (s *site) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionName); err == nil {
		s.auth.mu.Lock()
		delete(s.auth.sessions, sha256.Sum256([]byte(cookie.Value)))
		s.auth.mu.Unlock()
	}
	http.SetCookie(w, sessionCookie("", -1))
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
