package web

import (
	"net/http"
	"time"

	"github.com/jayzone91/mail-archive/internal/retention"
)

func (s *site) retentionPreview(w http.ResponseWriter, r *http.Request) {
	groups, err := retention.Preview(r.Context(), s.db, time.Now())
	if err != nil {
		s.failure(w, err)
		return
	}
	s.render(w, r, http.StatusOK, layout("Aufbewahrungsvorschau", true, retentionView(groups)))
}
