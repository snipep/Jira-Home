# Jira Home

A self-hosted, single-binary Jira-style issue tracker for one person: a board, a backlog, sprints, and Epics & Stories for organizing work — no account, no cloud, no external services, no project switcher. Everything runs from one Go binary backed by a single SQLite file.

## Features

- **One workspace** — no multi-project bifurcation; use Epics to organize work instead
- **Board** view for the active sprint with drag-and-drop status columns
- **Backlog** with per-sprint grouping and filtering (type / label / priority / text)
- **Epics & Stories** managed separately from the sprint-based backlog (they're never scheduled into a sprint) — this is how you bifurcate work instead of separate projects
- **Sprints**: create, start, and complete — completing a sprint moves unfinished issues to the next planned sprint (or back to the backlog) while preserving a full history of what was in the sprint via a sprint report
- **Issues**: create, edit, delete, comment, link to other issues (blocks / relates to), optional parent (e.g. a subtask's parent task), labels/components with autocomplete
- **Custom issue types and statuses**, configurable in Settings — including the built-in ones, as long as nothing still uses them
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
| GET | `/epics-stories` | Epics & Stories view |
| GET | `/issues` | List issues |
| POST | `/issues` | Create an issue |
| GET | `/issues/{num}` | Issue detail |
| PATCH / DELETE | `/issues/{num}` | Update / delete an issue |
| PATCH | `/issues/{num}/move` | Move an issue (status, sprint, position) |
| POST | `/issues/{num}/comments` | Add a comment |
| PATCH / DELETE | `/issues/{num}/comments/{id}` | Update / delete a comment |
| POST / DELETE | `/issues/{num}/links` / `/links/{id}` | Link / unlink issues |
| GET | `/labels`, `/components` | Autocomplete sources |
| GET | `/sprints` | List sprints |
| POST | `/sprints` | Create a sprint |
| PATCH | `/sprints/{id}` | Update a sprint |
| POST | `/sprints/{id}/start` | Start a sprint |
| GET / POST | `/sprints/{id}/complete` | Preview / perform sprint completion |
| GET | `/sprints/{id}/report` | Sprint report (issue history) |
| GET / POST | `/settings/statuses` | List / create statuses |
| PATCH / DELETE | `/settings/statuses/{id}` | Update / delete a status |
| POST | `/settings/statuses/reorder` | Reorder statuses |
| GET / POST | `/settings/issue-types` | List / create issue types |
| PATCH / DELETE | `/settings/issue-types/{id}` | Update / delete an issue type |
| GET / POST | `/settings/workspace` | View / rename the workspace |

## Design notes

- There's exactly one workspace (`Store.DefaultProject`), auto-created on first run — no project switcher, no multi-tenancy. Issue numbering is still atomic and prefixed (e.g. `HOME-1`, `HOME-2`, ...), a holdover from the schema's project-scoped design.
- Issue types have a `no_sprint` flag — Epics and Stories are seeded this way, which is what routes them to the Epics & Stories view instead of the sprint backlog and blocks them from being scheduled into a sprint. This is the intended way to bifurcate work instead of separate projects.
- Sprint history is tracked in a separate `sprint_issue` table (not just `issue.sprint_id`), so completing a sprint doesn't lose the record of what was actually in it — sprint reports remain accurate after issues move on.
- The database only ever has one open connection (`SetMaxOpenConns(1)`), since SQLite `PRAGMA` settings are per-connection and this app wants single-writer semantics — there's no concurrent-write contention to design around.
- There's no schema migration framework (`schema.sql` only runs `CREATE TABLE IF NOT EXISTS`), so the `project` table and `project_id` columns are permanent, even though the UI now only ever shows one project.

See `docs/design/` for the full feature spec this implementation follows.
