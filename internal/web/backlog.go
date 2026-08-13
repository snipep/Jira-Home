package web

import (
	"net/http"

	"jira-home/internal/model"
)

type SprintSection struct {
	Sprint model.Sprint
	Issues []model.Issue
	Points int
}

type BacklogData struct {
	Project      model.Project
	SprintGroups []SprintSection
	Backlog      []model.Issue
	BacklogPts   int
}

func sumPoints(issues []model.Issue) int {
	total := 0
	for _, i := range issues {
		if i.StoryPoints != nil {
			total += *i.StoryPoints
		}
	}
	return total
}

func (s *Server) handleBacklog(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}

	sprints, err := s.store.ListSprints(project.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	data := BacklogData{Project: project}
	for _, sp := range sprints {
		if sp.State == "completed" {
			continue // completed sprints live in the Sprints/History view, not here
		}
		issues, err := s.store.ListSprintIssuesCurrent(sp.ID)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		data.SprintGroups = append(data.SprintGroups, SprintSection{Sprint: sp, Issues: issues, Points: sumPoints(issues)})
	}

	filter := parseIssueFilter(r)
	backlogIssues, err := s.store.ListBacklog(project.ID, filter)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	data.Backlog = backlogIssues
	data.BacklogPts = sumPoints(backlogIssues)

	s.render(w, r, "backlog.html", data, "Backlog", "backlog")
}
