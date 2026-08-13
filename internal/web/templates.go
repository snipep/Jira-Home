package web

import (
	"embed"
	"fmt"
	"html/template"
	"strings"
	"time"
)

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

var priorityIcons = map[string]string{
	"urgent": "🔴", "high": "🔺", "medium": "🟠", "low": "🔵",
}
var priorityLabels = map[string]string{
	"urgent": "Urgent", "high": "High", "medium": "Medium", "low": "Low",
}

// statusCategoryClass maps a status's category (the only 3-way distinction
// the data model gives us — statuses themselves are user-renamable) to a
// pill color: todo/in_progress/done.
func statusCategoryClass(category string) string {
	switch category {
	case "done":
		return "status-done"
	case "in_progress":
		return "status-inprogress"
	default:
		return "status-todo"
	}
}

var templateFuncs = template.FuncMap{
	"priorityIcon":  func(p string) string { return priorityIcons[p] },
	"priorityLabel": func(p string) string { return priorityLabels[p] },
	"statusClass":   statusCategoryClass,
	"upper":         strings.ToUpper,
	"default": func(fallback, v string) string {
		if v == "" {
			return fallback
		}
		return v
	},
	"pointsOrDash": func(p *int) string {
		if p == nil {
			return "—"
		}
		return fmt.Sprintf("%d", *p)
	},
	"int64PtrEq": func(p *int64, id int64) bool { return p != nil && *p == id },
	"join":    strings.Join,
	"fmtTime": formatTimestamp,
	"firstChar": func(s string) string {
		if s == "" {
			return ""
		}
		return string([]rune(s)[0])
	},
}

// formatTimestamp renders a SQLite datetime('now') string ("2006-01-02
// 15:04:05", UTC) as something readable. Falls back to the raw value if it
// doesn't parse, rather than erroring the whole page render.
func formatTimestamp(raw string) string {
	t, err := time.Parse("2006-01-02 15:04:05", raw)
	if err != nil {
		return raw
	}
	return t.Format("Jan 2, 2006 3:04 PM")
}

func loadTemplates() *template.Template {
	tmpl := template.New("").Funcs(templateFuncs)
	return template.Must(tmpl.ParseFS(templateFS, "templates/*.html", "templates/partials/*.html"))
}
