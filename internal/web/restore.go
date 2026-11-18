package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jayzone91/mail-archive/internal/accounts"
	"github.com/jayzone91/mail-archive/internal/restore"
)

type restoreChoice struct{ ID, Label string }

func (s *site) restoreRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /restore", s.protect(s.restorePage))
	mux.HandleFunc("GET /restore/status", s.protect(s.restoreStatusFragment))
	mux.HandleFunc("POST /restore", s.protect(s.restoreSubmit))
	mux.HandleFunc("GET /restore/{job}", s.protect(s.restoreIssues))
	mux.HandleFunc("POST /restore/{job}/retry", s.protect(s.restoreRetry))
}

func (s *site) restoreStatusFragment(w http.ResponseWriter, r *http.Request) {
	jobs, err := restore.List(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, restoreJobs(jobs))
}

func (s *site) restorePage(w http.ResponseWriter, r *http.Request) {
	req := restore.Request{Source: restore.Source{Kind: r.URL.Query().Get("kind"), ID: r.URL.Query().Get("source")},
		Target: r.URL.Query().Get("target"), PreserveDate: true, PreserveFlags: true}
	if selected := r.URL.Query()["selected"]; len(selected) > 0 && len(selected) <= 100 {
		req.Source = restore.Source{Kind: "messages", ID: strings.Join(selected, ",")}
	}
	s.renderRestore(w, r, http.StatusOK, req, 0, "")
}

func (s *site) renderRestore(w http.ResponseWriter, r *http.Request, status int, req restore.Request, count int, note string) {
	accountsList, err := s.accounts.List(r.Context())
	if err != nil {
		s.failure(w, err)
		return
	}
	var targets []accounts.Account
	for _, account := range accountsList {
		if account.Enabled {
			targets = append(targets, account)
		}
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT 'account:'||a.id,a.name FROM accounts a
 WHERE EXISTS(SELECT 1 FROM folders f JOIN messages m ON m.folder_id=f.id WHERE f.account_id=a.id)
 UNION ALL SELECT 'folder:'||f.id,a.name||' / '||f.name FROM folders f JOIN accounts a ON a.id=f.account_id
 WHERE EXISTS(SELECT 1 FROM messages m WHERE m.folder_id=f.id)
 ORDER BY 1,2`)
	if err != nil {
		s.failure(w, err)
		return
	}
	defer rows.Close()
	var sources []restoreChoice
	for rows.Next() {
		var entry restoreChoice
		if err := rows.Scan(&entry.ID, &entry.Label); err != nil {
			s.failure(w, err)
			return
		}
		sources = append(sources, entry)
	}
	if err := rows.Err(); err != nil {
		s.failure(w, err)
		return
	}
	jobs, err := restore.List(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, status, layout("Wiederherstellung", true, restoreView(sources, targets, jobs, req, count, note)))
}

func (s *site) restoreSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	kind, id, _ := strings.Cut(r.PostForm.Get("source"), ":")
	req := restore.Request{Source: restore.Source{Kind: kind, ID: id}, Target: r.PostForm.Get("target"),
		Folder: strings.TrimSpace(r.PostForm.Get("folder")), PreserveDate: r.PostForm.Get("date") == "on", PreserveFlags: r.PostForm.Get("flags") == "on"}
	switch r.PostForm.Get("action") {
	case "preview":
		count, err := restore.Preview(r.Context(), s.db, req)
		if err != nil {
			s.restoreFormError(w, r, req, err)
			return
		}
		s.renderRestore(w, r, http.StatusOK, req, count, "")
	case "queue":
		if err := restore.Queue(r.Context(), s.db, req); err != nil {
			s.restoreFormError(w, r, req, err)
			return
		}
		http.Redirect(w, r, "/restore", http.StatusSeeOther)
	default:
		http.Error(w, "Ungültige Wiederherstellungsaktion.", http.StatusBadRequest)
	}
}

func (s *site) restoreFormError(w http.ResponseWriter, r *http.Request, req restore.Request, err error) {
	if errors.Is(err, restore.ErrSelection) || errors.Is(err, restore.ErrEmpty) {
		s.renderRestore(w, r, http.StatusBadRequest, req, 0, err.Error())
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		s.renderRestore(w, r, http.StatusConflict, req, 0, "Das Zielpostfach steht nicht zur Verfügung oder hat bereits einen Restore-Auftrag.")
		return
	}
	s.failure(w, err)
}

func (s *site) restoreIssues(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("job"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	issues, err := restore.Issues(r.Context(), s.db, id)
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, layout("Wiederherstellungsprotokoll", true, restoreIssuesView(issues)))
}

func (s *site) restoreRetry(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("job"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	if err := restore.Retry(r.Context(), s.db, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Der Auftrag kann derzeit nicht fortgesetzt werden.", http.StatusConflict)
			return
		}
		s.failure(w, err)
		return
	}
	http.Redirect(w, r, "/restore", http.StatusSeeOther)
}
