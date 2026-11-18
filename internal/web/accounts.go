package web

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jayzone91/mail-archive/internal/accounts"
	"github.com/jayzone91/mail-archive/internal/imap"
)

func (s *site) accountRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /accounts", s.protect(s.listAccounts))
	mux.HandleFunc("GET /accounts/new", s.protect(s.editAccount))
	mux.HandleFunc("POST /accounts/new", s.protect(s.saveAccount))
	mux.HandleFunc("GET /accounts/{id}", s.protect(s.editAccount))
	mux.HandleFunc("POST /accounts/{id}", s.protect(s.saveAccount))
	mux.HandleFunc("POST /accounts/{id}/test", s.protect(s.testAccount))
	mux.HandleFunc("POST /accounts/{id}/remove", s.protect(s.removeAccount))
	mux.HandleFunc("GET /accounts/{id}/remove", s.protect(s.confirmRemoveAccount))
}

func (s *site) listAccounts(w http.ResponseWriter, r *http.Request) {
	values, err := s.accounts.List(r.Context())
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, layout("Postfächer", true, accountsView(values)))
}

func (s *site) editAccount(w http.ResponseWriter, r *http.Request) {
	value := accounts.Account{Endpoint: imap.Endpoint{Port: 993, TLSMode: "implicit"}, Enabled: true, AutoBackup: true}
	if id := r.PathValue("id"); id != "" {
		var err error
		value, err = s.accounts.Get(r.Context(), id)
		if err != nil {
			s.accountError(w, err)
			return
		}
	}
	s.render(w, r, http.StatusOK, layout("Postfach bearbeiten", true, accountView(value, "")))
}

func (s *site) saveAccount(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	get := func(name string) string { return strings.TrimSpace(r.PostForm.Get(name)) }
	port, _ := strconv.Atoi(get("port"))
	retention, retentionErr := strconv.Atoi(get("retention"))
	if get("retention") != "" && (retentionErr != nil || retention < 1) {
		retention = -1
	}
	value := accounts.Account{
		Endpoint: imap.Endpoint{Host: get("host"), Username: get("username"), TLSMode: get("tls"), Port: port},
		ID:       r.PathValue("id"), Name: get("name"), Email: get("email"),
		Enabled: get("enabled") == "on", AutoBackup: get("auto_backup") == "on", RetentionMonths: retention,
	}
	password := []byte(r.PostForm.Get("password"))
	defer clear(password)
	if err := s.accounts.Save(r.Context(), value, password); err != nil {
		switch {
		case errors.Is(err, accounts.ErrInvalid), errors.Is(err, accounts.ErrPassword):
			s.render(w, r, http.StatusBadRequest, layout("Postfach bearbeiten", true, accountView(value, err.Error())))
		case errors.Is(err, accounts.ErrBusy), errors.Is(err, accounts.ErrSource):
			s.render(w, r, http.StatusConflict, layout("Postfach bearbeiten", true, accountView(value, err.Error())))
		case errors.Is(err, sql.ErrNoRows):
			http.NotFound(w, r)
		default:
			var secretError *accounts.CredentialError
			if errors.As(err, &secretError) {
				s.render(w, r, http.StatusServiceUnavailable, layout("Postfach bearbeiten", true, accountView(value, secretError.Error())))
				return
			}
			s.failure(w, err)
		}
		return
	}
	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

func (s *site) confirmRemoveAccount(w http.ResponseWriter, r *http.Request) {
	value, err := s.accounts.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.accountError(w, err)
		return
	}
	s.render(w, r, http.StatusOK, layout("Postfachkonfiguration entfernen", true, removeAccountView(value)))
}

func (s *site) removeAccount(w http.ResponseWriter, r *http.Request) {
	if err := s.accounts.Remove(r.Context(), r.PathValue("id")); err != nil {
		s.accountError(w, err)
		return
	}
	http.Redirect(w, r, "/accounts", http.StatusSeeOther)
}

func (s *site) testAccount(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	value, err := s.accounts.Get(ctx, r.PathValue("id"))
	if err != nil {
		s.accountError(w, err)
		return
	}
	password, err := s.accounts.Password(ctx, value.ID)
	if err != nil {
		var secretError *accounts.CredentialError
		if errors.As(err, &secretError) || errors.Is(err, accounts.ErrPassword) {
			s.render(w, r, http.StatusServiceUnavailable, layout("Verbindungstest", true, connectionView(nil, err.Error())))
			return
		}
		s.accountError(w, err)
		return
	}
	defer clear(password)
	folders, err := imap.Discover(ctx, value.Endpoint, password)
	if err != nil {
		s.render(w, r, http.StatusBadGateway, layout("Verbindungstest", true, connectionView(nil, err.Error())))
		return
	}
	names := make([]string, 0, len(folders))
	for _, folder := range folders {
		names = append(names, folder.Mailbox)
	}
	s.render(w, r, http.StatusOK, layout("Verbindungstest", true, connectionView(names, "")))
}

func (s *site) accountError(w http.ResponseWriter, err error) {
	if errors.Is(err, accounts.ErrBusy) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Postfach nicht gefunden.", http.StatusNotFound)
		return
	}
	s.failure(w, err)
}
