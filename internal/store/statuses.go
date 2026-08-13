package store

import (
	"database/sql"
	"errors"
	"fmt"

	"jira-home/internal/model"
)

func (s *Store) ListStatuses() ([]model.Status, error) {
	rows, err := s.db.Query(`SELECT id, name, category, sort_order FROM status ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("list statuses: %w", err)
	}
	defer rows.Close()

	var out []model.Status
	for rows.Next() {
		var st model.Status
		if err := rows.Scan(&st.ID, &st.Name, &st.Category, &st.SortOrder); err != nil {
			return nil, fmt.Errorf("scan status: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Store) GetStatusByID(id int64) (model.Status, error) {
	return getStatusByID(s.db, id)
}

// getStatusByID takes a querier so it can run on an already-open
// transaction's connection — see the comment on getIssueTypeByID for why
// that matters with a single-connection pool.
func getStatusByID(q querier, id int64) (model.Status, error) {
	var st model.Status
	err := q.QueryRow(`SELECT id, name, category, sort_order FROM status WHERE id = ?`, id).
		Scan(&st.ID, &st.Name, &st.Category, &st.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	if err != nil {
		return st, fmt.Errorf("get status: %w", err)
	}
	return st, nil
}

func (s *Store) GetStatusByName(name string) (model.Status, error) {
	return getStatusByName(s.db, name)
}

func getStatusByName(q querier, name string) (model.Status, error) {
	var st model.Status
	err := q.QueryRow(`SELECT id, name, category, sort_order FROM status WHERE name = ?`, name).
		Scan(&st.ID, &st.Name, &st.Category, &st.SortOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return st, ErrNotFound
	}
	if err != nil {
		return st, fmt.Errorf("get status by name: %w", err)
	}
	return st, nil
}

func (s *Store) CreateStatus(name, category string) (model.Status, error) {
	var maxOrder int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) FROM status`).Scan(&maxOrder); err != nil {
		return model.Status{}, fmt.Errorf("max sort order: %w", err)
	}
	res, err := s.db.Exec(`INSERT INTO status (name, category, sort_order) VALUES (?, ?, ?)`, name, category, maxOrder+1)
	if err != nil {
		return model.Status{}, fmt.Errorf("create status: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.Status{}, err
	}
	return s.GetStatusByID(id)
}

func (s *Store) UpdateStatus(id int64, name, category string) error {
	_, err := s.db.Exec(`UPDATE status SET name = ?, category = ? WHERE id = ?`, name, category, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

// ReorderStatuses persists a full new ordering (list of status IDs, in the
// desired order) — sort_order becomes each id's position in the slice.
func (s *Store) ReorderStatuses(orderedIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin reorder: %w", err)
	}
	defer tx.Rollback()

	for i, id := range orderedIDs {
		if _, err := tx.Exec(`UPDATE status SET sort_order = ? WHERE id = ?`, i+1, id); err != nil {
			return fmt.Errorf("reorder status %d: %w", id, err)
		}
	}
	return tx.Commit()
}

// DeleteStatus blocks deleting a status still in use by an issue — same
// app-logic rule as DeleteIssueType. Statuses have no is_default flag (all
// four seeded ones are just regular rows), so only the in-use check applies.
func (s *Store) DeleteStatus(id int64) error {
	st, err := s.GetStatusByID(id)
	if err != nil {
		return err
	}
	var inUse int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM issue WHERE status_id = ?`, id).Scan(&inUse); err != nil {
		return fmt.Errorf("check status usage: %w", err)
	}
	if inUse > 0 {
		return fmt.Errorf("can't delete %q — used by %d issue(s)", st.Name, inUse)
	}
	if _, err := s.db.Exec(`DELETE FROM status WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete status: %w", err)
	}
	return nil
}
