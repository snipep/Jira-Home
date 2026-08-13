package web

import (
	"encoding/json"
	"net/http"
)

// handleListLabels/handleListComponents back autocomplete only — labels and
// components are freeform tags on the issue, not a managed list. No HTML
// view; JSON is the only useful response here.
func (s *Server) handleListLabels(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	labels, err := s.store.ListDistinctLabels(project.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(labels)
}

func (s *Server) handleListComponents(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	components, err := s.store.ListDistinctComponents(project.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(components)
}
