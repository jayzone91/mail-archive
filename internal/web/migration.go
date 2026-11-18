package web

import (
	"errors"
	"net/http"

	"github.com/jayzone91/mail-archive/internal/accounts"
	"github.com/jayzone91/mail-archive/internal/migration"
)

func (s *site) migrationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /migration", s.protect(s.migrationPage))
	mux.HandleFunc("POST /migration", s.protect(s.saveMigration))
}

func (s *site) migrationPage(w http.ResponseWriter, r *http.Request) {
	mappings, err := migration.List(r.Context(), s.db)
	if err != nil {
		s.failure(w, err)
		return
	}
	values, err := s.accounts.List(r.Context())
	if err != nil {
		s.failure(w, err)
		return
	}
	var targets []accounts.Account
	for _, value := range values {
		if value.Enabled {
			targets = append(targets, value)
		}
	}
	s.render(w, r, http.StatusOK, layout("Postfachmigration", true, migrationView(mappings, targets)))
}

func (s *site) saveMigration(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Ungültiges Formular.", http.StatusBadRequest)
		return
	}
	if err := migration.Save(r.Context(), s.db, r.PostForm.Get("source"), r.PostForm.Get("target")); err != nil {
		if errors.Is(err, migration.ErrMapping) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.failure(w, err)
		return
	}
	http.Redirect(w, r, "/migration", http.StatusSeeOther)
}
