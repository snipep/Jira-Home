package web

import (
	"net/http"

	"jira-home/internal/model"
)

type TypeGroup struct {
	Type   model.IssueType
	Issues []model.Issue
	Points int
}

type EpicsStoriesData struct {
	Project model.Project
	Groups  []TypeGroup
}

func (s *Server) handleEpicsStories(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}

	types, err := s.store.ListIssueTypes()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	filter := parseIssueFilter(r)
	issues, err := s.store.ListEpicsStories(project.ID, filter)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	byType := map[int64][]model.Issue{}
	for _, iss := range issues {
		byType[iss.IssueTypeID] = append(byType[iss.IssueTypeID], iss)
	}

	data := EpicsStoriesData{Project: project}
	for _, t := range types {
		if !t.NoSprint {
			continue
		}
		group := byType[t.ID]
		data.Groups = append(data.Groups, TypeGroup{Type: t, Issues: group, Points: sumPoints(group)})
	}

	s.render(w, r, "epics_stories.html", data, "Epics & Stories", "epics-stories")
}
