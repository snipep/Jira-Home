package web

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"jira-home/internal/model"
	"jira-home/internal/store"
)

func (s *Server) handleListIssues(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	q := r.URL.Query().Get("q")
	issues, err := s.store.SearchIssues(&project.ID, q)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	s.render(w, r, "issue_list.html", struct {
		Project model.Project
		Issues  []model.Issue
		Query   string
	}{project, issues, q}, "Issues", "")
}

type IssueFormData struct {
	Project          model.Project
	IssueTypes       []model.IssueType
	SelectedType     string
	SprintID         *int64
	Editing          bool
	Issue            model.Issue
	ParentCandidates []model.Issue // other issues in the project, selectable as this issue's parent
	SelectedParentID *int64
}

func (s *Server) handleNewIssueForm(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	types, err := s.store.ListIssueTypes()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	candidates, err := s.store.SearchIssues(&project.ID, "")
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	data := IssueFormData{
		Project:          project,
		IssueTypes:       types,
		SelectedType:     r.URL.Query().Get("type"),
		SprintID:         queryInt64Ptr(r, "sprint_id"),
		ParentCandidates: candidates,
		SelectedParentID: queryInt64Ptr(r, "parent_id"),
	}
	if data.SelectedType == "" {
		data.SelectedType = "Task"
	}
	s.render(w, r, "issue_form.html", data, "New issue", "")
}

func queryInt64Ptr(r *http.Request, name string) *int64 {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func (s *Server) handleCreateIssue(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	typeName := strings.TrimSpace(r.FormValue("type"))
	issueType, err := s.store.GetIssueTypeByName(typeName)
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, "Unknown issue type "+typeName)
		return
	}
	summary := strings.TrimSpace(r.FormValue("summary"))
	if summary == "" {
		s.renderError(w, r, http.StatusBadRequest, "Summary is required")
		return
	}

	in := store.NewIssueInput{
		IssueTypeID: issueType.ID,
		Summary:     summary,
		Description: r.FormValue("description"),
		Priority:    strings.ToLower(strings.TrimSpace(r.FormValue("priority"))),
		StoryPoints: formIntPtr(r, "story_points"),
		DueDate:     strings.TrimSpace(r.FormValue("due_date")),
		SprintID:    formInt64Ptr(r, "sprint_id"),
		ParentID:    formInt64Ptr(r, "parent_id"),
		Labels:      splitTags(r.FormValue("labels")),
		Components:  splitTags(r.FormValue("components")),
	}

	issue, err := s.store.CreateIssue(project.ID, in)
	if err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	redirectTo := issueRedirectTarget(issue)
	if isHXRequest(r) {
		w.Header().Set("HX-Redirect", redirectTo)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

// issueRedirectTarget sends the user somewhere sensible after creating an
// issue: Epics & Stories for no_sprint types (Epic/Story), since they never
// appear on the board/backlog; the new issue's own detail page otherwise.
func issueRedirectTarget(issue model.Issue) string {
	if issue.TypeNoSprint {
		return "/epics-stories"
	}
	return "/issues/" + strconv.Itoa(issue.IssueNumber)
}

type IssueDetailData struct {
	Project  model.Project
	Issue    model.Issue
	Parent   *model.Issue // set when the issue has a parent (e.g. a subtask's parent task)
	Comments []model.Comment
	Links    []model.IssueLink
	Sprints  []model.Sprint // candidates for the Sprint-move dropdown
	Statuses []model.Status
}

func (s *Server) handleIssueDetail(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	issue, iok := s.issueFromPath(w, r)
	if !ok || !iok {
		return
	}

	comments, err := s.store.ListComments(issue.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	links, err := s.store.ListLinksForIssue(issue.ID)
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	var sprints []model.Sprint
	if !issue.TypeNoSprint {
		all, err := s.store.ListSprints(project.ID)
		if err != nil {
			s.renderError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		for _, sp := range all {
			if sp.State != "completed" {
				sprints = append(sprints, sp)
			}
		}
	}
	statuses, err := s.store.ListStatuses()
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	var parent *model.Issue
	if issue.ParentID != nil {
		p, err := s.store.GetIssueByID(*issue.ParentID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.renderError(w, r, http.StatusInternalServerError, err.Error())
			return
		}
		if err == nil {
			parent = &p
		}
	}

	s.render(w, r, "issue_detail.html", IssueDetailData{
		Project: project, Issue: issue, Parent: parent, Comments: comments, Links: links, Sprints: sprints, Statuses: statuses,
	}, issue.Key()+" "+issue.Summary, "")
}

func (s *Server) handleEditIssueForm(w http.ResponseWriter, r *http.Request) {
	project, ok := s.currentProject(w, r)
	issue, iok := s.issueFromPath(w, r)
	if !ok || !iok {
		return
	}
	all, err := s.store.SearchIssues(&project.ID, "")
	if err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}
	candidates := make([]model.Issue, 0, len(all))
	for _, c := range all {
		if c.ID != issue.ID {
			candidates = append(candidates, c)
		}
	}
	s.render(w, r, "issue_form.html", IssueFormData{
		Project: project, Editing: true, Issue: issue, SelectedType: issue.TypeName,
		ParentCandidates: candidates, SelectedParentID: issue.ParentID,
	}, "Edit "+issue.Key(), "")
}

func (s *Server) handleUpdateIssue(w http.ResponseWriter, r *http.Request) {
	issue, iok := s.issueFromPath(w, r)
	if !iok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	summary := strings.TrimSpace(r.FormValue("summary"))
	if summary == "" {
		s.renderError(w, r, http.StatusBadRequest, "Summary is required")
		return
	}

	in := store.UpdateIssueInput{
		Summary:     summary,
		Description: r.FormValue("description"),
		Priority:    strings.ToLower(strings.TrimSpace(r.FormValue("priority"))),
		StoryPoints: formIntPtr(r, "story_points"),
		DueDate:     strings.TrimSpace(r.FormValue("due_date")),
		ParentID:    formInt64Ptr(r, "parent_id"),
		Labels:      splitTags(r.FormValue("labels")),
		Components:  splitTags(r.FormValue("components")),
	}
	if err := s.store.UpdateIssue(issue.ID, in); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	redirectTo := "/issues/" + strconv.Itoa(issue.IssueNumber)
	if isHXRequest(r) {
		w.Header().Set("HX-Redirect", redirectTo)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

func (s *Server) handleDeleteIssue(w http.ResponseWriter, r *http.Request) {
	issue, iok := s.issueFromPath(w, r)
	if !iok {
		return
	}
	if err := s.store.DeleteIssue(issue.ID); err != nil {
		s.renderError(w, r, http.StatusInternalServerError, err.Error())
		return
	}

	redirectTo := "/board"
	if issue.TypeNoSprint {
		redirectTo = "/epics-stories"
	}
	if ref := r.Header.Get("Referer"); ref != "" && !strings.Contains(ref, "/issues/"+strconv.Itoa(issue.IssueNumber)) {
		redirectTo = ref
	}
	if isHXRequest(r) {
		w.Header().Set("HX-Redirect", redirectTo)
		return
	}
	http.Redirect(w, r, redirectTo, http.StatusFound)
}

func (s *Server) handleMoveIssue(w http.ResponseWriter, r *http.Request) {
	issue, iok := s.issueFromPath(w, r)
	if !iok {
		return
	}
	if err := r.ParseForm(); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	in := store.MoveIssueInput{
		StatusID: formInt64Ptr(r, "status_id"),
		Position: formFloat64Ptr(r, "position"),
	}
	if _, present := r.Form["sprint_id"]; present {
		in.SprintID = formInt64Ptr(r, "sprint_id")
		in.ClearSprint = in.SprintID == nil
	}

	if err := s.store.MoveIssue(issue.ID, in); err != nil {
		s.renderError(w, r, http.StatusBadRequest, err.Error())
		return
	}

	if ref := r.Header.Get("Referer"); ref != "" && isHXRequest(r) {
		w.Header().Set("HX-Redirect", ref)
		return
	}
	http.Redirect(w, r, "/board", http.StatusFound)
}
