package models

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// Finding represents a security finding or vulnerability associated with a Target.
type Finding struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TargetID    uint      `gorm:"index;not null" json:"target_id"`
	Fingerprint string    `gorm:"size:64;index;default:'';not null" json:"fingerprint"`
	ToolName    string    `gorm:"size:100;index;not null" json:"tool_name"`
	Severity    string    `gorm:"size:50;index;not null" json:"severity"` // e.g. info, low, medium, high, critical
	Title       string    `gorm:"size:255;not null" json:"title"`
	Description string    `gorm:"type:text" json:"description"`
	RawOutput   string    `gorm:"type:text" json:"raw_output"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`

	// Relational Target reference
	Target *Target `gorm:"foreignKey:TargetID" json:"target,omitempty"`
}

// ComputeFingerprint generates a unique SHA-256 hash for finding deduplication.
func (f *Finding) ComputeFingerprint() string {
	raw := f.RawOutput
	if raw == "" {
		raw = f.Description
	}
	data := fmt.Sprintf("%d|%s|%s|%s", f.TargetID, f.ToolName, f.Title, raw)
	hash := sha256.Sum256([]byte(data))
	f.Fingerprint = fmt.Sprintf("%x", hash)
	return f.Fingerprint
}
