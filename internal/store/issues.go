package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"jira-home/internal/model"
)

type scanner interface {
	Scan(dest ...any) error
}

// querier is satisfied by both *sql.DB and *sql.Tx. Any lookup that might be
// called from inside an open transaction must take a querier and use it
// (not s.db) — see the comment on getIssueTypeByID for why: this store's
// pool is capped at one connection, so reaching for s.db while a
// transaction holds that connection deadlocks forever.
type querier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

const issueSelectCols = `
	i.id, i.project_id, i.issue_number, i.issue_type_id, i.parent_id, i.summary, i.description,
	i.status_id, i.priority, i.story_points, i.due_date, i.sprint_id, i.position, i.created_at, i.updated_at,
	p.key_prefix, t.name, t.color, t.icon, t.no_sprint, st.name, st.category, COALESCE(sp.name, '')
`

const issueFromJoins = `
	FROM issue i
	JOIN project p ON p.id = i.project_id
	JOIN issue_type t ON t.id = i.issue_type_id
	JOIN status st ON st.id = i.status_id
	LEFT JOIN sprint sp ON sp.id = i.sprint_id
`

func scanIssue(row scanner) (model.Issue, error) {
	var i model.Issue
	var parentID, sprintID sql.NullInt64
	var storyPoints sql.NullInt64
	var description, dueDate sql.NullString

	err := row.Scan(
		&i.ID, &i.ProjectID, &i.IssueNumber, &i.IssueTypeID, &parentID, &i.Summary, &description,
		&i.StatusID, &i.Priority, &storyPoints, &dueDate, &sprintID, &i.Position, &i.CreatedAt, &i.UpdatedAt,
		&i.KeyPrefix, &i.TypeName, &i.TypeColor, &i.TypeIcon, &i.TypeNoSprint, &i.StatusName, &i.StatusCategory, &i.SprintName,
	)
	if err != nil {
		return i, err
	}
	if parentID.Valid {
		v := parentID.Int64
		i.ParentID = &v
	}
	if sprintID.Valid {
		v := sprintID.Int64
		i.SprintID = &v
	}
	if storyPoints.Valid {
		v := int(storyPoints.Int64)
		i.StoryPoints = &v
	}
	i.Description = description.String
	i.DueDate = dueDate.String
	return i, nil
}

// IssueFilter holds the optional query-param filters shared by the board,
// backlog, and epics-stories list views.
type IssueFilter struct {
	Types      []string // issue_type.name, e.g. "bug"
	Labels     []string
	Priorities []string
	Query      string // matches against summary or the computed key
}

func (f IssueFilter) apply(where *[]string, args *[]any) {
	if len(f.Types) > 0 {
		ph := placeholders(len(f.Types))
		*where = append(*where, "LOWER(t.name) IN ("+ph+")")
		for _, t := range f.Types {
			*args = append(*args, strings.ToLower(t))
		}
	}
	if len(f.Priorities) > 0 {
		ph := placeholders(len(f.Priorities))
		*where = append(*where, "i.priority IN ("+ph+")")
		for _, p := range f.Priorities {
			*args = append(*args, strings.ToLower(p))
		}
	}
	if len(f.Labels) > 0 {
		ph := placeholders(len(f.Labels))
		*where = append(*where, "i.id IN (SELECT issue_id FROM issue_label WHERE label IN ("+ph+"))")
		for _, l := range f.Labels {
			*args = append(*args, l)
		}
	}
	if f.Query != "" {
		*where = append(*where, "(i.summary LIKE ? OR (p.key_prefix || '-' || i.issue_number) LIKE ?)")
		like := "%" + f.Query + "%"
		*args = append(*args, like, like)
	}
}

func placeholders(n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ", ")
}

func (s *Store) listIssues(where string, args []any, orderBy string) ([]model.Issue, error) {
	query := "SELECT " + issueSelectCols + issueFromJoins
	if where != "" {
		query += " WHERE " + where
	}
	query += " ORDER BY " + orderBy

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list issues: %w", err)
	}
	defer rows.Close()

	var out []model.Issue
	for rows.Next() {
		issue, err := scanIssue(rows)
		if err != nil {
			return nil, fmt.Errorf("scan issue: %w", err)
		}
		out = append(out, issue)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.attachLabelsAndComponents(out); err != nil {
		return nil, err
	}
	if err := s.attachEpicAncestors(out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListBoard returns schedulable (non no_sprint) issues currently in the
// given sprint — what the Kanban board renders.
func (s *Store) ListBoard(projectID, sprintID int64, f IssueFilter) ([]model.Issue, error) {
	where := []string{"i.project_id = ?", "t.no_sprint = 0", "i.sprint_id = ?"}
	args := []any{projectID, sprintID}
	f.apply(&where, &args)
	return s.listIssues(strings.Join(where, " AND "), args, "st.sort_order, i.position, i.id")
}

// ListBacklog returns schedulable issues with no sprint assigned.
func (s *Store) ListBacklog(projectID int64, f IssueFilter) ([]model.Issue, error) {
	where := []string{"i.project_id = ?", "t.no_sprint = 0", "i.sprint_id IS NULL"}
	args := []any{projectID}
	f.apply(&where, &args)
	return s.listIssues(strings.Join(where, " AND "), args, "i.position, i.id")
}

// ListSprintIssuesCurrent returns whatever is CURRENTLY assigned to a given
// sprint_id, regardless of type — used to render a sprint's live row-list
// inside the Backlog view (planned/active sprints, not history).
func (s *Store) ListSprintIssuesCurrent(sprintID int64) ([]model.Issue, error) {
	return s.listIssues("i.sprint_id = ?", []any{sprintID}, "i.position, i.id")
}

// ListEpicsStories returns every no_sprint-type issue (Epic/Story) in a
// project — the dedicated management view, decoupled from Board/Backlog.
func (s *Store) ListEpicsStories(projectID int64, f IssueFilter) ([]model.Issue, error) {
	where := []string{"i.project_id = ?", "t.no_sprint = 1"}
	args := []any{projectID}
	f.apply(&where, &args)
	return s.listIssues(strings.Join(where, " AND "), args, "t.name, i.summary")
}

// SearchIssues backs the global (cross-project) and per-project search box.
func (s *Store) SearchIssues(projectID *int64, query string) ([]model.Issue, error) {
	where := []string{}
	args := []any{}
	if projectID != nil {
		where = append(where, "i.project_id = ?")
		args = append(args, *projectID)
	}
	f := IssueFilter{Query: query}
	f.apply(&where, &args)
	if len(where) == 0 {
		return nil, nil
	}
	return s.listIssues(strings.Join(where, " AND "), args, "i.updated_at DESC")
}

func (s *Store) GetIssueByID(id int64) (model.Issue, error) {
	row := s.db.QueryRow("SELECT "+issueSelectCols+issueFromJoins+" WHERE i.id = ?", id)
	issue, err := scanIssue(row)
	if errors.Is(err, sql.ErrNoRows) {
		return issue, ErrNotFound
	}
	if err != nil {
		return issue, fmt.Errorf("get issue: %w", err)
	}
	list := []model.Issue{issue}
	if err := s.attachLabelsAndComponents(list); err != nil {
		return issue, err
	}
	return list[0], nil
}

func (s *Store) GetIssueByKey(projectID int64, issueNumber int) (model.Issue, error) {
	row := s.db.QueryRow("SELECT "+issueSelectCols+issueFromJoins+" WHERE i.project_id = ? AND i.issue_number = ?", projectID, issueNumber)
	issue, err := scanIssue(row)
	if errors.Is(err, sql.ErrNoRows) {
		return issue, ErrNotFound
	}
	if err != nil {
		return issue, fmt.Errorf("get issue by key: %w", err)
	}
	list := []model.Issue{issue}
	if err := s.attachLabelsAndComponents(list); err != nil {
		return issue, err
	}
	return list[0], nil
}

// attachEpicAncestors resolves each issue's Epic-tier ancestor for display
// in list views — a Task/Bug's own parent, or a Subtask's grandparent
// (parent's parent) — in two batched queries total, not one per issue.
// Mirrors attachLabelsAndComponents: mutates issues in place.
func (s *Store) attachEpicAncestors(issues []model.Issue) error {
	parentIDSet := map[int64]struct{}{}
	for _, iss := range issues {
		if iss.ParentID != nil {
			parentIDSet[*iss.ParentID] = struct{}{}
		}
	}
	if len(parentIDSet) == 0 {
		return nil
	}
	parentIDs := make([]any, 0, len(parentIDSet))
	for id := range parentIDSet {
		parentIDs = append(parentIDs, id)
	}

	type ancestor struct {
		id       int64
		parentID *int64
		noSprint bool
		summary  string
	}
	byID := make(map[int64]ancestor, len(parentIDs))

	loadAncestors := func(ids []any) error {
		if len(ids) == 0 {
			return nil
		}
		rows, err := s.db.Query(`
			SELECT i.id, i.parent_id, t.no_sprint, i.summary
			FROM issue i JOIN issue_type t ON t.id = i.issue_type_id
			WHERE i.id IN (`+placeholders(len(ids))+`)`, ids...)
		if err != nil {
			return fmt.Errorf("load epic ancestors: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var a ancestor
			var parentID sql.NullInt64
			if err := rows.Scan(&a.id, &parentID, &a.noSprint, &a.summary); err != nil {
				return err
			}
			if parentID.Valid {
				v := parentID.Int64
				a.parentID = &v
			}
			byID[a.id] = a
		}
		return rows.Err()
	}

	if err := loadAncestors(parentIDs); err != nil {
		return err
	}

	// A parent that's itself not Epic-tier means the real issue is a
	// Subtask — its parent's parent (the grandparent) needs a second hop.
	var grandparentIDs []any
	seen := map[int64]struct{}{}
	for _, a := range byID {
		if !a.noSprint && a.parentID != nil {
			if _, ok := seen[*a.parentID]; !ok {
				seen[*a.parentID] = struct{}{}
				grandparentIDs = append(grandparentIDs, *a.parentID)
			}
		}
	}
	if err := loadAncestors(grandparentIDs); err != nil {
		return err
	}

	for i := range issues {
		if issues[i].ParentID == nil {
			continue
		}
		parent, ok := byID[*issues[i].ParentID]
		if !ok {
			continue
		}
		if parent.noSprint {
			issues[i].EpicID = &parent.id
			issues[i].EpicSummary = parent.summary
			continue
		}
		if parent.parentID == nil {
			continue
		}
		if grandparent, ok := byID[*parent.parentID]; ok && grandparent.noSprint {
			issues[i].EpicID = &grandparent.id
			issues[i].EpicSummary = grandparent.summary
		}
	}
	return nil
}

func (s *Store) attachLabelsAndComponents(issues []model.Issue) error {
	if len(issues) == 0 {
		return nil
	}
	byID := make(map[int64]*model.Issue, len(issues))
	ids := make([]any, len(issues))
	for i := range issues {
		byID[issues[i].ID] = &issues[i]
		ids[i] = issues[i].ID
	}
	ph := placeholders(len(ids))

	labelRows, err := s.db.Query(`SELECT issue_id, label FROM issue_label WHERE issue_id IN (`+ph+`) ORDER BY label`, ids...)
	if err != nil {
		return fmt.Errorf("load labels: %w", err)
	}
	defer labelRows.Close()
	for labelRows.Next() {
		var issueID int64
		var label string
		if err := labelRows.Scan(&issueID, &label); err != nil {
			return err
		}
		if issue, ok := byID[issueID]; ok {
			issue.Labels = append(issue.Labels, label)
		}
	}
	if err := labelRows.Err(); err != nil {
		return err
	}

	compRows, err := s.db.Query(`SELECT issue_id, component FROM issue_component WHERE issue_id IN (`+ph+`) ORDER BY component`, ids...)
	if err != nil {
		return fmt.Errorf("load components: %w", err)
	}
	defer compRows.Close()
	for compRows.Next() {
		var issueID int64
		var comp string
		if err := compRows.Scan(&issueID, &comp); err != nil {
			return err
		}
		if issue, ok := byID[issueID]; ok {
			issue.Components = append(issue.Components, comp)
		}
	}
	return compRows.Err()
}

// getIssueTierInfo fetches just enough about an issue to validate the
// parent hierarchy below — its own type's name and no_sprint flag — without
// pulling the full issue (labels/components included), which matters here
// because it must stay tx-safe (see the querier comment above) and a full
// GetIssueByID goes through s.db directly.
func getIssueTierInfo(q querier, id int64) (typeName string, noSprint bool, err error) {
	err = q.QueryRow(`
		SELECT t.name, t.no_sprint FROM issue i JOIN issue_type t ON t.id = i.issue_type_id WHERE i.id = ?`, id).
		Scan(&typeName, &noSprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrNotFound
	}
	return typeName, noSprint, err
}

// validateParentTier enforces the Epic > Task/Bug > Subtask hierarchy:
// Epic-tier (no_sprint) issues never have a parent; a Subtask's parent must
// be a non-Epic, non-Subtask issue; every other type's parent must be an
// Epic. Returns the parent id to actually persist (nil for Epic-tier,
// regardless of what was submitted). Pass required=false to let a nil
// parent through unvalidated — used on edit, so fixing an unrelated field on
// an older issue that predates this rule isn't blocked.
func validateParentTier(q querier, issueType model.IssueType, parentID *int64, required bool) (*int64, error) {
	if issueType.NoSprint {
		return nil, nil
	}
	if parentID == nil {
		if !required {
			return nil, nil
		}
		if issueType.Name == "Subtask" {
			return nil, fmt.Errorf("a subtask needs a parent task — create a Task or Bug under an epic first")
		}
		return nil, fmt.Errorf("%s needs a parent epic — create an Epic first", issueType.Name)
	}
	parentTypeName, parentNoSprint, err := getIssueTierInfo(q, *parentID)
	if errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("parent issue not found")
	}
	if err != nil {
		return nil, err
	}
	if issueType.Name == "Subtask" {
		if parentNoSprint || parentTypeName == "Subtask" {
			return nil, fmt.Errorf("a subtask's parent must be a task or bug, not an epic or another subtask")
		}
	} else if !parentNoSprint {
		return nil, fmt.Errorf("parent must be an epic")
	}
	return parentID, nil
}

// NewIssueInput is what a create-issue form submits.
type NewIssueInput struct {
	IssueTypeID int64
	Summary     string
	Description string
	Priority    string
	StoryPoints *int
	DueDate     string
	SprintID    *int64 // ignored (forced NULL) when the type is no_sprint
	ParentID    *int64
	Labels      []string
	Components  []string
}

func (s *Store) CreateIssue(projectID int64, in NewIssueInput) (model.Issue, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return model.Issue{}, fmt.Errorf("begin create issue: %w", err)
	}
	defer tx.Rollback()

	issueType, err := getIssueTypeByID(tx, in.IssueTypeID)
	if err != nil {
		return model.Issue{}, fmt.Errorf("issue type: %w", err)
	}
	sprintID := in.SprintID
	if issueType.NoSprint {
		sprintID = nil
	}

	parentID, err := validateParentTier(tx, issueType, in.ParentID, true)
	if err != nil {
		return model.Issue{}, err
	}

	num, err := s.NextIssueNumber(tx, projectID)
	if err != nil {
		return model.Issue{}, err
	}

	status, err := getStatusByName(tx, "To Do")
	if err != nil {
		return model.Issue{}, fmt.Errorf("default status: %w", err)
	}

	priority := in.Priority
	if priority == "" {
		priority = "medium"
	}

	res, err := tx.Exec(`
		INSERT INTO issue (project_id, issue_number, issue_type_id, parent_id, summary, description,
			status_id, priority, story_points, due_date, sprint_id, position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		projectID, num, in.IssueTypeID, parentID, in.Summary, in.Description,
		status.ID, priority, in.StoryPoints, nullIfEmpty(in.DueDate), sprintID)
	if err != nil {
		return model.Issue{}, fmt.Errorf("insert issue: %w", err)
	}
	issueID, err := res.LastInsertId()
	if err != nil {
		return model.Issue{}, err
	}

	if err := replaceTags(tx, "issue_label", "label", issueID, in.Labels); err != nil {
		return model.Issue{}, err
	}
	if err := replaceTags(tx, "issue_component", "component", issueID, in.Components); err != nil {
		return model.Issue{}, err
	}

	if sprintID != nil {
		if _, err := tx.Exec(`INSERT INTO sprint_issue (sprint_id, issue_id) VALUES (?, ?)`, *sprintID, issueID); err != nil {
			return model.Issue{}, fmt.Errorf("open sprint_issue: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return model.Issue{}, fmt.Errorf("commit create issue: %w", err)
	}
	return s.GetIssueByID(issueID)
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func replaceTags(tx *sql.Tx, table, col string, issueID int64, values []string) error {
	if _, err := tx.Exec(`DELETE FROM `+table+` WHERE issue_id = ?`, issueID); err != nil {
		return fmt.Errorf("clear %s: %w", table, err)
	}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO `+table+` (issue_id, `+col+`) VALUES (?, ?)`, issueID, v); err != nil {
			return fmt.Errorf("insert %s: %w", table, err)
		}
	}
	return nil
}

// UpdateIssueInput mirrors the fields the edit form is allowed to change.
// Type is deliberately absent — recategorizing an issue is delete-and-recreate,
// not an edit (see the Epics & Stories design review notes).
type UpdateIssueInput struct {
	Summary     string
	Description string
	Priority    string
	StoryPoints *int
	DueDate     string
	ParentID    *int64
	Labels      []string
	Components  []string
}

func (s *Store) UpdateIssue(id int64, in UpdateIssueInput) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin update issue: %w", err)
	}
	defer tx.Rollback()

	typeName, noSprint, err := getIssueTierInfo(tx, id)
	if err != nil {
		return fmt.Errorf("issue type: %w", err)
	}
	// required=false: editing an issue that predates this rule shouldn't be
	// blocked just because it still has no parent.
	parentID, err := validateParentTier(tx, model.IssueType{Name: typeName, NoSprint: noSprint}, in.ParentID, false)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE issue SET summary = ?, description = ?, priority = ?, story_points = ?, due_date = ?,
			parent_id = ?, updated_at = datetime('now')
		WHERE id = ?`,
		in.Summary, in.Description, in.Priority, in.StoryPoints, nullIfEmpty(in.DueDate), parentID, id)
	if err != nil {
		return fmt.Errorf("update issue: %w", err)
	}
	if err := replaceTags(tx, "issue_label", "label", id, in.Labels); err != nil {
		return err
	}
	if err := replaceTags(tx, "issue_component", "component", id, in.Components); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteIssue(id int64) error {
	_, err := s.db.Exec(`DELETE FROM issue WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete issue: %w", err)
	}
	return nil
}

// MoveIssueInput mirrors the /move endpoint's payload: any combination of a
// status change, a sprint change, and a manual position, all optional.
type MoveIssueInput struct {
	StatusID    *int64
	SprintID    *int64
	ClearSprint bool // explicit "move to backlog" vs. "sprint_id not part of this request"
	Position    *float64
}

// MoveIssue is the single endpoint behind board drag-and-drop (status),
// backlog drag-and-drop between sprint sections (sprint), and the detail
// view's Status/Sprint dropdowns (one field at a time). Any sprint_id change
// is mirrored into sprint_issue so sprint history/reports stay accurate.
func (s *Store) MoveIssue(id int64, in MoveIssueInput) error {
	issue, err := s.GetIssueByID(id)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin move: %w", err)
	}
	defer tx.Rollback()

	if in.StatusID != nil {
		if _, err := tx.Exec(`UPDATE issue SET status_id = ?, updated_at = datetime('now') WHERE id = ?`, *in.StatusID, id); err != nil {
			return fmt.Errorf("move status: %w", err)
		}
	}

	if in.SprintID != nil || in.ClearSprint {
		issueType, err := getIssueTypeByID(tx, issue.IssueTypeID)
		if err != nil {
			return err
		}
		if issueType.NoSprint {
			return fmt.Errorf("%s issues are never scheduled into a sprint", issueType.Name)
		}

		var newSprintID any
		if in.SprintID != nil {
			newSprintID = *in.SprintID
		}

		if issue.SprintID != nil {
			statusCat := issue.StatusCategory
			if in.StatusID != nil {
				st, err := getStatusByID(tx, *in.StatusID)
				if err != nil {
					return err
				}
				statusCat = st.Category
			}
			if _, err := tx.Exec(`
				UPDATE sprint_issue SET removed_at = datetime('now'), status_category_at_removal = ?
				WHERE sprint_id = ? AND issue_id = ? AND removed_at IS NULL`,
				statusCat, *issue.SprintID, id); err != nil {
				return fmt.Errorf("close sprint_issue: %w", err)
			}
		}
		if in.SprintID != nil {
			if _, err := tx.Exec(`INSERT INTO sprint_issue (sprint_id, issue_id) VALUES (?, ?)`, *in.SprintID, id); err != nil {
				return fmt.Errorf("open sprint_issue: %w", err)
			}
		}
		if _, err := tx.Exec(`UPDATE issue SET sprint_id = ?, updated_at = datetime('now') WHERE id = ?`, newSprintID, id); err != nil {
			return fmt.Errorf("move sprint: %w", err)
		}
	}

	if in.Position != nil {
		if _, err := tx.Exec(`UPDATE issue SET position = ? WHERE id = ?`, *in.Position, id); err != nil {
			return fmt.Errorf("move position: %w", err)
		}
	}

	return tx.Commit()
}

func (s *Store) ListDistinctLabels(projectID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT il.label FROM issue_label il JOIN issue i ON i.id = il.issue_id
		WHERE i.project_id = ? ORDER BY il.label`, projectID)
	if err != nil {
		return nil, fmt.Errorf("distinct labels: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) ListDistinctComponents(projectID int64) ([]string, error) {
	rows, err := s.db.Query(`
		SELECT DISTINCT ic.component FROM issue_component ic JOIN issue i ON i.id = ic.issue_id
		WHERE i.project_id = ? ORDER BY ic.component`, projectID)
	if err != nil {
		return nil, fmt.Errorf("distinct components: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- Links ----

func (s *Store) CreateLink(sourceID, targetID int64, linkType string) error {
	if sourceID == targetID {
		return fmt.Errorf("an issue can't link to itself")
	}
	if linkType == "relates_to" {
		// Symmetric relation: dedup the reverse direction before inserting.
		var exists int
		err := s.db.QueryRow(`
			SELECT COUNT(*) FROM issue_link
			WHERE link_type = 'relates_to' AND
			      ((source_issue_id = ? AND target_issue_id = ?) OR (source_issue_id = ? AND target_issue_id = ?))`,
			sourceID, targetID, targetID, sourceID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check existing link: %w", err)
		}
		if exists > 0 {
			return nil
		}
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO issue_link (source_issue_id, target_issue_id, link_type) VALUES (?, ?, ?)`,
		sourceID, targetID, linkType)
	if err != nil {
		return fmt.Errorf("create link: %w", err)
	}
	return nil
}

func (s *Store) DeleteLink(id int64) error {
	_, err := s.db.Exec(`DELETE FROM issue_link WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete link: %w", err)
	}
	return nil
}

func (s *Store) ListLinksForIssue(issueID int64) ([]model.IssueLink, error) {
	rows, err := s.db.Query(`
		SELECT l.id, l.source_issue_id, l.target_issue_id, l.link_type,
		       p.key_prefix, other.issue_number, other.summary
		FROM issue_link l
		JOIN issue other ON other.id = CASE WHEN l.source_issue_id = ? THEN l.target_issue_id ELSE l.source_issue_id END
		JOIN project p ON p.id = other.project_id
		WHERE l.source_issue_id = ? OR l.target_issue_id = ?`, issueID, issueID, issueID)
	if err != nil {
		return nil, fmt.Errorf("list links: %w", err)
	}
	defer rows.Close()

	var out []model.IssueLink
	for rows.Next() {
		var l model.IssueLink
		var keyPrefix string
		var otherNum int
		if err := rows.Scan(&l.ID, &l.SourceIssueID, &l.TargetIssueID, &l.LinkType, &keyPrefix, &otherNum, &l.OtherSummary); err != nil {
			return nil, err
		}
		l.OtherKey = fmt.Sprintf("%s-%d", keyPrefix, otherNum)
		l.OtherNumber = otherNum
		out = append(out, l)
	}
	return out, rows.Err()
}
