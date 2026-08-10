package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"django/internal/api"
	"django/internal/db"
	"django/internal/models"
	"django/internal/notifier"
	"django/internal/pipeline"
	"django/internal/worker"
	"gorm.io/gorm"
)

func setupTestServer(t *testing.T) (*api.Server, *gorm.DB) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_api.db")

	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize test db: %v", err)
	}

	registry := pipeline.NewRegistry()
	pool := worker.NewPool(database, registry, 1, 10)
	notif := notifier.NewNotifier("", "")

	server, err := api.NewServer(database, pool, notif, registry)
	if err != nil {
		t.Fatalf("failed to initialize API server: %v", err)
	}

	return server, database
}

func TestServer_DashboardPage(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "DJANGO") || !strings.Contains(body, "Total Targets") {
		t.Errorf("expected dashboard HTML response, got: %s", body)
	}
}

func TestServer_SubdomainsPage(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/subdomains", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Subdomains & Visual Triage") {
		t.Errorf("expected subdomains HTML response, got: %s", body)
	}
}

func TestServer_FindingsPage(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/findings", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Security Findings") {
		t.Errorf("expected findings HTML response, got: %s", body)
	}
}

func TestServer_CreateTargetsHTMX(t *testing.T) {
	server, _ := setupTestServer(t)

	form := url.Values{}
	form.Set("domains", "example.com\ntest.org")
	form.Set("pipeline_type", "pipeline_1")

	req := httptest.NewRequest(http.MethodPost, "/api/targets", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "enqueued") || !strings.Contains(body, "pipeline_1") {
		t.Errorf("expected toast alert response, got: %s", body)
	}
}

func TestServer_ActiveJobsPartial(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/active", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "active-jobs-table") {
		t.Errorf("expected active jobs partial HTML, got: %s", body)
	}
}

func TestServer_ScreenshotModal(t *testing.T) {
	server, _ := setupTestServer(t)

	imgAbsPath := "/home/motel/Documents/django/web/static/screenshots/screenshot/example.com/hash123.png"
	req := httptest.NewRequest(http.MethodGet, "/api/subdomains/modal?path="+url.QueryEscape(imgAbsPath), nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	expectedURL := "/screenshots/screenshot/example.com/hash123.png"
	if !strings.Contains(body, expectedURL) {
		t.Errorf("expected modal to contain image URL %s, got: %s", expectedURL, body)
	}
}

func TestServer_KatanaSortingFast(t *testing.T) {
	server, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/subdomains?sort_by=katana_desc", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}

func TestServer_MultiSelectFilters(t *testing.T) {
	server, _ := setupTestServer(t)

	// Test multi-select status codes (200 and 403)
	req := httptest.NewRequest(http.MethodGet, "/api/subdomains?status_code=200&status_code=403&has_screenshot=yes", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}
}

func TestServer_FindingDetailModal(t *testing.T) {
	server, db := setupTestServer(t)

	target := models.Target{Domain: "vuln.test"}
	db.Create(&target)
	finding := models.Finding{
		TargetID:    target.ID,
		ToolName:    "nuclei",
		Severity:    "high",
		Title:       "SQL Injection Vulnerability",
		Description: "Detailed SQLi exploit description",
		RawOutput:   "[sql-injection] http://vuln.test/api?id=1' UNION SELECT",
	}
	db.Create(&finding)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/findings/detail?id=%d", finding.ID), nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "SQL Injection Vulnerability") || !strings.Contains(body, "HIGH SEVERITY") {
		t.Errorf("expected modal to contain finding title and severity, got: %s", body)
	}
}

func TestServer_CancelJob(t *testing.T) {
	server, db := setupTestServer(t)

	target := models.Target{Domain: "cancel.test"}
	db.Create(&target)
	job := models.ScanJob{TargetID: target.ID, Status: "running", PipelineType: "pipeline_1"}
	db.Create(&job)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/jobs/cancel?id=%d", job.ID), nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	var updatedJob models.ScanJob
	db.First(&updatedJob, job.ID)
	if updatedJob.Status != "cancelled" {
		t.Errorf("expected job status 'cancelled', got '%s'", updatedJob.Status)
	}
}

func TestServer_DeleteTarget(t *testing.T) {
	server, db := setupTestServer(t)

	target := models.Target{Domain: "delete-me.com"}
	db.Create(&target)
	sub := models.Subdomain{TargetID: target.ID, Host: "sub.delete-me.com"}
	db.Create(&sub)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/targets?id=%d", target.ID), nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	var count int64
	db.Model(&models.Target{}).Where("id = ?", target.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected target to be deleted from DB, but count was %d", count)
	}
}
