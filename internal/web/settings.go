package web

import (
	"net/http"
	"strconv"
	"strings"

	"jira-home/internal/model"
)

type settingsStatusesData struct {
	Statuses []model.Status
	Error    string
}

func (s *Server) handleListStatuses(w http.ResponseWriter, r *http.Request) {
	statuses, err := s.store.ListStatuses()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.render(w, r, "settings_statuses.html", settingsStatusesData{
		Statuses: statuses, Error: r.URL.Query().Get("error"),
	}, "Settings · Statuses", "")
}

func (s *Server) handleCreateStatus(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	category := r.FormValue("category")
	if name == "" {
		s.renderError(w, r, http.StatusBadRequest, "Status name is required")
		return
	}
	if _, err := s.store.CreateStatus(name, category); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.redirectToSettings(w, r, "/settings/statuses")
}

func (s *Server) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	category := r.FormValue("category")
	if err := s.store.UpdateStatus(id, name, category); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.redirectToSettings(w, r, "/settings/statuses")
}

func (s *Server) handleDeleteStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteStatus(id); err != nil {
		s.redirectWithError(w, r, "/settings/statuses", err.Error())
		return
	}
	s.redirectToSettings(w, r, "/settings/statuses")
}

// handleReorderStatuses accepts repeated id= form values, in the desired
// order, from a drag-and-drop reorder control.
func (s *Server) handleReorderStatuses(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	var ids []int64
	for _, raw := range r.Form["id"] {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := s.store.ReorderStatuses(ids); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.redirectToSettings(w, r, "/settings/statuses")
}

func (s *Server) redirectToSettings(w http.ResponseWriter, r *http.Request, path string) {
	if isHXRequest(r) {
		w.Header().Set("HX-Redirect", path)
		return
	}
	http.Redirect(w, r, path, http.StatusFound)
}

type settingsTypesData struct {
	Types []model.IssueType
	Error string
}

func (s *Server) handleListIssueTypes(w http.ResponseWriter, r *http.Request) {
	types, err := s.store.ListIssueTypes()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.render(w, r, "settings_types.html", settingsTypesData{
		Types: types, Error: r.URL.Query().Get("error"),
	}, "Settings · Issue Types", "")
}

func (s *Server) handleCreateIssueType(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.renderError(w, r, http.StatusBadRequest, "Type name is required")
		return
	}
	color := strings.TrimSpace(r.FormValue("color"))
	if color == "" {
		color = "#579DFF"
	}
	icon := strings.TrimSpace(r.FormValue("icon"))
	if icon == "" {
		icon = "✔"
	}
	if _, err := s.store.CreateIssueType(name, color, icon, formBool(r, "no_sprint")); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.redirectToSettings(w, r, "/settings/issue-types")
}

func (s *Server) handleUpdateIssueType(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	color := strings.TrimSpace(r.FormValue("color"))
	icon := strings.TrimSpace(r.FormValue("icon"))
	if err := s.store.UpdateIssueType(id, color, icon, formBool(r, "no_sprint")); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	s.redirectToSettings(w, r, "/settings/issue-types")
}

func (s *Server) handleDeleteIssueType(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteIssueType(id); err != nil {
		s.redirectWithError(w, r, "/settings/issue-types", err.Error())
		return
	}
	s.redirectToSettings(w, r, "/settings/issue-types")
}

// handleWorkspaceSettings/handleUpdateWorkspace let the one workspace
// project be renamed (name, description) — there's no project switcher, so
// this replaces the old create/edit/delete-project UI.
func (s *Server) handleWorkspaceSettings(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	s.render(w, r, "settings_workspace.html", struct {
		Project model.Project
		Error   string
	}{project, r.URL.Query().Get("error")}, "Settings · Workspace", "")
}

func (s *Server) handleUpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.redirectWithError(w, r, "/settings/workspace", "Name is required")
		return
	}
	if err := s.store.UpdateProject(project.ID, name, r.FormValue("description")); err != nil {
		s.redirectWithError(w, r, "/settings/workspace", err.Error())
		return
	}
	s.redirectToSettings(w, r, "/settings/workspace")
}
