package web

import (
	"net/http"
	"strconv"

	"github.com/jayzone91/mail-archive/internal/backup"
)

func backupStatus(status string) string {
	switch status {
	case "queued":
		return "Wartet"
	case "running":
		return "Läuft"
	case "succeeded":
		return "Abgeschlossen"
	case "failed":
		return "Fehlgeschlagen"
	default:
		return status
	}
}

func (s *site) backupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /backups", s.protect(s.listBackups))
	mux.HandleFunc("GET /backups/status", s.protect(s.backupStatusFragment))
	mux.HandleFunc("POST /accounts/{id}/backup", s.protect(s.startBackup))
	mux.HandleFunc("POST /backups", s.protect(s.startBackup))
	mux.HandleFunc("POST /backups/{job}/retry", s.protect(s.retryBackup))
}

func (s *site) backupStatusFragment(w http.ResponseWriter, r *http.Request) {
	jobs, err := backup.List(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, backupJobs(jobs))
}

func (s *site) listBackups(w http.ResponseWriter, r *http.Request) {
	jobs, err := backup.List(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, layout("Sicherungen", true, backupsView(jobs)))
}

func (s *site) startBackup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	mode := r.PostForm.Get("mode")
	if mode != "full" && mode != "incremental" {
		http.Error(w, "Ungültiger Sicherungsmodus.", http.StatusBadRequest)
		return
	}
	added, err := backup.Enqueue(r.Context(), s.db, r.PathValue("id"), mode)
	if err != nil {
		s.failure(w, err)
		return
	}
	if !added {
		http.Error(w, "Für die ausgewählten Postfächer kann derzeit keine weitere Sicherung gestartet werden.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/backups", http.StatusSeeOther)
}

func (s *site) retryBackup(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("job"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	queued, err := backup.Retry(r.Context(), s.db, id)
	if err != nil {
		s.failure(w, err)
		return
	}
	if !queued {
		http.Error(w, "Dieser Auftrag kann zurzeit nicht fortgesetzt werden.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/backups", http.StatusSeeOther)
}
