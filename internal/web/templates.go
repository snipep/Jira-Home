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

var priorityLabels = map[string]string{
	"highest": "Highest", "high": "High", "medium": "Medium", "low": "Low", "lowest": "Lowest",
}

// priorityIcons render as Jira's own five-level priority glyphs — stacked
// chevrons for highest/lowest, a single chevron for high/low, an equals
// sign for medium — rather than colored dot emoji — inline SVG so the exact
// shape and color are under our control instead of the platform's emoji font.
var priorityIcons = map[string]template.HTML{
	"highest": svgIcon("#CD1317", `<path d="M4 9 L8 5 L12 9"/><path d="M4 13 L8 9 L12 13"/>`),
	"high":    svgIcon("#E2483D", `<path d="M4 11 L8 7 L12 11"/>`),
	"medium":  svgIcon("#FF8B00", `<path d="M3 6 H13"/><path d="M3 10 H13"/>`),
	"low":     svgIcon("#0C66E4", `<path d="M4 5 L8 9 L12 5"/>`),
	"lowest":  svgIcon("#0C66E4", `<path d="M4 3 L8 7 L12 3"/><path d="M4 7 L8 11 L12 7"/>`),
}

func svgIcon(color, paths string) template.HTML {
	return template.HTML(`<svg class="priority-icon" viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="` +
		color + `" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`)
}

// statusCategoryClass maps a status's category (the only 4-way distinction
// the data model gives us — statuses themselves are user-renamable) to a
// pill color: todo/in_progress/done/retired.
func statusCategoryClass(category string) string {
	switch category {
	case "done":
		return "status-done"
	case "in_progress":
		return "status-inprogress"
	case "retired":
		return "status-retired"
	default:
		return "status-todo"
	}
}

// epicColorPalette gives each Epic a distinct, stable color so its
// Task/Bug/Subtask descendants can be visually grouped by which epic they
// belong to in list views — deterministic from the epic's id (see
// epicColor), so no extra column/storage is needed.
var epicColorPalette = []string{
	"#8F7EE7", "#579DFF", "#4BCE97", "#F87168",
	"#FFC400", "#FF8B00", "#00C7E6", "#E774BB",
}

func epicColor(epicID *int64) string {
	if epicID == nil || *epicID <= 0 {
		return epicColorPalette[0]
	}
	return epicColorPalette[*epicID%int64(len(epicColorPalette))]
}

var templateFuncs = template.FuncMap{
	"priorityIcon":  func(p string) template.HTML { return priorityIcons[p] },
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
	"pct": func(part, total int) int {
		if total <= 0 {
			return 0
		}
		return part * 100 / total
	},
	"epicColor":  epicColor,
	"join":       strings.Join,
	"fmtTime":    formatTimestamp,
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
