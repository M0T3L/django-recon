package api

import (
	"crypto/subtle"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"gorm.io/gorm"

	"django/internal/notifier"
	"django/internal/pipeline"
	"django/internal/worker"
	"django/web"
)

// Server represents the HTTP web API server.
type Server struct {
	db               *gorm.DB
	workerPool       *worker.Pool
	notifier         *notifier.Notifier
	pipelineRegistry *pipeline.Registry
	templates        map[string]*template.Template
	router           *http.ServeMux
	basicUser        string
	basicPass        string
}

// NewServer constructs and initializes the HTTP Server.
func NewServer(db *gorm.DB, pool *worker.Pool, notif *notifier.Notifier, registry *pipeline.Registry) (*Server, error) {
	s := &Server{
		db:               db,
		workerPool:       pool,
		notifier:         notif,
		pipelineRegistry: registry,
		router:           http.NewServeMux(),
	}

	if err := s.loadTemplates(); err != nil {
		return nil, fmt.Errorf("failed to load HTML templates: %w", err)
	}

	s.setupRoutes()

	return s, nil
}

// SetBasicAuth configures HTTP Basic Authentication credentials for the server.
func (s *Server) SetBasicAuth(user, pass string) {
	s.basicUser = user
	s.basicPass = pass
}

// templateFuncs provides custom functions available in all templates.
var templateFuncs = template.FuncMap{
	"add": func(a, b int) int { return a + b },
	"sub": func(a, b int) int { return a - b },
}

// loadTemplates parses embedded templates from web.TemplateFS.
func (s *Server) loadTemplates() error {
	s.templates = make(map[string]*template.Template)

	pages := []string{"dashboard.html", "subdomains.html", "gallery.html", "findings.html"}

	for _, page := range pages {
		tmpl, err := template.New(page).Funcs(templateFuncs).ParseFS(
			web.TemplateFS,
			"templates/layout.html",
			"templates/"+page,
			"templates/partials/*.html",
		)
		if err != nil {
			return fmt.Errorf("error parsing template %s: %w", page, err)
		}
		s.templates[page] = tmpl
	}

	// Parse individual partial templates for HTMX responses
	partials, err := fs.Glob(web.TemplateFS, "templates/partials/*.html")
	if err == nil {
		for _, partialPath := range partials {
			baseName := filepath.Base(partialPath)
			tmpl, err := template.New(baseName).Funcs(templateFuncs).ParseFS(web.TemplateFS, partialPath)
			if err == nil {
				s.templates[baseName] = tmpl
			}
		}
	}

	return nil
}

// setupRoutes registers all HTTP endpoints and static file servers.
func (s *Server) setupRoutes() {
	// HTML Page Routes
	s.router.HandleFunc("GET /{$}", s.handleDashboardPage)
	s.router.HandleFunc("GET /subdomains", s.handleSubdomainsPage)
	s.router.HandleFunc("GET /gallery", s.handleGalleryPage)
	s.router.HandleFunc("GET /findings", s.handleFindingsPage)

	// HTMX & API Endpoints
	s.router.HandleFunc("POST /api/targets", s.handleCreateTargets)
	s.router.HandleFunc("GET /api/jobs/active", s.handleActiveJobsPartial)
	s.router.HandleFunc("GET /api/subdomains", s.handleSubdomainsTablePartial)
	s.router.HandleFunc("GET /api/subdomains/modal", s.handleScreenshotModal)
	s.router.HandleFunc("GET /api/gallery", s.handleGalleryGridPartial)
	s.router.HandleFunc("GET /api/gallery/detail", s.handleAssetDetailModal)
	s.router.HandleFunc("GET /api/findings/detail", s.handleFindingDetailModal)
	s.router.HandleFunc("POST /api/jobs/cancel", s.handleCancelJob)
	s.router.HandleFunc("DELETE /api/targets", s.handleDeleteTarget)

	// Static Screenshots File Server
	_ = os.MkdirAll("web/static/screenshots", 0755)
	screenshotFS := http.Dir("web/static/screenshots")
	s.router.Handle("GET /screenshots/", http.StripPrefix("/screenshots/", http.FileServer(screenshotFS)))
}

// ServeHTTP delegates request handling to the internal router with optional Basic Auth protection.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.basicUser != "" && s.basicPass != "" {
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(s.basicUser)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(s.basicPass)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Django Recon Dashboard"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	s.router.ServeHTTP(w, r)
}

// renderTemplate safely renders HTML templates with fallback error response.
func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	tmpl, exists := s.templates[templateName]
	if !exists {
		log.Printf("Template %s not found", templateName)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("Error rendering template %s: %v", templateName, err)
	}
}

// renderPartial renders an individual partial component for HTMX responses.
func (s *Server) renderPartial(w http.ResponseWriter, partialName string, data interface{}) {
	tmpl, exists := s.templates[partialName]
	if !exists {
		// Fallback lookup by base name
		tmpl, exists = s.templates[filepath.Base(partialName)]
	}

	if !exists {
		log.Printf("Partial template %s not found", partialName)
		http.Error(w, "Partial template not found", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, partialName, data); err != nil {
		// Try executing without name if single template
		if err := tmpl.Execute(w, data); err != nil {
			log.Printf("Error rendering partial %s: %v", partialName, err)
		}
	}
}
