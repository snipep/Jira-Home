package web

import (
	"net/http"
	"strings"
)

func (s *Server) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	issue, ok := s.issueFromPath(w, r)
	if !ok {
		return
	}
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		s.renderError(w, r, http.StatusBadRequest, "Comment can't be empty")
		return
	}
	if _, err := s.store.CreateComment(issue.ID, body); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderCommentList(w, r, issue.ID)
}

func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	issue, ok := s.issueFromPath(w, r)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		s.renderError(w, r, http.StatusBadRequest, "Comment can't be empty")
		return
	}
	if err := s.store.UpdateComment(id, body); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderCommentList(w, r, issue.ID)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	issue, ok := s.issueFromPath(w, r)
	if !ok {
		return
	}
	id, ok := pathInt64(w, r, "id")
	if !ok {
		return
	}
	if err := s.store.DeleteComment(id); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderCommentList(w, r, issue.ID)
}

func (s *Server) renderCommentList(w http.ResponseWriter, r *http.Request, issueID int64) {
	comments, err := s.store.ListComments(issueID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.render(w, r, "comment_list.html", comments, "", "")
}
