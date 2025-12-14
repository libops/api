package dash

import (
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var pageTemplates map[string]*template.Template

// InitTemplates loads all HTML templates from web/templates
func InitTemplates(templatesDir string) error {
	pageTemplates = make(map[string]*template.Template)

	titleCaser := cases.Title(language.English)
	funcMap := template.FuncMap{
		"lower":       strings.ToLower,
		"upper":       strings.ToUpper,
		"title":       titleCaser.String,
		"singularize": singularize,
	}

	// 1. Parse shared templates
	sharedFiles := []string{
		filepath.Join(templatesDir, "base.html"),
		filepath.Join(templatesDir, "sidebar.html"),
		filepath.Join(templatesDir, "banner.html"),
	}

	baseTmpl, err := template.New("base").Funcs(funcMap).ParseFiles(sharedFiles...)
	if err != nil {
		return err
	}

	// 2. Identify all .html files
	files, err := filepath.Glob(filepath.Join(templatesDir, "*.html"))
	if err != nil {
		return err
	}

	for _, file := range files {
		name := filepath.Base(file)
		// Skip shared files
		if name == "base.html" || name == "sidebar.html" || name == "banner.html" {
			continue
		}

		// Clone base
		tmpl, err := baseTmpl.Clone()
		if err != nil {
			return err
		}

		// Parse the page file
		_, err = tmpl.ParseFiles(file)
		if err != nil {
			return err
		}

		pageTemplates[name] = tmpl
		slog.Info("Loaded page template", "name", name)
	}

	return nil
}

// RenderTemplate renders a template with the given name and data
func RenderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl, ok := pageTemplates[name]
	if !ok {
		slog.Error("Template not found", "name", name)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	err := tmpl.ExecuteTemplate(w, name, data)
	if err != nil {
		slog.Error("Failed to render template", "name", name, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// singularize converts plural resource names to singular
func singularize(str string) string {
	singular := map[string]string{
		"Organizations":  "Organization",
		"Projects":       "Project",
		"Sites":          "Site",
		"Secrets":        "Secret",
		"Firewall Rules": "Firewall Rule",
		"Members":        "Member",
		"organizations":  "organization",
		"projects":       "project",
		"sites":          "site",
		"secrets":        "secret",
		"firewall rules": "firewall rule",
		"members":        "member",
	}
	if s, ok := singular[str]; ok {
		return s
	}
	// Default: just remove trailing 's'
	return strings.TrimSuffix(str, "s")
}
