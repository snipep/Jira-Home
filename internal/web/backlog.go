package web

import (
	"net/http"

	"jira-home/internal/model"
)

type SprintSection struct {
	Sprint          model.Sprint
	Issues          []model.Issue
	Points          int
	TodoCount       int
	InProgressCount int
	DoneCount       int
}

// countByStatusCategory tallies a sprint's issues into the three status
// buckets the backlog header's mini count badges show.
func countByStatusCategory(issues []model.Issue) (todo, inProgress, done int) {
	for _, iss := range issues {
		switch iss.StatusCategory {
		case "done":
			done++
		case "in_progress":
			inProgress++
		default:
			todo++
		}
	}
	return todo, inProgress, done
}

type BacklogData struct {
	Project      model.Project
	SprintGroups []SprintSection
	Backlog      []model.Issue
	BacklogPts   int
	Error        string
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

	data := BacklogData{Project: project, Error: r.URL.Query().Get("error")}
	for _, sp := range sprints {
		if sp.State == "completed" {
			continue // completed sprints live in the Sprints/History view, not here
		}
		issues, err := s.store.ListSprintIssuesCurrent(sp.ID)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		todo, inProgress, done := countByStatusCategory(issues)
		data.SprintGroups = append(data.SprintGroups, SprintSection{
			Sprint: sp, Issues: issues, Points: sumPoints(issues),
			TodoCount: todo, InProgressCount: inProgress, DoneCount: done,
		})
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
