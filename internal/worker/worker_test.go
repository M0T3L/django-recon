package worker_test

import (
	"path/filepath"
	"testing"
	"time"

	"django/internal/db"
	"django/internal/models"
	"django/internal/notifier"
	"django/internal/pipeline"
	"django/internal/worker"
)

func TestWorkerPool_ExecutionAndDBStatusTransition(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_worker.db")

	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize test db: %v", err)
	}

	target := models.Target{
		Domain: "target.test",
	}
	if err := database.Create(&target).Error; err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	registry := pipeline.NewRegistry()
	pool := worker.NewPool(database, registry, 2, 10)
	pool.Start()
	defer pool.Stop()

	job := worker.Job{
		TargetID:     target.ID,
		Domain:       target.Domain,
		PipelineType: "pipeline_1",
	}

	if err := pool.Enqueue(job); err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}

	// Poll DB for ScanJob status transition to 'completed'
	var scanJob models.ScanJob
	success := false
	for i := 0; i < 30; i++ {
		time.Sleep(100 * time.Millisecond)
		err := database.First(&scanJob, "target_id = ?", target.ID).Error
		if err == nil && scanJob.Status == "completed" {
			success = true
			break
		}
	}

	if !success {
		t.Fatalf("job status did not transition to 'completed', current status: %s", scanJob.Status)
	}

	if scanJob.StartedAt == nil || scanJob.CompletedAt == nil {
		t.Errorf("expected StartedAt and CompletedAt timestamps to be set")
	}
}

func TestWorkerPool_WithNotifier(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_worker_notif.db")

	database, err := db.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to initialize test db: %v", err)
	}

	target := models.Target{
		Domain: "notif.test",
	}
	if err := database.Create(&target).Error; err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	registry := pipeline.NewRegistry()
	pool := worker.NewPool(database, registry, 1, 10)

	// Create disabled notifier (or empty token/chatID)
	notif := notifier.NewNotifier("", "")
	pool.SetNotifier(notif)

	pool.Start()
	defer pool.Stop()

	job := worker.Job{
		TargetID:     target.ID,
		Domain:       target.Domain,
		PipelineType: "pipeline_1",
	}

	if err := pool.Enqueue(job); err != nil {
		t.Fatalf("failed to enqueue job: %v", err)
	}

	time.Sleep(500 * time.Millisecond)
}
