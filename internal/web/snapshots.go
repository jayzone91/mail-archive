package web

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jayzone91/mail-archive/internal/snapshot"
)

func (s *site) snapshotRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /snapshots", s.protect(s.listSnapshots))
	mux.HandleFunc("GET /snapshots/status", s.protect(s.snapshotStatusFragment))
	mux.HandleFunc("POST /snapshots", s.protect(s.createSnapshot))
	mux.HandleFunc("POST /snapshots/automatic", s.protect(s.configureSnapshots))
	mux.HandleFunc("POST /snapshots/export", s.protect(s.exportInstallation))
}

func (s *site) exportInstallation(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	password := r.PostForm.Get("password")
	if password != r.PostForm.Get("password_confirm") {
		http.Error(w, "Die Sicherungspasswörter stimmen nicht überein.", http.StatusBadRequest)
		return
	}
	if !s.exportMu.TryLock() {
		http.Error(w, "Ein Sicherungspaket wird bereits erstellt.", http.StatusConflict)
		return
	}
	defer s.exportMu.Unlock()
	// Large databases and snapshots can take longer than ordinary page requests.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	file, err := os.CreateTemp(s.dataDir, ".encrypted-")
	if err != nil {
		s.failure(w, err)
		return
	}
	defer os.Remove(file.Name())
	defer file.Close()
	err = snapshot.ExportPortable(r.Context(), s.db, s.dataDir, r.PostForm.Get("snapshot"), password, file)
	switch {
	case errors.Is(err, snapshot.ErrPortableSnapshot), errors.Is(err, snapshot.ErrPortablePassword):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "Bitte einen verifizierten Snapshot auswählen.", http.StatusBadRequest)
		return
	case err != nil:
		s.failure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="mailarchive-transfer.age"`)
	http.ServeContent(w, r, "mailarchive-transfer.age", time.Time{}, file)
}

func (s *site) listSnapshots(w http.ResponseWriter, r *http.Request) {
	s.renderSnapshots(w, r, false)
}

func (s *site) snapshotStatusFragment(w http.ResponseWriter, r *http.Request) {
	s.renderSnapshots(w, r, true)
}

func (s *site) renderSnapshots(w http.ResponseWriter, r *http.Request, fragment bool) {
	target, err := snapshot.AutomaticTarget(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	coverage, err := snapshot.CurrentCoverage(r.Context(), s.db, target)
	if err != nil {
		s.failure(w, err)
		return
	}
	jobs, err := snapshot.List(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	if fragment {
		s.render(w, r, http.StatusOK, snapshotJobs(jobs, coverage))
		return
	}
	s.render(w, r, http.StatusOK, layout("Zweitsicherungen", true, snapshotsView(target, jobs, coverage)))
}

func (s *site) configureSnapshots(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	target := strings.TrimSpace(r.PostForm.Get("target"))
	switch r.PostForm.Get("action") {
	case "enable":
		if target == "" || r.PostForm.Get("independent") != "on" {
			http.Error(w, "Bitte ein unabhängiges Speicherziel angeben und bestätigen.", http.StatusBadRequest)
			return
		}
	case "disable":
		target = ""
	default:
		http.Error(w, "Ungültige Aktion.", http.StatusBadRequest)
		return
	}
	err := snapshot.ConfigureAutomatic(r.Context(), s.db, s.dataDir, s.archiveLocation.Current(), target)
	if errors.Is(err, snapshot.ErrTarget) || errors.Is(err, snapshot.ErrSameStorage) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		s.failure(w, err)
		return
	}
	http.Redirect(w, r, "/snapshots", http.StatusSeeOther)
}

func (s *site) createSnapshot(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	added, err := snapshot.Enqueue(r.Context(), s.db, s.dataDir, s.archiveLocation.Current(), strings.TrimSpace(r.PostForm.Get("target")))
	if errors.Is(err, snapshot.ErrTarget) || errors.Is(err, snapshot.ErrSameStorage) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err != nil {
		s.failure(w, err)
		return
	}
	if !added {
		http.Error(w, "Eine Zweitsicherung ist bereits eingeplant oder läuft.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/snapshots", http.StatusSeeOther)
}

func snapshotStatus(status string) string {
	if status == "verified" {
		return "Dateien und Prüfsummen verifiziert"
	}
	return backupStatus(status)
}
