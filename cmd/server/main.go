package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"django/internal/api"
	"django/internal/config"
	"django/internal/db"
	"django/internal/logger"
	"django/internal/notifier"
	"django/internal/pipeline"
	"django/internal/worker"
)

func main() {
	// 0. Initialize File-based and Structured Logger (logs/app.log + terminal)
	if _, err := logger.Init(); err != nil {
		log.Fatalf("Fatal: Logger initialization failed: %v", err)
	}
	defer logger.Close()

	logger.Info("App", "Starting Django Recon Application backend & Web Dashboard...")

	// 1. Load configuration (.env / environment variables)
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("App", "Fatal: Configuration loading failed", logger.Err(err))
		log.Fatalf("Fatal: Configuration loading failed: %v", err)
	}

	logger.Info("App", fmt.Sprintf("Configuration loaded successfully [Env: %s, Port: %s, DB: %s]", cfg.AppEnv, cfg.Port, cfg.DBPath))

	// 2. Initialize SQLite Database & Auto-Migrate models
	database, err := db.InitDB(cfg.DBPath)
	if err != nil {
		logger.Error("App", "Fatal: Database initialization failed", logger.Err(err))
		log.Fatalf("Fatal: Database initialization failed: %v", err)
	}

	// 3. Initialize Telegram Notifier
	notif := notifier.NewNotifierFromConfig(cfg)
	notif.Start()

	// 4. Initialize Pipeline Strategy Registry
	registry := pipeline.NewRegistry()

	// 5. Initialize Worker Pool
	workerPool := worker.NewPool(database, registry, 3, 100)
	workerPool.SetNotifier(notif)
	workerPool.Start()

	// 6. Initialize Web Server and Handlers
	handler, err := api.NewServer(database, workerPool, notif, registry)
	if err != nil {
		logger.Error("App", "Fatal: Web Dashboard initialization failed", logger.Err(err))
		log.Fatalf("Fatal: Web Dashboard initialization failed: %v", err)
	}

	if cfg.BasicAuthUser != "" && cfg.BasicAuthPass != "" {
		handler.SetBasicAuth(cfg.BasicAuthUser, cfg.BasicAuthPass)
		logger.Info("App", fmt.Sprintf("HTTP Basic Authentication enabled for user '%s'", cfg.BasicAuthUser))
	}

	addr := fmt.Sprintf(":%s", cfg.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB max header
	}

	// Channel to signal server shutdown completion
	serverErrors := make(chan error, 1)

	go func() {
		logger.Info("App", fmt.Sprintf("⚡ Django Recon Dashboard is live on http://localhost%s", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Signal handling for Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		logger.Error("App", "Fatal: HTTP server crashed", logger.Err(err))
	case sig := <-quit:
		logger.Info("App", fmt.Sprintf("Shutdown signal received (%v), starting graceful shutdown...", sig))

		// 1. Stop receiving new HTTP requests (10s deadline)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("App", "HTTP server forced shutdown error", logger.Err(err))
		}

		// 2. Stop Worker Pool (cancels running jobs and waits for workers)
		logger.Info("App", "Stopping Worker Pool...")
		workerPool.Stop()

		// 3. Stop Notifier (drains notification queue)
		logger.Info("App", "Stopping Notifier...")
		notif.Stop()

		logger.Info("App", "Application shutdown complete.")
	}
}
