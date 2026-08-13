package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"jira-home/internal/model"
	"jira-home/internal/store"
)

// currentProject returns the single workspace project every route
// implicitly operates against, writing a 500 if it's somehow missing (it's
// created at startup by store.EnsureDefaultProject, so this should never
// actually happen).
func (s *Server) currentProject(w http.ResponseWriter, r *http.Request) (model.Project, bool) {
	p, err := s.store.DefaultProject()
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusInternalServerError, "no workspace project is configured")
		return p, false
	}
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return p, false
	}
	return p, true
}

// issueFromPath resolves /issues/{num} to an issue in the workspace project.
func (s *Server) issueFromPath(w http.ResponseWriter, r *http.Request) (model.Issue, bool) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return model.Issue{}, false
	}
	num, err := strconv.Atoi(r.PathValue("num"))
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "invalid issue number")
		return model.Issue{}, false
	}
	issue, err := s.store.GetIssueByKey(project.ID, num)
	if errors.Is(err, store.ErrNotFound) {
		s.renderError(w, r, http.StatusNotFound, "No such issue")
		return issue, false
	}
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return issue, false
	}
	return issue, true
}

func pathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		http.Error(w, "invalid "+name, http.StatusBadRequest)
		return 0, false
	}
	return v, true
}

// splitTags parses a comma-separated form field ("frontend, backend") into
// trimmed, non-empty values — used for both labels and components.
func splitTags(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func formIntPtr(r *http.Request, name string) *int {
	raw := strings.TrimSpace(r.FormValue(name))
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil
	}
	return &n
}

func formInt64Ptr(r *http.Request, name string) *int64 {
	raw := strings.TrimSpace(r.FormValue(name))
	if raw == "" {
		return nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func formFloat64Ptr(r *http.Request, name string) *float64 {
	raw := strings.TrimSpace(r.FormValue(name))
	if raw == "" {
		return nil
	}
	n, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &n
}

func formBool(r *http.Request, name string) bool {
	v := strings.ToLower(strings.TrimSpace(r.FormValue(name)))
	return v == "on" || v == "true" || v == "1"
}
