# Local Jira — MVP Feature Scope

Personal work tracker inspired by Jira, scoped for solo use (no team/admin features). Finalized after reviewing the full Jira feature set and trimming to what earns its keep for one person.

## Work items

Issue types: Epic, Story, Task, Subtask, Bug — plus support for defining custom types.

Fields per issue: key (e.g. PROJ-123), summary, rich-text description, priority, status, labels, components, due date.

Deliberately excluded: assignee/reporter (redundant when you're the only user), full link taxonomy (duplicates/clones — those exist for multi-reporter teams).

## Hierarchy & relationships

- Epic → Story/Task → Subtask nesting (kept because work is organized into longer-running projects that accumulate tasks over time) — **status: no UI currently sets or shows this; see the open item under Issues in the route design below**
- Issue linking: "blocks" and "relates to"
- Parent-child links

Excluded: cross-project linking (not needed without multiple concurrently active, cross-referencing projects).

## Agile planning

- Backlog view for unscheduled work
- Sprints: create, start, complete — with a goal and date range
- Story point estimation (kept for sizing even without velocity tracking downstream)

Excluded: velocity tracking (a team capacity-planning metric, not useful solo).

## Workflows

- Fully customizable status workflow — add/remove statuses, not just reorder defaults
- Comments/notes per issue (running log of what was tried)

Excluded: workflow conditions/validators/post-functions (a rules engine not needed when you're the only one moving tickets).

## Reporting

- Sprint reports (planned vs. completed)

Excluded: burndown/burnup, velocity charts, cumulative flow diagrams, control charts, release/version reports — all team-health/predictability metrics that need either multiple people or high ticket volume to mean anything.

## Views

- Kanban board (status columns)
- List view (Backlog)
- Search/filtering across issues
- Epics & Stories (dedicated management view, added after the wireframe pass — see Design review notes below)
- Sprints / History (planned+active summary, plus completed sprints with their full issue list kept in place — see Design review notes below)

## Rationale summary

Full Jira feature set reviewed category by category; cut items were those that primarily serve team coordination (permissions, assignee/reporter, cross-project links), enterprise workflow customization (conditions/validators/post-functions), or analytics that need scale to be meaningful (velocity, CFD, control charts). Kept items are load-bearing for solo day-to-day use: organizing longer projects (epic/story/subtask hierarchy), planning in cycles (backlog/sprints), tracking status your own way (customizable workflow), and seeing/finding work (board, list, search).

## Design review notes (2026-08-12)

A pre-implementation review of this doc surfaced one real bug and a few undecided details. Decisions below are reflected in the schema, routes, and algorithm sections throughout this doc — this section is just the changelog.

- **Sprint report data loss (bug).** Sprint completion reassigns unfinished issues' `sprint_id` to the next sprint/backlog. That means a completed sprint's own report — queried as `issue WHERE sprint_id = X` — would only ever see issues that finished, since anything that carried over literally left the sprint. Fixed by adding a `sprint_issue` history table that records membership independent of the issue's current `sprint_id` (see Entities and schema below).
- **No persisted manual ordering.** Neither the backlog list nor board columns had any field to persist drag-to-reorder. Added `issue.position` (fractional-index float) so both are stable across reloads.
- **Rich-text description format.** Decided: Markdown stored as plain text, rendered to sanitized HTML at display time (e.g. `goldmark` + `bluemonday` in Go). No schema change — `description` stays `TEXT`.
- **Topbar search scope.** The wireframe's topbar search box reads as global, but the route design only had project-scoped search. Decided: it is global — added a top-level `GET /search` route.
- **Custom issue types had no visual identity.** Added `color` and `icon` columns to `issue_type` so custom types render consistently with the seeded ones (the wireframe hardcodes a color/icon per type).
- **Epic-only sprint exclusion was name-keyed.** The "Epics never get a `sprint_id`" rule was going to be enforced by checking `issue_type.name == 'Epic'`, which breaks if that type is ever renamed and doesn't generalize to a custom top-level type. Replaced with a boolean `issue_type.no_sprint` flag.
- **Concurrent active sprints.** Decided: no constraint — a project may have more than one `active` sprint at a time. Revisit if this turns out to feel wrong in practice.
- **Symmetric `relates_to` links.** `issue_link` only had a UNIQUE constraint preventing an exact duplicate row; it didn't stop `(A,B,relates_to)` and `(B,A,relates_to)` both existing, which would show as a duplicate in the UI. Left as an app-layer dedup check (query for either direction before inserting) rather than a schema constraint, since `blocks` is directional and shares the table. Also added a `CHECK (source_issue_id != target_issue_id)` to block self-links outright.
- **`updated_at` has no trigger.** Every UPDATE handler is responsible for setting it manually — noted here so it's handled consistently across handlers rather than piecemeal.

## Design review notes — Epics & Stories, delete, edit (2026-08-12)

A wireframe pass surfaced a real product-model change (Epics and Stories don't belong in the Task/Bug backlog after all) plus two previously-missing operations (delete, edit). This section records what changed; the schema and routes below already reflect it.

- **Story becomes a `no_sprint` type, same as Epic.** Original scope treated Story as a normal sprint-plannable item (like Task/Bug). Revised: Epics *and* Stories are both organizational/planning-level items, managed together in a dedicated view, and never carry a `sprint_id`. Only Task/Subtask/Bug (and any future custom type with `no_sprint = 0`) are schedulable into a sprint or sit in the per-project Backlog. Seed data changes: `issue_type.no_sprint = 1` for Story, not just Epic.
- **New dedicated view for Epics & Stories**, separate from Board/Backlog. New route: `GET /projects/{key}/epics-stories` (see Routes below). Creating an Epic or Story from *any* "+" entry point in the app always lands it here — the issue's `issue_type`, not which button was clicked, decides where it's listed.
- **Backlog/Board queries now exclude `no_sprint` types by construction** — `GET .../board` and `GET .../backlog` only ever return issues whose type has `no_sprint = 0`. This is a query-filter change, not a new column.
- **Epic-grouping-in-Backlog is removed, and so is the parent-epic picker on issue creation — flagged as an open conflict with earlier scope.** The original feature scope explicitly kept "Epic → Story/Task → Subtask nesting... because work is organized into longer-running projects." Pulling Epics and Stories out of the Backlog removed the only UI that showed or set that nesting (there's no more "which epic is this under" control anywhere, for any issue type). `parent_id` still exists in the `issue` table, untouched, but nothing in the current design writes to it. **This needs a decision:** either (a) add a parent/child control somewhere sensible now that Epics live in their own view — e.g. an Epic's detail page listing/adding its child Stories — or (b) formally descope issue hierarchy from the MVP UI and keep `parent_id` as schema-only groundwork for later. Left open rather than guessed.
- **Delete was already routed (`DELETE /projects/{key}/issues/{num}`) but had no UI.** Now triggered from two places: inline row actions in the Epics & Stories view, and a delete action on the issue detail view. Both gate the call behind a confirmation step (`hx-confirm` in the real app, matching htmx's built-in browser-confirm attribute — the wireframe's plain `confirm()` stands in for that).
- **Edit reuses the existing `GET .../issues/{num}/edit` fragment**, with one new UI rule: **issue type is not editable after creation.** The edit form omits the type picker entirely — changing an issue's category is delete-and-recreate, not a PATCH. `PATCH .../issues/{num}` should therefore ignore or reject a `type` change coming from this form (the field stays in the schema/route as technically settable, since nothing stops a future admin-style tool from using it, but the primary edit UI never sends it).
- **Status and Sprint fields in the issue detail view are both just the `/move` endpoint.** No new route: the Status dropdown sends `{status_id}` only; the Sprint dropdown (hidden for Epic/Story, since they're `no_sprint`) sends `{sprint_id}` only; board drag-and-drop and backlog drag-and-drop between sprint sections send the same shapes respectively. One endpoint, four entry points (two clicks, two drags).
- **The Sprints/History view needs issues eager-loaded per sprint, not just counts.** Completed sprints render their full historical issue list inline (via `sprint_issue`), so `GET /projects/{key}/sprints` needs to return each sprint's member issues, not only aggregate stats — see the updated route note below.
- **Board now tracks "whichever sprint was most recently started."** Since a project can have multiple active sprints (no constraint, per the earlier review) but the Board can only sensibly display one at a time, `GET /projects/{key}/board` should default its `?sprint=` query param to the most-recently-started active sprint when the param is omitted, rather than an arbitrary one.
- **Comment timestamps** were already `comment.created_at` in the schema — this is a rendering fix only (the UI now displays it), not a data model or route change.

## Data model / schema (tables/fields)

Built from the finalized feature scope and tech stack decisions. Answers locked in during design: multiple projects (each with its own key prefix and sprints/backlog), one global status list shared across issue types, freeform labels/components (not managed lookup lists), fixed priority set (not customizable).

## Entities

**Project** — `key_prefix` (unique, e.g. `WEB`), name, description, `next_issue_number` counter used to generate issue keys.

**Issue Type** — managed table (not a hardcoded enum), seeded with Epic/Story/Task/Subtask/Bug. Supports the "define your own custom issue types" requirement from the feature scope. `color` + `icon` give every type (including custom ones) a consistent visual identity. `no_sprint` marks types whose issues never carry a `sprint_id` — a flag rather than a name check, so it survives renames and applies to custom types too. **Both Epic and Story are seeded with `no_sprint = 1`**: they're managed together in a dedicated Epics & Stories view, not scheduled into sprints or listed in the per-project Backlog like Task/Subtask/Bug.

**Status** — one global table (To Do/In Progress/Blocked/Done seeded), editable (add/remove/reorder via `sort_order`). Doubles as the Kanban board's column definition — no separate board table needed.

**Sprint** — scoped to a project. `state` is planned/active/completed. No constraint on how many sprints in a project can be `active` at once.

**Issue** — the core entity. Key fields: `project_id`, `issue_number` (per-project sequence), `issue_type_id`, `parent_id` (self-referential — handles the Epic → Story/Task → Subtask hierarchy without a separate hierarchy table), `status_id`, `priority` (CHECK-constrained enum: low/medium/high/urgent), `story_points`, `due_date`, `sprint_id` (NULL = backlog item), `position` (float, manual drag order within the backlog or within a board column — assigned via fractional indexing so reordering never requires renumbering siblings).

**Issue Label / Issue Component** — join tables (`issue_id` + free text value) rather than a text column, so freeform tags stay filterable without a management screen.

**Issue Link** — `source_issue_id`, `target_issue_id`, `link_type` (`blocks` or `relates_to`). Directional: "A blocks B" is stored as one row; the UI derives "B is blocked by A" from the same row rather than storing both directions. Self-links are blocked at the DB level; app logic dedups the reverse-direction case for the symmetric `relates_to` type before inserting.

**Comment** — simple issue-scoped text log, no author field (single user).

**Sprint Issue** — history table recording every issue that was ever a member of a sprint, independent of the issue's *current* `sprint_id`. Exists solely so a completed sprint's report stays accurate after unfinished issues are reassigned elsewhere by the completion algorithm (see Design review notes above and sprint-completion-behavior below). Each row tracks when the issue was added, and — once it leaves — when and at what status category.

## Design notes / open items

- The visible issue key (e.g. `WEB-42`) is computed as `project.key_prefix + '-' + issue.issue_number`, not stored — avoids staleness if a prefix ever changes. Treat project prefixes as set-once-at-creation in the app; renaming one on an active project is not handled by this schema.
- Hierarchy rules (e.g. "a Subtask shouldn't parent another Subtask") are NOT enforced at the DB level — `parent_id` is a plain self-reference on `issue`. Enforce in application logic if desired; left flexible since these rules may change as the tool gets used.
- Priority is a CHECK-constrained enum on the issue row, not a table, per the "fixed set" decision — revisit only if you actually want to redefine priority levels later.

## File

`schema.sql` (in this project's files) — full SQLite DDL, includes seed data for default issue types and statuses. Designed to be embedded via Go's `embed` package and run with `CREATE TABLE IF NOT EXISTS` on startup, per the tech stack decision (no migration framework).

```bash
-- Local Jira — SQLite schema
-- See data-model.md in the project docs for design rationale.

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
    no_sprint INTEGER NOT NULL DEFAULT 0,   -- 1 = issues of this type never carry a sprint_id (Epic-like)
    is_default INTEGER NOT NULL DEFAULT 0   -- protects seeded types from accidental deletion
);

CREATE TABLE IF NOT EXISTS status (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,              -- To Do, In Progress, Done, Blocked, ...
    category TEXT NOT NULL DEFAULT 'todo' CHECK (category IN ('todo','in_progress','done')),
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
    priority TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low','medium','high','urgent')),
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
    status_category_at_removal TEXT CHECK (status_category_at_removal IN ('todo','in_progress','done'))
);

CREATE INDEX IF NOT EXISTS idx_issue_project ON issue(project_id);
CREATE INDEX IF NOT EXISTS idx_issue_sprint  ON issue(sprint_id);
CREATE INDEX IF NOT EXISTS idx_issue_status  ON issue(status_id);
CREATE INDEX IF NOT EXISTS idx_issue_parent  ON issue(parent_id);
CREATE INDEX IF NOT EXISTS idx_comment_issue ON comment(issue_id);
CREATE INDEX IF NOT EXISTS idx_sprint_issue_sprint ON sprint_issue(sprint_id);
CREATE INDEX IF NOT EXISTS idx_sprint_issue_issue  ON sprint_issue(issue_id);

-- Seed data: default issue types and statuses (safe to re-run)
INSERT OR IGNORE INTO issue_type (name, color, icon, no_sprint, is_default) VALUES
    ('Epic',    '#8F7EE7', '⚡', 1, 1),
    ('Story',   '#4BCE97', '🔖', 1, 1),
    ('Task',    '#579DFF', '✔', 0, 1),
    ('Subtask', '#85B8FF', '↳', 0, 1),
    ('Bug',     '#F87168', '🐞', 0, 1);

INSERT OR IGNORE INTO status (name, category, sort_order) VALUES
    ('To Do', 'todo', 1),
    ('In Progress', 'in_progress', 2),
    ('Blocked', 'in_progress', 3),
    ('Done', 'done', 4);
```

## API/route design (which endpoints the htmx frontend calls)

Decisions locked in: dual HTML + JSON (via content negotiation, not duplicate route trees), routes nested under project key, filter state lives in URL query params, and a backlog-to-board drag both assigns the sprint and sets status in one action.

## Rendering strategy

One route tree, not two. Each handler fetches its data once, then picks a renderer based on request headers:

- `HX-Request: true` (set automatically by htmx) → render an HTML fragment
- Plain browser navigation (no HX-Request header, `Accept: text/html`) → render the full page (fragment wrapped in layout)
- `Accept: application/json` → marshal the same data as JSON

htmx can issue real PATCH/DELETE requests directly (`hx-patch`, `hx-delete`) — no method-override workarounds needed like with plain HTML forms.

## Routes

### Search (global)

- `GET /search` — cross-project search, backs the topbar search box; `?q=` for live search-as-you-type, results grouped by project. Distinct from the per-project `GET /projects/{key}/issues?q=` below, which stays scoped to one project (e.g. for the backlog/board filter inputs).

### Projects

- `GET /projects` — list (home page)
- `POST /projects` — create
- `GET /projects/new` — new-project form fragment
- `GET /projects/{key}` — overview (likely redirects to board)
- `PATCH /projects/{key}` — update
- `DELETE /projects/{key}` — delete

### Board & backlog

- `GET /projects/{key}/board` — Kanban board; columns = Status rows; only issues whose type has `no_sprint = 0` are eligible. Query params (`sprint`, `label`, `component`, ...) pushed into URL via `hx-push-url`. `sprint` defaults to the most-recently-started `active` sprint when omitted, since a project may have more than one active sprint but the board only ever displays one at a time.
- `GET /projects/{key}/backlog` — unscheduled issues, same filter-param pattern, same `no_sprint = 0` restriction (Epics/Stories never appear here — see Epics & Stories below).

### Epics & Stories

- `GET /projects/{key}/epics-stories` — lists every issue whose type has `no_sprint = 1` (Epic, Story, and any future custom type flagged the same way), grouped by type. This is a dedicated management view, not a filtered slice of the Backlog — Epics and Stories are never scheduled into a sprint, so they don't otherwise appear in `/board` or `/backlog`.
- Creation, editing, and deletion of Epics/Stories reuse the standard Issue routes below (`POST/PATCH/DELETE .../issues[...]`) — there's no separate CRUD surface, just a different listing page and different default landing spot for the create form.

### Issues

- `GET /projects/{key}/issues` — search/list; also serves live search-as-you-type via `q=`
- `POST /projects/{key}/issues` — create
- `GET /projects/{key}/issues/new` — new-issue form fragment
- `GET /projects/{key}/issues/{num}` — detail page
- `GET /projects/{key}/issues/{num}/edit` — inline edit form fragment; does not include a type selector (see open item below)
- `PATCH /projects/{key}/issues/{num}` — update fields (summary, description, priority, points, due date, parent, labels, components). `type` is intentionally excluded from what the edit UI sends — an issue's type is fixed at creation; recategorizing is delete-and-recreate, not an edit. (The column/route can still technically accept a `type` change for future tooling; the primary UI just never sends one.)
- `DELETE /projects/{key}/issues/{num}` — delete; the UI gates this behind a confirmation step (`hx-confirm` in the real app) from two entry points: inline row actions in the Epics & Stories view, and a delete action on the issue detail view
- `PATCH /projects/{key}/issues/{num}/move` — dedicated endpoint for status/sprint changes, used by four distinct UI actions: the board's drag-and-drop (sends `status_id`), the backlog's drag-and-drop between sprint sections (sends `sprint_id`), and the issue detail view's Status and Sprint dropdowns (send `status_id` or `sprint_id` respectively, one at a time). Also accepts `position` for persisting manual drag order within the destination column/backlog; a pure backlog reorder sends just `position`. Returns the updated card fragment. Any change to `sprint_id` here also updates `sprint_issue` bookkeeping (closes the old membership row, opens a new one — see schema).

App-level rule: issues of a `no_sprint` issue type (Epic and Story, by default) never receive a `sprint_id` — the `/move` endpoint should reject or ignore `sprint_id` for them, and the detail view hides the Sprint dropdown entirely for these types.

**Open item — issue hierarchy has no UI.** The feature scope kept "Epic → Story/Task → Subtask nesting," and the schema still has `issue.parent_id` for it, but pulling Epics/Stories into their own view removed the only control that showed or set a parent (there's no "which epic is this under" picker anywhere now, for any type). Needs a decision: add a parent/child control somewhere sensible (e.g. an Epic's detail page listing/adding its child Stories), or formally descope hierarchy from the MVP UI and leave `parent_id` as schema-only groundwork.

### Comments (nested under issue)

- `POST /projects/{key}/issues/{num}/comments`
- `PATCH /projects/{key}/issues/{num}/comments/{id}`
- `DELETE /projects/{key}/issues/{num}/comments/{id}`

Each returns the comment-list fragment.

### Links (nested under issue)

- `POST /projects/{key}/issues/{num}/links` — body: target issue + `blocks`/`relates_to`. For `relates_to`, the handler checks both directions before inserting (symmetric relation — `(A,B)` and `(B,A)` are the same link and shouldn't both exist).
- `DELETE /projects/{key}/issues/{num}/links/{id}`

### Labels / Components (freeform — no CRUD, just suggestions)

- `GET /projects/{key}/labels` — distinct label values already used in the project (autocomplete only)
- `GET /projects/{key}/components` — same, for components

Labels/components are set via array fields on the issue update payload, not managed separately.

### Sprints

- `GET /projects/{key}/sprints` — list/history. Returns each sprint with its member issues eager-loaded (via `sprint_issue` for completed sprints, `issue.sprint_id` for planned/active ones) rather than just aggregate counts — the History view renders a completed sprint's full issue list inline, not only stats, so the fragment needs the rows, not just a number.
- `POST /projects/{key}/sprints` — create
- `PATCH /projects/{key}/sprints/{id}` — edit name/goal/dates
- `POST /projects/{key}/sprints/{id}/start` — state transition (planned → active); no constraint against multiple active sprints per project
- `GET /projects/{key}/sprints/{id}/complete` — completion preview fragment: shows the count of unfinished issues and the destination (next planned sprint by name, or "Backlog" if none exists), with a Confirm button. Skip straight to completion (no preview) when there are zero unfinished issues.
- `POST /projects/{key}/sprints/{id}/complete` — executes: moves unfinished issues (status unchanged) to the target sprint determined above, or to the backlog if no planned sprint exists; closes their `sprint_issue` rows (recording status category at that moment) and opens new ones against the target; sets this sprint's state to `completed`.
- `GET /projects/{key}/sprints/{id}/report` — sprint report (planned vs. completed); queries `sprint_issue` for this `sprint_id` (not `issue.sprint_id` directly), so issues that carried over still show up as "not completed" in this sprint's history

See `sprint-completion-behavior.md` for the full completion algorithm and rationale.

### Global settings (statuses & issue types are global, not per-project, per the data model)

- `GET/POST /settings/statuses`, `PATCH/DELETE /settings/statuses/{id}` — includes reordering for board column order
- `GET/POST /settings/issue-types`, `PATCH/DELETE /settings/issue-types/{id}` — includes `color`, `icon`, `no_sprint`

**Open item:** deleting a status/type still in use by an issue should be blocked or require reassignment first — app-logic rule, not a routing concern, but needs to be built.

## Wireframe of board/list views

[wireframe.html](Local%20Jira%20%E2%80%94%20MVP%20Feature%20Scope/wireframe.html)

## Decide sprint-completion behavior for unfinished issues

Decided: unfinished issues move directly to the next sprint when the current sprint completes.

## Algorithm

1. Trigger: completing a sprint (`POST /projects/{key}/sprints/{id}/complete`).
2. Gather every issue in this sprint whose status category is not `done`. Issues of a `no_sprint` type (Epic and Story, by default) are never sprint items (see below), so they're excluded from this by construction, not by a filter.
3. Determine the target for those unfinished issues:
    - Find the project's planned sprint with the earliest `start_date`.
    - If one exists → that's the target; unfinished issues get its `sprint_id`.
    - If none exists → target is the backlog (`sprint_id = NULL`).
4. Show a confirmation summary before committing — e.g. "3 issues will move to Sprint 5" or "3 issues will move to Backlog (no upcoming sprint yet)." If there are zero unfinished issues, skip the confirmation and complete immediately.
5. On confirm, for each unfinished issue: close its open `sprint_issue` row for this sprint (`removed_at = now`, `status_category_at_removal` = its current status category), update `issue.sprint_id` to the target (status is left unchanged — an "In Progress" issue stays "In Progress" in its new sprint/backlog slot), and open a new `sprint_issue` row against the target sprint (skip this last step if the target is the backlog — backlog isn't a sprint). For issues that *did* finish, no `sprint_issue` row needs to change — they keep the completed sprint as their sole membership record. Set the completed sprint's `state` to `completed`.

## Decisions and rationale

**No next sprint exists yet → fall back to backlog.** Completing a sprint should never be blocked on having pre-planned the next one. Anything that lands in the backlog can be re-added to a sprint once you create it.

**Multiple planned sprints exist → earliest start date wins**, no picker. Keeps sprint completion a single click in the common case; you're not asked to make a decision that has an obvious default.

**Confirmation summary shown before committing.** Cheap to build (just a count + destination name) and prevents an accidental sprint-complete from silently reshuffling issues without you noticing.

**Epics and Stories are excluded from sprint assignment entirely.** Epics span multiple sprints by definition (per the feature scope); Stories were added to the same `no_sprint` group later, once Epics and Stories became a dedicated management view separate from the Backlog (see the Epics & Stories design review notes above). Neither ever gets a `sprint_id` in the first place, so sprint completion logic only ever touches Task/Bug/Subtask. Enforce this at the application layer (e.g. the `/issues/{num}/move` handler rejects `sprint_id` on issues whose type has `no_sprint = 1`) since the DB schema doesn't restrict which issue types can carry a `sprint_id`.

**`sprint_issue` history exists so the report survives completion.** Step 5's `issue.sprint_id` reassignment is exactly what would otherwise make a completed sprint's own report look like 100% completion — the unfinished issues literally leave that `sprint_id`. Every `sprint_id` change (here and in the `/move` endpoint) is mirrored into `sprint_issue`, so `GET .../sprints/{id}/report` has an accurate, permanent record of who was ever in the sprint and how they left it.

## Still open / assumed default

Hierarchy behavior isn't fully pinned down: if a Story completes but one of its Subtasks doesn't, the current assumption is that **every issue is evaluated independently by its own status** — the Subtask moves to the next sprint (or backlog) on its own, regardless of what happened to its parent. Parent and child are not forced to move together. Revisit this if it turns out to feel wrong in practice (e.g. you might want "move parent → move its incomplete children with it" instead).

## Tech stack decision

Backend language: Go (chosen for familiarity). Philosophy throughout: local, single-user, zero-ops — optimize for "one binary, no server to manage," not for scale.

## Database: SQLite

A file, not a server — nothing to install, run, or manage separately.

Use `modernc.org/sqlite` (pure Go, no CGO) instead of `mattn/go-sqlite3` — compiles and cross-compiles cleanly without a C toolchain.

**Open item:** decide where the `.db` file lives and how it's backed up (e.g. a synced folder like Dropbox/Drive, or a periodic copy job). No server behind it means no built-in redundancy — worth deciding before real data accumulates, not after.

## Backend framework

Go 1.22+ stdlib `net/http` with its method+path routing (`mux.HandleFunc("GET /issues/{id}", ...)`) is enough at this scale. Reach for `chi` only if/when middleware chaining becomes a real need — don't start there.

## Schema / migrations

No migration framework (skip golang-migrate/goose for now). Embed `schema.sql` via Go's `embed` package, run `CREATE TABLE IF NOT EXISTS` on startup. Single environment, single owner — no staging/prod drift to manage.

## Auth

None. Localhost, single user — this whole complexity category is skipped.

## Frontend

Server-rendered Go templates (`html/template`) + htmx for interactivity (status changes, comments, filtering) without full page reloads, + Alpine.js for small bits of client-side state. SortableJS handles Kanban drag-and-drop, firing an htmx request on drop to persist the column/status change. Styling via Tailwind through a CDN link or precompiled CSS — no build step.

Rejected: React/Vue SPA + separate Go JSON API. That pattern's complexity (build tooling, CORS, two codebases, API versioning) pays off with a frontend team or multi-platform clients — for a solo local tool it's overhead without payoff.

Whole app — templates, static JS/CSS — embeds via `embed.FS` into a single binary. No npm/webpack/vite.

**Watch item:** if sprint reports or future features want actual charts (e.g. planned-vs-completed bars), drop in Chart.js via CDN rather than treating it as a reason to reconsider the stack.

## Summary

Go stdlib backend + SQLite (pure-Go driver) + htmx/Alpine/SortableJS frontend, all embedded into one binary, no auth, no separate build pipeline. Matches the finalized feature scope with no conflicts.