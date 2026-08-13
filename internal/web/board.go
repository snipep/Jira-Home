package web

import (
	"net/http"
	"strconv"

	"jira-home/internal/model"
	"jira-home/internal/store"
)

type BoardColumn struct {
	Status model.Status
	Issues []model.Issue
}

type BoardData struct {
	Project model.Project
	Sprint  *model.Sprint
	Columns []BoardColumn
}

// currentBoardSprint resolves which sprint the board shows: the ?sprint=
// query param if given, otherwise the most-recently-started active sprint
// (approximated as the highest-id active sprint, since the schema doesn't
// track a separate "started_at" timestamp — see the design review note on
// the board defaulting behavior).
func (s *Server) currentBoardSprint(projectID int64, r *http.Request) (*model.Sprint, error) {
	if idStr := r.URL.Query().Get("sprint"); idStr != "" {
		if id, err := strconv.ParseInt(idStr, 10, 64); err == nil {
			sp, err := s.store.GetSprintByID(id)
			if err == nil && sp.ProjectID == projectID {
				return &sp, nil
			}
		}
	}
	sprints, err := s.store.ListSprints(projectID)
	if err != nil {
		return nil, err
	}
	var best *model.Sprint
	for i := range sprints {
		if sprints[i].State != "active" {
			continue
		}
		if best == nil || sprints[i].ID > best.ID {
			best = &sprints[i]
		}
	}
	return best, nil
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}

	sprint, err := s.currentBoardSprint(project.ID, r)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	statuses, err := s.store.ListStatuses()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	data := BoardData{Project: project, Sprint: sprint}
	filter := parseIssueFilter(r)

	if sprint != nil {
		issues, err := s.store.ListBoard(project.ID, sprint.ID, filter)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		byStatus := map[int64][]model.Issue{}
		for _, iss := range issues {
			byStatus[iss.StatusID] = append(byStatus[iss.StatusID], iss)
		}
		for _, st := range statuses {
			data.Columns = append(data.Columns, BoardColumn{Status: st, Issues: byStatus[st.ID]})
		}
	} else {
		for _, st := range statuses {
			data.Columns = append(data.Columns, BoardColumn{Status: st})
		}
	}

	s.render(w, r, "board.html", data, "Board", "board")
}

// parseIssueFilter reads the shared Type/Label/Priority/q query params used
// by Board, Backlog, and Epics & Stories.
func parseIssueFilter(r *http.Request) store.IssueFilter {
	q := r.URL.Query()
	return store.IssueFilter{
		Types:      q["type"],
		Labels:     q["label"],
		Priorities: q["priority"],
		Query:      q.Get("q"),
	}
}
