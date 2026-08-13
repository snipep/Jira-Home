// Package web is the HTTP layer: routing, content negotiation, and
// templates. Handlers call into store for data and never touch SQL.
package web

import (
	"html/template"
	"net/http"

	"jira-home/internal/store"
)

type Server struct {
	store     *store.Store
	templates *template.Template
	mux       *http.ServeMux
}

func NewServer(st *store.Store) *Server {
	s := &Server{
		store:     st,
		templates: loadTemplates(),
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	m := s.mux

	// Global search
	m.HandleFunc("GET /search", s.handleSearch)

	// Board & backlog — this app is single-workspace, so these (and
	// everything below) operate on the one project directly, with no
	// /projects/{key} prefix.
	m.HandleFunc("GET /board", s.handleBoard)
	m.HandleFunc("GET /backlog", s.handleBacklog)

	// Epics & Stories
	m.HandleFunc("GET /epics-stories", s.handleEpicsStories)

	// Issues
	m.HandleFunc("GET /issues", s.handleListIssues)
	m.HandleFunc("POST /issues", s.handleCreateIssue)
	m.HandleFunc("GET /issues/new", s.handleNewIssueForm)
	m.HandleFunc("GET /issues/{num}", s.handleIssueDetail)
	m.HandleFunc("GET /issues/{num}/edit", s.handleEditIssueForm)
	m.HandleFunc("PATCH /issues/{num}", s.handleUpdateIssue)
	m.HandleFunc("DELETE /issues/{num}", s.handleDeleteIssue)
	m.HandleFunc("PATCH /issues/{num}/move", s.handleMoveIssue)

	// Comments
	m.HandleFunc("POST /issues/{num}/comments", s.handleCreateComment)
	m.HandleFunc("PATCH /issues/{num}/comments/{id}", s.handleUpdateComment)
	m.HandleFunc("DELETE /issues/{num}/comments/{id}", s.handleDeleteComment)

	// Links
	m.HandleFunc("POST /issues/{num}/links", s.handleCreateLink)
	m.HandleFunc("DELETE /issues/{num}/links/{id}", s.handleDeleteLink)

	// Labels / components (autocomplete only)
	m.HandleFunc("GET /labels", s.handleListLabels)
	m.HandleFunc("GET /components", s.handleListComponents)

	// Sprints
	m.HandleFunc("GET /sprints", s.handleListSprints)
	m.HandleFunc("POST /sprints", s.handleCreateSprint)
	m.HandleFunc("GET /sprints/new", s.handleNewSprintForm)
	m.HandleFunc("PATCH /sprints/{id}", s.handleUpdateSprint)
	m.HandleFunc("POST /sprints/{id}/start", s.handleStartSprint)
	m.HandleFunc("GET /sprints/{id}/complete", s.handleCompleteSprintPreview)
	m.HandleFunc("POST /sprints/{id}/complete", s.handleCompleteSprint)
	m.HandleFunc("GET /sprints/{id}/report", s.handleSprintReport)

	// Global settings
	m.HandleFunc("GET /settings/statuses", s.handleListStatuses)
	m.HandleFunc("POST /settings/statuses", s.handleCreateStatus)
	m.HandleFunc("PATCH /settings/statuses/{id}", s.handleUpdateStatus)
	m.HandleFunc("DELETE /settings/statuses/{id}", s.handleDeleteStatus)
	m.HandleFunc("POST /settings/statuses/reorder", s.handleReorderStatuses)
	m.HandleFunc("GET /settings/issue-types", s.handleListIssueTypes)
	m.HandleFunc("POST /settings/issue-types", s.handleCreateIssueType)
	m.HandleFunc("PATCH /settings/issue-types/{id}", s.handleUpdateIssueType)
	m.HandleFunc("DELETE /settings/issue-types/{id}", s.handleDeleteIssueType)
	m.HandleFunc("GET /settings/workspace", s.handleWorkspaceSettings)
	m.HandleFunc("POST /settings/workspace", s.handleUpdateWorkspace)

	m.HandleFunc("GET /", s.handleRoot)
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/board", http.StatusFound)
}
