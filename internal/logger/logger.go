package logger

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	defaultLogger *slog.Logger
	logFile       *os.File
	once          sync.Once
)

// Config holds configuration parameters for the logger.
type Config struct {
	LogDir   string
	FileName string
}

// Init initializes the structured file logger and multi-writer output (os.Stdout + logs/app.log).
func Init(cfg ...Config) (*slog.Logger, error) {
	var err error
	once.Do(func() {
		dir := "logs"
		file := "app.log"

		if len(cfg) > 0 {
			if cfg[0].LogDir != "" {
				dir = cfg[0].LogDir
			}
			if cfg[0].FileName != "" {
				file = cfg[0].FileName
			}
		}

		// Ensure logs/ directory exists automatically
		if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
			err = fmt.Errorf("failed to create log directory '%s': %w", dir, mkErr)
			return
		}

		logPath := filepath.Join(dir, file)
		f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if openErr != nil {
			err = fmt.Errorf("failed to open log file '%s': %w", logPath, openErr)
			return
		}
		logFile = f

		// MultiWriter outputs to terminal (os.Stdout) and logs/app.log concurrently
		mw := io.MultiWriter(os.Stdout, logFile)

		// Set default standard library log output to MultiWriter
		log.SetOutput(mw)
		log.SetFlags(log.Ldate | log.Ltime)

		// Create slog handler
		opts := &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}
		handler := slog.NewTextHandler(mw, opts)
		defaultLogger = slog.New(handler)
		slog.SetDefault(defaultLogger)
	})

	if err != nil {
		return nil, err
	}
	return defaultLogger, nil
}

// Get returns the default initialized slog.Logger instance.
func Get() *slog.Logger {
	if defaultLogger == nil {
		l, err := Init()
		if err != nil {
			return slog.Default()
		}
		return l
	}
	return defaultLogger
}

// Info logs an INFO level message with module context.
func Info(module string, msg string, args ...any) {
	allArgs := append([]any{slog.String("module", module)}, args...)
	Get().Info(msg, allArgs...)
}

// Warn logs a WARN level message with module context.
func Warn(module string, msg string, args ...any) {
	allArgs := append([]any{slog.String("module", module)}, args...)
	Get().Warn(msg, allArgs...)
}

// Error logs an ERROR level message with module context.
func Error(module string, msg string, args ...any) {
	allArgs := append([]any{slog.String("module", module)}, args...)
	Get().Error(msg, allArgs...)
}

// Helper attr constructors for clean structured context attributes

func JobID(id uint) slog.Attr {
	return slog.Uint64("job_id", uint64(id))
}

func Target(t string) slog.Attr {
	return slog.String("target", t)
}

func Tool(name string) slog.Attr {
	return slog.String("tool", name)
}

func Err(err error) slog.Attr {
	if err == nil {
		return slog.String("error", "")
	}
	return slog.String("error", err.Error())
}

func Raw(data string) slog.Attr {
	// Truncate raw data if overly long to prevent log spam
	if len(data) > 300 {
		data = data[:300] + "...[truncated]"
	}
	return slog.String("raw_data", data)
}

// Close flushes and closes the underlying log file handle.
func Close() {
	if logFile != nil {
		_ = logFile.Sync()
		_ = logFile.Close()
	}
}
