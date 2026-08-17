package web

import (
	"net/http"

	"jira-home/internal/model"
)

// SprintAnalysis summarizes one completed sprint's outcome for the Analysis
// view: how much of what was planned actually finished, by issue count and
// by story points. Computed fresh from Sprint/SprintReport on every request
// (nothing is cached), so deleting a sprint (Store.DeleteSprint) removes it
// from this view automatically — there's no separate analytics table to
// reconcile.
type SprintAnalysis struct {
	Sprint      model.Sprint
	DoneIssues  int
	TotalIssues int
	DonePoints  int
	TotalPoints int
}

type AnalysisData struct {
	Project    model.Project
	Sprints    []SprintAnalysis
	MaxPoints  int // largest TotalPoints across all sprints, for chart bar scaling
	HasSprints bool
}

func (s *Server) handleAnalysis(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}

	sprints, err := s.store.ListSprints(project.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	data := AnalysisData{Project: project}
	for _, sp := range sprints {
		if sp.State != "completed" {
			continue
		}
		rows, err := s.store.SprintReport(sp.ID)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err.Error())
			return
		}

		entry := SprintAnalysis{Sprint: sp, TotalIssues: len(rows)}
		for _, row := range rows {
			pts := 0
			if row.Issue.StoryPoints != nil {
				pts = *row.Issue.StoryPoints
			}
			entry.TotalPoints += pts
			if row.Completed {
				entry.DoneIssues++
				entry.DonePoints += pts
			}
		}
		if entry.TotalPoints > data.MaxPoints {
			data.MaxPoints = entry.TotalPoints
		}
		data.Sprints = append(data.Sprints, entry)
	}
	data.HasSprints = len(data.Sprints) > 0

	s.render(w, r, "analysis.html", data, "Analysis", "analysis")
}
