package logger

import (
	"log/slog"
	"os"
)

var (
	// Log is the global logger instance
	Log *slog.Logger
)

// Config holds logger configuration
type Config struct {
	Level   string // "debug", "info", "warn", "error"
	Service string // service name for identification
	Format  string // "json" or "text"
}

// Init initializes the global logger
func Init(cfg Config) {
	level := parseLevel(cfg.Level)
	
	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	Log = slog.New(handler).With(
		slog.String("service", cfg.Service),
	)
}

// parseLevel converts string level to slog.Level
func parseLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Info logs an info message with optional key-value pairs
func Info(msg string, args ...any) {
	if Log == nil {
		slog.Info(msg, args...)
		return
	}
	Log.Info(msg, args...)
}

// Error logs an error message with optional key-value pairs
func Error(msg string, args ...any) {
	if Log == nil {
		slog.Error(msg, args...)
		return
	}
	Log.Error(msg, args...)
}

// Warn logs a warning message with optional key-value pairs
func Warn(msg string, args ...any) {
	if Log == nil {
		slog.Warn(msg, args...)
		return
	}
	Log.Warn(msg, args...)
}

// Debug logs a debug message with optional key-value pairs
func Debug(msg string, args ...any) {
	if Log == nil {
		slog.Debug(msg, args...)
		return
	}
	Log.Debug(msg, args...)
}

// WithRequest returns a logger with request context
func WithRequest(requestID, userID, method, path string) *slog.Logger {
	if Log == nil {
		return slog.Default()
	}
	return Log.With(
		slog.String("request_id", requestID),
		slog.String("user_id", userID),
		slog.String("method", method),
		slog.String("path", path),
	)
}
