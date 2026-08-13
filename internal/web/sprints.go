package web

import (
	"net/http"
	"strings"

	"jira-home/internal/model"
	"jira-home/internal/store"
)

type SprintHistoryEntry struct {
	Sprint model.Sprint
	Rows   []store.SprintReportRow // only populated for completed sprints
	Done   int
	Total  int
}

// handleListSprints is the Sprints/History view: a quick summary of what's
// currently planned/active, plus every completed sprint rendered with its
// full historical issue list (via sprint_issue) so carried-over work stays
// visible instead of just disappearing when it moves on.
func (s *Server) handleListSprints(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	sprints, err := s.store.ListSprints(project.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	var current, completed []SprintHistoryEntry
	for _, sp := range sprints {
		entry := SprintHistoryEntry{Sprint: sp}
		if sp.State == "completed" {
			rows, err := s.store.SprintReport(sp.ID)
			if err != nil {
				s.renderError(w, r, http.StatusInternalServerError, err.Error())
				return
			}
			entry.Rows = rows
			entry.Total = len(rows)
			for _, row := range rows {
				if row.Completed {
					entry.Done++
				}
			}
			completed = append(completed, entry)
		} else {
			current = append(current, entry)
		}
	}
	// Most-recently-completed first.
	for i, j := 0, len(completed)-1; i < j; i, j = i+1, j-1 {
		completed[i], completed[j] = completed[j], completed[i]
	}

	s.render(w, r, "sprints.html", struct {
		Project   model.Project
		Current   []SprintHistoryEntry
		Completed []SprintHistoryEntry
	}{project, current, completed}, "Sprints", "sprints")
}

func (s *Server) handleNewSprintForm(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	s.render(w, r, "sprint_form.html", project, "New sprint", "backlog")
}

func (s *Server) handleCreateSprint(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.renderError(w, r, http.StatusBadRequest, "Sprint name is required")
		return
	}
	_, err := s.store.CreateSprint(project.ID, name, r.FormValue("goal"), r.FormValue("start_date"), r.FormValue("end_date"))
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.redirectToBacklog(w, r)
}

func (s *Server) handleUpdateSprint(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	existing, err := s.store.GetSprintByID(id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, err.Error())
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = existing.Name
	}
	if err := s.store.UpdateSprint(id, name, r.FormValue("goal"), r.FormValue("start_date"), r.FormValue("end_date")); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.redirectToBacklog(w, r)
}

func (s *Server) handleStartSprint(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.StartSprint(id); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.redirectToBacklog(w, r)
}

func (s *Server) redirectToBacklog(w http.ResponseWriter, r *http.Request) {
	if isHXRequest(r) {
		w.Header().Set("HX-Redirect", "/backlog")
		return
	}
	http.Redirect(w, r, "/backlog", http.StatusFound)
}

func (s *Server) handleCompleteSprintPreview(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	sprint, err := s.store.GetSprintByID(id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, err.Error())
		return
	}
	target, err := s.store.PreviewCompletion(id)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	// Zero unfinished issues: skip the confirmation and complete immediately.
	if target.UnfinishedCount == 0 {
		if err := s.store.CompleteSprint(id); err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		s.redirectToBacklog(w, r)
		return
	}

	s.render(w, r, "sprint_complete.html", struct {
		Project model.Project
		Sprint  model.Sprint
		Target  store.CompletionTarget
	}{project, sprint, target}, "Complete "+sprint.Name, "backlog")
}

func (s *Server) handleCompleteSprint(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.CompleteSprint(id); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.redirectToBacklog(w, r)
}

func (s *Server) handleSprintReport(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	sprint, err := s.store.GetSprintByID(id)
	if err != nil {
		s.renderError(w, r, http.StatusNotFound, err.Error())
		return
	}
	rows, err := s.store.SprintReport(id)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	done := 0
	for _, row := range rows {
		if row.Completed {
			done++
		}
	}
	s.render(w, r, "sprint_report.html", struct {
		Project model.Project
		Sprint  model.Sprint
		Rows    []store.SprintReportRow
		Done    int
		Total   int
	}{project, sprint, rows, done, len(rows)}, sprint.Name+" report", "sprints")
}
