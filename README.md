# Jira Home

A self-hosted, single-binary Jira-style issue tracker for one person: a board, a backlog, sprints, and Epics for organizing work — no account, no cloud, no external services, no project switcher. Everything runs from one Go binary backed by a single SQLite file.

## Features

- **One workspace** — no multi-project bifurcation; use Epics to organize work instead
- **A strict hierarchy — Epic > Task/Bug (or any custom type) > Subtask.** Every Task, Bug, or custom type needs a parent Epic; every Subtask needs a parent Task/Bug. Epics themselves have no parent and are never scheduled into a sprint.
- **Color-coded by Epic** — each Epic gets its own stable color, shown as a chip and a left-border accent on every Task/Bug/Subtask under it (on the Board and in the Backlog), so it's obvious at a glance which epic a card belongs to
- **Board** view for the active sprint with drag-and-drop status columns
- **Backlog** with per-sprint grouping and filtering (type / label / priority / text), each sprint header showing live To Do / In Progress / Done counts
- **A slide-over detail panel** — clicking any card or row opens it in a sidebar over whatever you were looking at (view, edit, comment, change status/sprint) instead of navigating away; direct links to an issue still load it as a full page
- **Epics** managed separately from the sprint-based backlog — this is how you organize work instead of separate projects
- **Sprints**: create, start, complete, edit, and delete, each with an optional **auto-complete** toggle (needs an end date) — when it ends, unfinished work carries over and a same-length successor starts immediately, with auto-complete carried forward so the chain keeps going until it's unchecked on some future sprint
- **Sprint history** preserved via a full sprint report, whether a sprint was completed by hand or automatically
- **Issues**: create, edit, delete, comment, with freeform labels; five-level priority (Highest → Lowest) rendered as Jira-style chevron/equals-sign icons
- **Custom issue types and statuses**, configurable in Settings — including the built-in ones, as long as nothing still uses them — with drag-to-reorder for statuses (sets the board's column order)
- **Retired/Invalid status category** — mark a ticket Retired mid-sprint and it rides along normally until the sprint completes, then it's pulled into a separate Retired list below the Backlog (not carried to the next sprint, not cluttering the backlog) until it's restored back to To Do
- **Analysis** — per-completed-sprint charts of how much work actually finished (issues and story points), plus a velocity trend across sprints; computed live, so deleting a sprint updates it immediately
- Sprints can be deleted from history too, not just while planned/active — permanently removes them from Analysis
- **Global search**
- **Dark mode**
- **No auth, no external dependencies at runtime** — everything (templates, schema) is embedded in the binary; the only state is one `.db` file

## Tech stack

- Go standard library `net/http` (1.22+ method/path routing, no framework)
- SQLite via [`modernc.org/sqlite`](https://gitlab.com/cznic/sqlite) — pure Go, no CGO required
- `html/template` server-rendered pages + [htmx](https://htmx.org/) for interactivity — no JS build step, no SPA
- Everything embedded via `embed.FS`: HTML templates and the database schema ship inside the compiled binary

## Getting started

### Option 1: Docker Compose (recommended)

```sh
docker compose up -d
```

The app is now running at [http://localhost:8080](http://localhost:8080). Data persists in a named Docker volume (`jira-home-data`) so it survives container restarts and rebuilds.

To stop it:

```sh
docker compose down
```

To also delete the stored data:

```sh
docker compose down -v
```

### Option 2: Run locally with Go

Requires Go 1.25+.

```sh
go build -o jira-home .
./jira-home
```

By default this listens on `:8080` and stores its database at `data/jira-home.db` (created automatically). Open [http://localhost:8080](http://localhost:8080).

### Configuration

The app is configured entirely through environment variables:

| Variable          | Default                 | Description                          |
|--------------------|--------------------------|---------------------------------------|
| `JIRA_HOME_ADDR`    | `:8080`                  | Address/port the server listens on    |
| `JIRA_HOME_DB`      | `data/jira-home.db`      | Path to the SQLite database file      |

## Project layout

```
main.go                    # entrypoint: config, store, server wiring
internal/model/             # shared domain structs (Project, Issue, Sprint, ...)
internal/store/             # SQLite access layer — no HTTP concerns
internal/web/                # HTTP handlers, routing, and templates
internal/web/templates/      # html/template views (embedded at build time)
docs/design/                 # design spec and wireframe
```

## API overview

Every route serves a full HTML page by default. Requests from htmx (`HX-Request` header) get an HTML fragment instead of a full page, and requests with `Accept: application/json` get a JSON response — so the same routes double as a plain JSON API.

| Method | Path | Description |
|---|---|---|
| GET | `/search?q=` | Global search |
| GET | `/board` | Active sprint board |
| GET | `/backlog` | Backlog grouped by sprint |
| GET | `/epics-stories` | Epics view |
| GET | `/issues` | List issues |
| POST | `/issues` | Create an issue |
| GET | `/issues/{num}` | Issue detail |
| PATCH / DELETE | `/issues/{num}` | Update / delete an issue |
| PATCH | `/issues/{num}/move` | Move an issue (status, sprint, position) |
| POST | `/issues/{num}/comments` | Add a comment |
| PATCH / DELETE | `/issues/{num}/comments/{id}` | Update / delete a comment |
| POST / DELETE | `/issues/{num}/links` / `/links/{id}` | Link / unlink issues *(no UI currently — see below)* |
| GET | `/labels`, `/components` | Autocomplete sources *(no UI currently — see below)* |
| GET | `/sprints` | List sprints |
| POST | `/sprints` | Create a sprint |
| GET | `/sprints/{id}/edit` | Edit-sprint form |
| PATCH / DELETE | `/sprints/{id}` | Update / delete a sprint |
| POST | `/sprints/{id}/start` | Start a sprint |
| GET / POST | `/sprints/{id}/complete` | Preview / perform sprint completion |
| GET | `/sprints/{id}/report` | Sprint report (issue history) |
| GET | `/analysis` | Per-completed-sprint completion charts |
| GET / POST | `/settings/statuses` | List / create statuses |
| PATCH / DELETE | `/settings/statuses/{id}` | Update / delete a status |
| POST | `/settings/statuses/reorder` | Reorder statuses |
| GET / POST | `/settings/issue-types` | List / create issue types |
| PATCH / DELETE | `/settings/issue-types/{id}` | Update / delete an issue type |
| GET / POST | `/settings/workspace` | View / rename the workspace |

## Design notes

- There's exactly one workspace (`Store.DefaultProject`), auto-created on first run — no project switcher, no multi-tenancy. Issue numbering is still atomic and prefixed (e.g. `HOME-1`, `HOME-2`, ...), a holdover from the schema's project-scoped design.
- Issue types have a `no_sprint` flag — Epic is seeded this way, which is what routes it to the Epics view instead of the sprint backlog and blocks it from being scheduled into a sprint.
- The hierarchy (Epic > Task/Bug/custom > Subtask) is enforced in `store.validateParentTier`, called from both create and update: a Subtask's parent must be a non-Epic, non-Subtask issue; every other non-Epic type's parent must be an Epic. Creating without a valid parent is rejected outright; editing an issue that predates this rule (no parent) is allowed to go through unrelated changes without being forced to backfill one.
- Each Epic's color (`epicColor` in `internal/web/templates.go`) is derived deterministically from its id against a fixed palette — nothing is stored, so it's stable across restarts without a schema column. A Task/Bug/Subtask's epic is resolved for display via `store.attachEpicAncestors`, walking up one hop (its own parent) or two (a Subtask's parent's parent) in a couple of batched queries, not one query per row.
- The slide-over panel is plain htmx: every card/row link both points at the real `/issues/{num}` URL (for a direct visit or right-click-open-in-new-tab) and carries `hx-get`/`hx-target="#detail-panel-body"` so a normal click swaps the fragment into the always-present (but hidden) panel instead; a global `htmx:afterSwap` listener opens it whenever that target receives content. Saving an edit or adding a link re-renders into the same target rather than issuing an `HX-Redirect`, which would force a full navigation regardless of target and kick you out of the sidebar.
- Due date, Components, and the Links UI were removed from the issue detail view and forms by request; the routes/store methods/schema columns behind them are untouched, so they're one template change away from coming back if needed.
- **Sprint auto-completion** (`Store.RunSprintAutoCycle`, driven by a goroutine in `main.go` that ticks hourly, plus once at startup) is per-sprint, not global: it only acts on an *active* sprint with `auto_complete` set and an `end_date` that has passed. It derives the successor's length from that sprint's own start/end dates (falling back to 14 days if start_date is missing), creates it, `CompleteSprint`s the old one into it (`PreviewCompletion` targets the earliest-start planned sprint, which the just-created successor always is), then starts it — carrying `auto_complete` forward so the chain continues until someone unchecks it.
- `Store.DeleteSprint` allows deleting a sprint in any state, including completed ones — it's the only way to remove a sprint from history (and, since Analysis reads live, from Analysis too). Any issue still pointing at it — a planned/active sprint's issues, or a completed sprint's finished issues, which stay assigned per `CompleteSprint` — falls back to the backlog (`issue.sprint_id` is `ON DELETE SET NULL`); `sprint_issue` history rows cascade-delete with it.
- Statuses have a fourth category, `retired`, alongside `todo`/`in_progress`/`done` — it's just another board column (a `Status.SortOrder` position like any other, seeded immediately before the first Done column), so a mid-sprint Retired ticket rides along normally. The special-casing is entirely in `Store.CompleteSprint`: a retired-category issue is never carried to the next sprint/backlog target the way other unfinished issues are — it's always dropped straight to `sprint_id = NULL`, which is what lands it in `Store.ListRetired`'s result (the Backlog page's separate Retired section) instead of `Store.ListBacklog`'s. A "Restore" action moves it to `Store.DefaultTodoStatus()`, returning it to normal circulation.
- Sprint history is tracked in a separate `sprint_issue` table (not just `issue.sprint_id`), so completing a sprint — by hand or automatically — doesn't lose the record of what was actually in it; sprint reports remain accurate after issues move on.
- The database only ever has one open connection (`SetMaxOpenConns(1)`), since SQLite `PRAGMA` settings are per-connection and this app wants single-writer semantics — there's no concurrent-write contention to design around.
- Priority is Jira's own five levels — Highest, High, Medium, Low, Lowest — rendered as inline SVG chevrons/equals-sign glyphs (`priorityIcons` in `internal/web/templates.go`) rather than emoji, so shape and color are exact instead of platform-dependent.
- There's no schema migration framework (`schema.sql` only runs `CREATE TABLE IF NOT EXISTS`), with two deliberate exceptions, both on startup and both safe to no-op on a fresh or already-migrated database: `store.ensureColumn` runs an additive `ALTER TABLE ADD COLUMN` for columns added after a database was first created (currently `sprint.auto_complete`); `store.migratePriorityLevels` rebuilds the `issue` table under SQLite's documented procedure (new table, copy rows, drop, rename) when its `priority` `CHECK` constraint still only allows the old four levels — a `CHECK` is baked into `CREATE TABLE` and can't be widened additively the way a column can. Old `'urgent'` rows become `'highest'` during the copy; nothing is lost. Story was dropped from the seeded issue types in favor of the Epic hierarchy; `store.removeUnusedSeedType` cleans it up on startup for existing databases too, but only if no issue still uses it.

See `docs/design/` for the full feature spec this implementation follows.
