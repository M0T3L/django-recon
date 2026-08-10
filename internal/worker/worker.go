package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"django/internal/logger"
	"django/internal/models"
	"django/internal/notifier"
	"django/internal/pipeline"
)

// Job represents a unit of work submitted to the worker pool.
type Job struct {
	JobID        uint
	TargetID     uint
	Domain       string
	PipelineType string
	Context      context.Context
}

// Pool represents the Worker Pool task queue manager.
type Pool struct {
	db               *gorm.DB
	pipelineRegistry *pipeline.Registry
	notifier         *notifier.Notifier
	jobQueue         chan Job
	runningJobs      map[uint]context.CancelFunc
	mu               sync.Mutex
	workerCount      int
	wg               sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
}

// NewPool initializes a new Worker Pool with specified worker concurrency and queue buffer capacity.
func NewPool(db *gorm.DB, registry *pipeline.Registry, workerCount int, queueCapacity int) *Pool {
	if workerCount <= 0 {
		workerCount = 3
	}
	if queueCapacity <= 0 {
		queueCapacity = 100
	}
	if registry == nil {
		registry = pipeline.NewRegistry()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Pool{
		db:               db,
		pipelineRegistry: registry,
		runningJobs:      make(map[uint]context.CancelFunc),
		jobQueue:         make(chan Job, queueCapacity),
		workerCount:      workerCount,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// SetNotifier attaches an event Notifier to the Worker Pool.
func (p *Pool) SetNotifier(n *notifier.Notifier) {
	p.notifier = n
}

// Start launches the worker goroutines.
func (p *Pool) Start() {
	for i := 1; i <= p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	logger.Info("Worker", fmt.Sprintf("Worker Pool started with %d workers", p.workerCount))
}

// Enqueue submits a new scan job to the queue and records/updates ScanJob status in DB.
func (p *Pool) Enqueue(job Job) error {
	if p.ctx.Err() != nil {
		return errors.New("worker pool is stopped")
	}

	// Verify target exists if DB is present
	if p.db != nil && job.TargetID > 0 {
		var target models.Target
		if err := p.db.First(&target, job.TargetID).Error; err != nil {
			return fmt.Errorf("target ID %d not found in database: %w", job.TargetID, err)
		}
		if job.Domain == "" {
			job.Domain = target.Domain
		}
	}

	// Initialize or retrieve ScanJob record
	if p.db != nil {
		if job.JobID == 0 {
			scanJob := models.ScanJob{
				TargetID:     job.TargetID,
				PipelineType: job.PipelineType,
				Status:       models.JobStatusPending,
			}
			if err := p.db.Create(&scanJob).Error; err != nil {
				return fmt.Errorf("failed to create ScanJob DB record: %w", err)
			}
			job.JobID = scanJob.ID
		} else {
			p.db.Model(&models.ScanJob{}).Where("id = ?", job.JobID).Updates(map[string]interface{}{
				"status":        models.JobStatusPending,
				"pipeline_type": job.PipelineType,
			})
		}
	}

	select {
	case p.jobQueue <- job:
		logger.Info("Worker", fmt.Sprintf("Job #%d queued (Strategy: %s)", job.JobID, job.PipelineType), logger.JobID(job.JobID), logger.Target(job.Domain))
		return nil
	case <-p.ctx.Done():
		return errors.New("worker pool context cancelled")
	}
}

// worker is the main consumer loop executed by each worker goroutine.
func (p *Pool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobQueue:
			if !ok {
				return
			}
			p.processJob(id, job)
		}
	}
}

// CancelJob cancels a running or pending scan job by ID.
func (p *Pool) CancelJob(jobID uint) error {
	p.mu.Lock()
	if cancel, exists := p.runningJobs[jobID]; exists {
		cancel()
		delete(p.runningJobs, jobID)
	}
	p.mu.Unlock()

	now := time.Now()
	if p.db != nil && jobID > 0 {
		p.db.Model(&models.ScanJob{}).Where("id = ? AND status IN (?, ?)", jobID, models.JobStatusPending, models.JobStatusRunning).Updates(map[string]interface{}{
			"status":       models.JobStatusCancelled,
			"completed_at": &now,
		})
	}
	return nil
}

// processJob executes the selected pipeline strategy and manages DB ScanJob status transitions.
func (p *Pool) processJob(workerID int, job Job) {
	logger.Info("Worker", fmt.Sprintf("[Worker %d] Processing Job #%d", workerID, job.JobID), logger.JobID(job.JobID), logger.Target(job.Domain))

	now := time.Now()

	// Check if already cancelled
	if p.db != nil && job.JobID > 0 {
		var currentJob models.ScanJob
		if err := p.db.First(&currentJob, job.JobID).Error; err == nil && currentJob.Status == models.JobStatusCancelled {
			logger.Info("Worker", fmt.Sprintf("[Worker %d] Job #%d was cancelled before execution", workerID, job.JobID))
			return
		}
	}

	// Create job-specific context with cancellation
	parentCtx := p.ctx
	if job.Context != nil {
		parentCtx = job.Context
	}
	jobCtx, cancelJob := context.WithCancel(parentCtx)

	p.mu.Lock()
	p.runningJobs[job.JobID] = cancelJob
	p.mu.Unlock()

	defer func() {
		cancelJob()
		p.mu.Lock()
		delete(p.runningJobs, job.JobID)
		p.mu.Unlock()
	}()

	// Update DB status to 'running'
	if p.db != nil && job.JobID > 0 {
		p.db.Model(&models.ScanJob{}).Where("id = ?", job.JobID).Updates(map[string]interface{}{
			"status":     models.JobStatusRunning,
			"started_at": &now,
		})
	}

	if p.notifier != nil {
		p.notifier.NotifyJobStarted(job.JobID, job.Domain, job.PipelineType)
	}

	pipelineStrategy, exists := p.pipelineRegistry.Get(job.PipelineType)
	if !exists {
		// Fallback to default pipeline_1 if unspecified or invalid
		pipelineStrategy, _ = p.pipelineRegistry.Get("pipeline_1")
	}

	// Progress step callback logger
	stepCallback := func(stepName string, stepIndex int, totalSteps int) {
		logger.Info("Worker", fmt.Sprintf("[Worker %d] Job #%d step %d/%d: %s", workerID, job.JobID, stepIndex, totalSteps, stepName), logger.JobID(job.JobID), logger.Target(job.Domain))
	}

	delta, err := pipelineStrategy.ExecuteContextWithNotifier(jobCtx, job.Domain, job.TargetID, p.db, p.notifier, stepCallback)

	completedAt := time.Now()
	duration := completedAt.Sub(now)

	if err != nil {
		finalStatus := models.JobStatusFailed
		if errors.Is(err, context.Canceled) || jobCtx.Err() != nil {
			finalStatus = models.JobStatusCancelled
		}
		logger.Error("Worker", fmt.Sprintf("[Worker %d] Job #%d %s", workerID, job.JobID, finalStatus), logger.JobID(job.JobID), logger.Target(job.Domain), logger.Err(err))
		if p.db != nil && job.JobID > 0 {
			p.db.Model(&models.ScanJob{}).Where("id = ?", job.JobID).Updates(map[string]interface{}{
				"status":       finalStatus,
				"completed_at": &completedAt,
			})
		}
		if p.notifier != nil && finalStatus != "cancelled" {
			p.notifier.NotifyJobFailed(job.JobID, job.Domain, duration, err.Error())
		}
		return
	}

	logger.Info("Worker", fmt.Sprintf("[Worker %d] Job #%d completed successfully in %v (Delta: +%d new subs, +%d new findings)", workerID, job.JobID, duration, delta.NewSubdomains, delta.NewFindings), logger.JobID(job.JobID), logger.Target(job.Domain))
	if p.db != nil && job.JobID > 0 {
		p.db.Model(&models.ScanJob{}).Where("id = ?", job.JobID).Updates(map[string]interface{}{
			"status":       models.JobStatusCompleted,
			"completed_at": &completedAt,
		})
	}

	if p.notifier != nil {
		var liveSubdomains int64
		var totalFindings int64
		if p.db != nil && job.TargetID > 0 {
			p.db.Model(&models.Subdomain{}).Where("target_id = ?", job.TargetID).Count(&liveSubdomains)
			p.db.Model(&models.Finding{}).Where("target_id = ?", job.TargetID).Count(&totalFindings)
		}
		p.notifier.NotifyJobCompleted(job.JobID, job.Domain, duration, int(liveSubdomains), int(totalFindings), delta.NewSubdomains, delta.NewFindings)
	}
}

// Stop gracefully shuts down the worker pool and waits for queued jobs to complete.
func (p *Pool) Stop() {
	p.cancel()
	close(p.jobQueue)
	p.wg.Wait()
	logger.Info("Worker", "Worker Pool gracefully stopped")
}
