package web

import (
	"bytes"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

// PageData wraps a pre-rendered fragment for the layout template. Handlers
// never build layout HTML themselves — render() decides, per request,
// whether the fragment stands alone (htmx) or gets wrapped (full navigation).
type PageData struct {
	Title  string
	Active string // which top-level nav tab is highlighted
	Body   template.HTML
}

func isHXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func wantsJSON(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html")
}

// render executes tmplName as the response. JSON is served straight from
// data; HTML is rendered through the named template and, for a plain
// browser navigation (no HX-Request header), wrapped in the shared layout.
func (s *Server) render(w http.ResponseWriter, r *http.Request, tmplName string, data any, title, active string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, tmplName, data); err != nil {
		http.Error(w, "render "+tmplName+": "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if isHXRequest(r) {
		buf.WriteTo(w)
		return
	}
	if err := s.templates.ExecuteTemplate(w, "layout.html", PageData{
		Title:  title,
		Active: active,
		Body:   template.HTML(buf.String()),
	}); err != nil {
		http.Error(w, "render layout: "+err.Error(), http.StatusInternalServerError)
	}
}

// redirectWithError sends the user back to path with an error message
// attached as a query param, for actions (like a blocked delete) whose
// failure needs to survive a redirect rather than render inline — htmx's
// default response handling doesn't reliably swap non-2xx bodies, so a
// renderError() fragment on a bare hx-delete button can fail silently.
func (s *Server) redirectWithError(w http.ResponseWriter, r *http.Request, path, message string) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	target := path + sep + "error=" + url.QueryEscape(message)
	if isHXRequest(r) {
		w.Header().Set("HX-Redirect", target)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// renderError renders a small error fragment for HTML clients, or a JSON
// error body for API clients — used for validation/business-rule failures
// that should surface inline rather than as a blank error page.
func (s *Server) renderError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	s.templates.ExecuteTemplate(w, "error.html", message)
}
