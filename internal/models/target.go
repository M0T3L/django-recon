package models

import (
	"time"
)

// Target represents a primary domain/target for recon.
type Target struct {
	ID        uint        `gorm:"primaryKey;autoIncrement" json:"id"`
	Domain    string      `gorm:"size:255;uniqueIndex;not null" json:"domain"`
	Status    string      `gorm:"size:50;default:'active';index" json:"status"`
	CreatedAt time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relationships
	Subdomains []Subdomain `gorm:"foreignKey:TargetID;constraint:OnDelete:CASCADE" json:"subdomains,omitempty"`
	Findings   []Finding   `gorm:"foreignKey:TargetID;constraint:OnDelete:CASCADE" json:"findings,omitempty"`
	ScanJobs   []ScanJob   `gorm:"foreignKey:TargetID;constraint:OnDelete:CASCADE" json:"scan_jobs,omitempty"`
}
