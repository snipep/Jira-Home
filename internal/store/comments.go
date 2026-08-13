package store

import (
	"fmt"

	"jira-home/internal/model"
)

func (s *Store) ListComments(issueID int64) ([]model.Comment, error) {
	rows, err := s.db.Query(`
		SELECT id, issue_id, body, created_at, updated_at FROM comment
		WHERE issue_id = ? ORDER BY created_at`, issueID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var out []model.Comment
	for rows.Next() {
		var c model.Comment
		if err := rows.Scan(&c.ID, &c.IssueID, &c.Body, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CreateComment(issueID int64, body string) (model.Comment, error) {
	res, err := s.db.Exec(`INSERT INTO comment (issue_id, body) VALUES (?, ?)`, issueID, body)
	if err != nil {
		return model.Comment{}, fmt.Errorf("create comment: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.Comment{}, err
	}
	var c model.Comment
	err = s.db.QueryRow(`SELECT id, issue_id, body, created_at, updated_at FROM comment WHERE id = ?`, id).
		Scan(&c.ID, &c.IssueID, &c.Body, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return c, fmt.Errorf("reload comment: %w", err)
	}
	return c, nil
}

func (s *Store) UpdateComment(id int64, body string) error {
	_, err := s.db.Exec(`UPDATE comment SET body = ?, updated_at = datetime('now') WHERE id = ?`, body, id)
	if err != nil {
		return fmt.Errorf("update comment: %w", err)
	}
	return nil
}

func (s *Store) DeleteComment(id int64) error {
	_, err := s.db.Exec(`DELETE FROM comment WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete comment: %w", err)
	}
	return nil
}
