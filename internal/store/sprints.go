package store

import (
	"database/sql"
	"errors"
	"fmt"

	"jira-home/internal/model"
)

func (s *Store) ListSprints(projectID int64) ([]model.Sprint, error) {
	rows, err := s.db.Query(`
		SELECT id, project_id, name, COALESCE(goal, ''), COALESCE(start_date, ''), COALESCE(end_date, ''), state
		FROM sprint WHERE project_id = ? ORDER BY start_date`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list sprints: %w", err)
	}
	defer rows.Close()

	var out []model.Sprint
	for rows.Next() {
		var sp model.Sprint
		if err := rows.Scan(&sp.ID, &sp.ProjectID, &sp.Name, &sp.Goal, &sp.StartDate, &sp.EndDate, &sp.State); err != nil {
			return nil, fmt.Errorf("scan sprint: %w", err)
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *Store) GetSprintByID(id int64) (model.Sprint, error) {
	var sp model.Sprint
	err := s.db.QueryRow(`
		SELECT id, project_id, name, COALESCE(goal, ''), COALESCE(start_date, ''), COALESCE(end_date, ''), state
		FROM sprint WHERE id = ?`, id).
		Scan(&sp.ID, &sp.ProjectID, &sp.Name, &sp.Goal, &sp.StartDate, &sp.EndDate, &sp.State)
	if errors.Is(err, sql.ErrNoRows) {
		return sp, ErrNotFound
	}
	if err != nil {
		return sp, fmt.Errorf("get sprint: %w", err)
	}
	return sp, nil
}

func (s *Store) CreateSprint(projectID int64, name, goal, startDate, endDate string) (model.Sprint, error) {
	res, err := s.db.Exec(`
		INSERT INTO sprint (project_id, name, goal, start_date, end_date) VALUES (?, ?, ?, ?, ?)`,
		projectID, name, nullIfEmpty(goal), nullIfEmpty(startDate), nullIfEmpty(endDate))
	if err != nil {
		return model.Sprint{}, fmt.Errorf("create sprint: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.Sprint{}, err
	}
	return s.GetSprintByID(id)
}

func (s *Store) UpdateSprint(id int64, name, goal, startDate, endDate string) error {
	_, err := s.db.Exec(`UPDATE sprint SET name = ?, goal = ?, start_date = ?, end_date = ? WHERE id = ?`,
		name, nullIfEmpty(goal), nullIfEmpty(startDate), nullIfEmpty(endDate), id)
	if err != nil {
		return fmt.Errorf("update sprint: %w", err)
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
		WHERE i.sprint_id = ? AND st.category != 'done'`, sprintID).Scan(&unfinished)
	if err != nil {
		return CompletionTarget{}, fmt.Errorf("count unfinished: %w", err)
	}

	target := CompletionTarget{UnfinishedCount: unfinished, TargetName: "Backlog"}
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
// (status category != done) move to the earliest-starting planned sprint in
// the project, or the backlog if none exists; their status is left
// unchanged. Finished issues stay put, so they remain part of this sprint's
// permanent history. Reuses MoveIssue so the sprint_issue bookkeeping is
// exactly the same code path as any other sprint reassignment.
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

	rows, err := s.db.Query(`
		SELECT i.id FROM issue i JOIN status st ON st.id = i.status_id
		WHERE i.sprint_id = ? AND st.category != 'done'`, sprintID)
	if err != nil {
		return fmt.Errorf("find unfinished issues: %w", err)
	}
	var unfinishedIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		unfinishedIDs = append(unfinishedIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, issueID := range unfinishedIDs {
		move := MoveIssueInput{ClearSprint: true}
		if target.TargetSprintID != nil {
			move.SprintID = target.TargetSprintID
		}
		if err := s.MoveIssue(issueID, move); err != nil {
			return fmt.Errorf("move issue %d off completed sprint: %w", issueID, err)
		}
	}

	if _, err := s.db.Exec(`UPDATE sprint SET state = 'completed' WHERE id = ?`, sprintID); err != nil {
		return fmt.Errorf("mark sprint completed: %w", err)
	}
	return nil
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
