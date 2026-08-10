package models

import (
	"strings"
	"time"
)

// Subdomain represents a discovered subdomain for a given Target.
type Subdomain struct {
	ID             uint        `gorm:"primaryKey;autoIncrement" json:"id"`
	TargetID       uint        `gorm:"uniqueIndex:idx_target_host;not null" json:"target_id"`
	Host           string      `gorm:"size:255;uniqueIndex:idx_target_host;not null" json:"host"`
	IP             string      `gorm:"size:100;index" json:"ip"`
	StatusCode     int         `gorm:"index" json:"status_code"`
	Title          string      `gorm:"size:512" json:"title"`
	Technologies   StringArray `gorm:"type:text" json:"technologies"`
	ContentLength  int64       `json:"content_length"`
	ScreenshotPath string      `gorm:"size:512" json:"screenshot_path"`
	CreatedAt      time.Time   `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time   `gorm:"autoUpdateTime" json:"updated_at"`

	// Relational Target reference (optional preload)
	Target *Target `gorm:"foreignKey:TargetID" json:"target,omitempty"`
}

// ScreenshotURL returns the web-accessible relative URL path for the screenshot image.
func (s Subdomain) ScreenshotURL() string {
	if s.ScreenshotPath == "" {
		return ""
	}
	imgPath := s.ScreenshotPath
	relPath := imgPath
	if idx := strings.Index(imgPath, "web/static/screenshots/"); idx != -1 {
		relPath = imgPath[idx+len("web/static/screenshots/"):]
	} else if idx := strings.Index(imgPath, "screenshots/"); idx != -1 {
		relPath = imgPath[idx+len("screenshots/"):]
	}
	relPath = strings.TrimPrefix(relPath, "/")
	return "/screenshots/" + relPath
}
