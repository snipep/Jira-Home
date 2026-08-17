-- Local Jira — SQLite schema
-- See docs/design for design rationale. Embedded via Go's embed package and
-- run with CREATE TABLE IF NOT EXISTS on startup — no migration framework.

PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS project (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key_prefix TEXT NOT NULL UNIQUE,        -- e.g. 'WEB', 'ML'
    name TEXT NOT NULL,
    description TEXT,
    next_issue_number INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS issue_type (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,              -- Epic, Story, Task, Subtask, Bug, ...
    color TEXT NOT NULL DEFAULT '#579DFF',  -- swatch for the type-icon badge; lets custom types render consistently
    icon TEXT NOT NULL DEFAULT '✔',         -- single emoji/glyph shown in the badge
    no_sprint INTEGER NOT NULL DEFAULT 0,   -- 1 = issues of this type never carry a sprint_id (Epic/Story-like)
    is_default INTEGER NOT NULL DEFAULT 0   -- protects seeded types from accidental deletion
);

CREATE TABLE IF NOT EXISTS status (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,              -- To Do, In Progress, Done, Blocked, ...
    category TEXT NOT NULL DEFAULT 'todo' CHECK (category IN ('todo','in_progress','done','retired')),
    sort_order INTEGER NOT NULL DEFAULT 0   -- controls board column order
);

CREATE TABLE IF NOT EXISTS sprint (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    goal TEXT,
    start_date TEXT,
    end_date TEXT,
    state TEXT NOT NULL DEFAULT 'planned' CHECK (state IN ('planned','active','completed')),
    auto_complete INTEGER NOT NULL DEFAULT 0, -- 1 = auto-complete on end_date and start a same-length successor
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS issue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    issue_number INTEGER NOT NULL,          -- key = project.key_prefix + '-' + issue_number
    issue_type_id INTEGER NOT NULL REFERENCES issue_type(id),
    parent_id INTEGER REFERENCES issue(id) ON DELETE SET NULL,
    summary TEXT NOT NULL,
    description TEXT,                       -- Markdown; rendered to sanitized HTML at display time
    status_id INTEGER NOT NULL REFERENCES status(id),
    priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('lowest','low','medium','high','highest')),
    story_points INTEGER,
    due_date TEXT,
    sprint_id INTEGER REFERENCES sprint(id) ON DELETE SET NULL,  -- NULL = backlog
    position REAL NOT NULL DEFAULT 0,       -- manual order within backlog / within a board column
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (project_id, issue_number)
);

CREATE TABLE IF NOT EXISTS issue_label (
    issue_id INTEGER NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    PRIMARY KEY (issue_id, label)
);

CREATE TABLE IF NOT EXISTS issue_component (
    issue_id INTEGER NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    component TEXT NOT NULL,
    PRIMARY KEY (issue_id, component)
);

CREATE TABLE IF NOT EXISTS issue_link (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_issue_id INTEGER NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    target_issue_id INTEGER NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    link_type TEXT NOT NULL CHECK (link_type IN ('blocks','relates_to')),
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE (source_issue_id, target_issue_id, link_type),
    CHECK (source_issue_id != target_issue_id)
);

CREATE TABLE IF NOT EXISTS comment (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    issue_id INTEGER NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Sprint membership history: records every issue ever added to a sprint,
-- independent of the issue's *current* sprint_id. Needed because sprint
-- completion reassigns unfinished issues' sprint_id to the next sprint/
-- backlog — without this table, a completed sprint's own report would only
-- ever see the issues that finished, since the carried-over ones left.
CREATE TABLE IF NOT EXISTS sprint_issue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    sprint_id INTEGER NOT NULL REFERENCES sprint(id) ON DELETE CASCADE,
    issue_id INTEGER NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    added_at TEXT NOT NULL DEFAULT (datetime('now')),
    removed_at TEXT,                        -- set when the issue leaves this sprint (moved, or carried over)
    status_category_at_removal TEXT CHECK (status_category_at_removal IN ('todo','in_progress','done','retired'))
);

CREATE INDEX IF NOT EXISTS idx_issue_project ON issue(project_id);
CREATE INDEX IF NOT EXISTS idx_issue_sprint  ON issue(sprint_id);
CREATE INDEX IF NOT EXISTS idx_issue_status  ON issue(status_id);
CREATE INDEX IF NOT EXISTS idx_issue_parent  ON issue(parent_id);
CREATE INDEX IF NOT EXISTS idx_comment_issue ON comment(issue_id);
CREATE INDEX IF NOT EXISTS idx_sprint_issue_sprint ON sprint_issue(sprint_id);
CREATE INDEX IF NOT EXISTS idx_sprint_issue_issue  ON sprint_issue(issue_id);

-- Seed data: default issue types and statuses (safe to re-run). Hierarchy is
-- Epic > Task/Bug (and any custom type) > Subtask — see validateParentTier.
INSERT OR IGNORE INTO issue_type (name, color, icon, no_sprint, is_default) VALUES
    ('Epic',    '#8F7EE7', '⚡', 1, 1),
    ('Task',    '#579DFF', '✔', 0, 1),
    ('Subtask', '#85B8FF', '↳', 0, 1),
    ('Bug',     '#F87168', '🐞', 0, 1);

INSERT OR IGNORE INTO status (name, category, sort_order) VALUES
    ('To Do', 'todo', 1),
    ('In Progress', 'in_progress', 2),
    ('Blocked', 'in_progress', 3),
    ('Retired', 'retired', 4),
    ('Done', 'done', 5);
