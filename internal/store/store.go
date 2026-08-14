// Package store is the data-access layer: it owns the *sql.DB, the embedded
// schema, and every query the app issues. Handlers never see raw SQL.
package store

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

// Store wraps the database connection. SQLite allows only one writer at a
// time anyway, so we cap the pool at a single connection — that also avoids
// the classic gotcha where PRAGMA settings only apply to the connection that
// issued them.
type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if err := ensureColumn(db, "sprint", "auto_complete", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure auto_complete column: %w", err)
	}
	if err := migratePriorityLevels(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate priority levels: %w", err)
	}

	st := &Store{db: db}
	if err := st.removeUnusedSeedType("Story"); err != nil {
		db.Close()
		return nil, fmt.Errorf("remove Story type: %w", err)
	}
	return st, nil
}

// ensureColumn adds a column to an existing table if it's missing. schema.sql
// only ever runs CREATE TABLE IF NOT EXISTS, which no-ops against a table
// that already exists — so a new column added to the schema wouldn't
// otherwise reach a database created before it existed. This is the one
// deliberate exception to "no migration framework": additive only, no down
// path, safe to run on every startup.
func ensureColumn(db *sql.DB, table, column, ddl string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + ddl)
	return err
}

// migratePriorityLevels widens the issue.priority CHECK constraint from
// four levels (low/medium/high/urgent) to Jira's five (lowest/low/medium/
// high/highest), mapping old 'urgent' rows to 'highest'. Unlike a new
// column, a CHECK constraint is baked into the table's definition at
// CREATE TABLE time — schema.sql re-running CREATE TABLE IF NOT EXISTS
// can't touch it on a database that already has the old one, and SQLite
// has no ALTER TABLE for constraints. This follows SQLite's own documented
// procedure for that case: rebuild the table under a new name, copy the
// data across (translating the one changed value), drop the old one, and
// rename — a second deliberate exception to "no migration framework",
// still additive in spirit (no data is lost, only 'urgent' is renamed).
// A fresh database created from the current schema.sql already has the
// five-level constraint, so this is a no-op for it.
func migratePriorityLevels(db *sql.DB) error {
	var tableSQL string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'issue'`).Scan(&tableSQL)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // issue table doesn't exist yet (shouldn't happen post-schema-apply, but harmless)
	}
	if err != nil {
		return err
	}
	if strings.Contains(tableSQL, "'highest'") {
		return nil // already on the five-level constraint
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer db.Exec(`PRAGMA foreign_keys = ON`)

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE issue_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
			issue_number INTEGER NOT NULL,
			issue_type_id INTEGER NOT NULL REFERENCES issue_type(id),
			parent_id INTEGER REFERENCES issue_new(id) ON DELETE SET NULL,
			summary TEXT NOT NULL,
			description TEXT,
			status_id INTEGER NOT NULL REFERENCES status(id),
			priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('lowest','low','medium','high','highest')),
			story_points INTEGER,
			due_date TEXT,
			sprint_id INTEGER REFERENCES sprint(id) ON DELETE SET NULL,
			position REAL NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now')),
			UNIQUE (project_id, issue_number)
		)`); err != nil {
		return fmt.Errorf("create issue_new: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO issue_new
		SELECT id, project_id, issue_number, issue_type_id, parent_id, summary, description, status_id,
			CASE priority WHEN 'urgent' THEN 'highest' ELSE priority END,
			story_points, due_date, sprint_id, position, created_at, updated_at
		FROM issue`); err != nil {
		return fmt.Errorf("copy issue rows: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE issue`); err != nil {
		return fmt.Errorf("drop old issue table: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE issue_new RENAME TO issue`); err != nil {
		return fmt.Errorf("rename issue_new: %w", err)
	}
	// Indexes are dropped along with the table they're on; schema.sql's
	// CREATE INDEX IF NOT EXISTS already ran earlier this same startup,
	// before the table it targeted was replaced, so they need redoing here.
	for _, stmt := range []string{
		`CREATE INDEX IF NOT EXISTS idx_issue_project ON issue(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_issue_sprint  ON issue(sprint_id)`,
		`CREATE INDEX IF NOT EXISTS idx_issue_status  ON issue(status_id)`,
		`CREATE INDEX IF NOT EXISTS idx_issue_parent  ON issue(parent_id)`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("recreate index: %w", err)
		}
	}
	return tx.Commit()
}

// removeUnusedSeedType deletes a previously-seeded issue type that's no
// longer part of the default set (Story, dropped for the Epic > Task/Bug >
// Subtask hierarchy) — but only if nothing has come to depend on it. Same
// "never destroy data" rule as ensureColumn: if it's in use, this silently
// leaves it for the user to resolve by hand in Settings.
func (s *Store) removeUnusedSeedType(name string) error {
	t, err := s.GetIssueTypeByName(name)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_ = s.DeleteIssueType(t.ID) // ignore failure — still in use, leave it alone
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}
