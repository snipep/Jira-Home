package web

import (
	"net/http"

	"jira-home/internal/model"
)

// handleSearch backs the topbar's global search box — distinct from
// GET /issues?q=, which is the same search scoped to the Issues list page.
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	var issues []model.Issue
	if q != "" {
		var err error
		issues, err = s.store.SearchIssues(nil, q)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
	}

	s.render(w, r, "search.html", struct {
		Query  string
		Issues []model.Issue
	}{q, issues}, "Search", "")
}
