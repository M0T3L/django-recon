package db

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	applogger "django/internal/logger"
	"django/internal/models"
)

// InitDB connects to the SQLite database, configures pragmas, and triggers auto-migration.
func InitDB(dbPath string) (*gorm.DB, error) {
	if dbPath == "" {
		dbPath = "recon.db"
	}

	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory for DB path: %w", err)
		}
	}

	// SQLite connection options: WAL mode for higher concurrency, 5s busy timeout, enabled foreign keys
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON", dbPath)

	gormLogLevel := logger.Warn
	if os.Getenv("APP_ENV") == "development" {
		gormLogLevel = logger.Info
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database at %s: %w", dbPath, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve underlying sql.DB: %w", err)
	}

	// Optimize pool for SQLite
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(time.Hour)

	applogger.Info("DB", fmt.Sprintf("Connected to SQLite database at: %s", dbPath))

	// Execute database auto-migration
	if err := AutoMigrate(db); err != nil {
		return nil, fmt.Errorf("database auto-migration failed: %w", err)
	}

	return db, nil
}

// AutoMigrate migrates all GORM data models into the database tables.
func AutoMigrate(db *gorm.DB) error {
	applogger.Info("DB", "Executing GORM auto-migrations...")

	err := db.AutoMigrate(
		&models.Target{},
		&models.Subdomain{},
		&models.Finding{},
		&models.ScanJob{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto-migrate database models: %w", err)
	}

	// Backfill missing fingerprints for existing findings records if needed (instant batch update)
	_ = db.Exec("UPDATE findings SET fingerprint = printf('%d_%d', target_id, id) WHERE fingerprint = '' OR fingerprint IS NULL;").Error

	// Clean up duplicate subdomains & findings before creating unique composite indexes
	_ = db.Exec("DELETE FROM subdomains WHERE id NOT IN (SELECT MAX(id) FROM subdomains GROUP BY target_id, host);").Error
	_ = db.Exec("DELETE FROM findings WHERE id NOT IN (SELECT MAX(id) FROM findings GROUP BY target_id, fingerprint);").Error

	// Ensure composite unique indexes exist for SQLite ON CONFLICT clauses
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_target_host ON subdomains(target_id, host);").Error; err != nil {
		applogger.Warn("DB", "Warning: Failed to ensure composite index idx_target_host", applogger.Err(err))
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_target_finding_fp ON findings(target_id, fingerprint);").Error; err != nil {
		applogger.Warn("DB", "Warning: Failed to ensure composite index idx_target_finding_fp", applogger.Err(err))
	}

	applogger.Info("DB", "Database auto-migration completed successfully.")
	return nil
}
