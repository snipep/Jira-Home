package store

import (
	"database/sql"
	"errors"
	"fmt"

	"jira-home/internal/model"
)

var ErrNotFound = errors.New("not found")

const projectSelectCols = `id, key_prefix, name, description, next_issue_number, created_at, updated_at`

func scanProject(row scanner) (model.Project, error) {
	var p model.Project
	err := row.Scan(&p.ID, &p.KeyPrefix, &p.Name, &p.Description, &p.NextIssueNumber, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *Store) ListProjects() ([]model.Project, error) {
	rows, err := s.db.Query(`SELECT ` + projectSelectCols + ` FROM project ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var out []model.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetProjectByKey(keyPrefix string) (model.Project, error) {
	row := s.db.QueryRow(`SELECT `+projectSelectCols+` FROM project WHERE key_prefix = ?`, keyPrefix)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("get project: %w", err)
	}
	return p, nil
}

func (s *Store) CreateProject(keyPrefix, name, description string) (model.Project, error) {
	res, err := s.db.Exec(`
		INSERT INTO project (key_prefix, name, description) VALUES (?, ?, ?)`,
		keyPrefix, name, description)
	if err != nil {
		return model.Project{}, fmt.Errorf("create project: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.Project{}, err
	}
	return s.GetProjectByID(id)
}

func (s *Store) GetProjectByID(id int64) (model.Project, error) {
	row := s.db.QueryRow(`SELECT `+projectSelectCols+` FROM project WHERE id = ?`, id)
	p, err := scanProject(row)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, fmt.Errorf("get project by id: %w", err)
	}
	return p, nil
}

func (s *Store) UpdateProject(id int64, name, description string) error {
	_, err := s.db.Exec(`
		UPDATE project SET name = ?, description = ?, updated_at = datetime('now') WHERE id = ?`,
		name, description, id)
	if err != nil {
		return fmt.Errorf("update project: %w", err)
	}
	return nil
}

func (s *Store) DeleteProject(id int64) error {
	_, err := s.db.Exec(`DELETE FROM project WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

// DefaultProject returns the single project every route implicitly operates
// against — this app has no project switcher, so there's always exactly one.
func (s *Store) DefaultProject() (model.Project, error) {
	projects, err := s.ListProjects()
	if err != nil {
		return model.Project{}, err
	}
	if len(projects) == 0 {
		return model.Project{}, ErrNotFound
	}
	return projects[0], nil
}

// EnsureDefaultProject creates the single workspace project on first run —
// called once at startup so every later request always has one to find.
func (s *Store) EnsureDefaultProject() error {
	projects, err := s.ListProjects()
	if err != nil {
		return err
	}
	if len(projects) > 0 {
		return nil
	}
	_, err = s.CreateProject("HOME", "My Work", "")
	return err
}

// NextIssueNumber atomically reserves and returns the next issue number for
// a project, bumping the counter in the same statement.
func (s *Store) NextIssueNumber(tx *sql.Tx, projectID int64) (int, error) {
	var n int
	err := tx.QueryRow(`UPDATE project SET next_issue_number = next_issue_number + 1
		WHERE id = ? RETURNING next_issue_number - 1`, projectID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("reserve issue number: %w", err)
	}
	return n, nil
}
