package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"jira-home/internal/model"
)

const sprintDateLayout = "2006-01-02"

const sprintSelectCols = `id, project_id, name, COALESCE(goal, ''), COALESCE(start_date, ''), COALESCE(end_date, ''), state, auto_complete`

func scanSprint(row scanner) (model.Sprint, error) {
	var sp model.Sprint
	err := row.Scan(&sp.ID, &sp.ProjectID, &sp.Name, &sp.Goal, &sp.StartDate, &sp.EndDate, &sp.State, &sp.AutoComplete)
	return sp, err
}

func (s *Store) ListSprints(projectID int64) ([]model.Sprint, error) {
	rows, err := s.db.Query(`SELECT `+sprintSelectCols+` FROM sprint WHERE project_id = ? ORDER BY start_date`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list sprints: %w", err)
	}
	defer rows.Close()

	var out []model.Sprint
	for rows.Next() {
		sp, err := scanSprint(rows)
		if err != nil {
			return nil, fmt.Errorf("scan sprint: %w", err)
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *Store) GetSprintByID(id int64) (model.Sprint, error) {
	row := s.db.QueryRow(`SELECT `+sprintSelectCols+` FROM sprint WHERE id = ?`, id)
	sp, err := scanSprint(row)
	if errors.Is(err, sql.ErrNoRows) {
		return sp, ErrNotFound
	}
	if err != nil {
		return sp, fmt.Errorf("get sprint: %w", err)
	}
	return sp, nil
}

func (s *Store) CreateSprint(projectID int64, name, goal, startDate, endDate string, autoComplete bool) (model.Sprint, error) {
	res, err := s.db.Exec(`
		INSERT INTO sprint (project_id, name, goal, start_date, end_date, auto_complete) VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, name, nullIfEmpty(goal), nullIfEmpty(startDate), nullIfEmpty(endDate), autoComplete)
	if err != nil {
		return model.Sprint{}, fmt.Errorf("create sprint: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.Sprint{}, err
	}
	return s.GetSprintByID(id)
}

func (s *Store) UpdateSprint(id int64, name, goal, startDate, endDate string, autoComplete bool) error {
	_, err := s.db.Exec(`UPDATE sprint SET name = ?, goal = ?, start_date = ?, end_date = ?, auto_complete = ? WHERE id = ?`,
		name, nullIfEmpty(goal), nullIfEmpty(startDate), nullIfEmpty(endDate), autoComplete, id)
	if err != nil {
		return fmt.Errorf("update sprint: %w", err)
	}
	return nil
}

// DeleteSprint removes a sprint in any state, planned/active/completed —
// including permanently from history, which also drops it from Analysis
// (both read live from the sprint/sprint_issue tables, nothing to reconcile).
// Any issue still pointing at it (an in-progress sprint's issues, or a
// completed sprint's finished issues, which stay assigned per CompleteSprint)
// falls back to the backlog (issue.sprint_id is ON DELETE SET NULL);
// sprint_issue history rows cascade-delete with it.
func (s *Store) DeleteSprint(id int64) error {
	if _, err := s.GetSprintByID(id); err != nil {
		return err
	}
	if _, err := s.db.Exec(`DELETE FROM sprint WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete sprint: %w", err)
	}
	return nil
}

// StartSprint transitions planned -> active. No constraint against multiple
// concurrently active sprints per project (decided during design review).
func (s *Store) StartSprint(id int64) error {
	_, err := s.db.Exec(`UPDATE sprint SET state = 'active' WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("start sprint: %w", err)
	}
	return nil
}

// CompletionTarget describes where a sprint's unfinished issues will land.
type CompletionTarget struct {
	UnfinishedCount int
	RetiredCount    int    // retired-category issues — always dropped to the Retired holding area, never carried to TargetSprintID
	TargetSprintID  *int64 // nil means "the backlog"
	TargetName      string // sprint name, or "Backlog"
}

// PreviewCompletion computes what CompleteSprint would do, without doing it —
// backs the confirmation step required before a sprint completes.
func (s *Store) PreviewCompletion(sprintID int64) (CompletionTarget, error) {
	sprint, err := s.GetSprintByID(sprintID)
	if err != nil {
		return CompletionTarget{}, err
	}

	var unfinished int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM issue i JOIN status st ON st.id = i.status_id
		WHERE i.sprint_id = ? AND st.category NOT IN ('done', 'retired')`, sprintID).Scan(&unfinished)
	if err != nil {
		return CompletionTarget{}, fmt.Errorf("count unfinished: %w", err)
	}
	var retired int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM issue i JOIN status st ON st.id = i.status_id
		WHERE i.sprint_id = ? AND st.category = 'retired'`, sprintID).Scan(&retired)
	if err != nil {
		return CompletionTarget{}, fmt.Errorf("count retired: %w", err)
	}

	target := CompletionTarget{UnfinishedCount: unfinished, RetiredCount: retired, TargetName: "Backlog"}
	var targetID sql.NullInt64
	var targetName sql.NullString
	err = s.db.QueryRow(`
		SELECT id, name FROM sprint
		WHERE project_id = ? AND state = 'planned' AND id != ? AND start_date IS NOT NULL
		ORDER BY start_date ASC LIMIT 1`, sprint.ProjectID, sprintID).Scan(&targetID, &targetName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return CompletionTarget{}, fmt.Errorf("find target sprint: %w", err)
	}
	if targetID.Valid {
		id := targetID.Int64
		target.TargetSprintID = &id
		target.TargetName = targetName.String
	}
	return target, nil
}

// CompleteSprint runs the algorithm from the design doc: unfinished issues
// (status category not done, not retired) move to the earliest-starting
// planned sprint in the project, or the backlog if none exists; their status
// is left unchanged. Retired-category issues never carry to that target —
// they're always dropped straight to the backlog (sprint_id cleared, no
// reassignment), which is what puts them in the Retired holding area instead
// of back in normal backlog/sprint flow (see Store.ListRetired). Finished
// issues stay put, so they remain part of this sprint's permanent history.
// Reuses MoveIssue so the sprint_issue bookkeeping is exactly the same code
// path as any other sprint reassignment.
func (s *Store) CompleteSprint(sprintID int64) error {
	sprint, err := s.GetSprintByID(sprintID)
	if err != nil {
		return err
	}
	if sprint.State != "active" {
		return fmt.Errorf("only an active sprint can be completed (this one is %s)", sprint.State)
	}

	target, err := s.PreviewCompletion(sprintID)
	if err != nil {
		return err
	}

	queryIDs := func(where string) ([]int64, error) {
		rows, err := s.db.Query(`
			SELECT i.id FROM issue i JOIN status st ON st.id = i.status_id
			WHERE i.sprint_id = ? AND `+where, sprintID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var ids []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			ids = append(ids, id)
		}
		return ids, rows.Err()
	}

	unfinishedIDs, err := queryIDs("st.category NOT IN ('done', 'retired')")
	if err != nil {
		return fmt.Errorf("find unfinished issues: %w", err)
	}
	retiredIDs, err := queryIDs("st.category = 'retired'")
	if err != nil {
		return fmt.Errorf("find retired issues: %w", err)
	}

	for _, issueID := range unfinishedIDs {
		move := MoveIssueInput{ClearSprint: true}
		if target.TargetSprintID != nil {
			move.SprintID = target.TargetSprintID
		}
		if err := s.MoveIssue(issueID, move); err != nil {
			return fmt.Errorf("move issue %d off completed sprint: %w", issueID, err)
		}
	}
	for _, issueID := range retiredIDs {
		if err := s.MoveIssue(issueID, MoveIssueInput{ClearSprint: true}); err != nil {
			return fmt.Errorf("move retired issue %d off completed sprint: %w", issueID, err)
		}
	}

	if _, err := s.db.Exec(`UPDATE sprint SET state = 'completed' WHERE id = ?`, sprintID); err != nil {
		return fmt.Errorf("mark sprint completed: %w", err)
	}
	return nil
}

// RunSprintAutoCycle rolls over every active sprint whose auto_complete flag
// is set and whose end_date has passed: it creates a same-length successor
// starting the very next day (carrying auto_complete forward, so the chain
// keeps going until someone turns it off), completes the old sprint into it
// — reusing CompleteSprint so unfinished issues carry over exactly like a
// manual completion — then starts the new one. Sprints without
// auto_complete, or without an end_date to cycle off of, are left alone for
// their owner to complete by hand. Safe to call on every tick: a no-op
// unless a rollover is actually due.
func (s *Store) RunSprintAutoCycle() error {
	project, err := s.DefaultProject()
	if err != nil {
		return err
	}
	sprints, err := s.ListSprints(project.ID)
	if err != nil {
		return err
	}

	for _, sprint := range sprints {
		if sprint.State != "active" || !sprint.AutoComplete || sprint.EndDate == "" {
			continue
		}
		end, err := time.Parse(sprintDateLayout, sprint.EndDate)
		if err != nil {
			continue
		}
		rolloverAt := end.AddDate(0, 0, 1) // midnight of the day after end_date
		if time.Now().Before(rolloverAt) {
			continue // not due yet
		}

		length := 14 // fallback when start_date is missing, so length can't be derived
		if sprint.StartDate != "" {
			if start, err := time.Parse(sprintDateLayout, sprint.StartDate); err == nil {
				if days := int(end.Sub(start).Hours()/24) + 1; days > 0 {
					length = days
				}
			}
		}

		next, err := s.CreateSprint(project.ID, nextSprintName(sprint.Name), "",
			rolloverAt.Format(sprintDateLayout), rolloverAt.AddDate(0, 0, length-1).Format(sprintDateLayout), true)
		if err != nil {
			return fmt.Errorf("create next sprint: %w", err)
		}
		// CompleteSprint's target is "the earliest-start planned sprint" —
		// since next was just created with a concrete start_date, it's the
		// one picked, so the carry-over lands exactly where expected.
		if err := s.CompleteSprint(sprint.ID); err != nil {
			return fmt.Errorf("auto-complete sprint %d: %w", sprint.ID, err)
		}
		if err := s.StartSprint(next.ID); err != nil {
			return fmt.Errorf("auto-start sprint %d: %w", next.ID, err)
		}
	}
	return nil
}

// nextSprintName tries "Sprint N" -> "Sprint N+1"; falls back to appending
// " (2)" for anything that doesn't fit that pattern, so auto-cycled sprints
// still get a sensible, distinct name from whatever the prior one was
// called.
func nextSprintName(prev string) string {
	var n int
	if _, err := fmt.Sscanf(prev, "Sprint %d", &n); err == nil {
		return fmt.Sprintf("Sprint %d", n+1)
	}
	return prev + " (2)"
}

// SprintReport returns every issue ever a member of this sprint (via
// sprint_issue, not issue.sprint_id) with its outcome: still-Done issues
// carry their live status; carried-over ones show the status they had at
// the moment they left. This is what keeps the report accurate even after
// CompleteSprint reassigns unfinished issues elsewhere.
type SprintReportRow struct {
	Issue                   model.Issue
	Completed               bool
	StatusCategoryAtRemoval string // "" if still a current member
}

func (s *Store) SprintReport(sprintID int64) ([]SprintReportRow, error) {
	rows, err := s.db.Query(`
		SELECT issue_id, removed_at, status_category_at_removal
		FROM sprint_issue WHERE sprint_id = ? ORDER BY added_at`, sprintID)
	if err != nil {
		return nil, fmt.Errorf("sprint_issue rows: %w", err)
	}
	defer rows.Close()

	type membership struct {
		issueID  int64
		removed  bool
		category sql.NullString
	}
	var memberships []membership
	for rows.Next() {
		var m membership
		var removedAt sql.NullString
		if err := rows.Scan(&m.issueID, &removedAt, &m.category); err != nil {
			return nil, err
		}
		m.removed = removedAt.Valid
		memberships = append(memberships, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]SprintReportRow, 0, len(memberships))
	for _, m := range memberships {
		issue, err := s.GetIssueByID(m.issueID)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue // issue was deleted after the sprint tracked it
			}
			return nil, err
		}
		row := SprintReportRow{Issue: issue}
		if m.removed {
			row.StatusCategoryAtRemoval = m.category.String
			row.Completed = m.category.String == "done"
		} else {
			row.Completed = issue.StatusCategory == "done"
		}
		out = append(out, row)
	}
	return out, nil
}
