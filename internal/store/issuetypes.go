package store

import (
	"database/sql"
	"errors"
	"fmt"

	"jira-home/internal/model"
)

func (s *Store) ListIssueTypes() ([]model.IssueType, error) {
	rows, err := s.db.Query(`SELECT id, name, color, icon, no_sprint, is_default FROM issue_type ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("list issue types: %w", err)
	}
	defer rows.Close()

	var out []model.IssueType
	for rows.Next() {
		var t model.IssueType
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Icon, &t.NoSprint, &t.IsDefault); err != nil {
			return nil, fmt.Errorf("scan issue type: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetIssueTypeByID(id int64) (model.IssueType, error) {
	return getIssueTypeByID(s.db, id)
}

// getIssueTypeByID takes a querier (either *sql.DB or an open *sql.Tx) so
// callers already inside a transaction can look this up on the SAME
// connection — using s.db instead would try to check out a second
// connection from a pool that (deliberately, see store.go) only has one,
// and deadlock forever waiting for the transaction that's blocking it.
func getIssueTypeByID(q querier, id int64) (model.IssueType, error) {
	var t model.IssueType
	err := q.QueryRow(`SELECT id, name, color, icon, no_sprint, is_default FROM issue_type WHERE id = ?`, id).
		Scan(&t.ID, &t.Name, &t.Color, &t.Icon, &t.NoSprint, &t.IsDefault)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, fmt.Errorf("get issue type: %w", err)
	}
	return t, nil
}

func (s *Store) GetIssueTypeByName(name string) (model.IssueType, error) {
	var t model.IssueType
	err := s.db.QueryRow(`SELECT id, name, color, icon, no_sprint, is_default FROM issue_type WHERE name = ?`, name).
		Scan(&t.ID, &t.Name, &t.Color, &t.Icon, &t.NoSprint, &t.IsDefault)
	if errors.Is(err, sql.ErrNoRows) {
		return t, ErrNotFound
	}
	if err != nil {
		return t, fmt.Errorf("get issue type by name: %w", err)
	}
	return t, nil
}

func (s *Store) CreateIssueType(name, color, icon string, noSprint bool) (model.IssueType, error) {
	res, err := s.db.Exec(`INSERT INTO issue_type (name, color, icon, no_sprint, is_default) VALUES (?, ?, ?, ?, 0)`,
		name, color, icon, noSprint)
	if err != nil {
		return model.IssueType{}, fmt.Errorf("create issue type: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.IssueType{}, err
	}
	return s.GetIssueTypeByID(id)
}

func (s *Store) UpdateIssueType(id int64, color, icon string, noSprint bool) error {
	_, err := s.db.Exec(`UPDATE issue_type SET color = ?, icon = ?, no_sprint = ? WHERE id = ?`, color, icon, noSprint, id)
	if err != nil {
		return fmt.Errorf("update issue type: %w", err)
	}
	return nil
}

// DeleteIssueType blocks deleting a type still used by at least one issue —
// built-in (seeded) types are deletable too, as long as nothing references
// them.
func (s *Store) DeleteIssueType(id int64) error {
	t, err := s.GetIssueTypeByID(id)
	if err != nil {
		return err
	}
	var inUse int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM issue WHERE issue_type_id = ?`, id).Scan(&inUse); err != nil {
		return fmt.Errorf("check issue type usage: %w", err)
	}
	if inUse > 0 {
		return fmt.Errorf("can't delete %q — used by %d issue(s)", t.Name, inUse)
	}
	if _, err := s.db.Exec(`DELETE FROM issue_type WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete issue type: %w", err)
	}
	return nil
}
