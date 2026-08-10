package api

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"django/internal/models"
	"django/internal/worker"
)

// isValidTargetDomain validates that target domain/host is safe, well-formed, and not an internal IP/CLI flag injection.
func isValidTargetDomain(domain string) bool {
	d := strings.TrimSpace(strings.ToLower(domain))
	if d == "" || len(d) > 253 {
		return false
	}
	if strings.HasPrefix(d, "-") {
		return false
	}
	if d == "localhost" || strings.HasSuffix(d, ".localhost") || d == "127.0.0.1" || d == "::1" || d == "0.0.0.0" {
		return false
	}
	if strings.HasPrefix(d, "10.") || strings.HasPrefix(d, "192.168.") || strings.HasPrefix(d, "169.254.") {
		return false
	}
	if ip := net.ParseIP(d); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return false
		}
	}
	for _, char := range d {
		if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '.' || char == ':') {
			return false
		}
	}
	return true
}

// handleDashboardPage renders the main dashboard overview page.
func (s *Server) handleDashboardPage(w http.ResponseWriter, r *http.Request) {
	var totalTargets int64
	var liveSubdomains int64
	var totalFindings int64
	var activeJobs int64

	if s.db != nil {
		s.db.Model(&models.Target{}).Count(&totalTargets)
		s.db.Model(&models.Subdomain{}).Count(&liveSubdomains)
		s.db.Model(&models.Finding{}).Where("tool_name != ?", "katana").Count(&totalFindings)
		s.db.Model(&models.ScanJob{}).Where("status IN ?", []string{"pending", "running"}).Count(&activeJobs)
	}

	var jobs []models.ScanJob
	if s.db != nil {
		s.db.Preload("Target").Order("id desc").Limit(15).Find(&jobs)
	}

	data := struct {
		ActivePage     string
		TotalTargets   int64
		LiveSubdomains int64
		TotalFindings  int64
		ActiveJobs     int64
		Jobs           []models.ScanJob
	}{
		ActivePage:     "dashboard",
		TotalTargets:   totalTargets,
		LiveSubdomains: liveSubdomains,
		TotalFindings:  totalFindings,
		ActiveJobs:     activeJobs,
		Jobs:           jobs,
	}

	s.renderTemplate(w, "dashboard.html", data)
}

// handleActiveJobsPartial renders the active jobs table partial for HTMX polling.
func (s *Server) handleActiveJobsPartial(w http.ResponseWriter, r *http.Request) {
	var jobs []models.ScanJob
	if s.db != nil {
		s.db.Preload("Target").Order("id desc").Limit(15).Find(&jobs)
	}

	data := struct {
		Jobs []models.ScanJob
	}{
		Jobs: jobs,
	}

	s.renderPartial(w, "active_jobs.html", data)
}

// handleCreateTargets processes target submission forms and enqueues scan jobs.
func (s *Server) handleCreateTargets(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.renderPartial(w, "toast.html", map[string]interface{}{
			"Success": false,
			"Message": "Invalid form submission",
		})
		return
	}

	domainsInput := r.FormValue("domains")
	pipelineType := r.FormValue("pipeline_type")
	if pipelineType == "" {
		pipelineType = "pipeline_1"
	}

	rawLines := strings.Split(domainsInput, "\n")
	var validDomains []string
	for _, line := range rawLines {
		d := strings.TrimSpace(line)
		d = strings.TrimPrefix(d, "http://")
		d = strings.TrimPrefix(d, "https://")
		d = strings.Split(d, "/")[0]
		if isValidTargetDomain(d) {
			validDomains = append(validDomains, d)
		}
	}

	if len(validDomains) == 0 {
		s.renderPartial(w, "toast.html", map[string]interface{}{
			"Success": false,
			"Message": "Please provide at least one valid target domain.",
		})
		return
	}

	enqueuedCount := 0
	for _, domainStr := range validDomains {
		var target models.Target
		if s.db != nil {
			err := s.db.Where("domain = ?", domainStr).FirstOrCreate(&target, models.Target{Domain: domainStr}).Error
			if err != nil {
				continue
			}
		} else {
			target.Domain = domainStr
		}

		job := worker.Job{
			TargetID:     target.ID,
			Domain:       target.Domain,
			PipelineType: pipelineType,
		}

		if s.workerPool != nil {
			if err := s.workerPool.Enqueue(job); err == nil {
				enqueuedCount++
			}
		} else {
			enqueuedCount++
		}
	}

	msg := "Enqueued target for " + pipelineType + " scanning!"
	if enqueuedCount > 1 {
		msg = "Successfully enqueued " + strconv.Itoa(enqueuedCount) + " targets for " + pipelineType + " scanning!"
	}

	s.renderPartial(w, "toast.html", map[string]interface{}{
		"Success": true,
		"Message": msg,
	})
}

// SubdomainPageData holds all data for the subdomains page template.
type SubdomainPageData struct {
	ActivePage string
	Subdomains []models.Subdomain

	// View Mode ("table" or "grid")
	ViewMode string

	// Pagination
	CurrentPage int
	TotalPages  int
	TotalCount  int64
	PerPage     int
	HasPrev     bool
	HasNext     bool

	// Current filter/sort state (to preserve in HTMX requests)
	Search        string
	StatusCode    string
	TargetDomain  string
	HasScreenshot string
	HasTech       string
	SortBy        string

	// Available target domains for the dropdown
	TargetDomains []string
}

func (d SubdomainPageData) IsStatusCodeSelected(code string) bool {
	if d.StatusCode == "" { return false }
	for _, c := range strings.Split(d.StatusCode, ",") {
		if strings.TrimSpace(c) == code {
			return true
		}
	}
	return false
}

func (d SubdomainPageData) IsTargetDomainSelected(domain string) bool {
	if d.TargetDomain == "" { return false }
	for _, t := range strings.Split(d.TargetDomain, ",") {
		if strings.TrimSpace(t) == domain {
			return true
		}
	}
	return false
}

func (d SubdomainPageData) IsHasScreenshotSelected(val string) bool {
	if d.HasScreenshot == "" { return false }
	for _, v := range strings.Split(d.HasScreenshot, ",") {
		if strings.TrimSpace(v) == val {
			return true
		}
	}
	return false
}

func (d SubdomainPageData) IsHasTechSelected(val string) bool {
	if d.HasTech == "" { return false }
	for _, v := range strings.Split(d.HasTech, ",") {
		if strings.TrimSpace(v) == val {
			return true
		}
	}
	return false
}

func (d SubdomainPageData) ActiveFilterCount() int {
	count := 0
	if d.StatusCode != "" {
		for _, s := range strings.Split(d.StatusCode, ",") {
			if strings.TrimSpace(s) != "" { count++ }
		}
	}
	if d.TargetDomain != "" {
		for _, s := range strings.Split(d.TargetDomain, ",") {
			if strings.TrimSpace(s) != "" { count++ }
		}
	}
	if d.HasScreenshot != "" {
		for _, s := range strings.Split(d.HasScreenshot, ",") {
			if strings.TrimSpace(s) != "" { count++ }
		}
	}
	if d.HasTech != "" {
		for _, s := range strings.Split(d.HasTech, ",") {
			if strings.TrimSpace(s) != "" { count++ }
		}
	}
	return count
}

// handleSubdomainsPage renders the subdomains triage page.
func (s *Server) handleSubdomainsPage(w http.ResponseWriter, r *http.Request) {
	data := s.buildSubdomainPageData(r)
	data.ActivePage = "subdomains"
	s.renderTemplate(w, "subdomains.html", data)
}

// handleSubdomainsTablePartial renders the subdomains table or visual grid partial for HTMX search/filters.
func (s *Server) handleSubdomainsTablePartial(w http.ResponseWriter, r *http.Request) {
	data := s.buildSubdomainPageData(r)
	if data.ViewMode == "grid" {
		s.renderPartial(w, "gallery_grid.html", data)
	} else {
		s.renderPartial(w, "subdomains_table.html", data)
	}
}

// buildSubdomainPageData constructs the full page data with filters, sorting, and pagination.
func (s *Server) buildSubdomainPageData(r *http.Request) SubdomainPageData {
	viewMode := strings.TrimSpace(r.URL.Query().Get("view_mode"))
	if viewMode == "" {
		viewMode = "table"
	}

	defaultPerPage := 50
	if viewMode == "grid" {
		defaultPerPage = 6
	}

	data := SubdomainPageData{
		ViewMode:    viewMode,
		PerPage:     defaultPerPage,
		CurrentPage: 1,
		SortBy:      "info_desc", // Default: most info first
	}

	if s.db == nil {
		return data
	}

	// Parse query params
	data.Search = strings.TrimSpace(r.URL.Query().Get("search"))
	data.StatusCode = strings.TrimSpace(r.URL.Query().Get("status_code"))
	data.TargetDomain = strings.TrimSpace(r.URL.Query().Get("target_domain"))
	data.HasScreenshot = strings.TrimSpace(r.URL.Query().Get("has_screenshot"))
	data.HasTech = strings.TrimSpace(r.URL.Query().Get("has_tech"))
	data.SortBy = strings.TrimSpace(r.URL.Query().Get("sort_by"))
	if data.SortBy == "" {
		data.SortBy = "info_desc"
	}

	pageStr := strings.TrimSpace(r.URL.Query().Get("page"))
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			data.CurrentPage = p
		}
	}

	perPageStr := strings.TrimSpace(r.URL.Query().Get("per_page"))
	if perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 && pp <= 200 {
			data.PerPage = pp
		}
	}

	// Fetch available target domains for the filter dropdown
	var targets []models.Target
	s.db.Select("domain").Order("domain asc").Find(&targets)
	for _, t := range targets {
		data.TargetDomains = append(data.TargetDomains, t.Domain)
	}

	// Build query
	query := s.db.Model(&models.Subdomain{}).Preload("Target")

	// --- Filters (Multi-select support) ---
	if data.Search != "" {
		query = query.Where("host LIKE ?", "%"+data.Search+"%")
	}

	// Multi-select Status Code parsing
	statusCodes := r.URL.Query()["status_code"]
	if len(statusCodes) == 0 && r.URL.Query().Get("status_code") != "" {
		statusCodes = strings.Split(r.URL.Query().Get("status_code"), ",")
	}
	var cleanStatusCodes []string
	for _, sc := range statusCodes {
		sc = strings.TrimSpace(sc)
		if sc != "" {
			cleanStatusCodes = append(cleanStatusCodes, sc)
		}
	}
	data.StatusCode = strings.Join(cleanStatusCodes, ",")

	if len(cleanStatusCodes) > 0 {
		var statusConditions []string
		var statusArgs []interface{}

		for _, sc := range cleanStatusCodes {
			switch sc {
			case "2xx":
				statusConditions = append(statusConditions, "(status_code >= 200 AND status_code < 300)")
			case "3xx":
				statusConditions = append(statusConditions, "(status_code >= 300 AND status_code < 400)")
			case "4xx":
				statusConditions = append(statusConditions, "(status_code >= 400 AND status_code < 500)")
			case "5xx":
				statusConditions = append(statusConditions, "(status_code >= 500 AND status_code < 600)")
			case "none":
				statusConditions = append(statusConditions, "(status_code = 0 OR status_code IS NULL)")
			default:
				if code, err := strconv.Atoi(sc); err == nil {
					statusConditions = append(statusConditions, "status_code = ?")
					statusArgs = append(statusArgs, code)
				}
			}
		}

		if len(statusConditions) > 0 {
			query = query.Where("("+strings.Join(statusConditions, " OR ")+")", statusArgs...)
		}
	}

	// Multi-select Target Domain parsing
	targetDomains := r.URL.Query()["target_domain"]
	if len(targetDomains) == 0 && r.URL.Query().Get("target_domain") != "" {
		targetDomains = strings.Split(r.URL.Query().Get("target_domain"), ",")
	}
	var cleanTargetDomains []string
	for _, td := range targetDomains {
		td = strings.TrimSpace(td)
		if td != "" {
			cleanTargetDomains = append(cleanTargetDomains, td)
		}
	}
	data.TargetDomain = strings.Join(cleanTargetDomains, ",")

	if len(cleanTargetDomains) > 0 {
		query = query.Joins("JOIN targets ON targets.id = subdomains.target_id").
			Where("targets.domain IN (?)", cleanTargetDomains)
	}

	// Multi-select Has Screenshot
	screenshots := r.URL.Query()["has_screenshot"]
	if len(screenshots) == 0 && r.URL.Query().Get("has_screenshot") != "" {
		screenshots = strings.Split(r.URL.Query().Get("has_screenshot"), ",")
	}
	data.HasScreenshot = strings.Join(screenshots, ",")
	hasYes := false
	hasNo := false
	for _, s := range screenshots {
		if s == "yes" { hasYes = true }
		if s == "no" { hasNo = true }
	}
	if hasYes && !hasNo {
		query = query.Where("screenshot_path != '' AND screenshot_path IS NOT NULL")
	} else if hasNo && !hasYes {
		query = query.Where("screenshot_path = '' OR screenshot_path IS NULL")
	}

	// Multi-select Has Tech
	techs := r.URL.Query()["has_tech"]
	if len(techs) == 0 && r.URL.Query().Get("has_tech") != "" {
		techs = strings.Split(r.URL.Query().Get("has_tech"), ",")
	}
	data.HasTech = strings.Join(techs, ",")
	hasTechYes := false
	hasTechNo := false
	for _, t := range techs {
		if t == "yes" { hasTechYes = true }
		if t == "no" { hasTechNo = true }
	}
	if hasTechYes && !hasTechNo {
		query = query.Where("technologies != '' AND technologies != '[]' AND technologies IS NOT NULL")
	} else if hasTechNo && !hasTechYes {
		query = query.Where("technologies = '' OR technologies = '[]' OR technologies IS NULL")
	}

	// --- Count total (for pagination) ---
	query.Count(&data.TotalCount)

	data.TotalPages = int(math.Ceil(float64(data.TotalCount) / float64(data.PerPage)))
	if data.TotalPages < 1 {
		data.TotalPages = 1
	}
	if data.CurrentPage > data.TotalPages {
		data.CurrentPage = data.TotalPages
	}

	data.HasPrev = data.CurrentPage > 1
	data.HasNext = data.CurrentPage < data.TotalPages

	offset := (data.CurrentPage - 1) * data.PerPage

	// --- Sorting ---
	// Note: "info_desc" is a virtual score. We use a CASE expression to sort by info richness.
	// SQLite-compatible ORDER BY.
	switch data.SortBy {
	case "katana_desc":
		query = query.Joins("LEFT JOIN (SELECT target_id, COUNT(*) AS katana_cnt FROM findings WHERE tool_name = 'katana' GROUP BY target_id) k ON k.target_id = subdomains.target_id").
			Order("COALESCE(k.katana_cnt, 0) DESC, host ASC")
	case "alpha_asc":
		query = query.Order("host ASC")
	case "alpha_desc":
		query = query.Order("host DESC")
	case "status_asc":
		query = query.Order("status_code ASC, host ASC")
	case "status_desc":
		query = query.Order("status_code DESC, host ASC")
	case "newest":
		query = query.Order("id DESC")
	case "oldest":
		query = query.Order("id ASC")
	case "info_desc":
		// Sort by Tech Stack richness: count of items in technologies JSON array DESC,
		// then technologies string content ASC so identical/similar tech stacks group together.
		query = query.Order(`
			(CASE WHEN technologies IS NOT NULL AND technologies != '' AND technologies != '[]' 
			      THEN (LENGTH(technologies) - LENGTH(REPLACE(technologies, ',', '')) + 1) 
			      ELSE 0 END) DESC, 
			technologies ASC, 
			host ASC`)
	default:
		query = query.Order(`
			(CASE WHEN technologies IS NOT NULL AND technologies != '' AND technologies != '[]' 
			      THEN (LENGTH(technologies) - LENGTH(REPLACE(technologies, ',', '')) + 1) 
			      ELSE 0 END) DESC, 
			technologies ASC, 
			host ASC`)
	}

	// --- Fetch with pagination ---
	var subdomains []models.Subdomain
	query.Offset(offset).Limit(data.PerPage).Find(&subdomains)
	data.Subdomains = subdomains

	return data
}

// handleScreenshotModal renders screenshot preview modal partial for HTMX.
func (s *Server) handleScreenshotModal(w http.ResponseWriter, r *http.Request) {
	imgPath := r.URL.Query().Get("path")

	relPath := imgPath
	if idx := strings.Index(imgPath, "web/static/screenshots/"); idx != -1 {
		relPath = imgPath[idx+len("web/static/screenshots/"):]
	} else if idx := strings.Index(imgPath, "screenshots/"); idx != -1 {
		relPath = imgPath[idx+len("screenshots/"):]
	}
	relPath = strings.TrimPrefix(relPath, "/")

	data := struct {
		Filename string
		URLPath  string
	}{
		Filename: filepath.Base(imgPath),
		URLPath:  "/screenshots/" + relPath,
	}

	s.renderPartial(w, "screenshot_modal.html", data)
}

// handleFindingsPage renders the vulnerability findings page (Nuclei security findings).
func (s *Server) handleFindingsPage(w http.ResponseWriter, r *http.Request) {
	var findings []models.Finding
	if s.db != nil {
		s.db.Where("tool_name != ?", "katana").Preload("Target").Order("id desc").Limit(200).Find(&findings)
	}

	data := struct {
		ActivePage string
		Findings   []models.Finding
	}{
		ActivePage: "findings",
		Findings:   findings,
	}

	s.renderTemplate(w, "findings.html", data)
}

// handleFindingDetailModal renders detailed vulnerability finding modal for HTMX.
func (s *Server) handleFindingDetailModal(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid finding ID", http.StatusBadRequest)
		return
	}

	var finding models.Finding
	if err := s.db.Preload("Target").First(&finding, id).Error; err != nil {
		http.Error(w, "Finding not found", http.StatusNotFound)
		return
	}

	data := struct {
		Finding models.Finding
	}{
		Finding: finding,
	}

	s.renderPartial(w, "finding_modal.html", data)
}

// GalleryPageData holds data for the screenshot gallery view.
type GalleryPageData struct {
	ActivePage string
	Subdomains []models.Subdomain

	// Pagination (default 6 items per page)
	CurrentPage int
	TotalPages  int
	TotalCount  int64
	PerPage     int
	HasPrev     bool
	HasNext     bool

	// Filters
	Search       string
	StatusCode   string
	TargetDomain string

	// Targets for dropdown
	TargetDomains []string
}

// handleGalleryPage renders the screenshot gallery page.
func (s *Server) handleGalleryPage(w http.ResponseWriter, r *http.Request) {
	data := s.buildGalleryPageData(r)
	data.ActivePage = "gallery"
	s.renderTemplate(w, "gallery.html", data)
}

// handleGalleryGridPartial renders the gallery grid partial for HTMX updates.
func (s *Server) handleGalleryGridPartial(w http.ResponseWriter, r *http.Request) {
	data := s.buildGalleryPageData(r)
	s.renderPartial(w, "gallery_grid.html", data)
}

// buildGalleryPageData constructs page data for screenshots gallery (default 6 items per page).
func (s *Server) buildGalleryPageData(r *http.Request) GalleryPageData {
	const defaultPerPage = 6

	data := GalleryPageData{
		PerPage:     defaultPerPage,
		CurrentPage: 1,
	}

	if s.db == nil {
		return data
	}

	data.Search = strings.TrimSpace(r.URL.Query().Get("search"))
	data.StatusCode = strings.TrimSpace(r.URL.Query().Get("status_code"))
	data.TargetDomain = strings.TrimSpace(r.URL.Query().Get("target_domain"))

	pageStr := strings.TrimSpace(r.URL.Query().Get("page"))
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			data.CurrentPage = p
		}
	}

	perPageStr := strings.TrimSpace(r.URL.Query().Get("per_page"))
	if perPageStr != "" {
		if pp, err := strconv.Atoi(perPageStr); err == nil && pp > 0 && pp <= 48 {
			data.PerPage = pp
		}
	}

	// Target domains dropdown
	var targets []models.Target
	s.db.Select("domain").Order("domain asc").Find(&targets)
	for _, t := range targets {
		data.TargetDomains = append(data.TargetDomains, t.Domain)
	}

	// Filter subdomains MUST have screenshot
	query := s.db.Model(&models.Subdomain{}).Preload("Target").
		Where("screenshot_path != '' AND screenshot_path IS NOT NULL")

	if data.Search != "" {
		query = query.Where("host LIKE ?", "%"+data.Search+"%")
	}

	if data.StatusCode != "" {
		if code, err := strconv.Atoi(data.StatusCode); err == nil {
			query = query.Where("status_code = ?", code)
		}
	}

	if data.TargetDomain != "" {
		query = query.Joins("JOIN targets ON targets.id = subdomains.target_id").
			Where("targets.domain = ?", data.TargetDomain)
	}

	query.Count(&data.TotalCount)

	data.TotalPages = int(math.Ceil(float64(data.TotalCount) / float64(data.PerPage)))
	if data.TotalPages < 1 {
		data.TotalPages = 1
	}
	if data.CurrentPage > data.TotalPages {
		data.CurrentPage = data.TotalPages
	}

	data.HasPrev = data.CurrentPage > 1
	data.HasNext = data.CurrentPage < data.TotalPages

	offset := (data.CurrentPage - 1) * data.PerPage

	var subdomains []models.Subdomain
	query.Order("id desc").Offset(offset).Limit(data.PerPage).Find(&subdomains)
	data.Subdomains = subdomains

	return data
}

// handleAssetDetailModal renders detailed asset info modal popup when gallery card or subdomain host is clicked.
func (s *Server) handleAssetDetailModal(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid subdomain ID", http.StatusBadRequest)
		return
	}

	var subdomain models.Subdomain
	if s.db != nil {
		if err := s.db.Preload("Target").First(&subdomain, id).Error; err != nil {
			http.Error(w, "Subdomain asset not found", http.StatusNotFound)
			return
		}
	}

	var katanaEndpoints []string
	if s.db != nil && subdomain.TargetID > 0 {
		var katanaFindings []models.Finding
		// Query Katana findings matching the specific host or target_id
		s.db.Where("tool_name = ? AND target_id = ? AND (raw_output LIKE ? OR description LIKE ?)",
			"katana", subdomain.TargetID, "%"+subdomain.Host+"%", "%"+subdomain.Host+"%").
			Order("id desc").Limit(50).Find(&katanaFindings)

		// Fallback: If no host-specific match, fetch top Katana endpoints for target
		if len(katanaFindings) == 0 {
			s.db.Where("tool_name = ? AND target_id = ?", "katana", subdomain.TargetID).
				Order("id desc").Limit(25).Find(&katanaFindings)
		}

		for _, f := range katanaFindings {
			ep := f.RawOutput
			if ep == "" {
				ep = f.Description
			}
			if ep != "" {
				katanaEndpoints = append(katanaEndpoints, ep)
			}
		}
	}

	data := struct {
		Subdomain       models.Subdomain
		ScreenshotURL   string
		KatanaEndpoints []string
	}{
		Subdomain:       subdomain,
		ScreenshotURL:   subdomain.ScreenshotURL(),
		KatanaEndpoints: katanaEndpoints,
	}

	s.renderPartial(w, "asset_modal.html", data)
}

// handleCancelJob stops / cancels an active or pending scan job.
func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid job ID", http.StatusBadRequest)
		return
	}

	if s.workerPool != nil {
		s.workerPool.CancelJob(uint(id))
	} else if s.db != nil {
		now := time.Now()
		s.db.Model(&models.ScanJob{}).Where("id = ? AND status IN ('pending', 'running')", id).Updates(map[string]interface{}{
			"status":       "cancelled",
			"completed_at": &now,
		})
	}

	s.handleActiveJobsPartial(w, r)
}

// handleDeleteTarget deletes a target by ID or domain and cascades deletion to associated subdomains, findings, and scan jobs.
func (s *Server) handleDeleteTarget(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	domainStr := r.URL.Query().Get("domain")

	if s.db != nil {
		var target models.Target
		var err error

		if idStr != "" {
			var id int
			if id, err = strconv.Atoi(idStr); err == nil {
				err = s.db.First(&target, id).Error
			}
		} else if domainStr != "" {
			err = s.db.Where("domain = ?", domainStr).First(&target).Error
		} else {
			http.Error(w, "Target ID or Domain required", http.StatusBadRequest)
			return
		}

		if err == nil && target.ID > 0 {
			// Cancel active scan jobs for this target
			var activeJobs []models.ScanJob
			s.db.Where("target_id = ? AND status IN ('pending', 'running')", target.ID).Find(&activeJobs)
			for _, job := range activeJobs {
				if s.workerPool != nil {
					s.workerPool.CancelJob(job.ID)
				}
			}

			// Delete target (GORM constraint OnDelete:CASCADE handles subdomains, findings, scan_jobs)
			s.db.Select(clause.Associations).Delete(&target)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="p-3 rounded-xl bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 text-xs font-mono shadow-md flex items-center justify-between">
		<span class="flex items-center space-x-1.5"><i class="fa-solid fa-circle-check"></i><span>Target and associated assets deleted successfully.</span></span>
		<button onclick="this.parentElement.remove()" class="text-mist-400 hover:text-white ml-2">&times;</button>
	</div>`)
}
