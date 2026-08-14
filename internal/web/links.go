package web

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	issue, iok := s.issueFromPath(w, r)
	if !ok || !iok {
		return
	}

	targetKey := strings.TrimSpace(strings.ToUpper(r.FormValue("target_key")))
	linkType := r.FormValue("link_type")
	num, err := strconv.Atoi(strings.TrimPrefix(targetKey, project.KeyPrefix+"-"))
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Enter a valid issue key, e.g. "+project.KeyPrefix+"-12")
		return
	}
	target, err := s.store.GetIssueByKey(project.ID, num)
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "No issue "+targetKey+" in this project")
		return
	}
	if err := s.store.CreateLink(issue.ID, target.ID, linkType); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := s.store.GetIssueByID(issue.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderIssueDetail(w, r, project, updated)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.issueFromPath(w, r); !ok {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteLink(id); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusOK)
}
