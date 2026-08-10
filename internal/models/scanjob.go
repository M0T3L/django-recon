package models

import (
	"time"
)

// ScanJob status constants for state machine transitions.
const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
	JobStatusCancelled = "cancelled"
)

// ScanJob represents an execution unit of a recon/vulnerability scan for a Target.
type ScanJob struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	TargetID     uint       `gorm:"index;not null" json:"target_id"`
	PipelineType string     `gorm:"size:100;default:'pipeline_1'" json:"pipeline_type"`
	Status       string     `gorm:"size:50;default:'pending';index;not null" json:"status"` // pending, running, completed, failed
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime" json:"updated_at"`

	// Relational Target reference
	Target *Target `gorm:"foreignKey:TargetID" json:"target,omitempty"`
}
