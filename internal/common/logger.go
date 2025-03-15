package common

import (
	"context"
	"io"
	"log/slog"
	"os"
	"time"
)

// LogLevel represents the logging level
type LogLevel string

// Log levels
const (
	DebugLevel LogLevel = "debug"
	InfoLevel  LogLevel = "info"
	WarnLevel  LogLevel = "warn"
	ErrorLevel LogLevel = "error"
)

// LoggerConfig holds configuration for the logger
type LoggerConfig struct {
	// Level is the minimum level of logs to output
	Level LogLevel
	// Output is where logs are written (defaults to os.Stdout)
	Output io.Writer
	// TimeFormat is the format for timestamps (defaults to RFC3339)
	TimeFormat string
	// IncludeSource adds source code location to logs
	IncludeSource bool
}

// DefaultLoggerConfig returns the default logger configuration
func DefaultLoggerConfig() LoggerConfig {
	return LoggerConfig{
		Level:         InfoLevel,
		Output:        os.Stdout,
		TimeFormat:    time.RFC3339,
		IncludeSource: true,
	}
}

// loggerKeyType is used as context key type
type loggerKeyType struct{}

// loggerKey is the context key for logger
var loggerKey = loggerKeyType{}

// ContextWithLogger adds logger to context
func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// LoggerFromContext gets logger from context
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// NewLogger creates a new structured logger
func NewLogger(config LoggerConfig) *slog.Logger {
	var level slog.Level
	switch config.Level {
	case DebugLevel:
		level = slog.LevelDebug
	case WarnLevel:
		level = slog.LevelWarn
	case ErrorLevel:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	if config.Output == nil {
		config.Output = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: config.IncludeSource,
	}

	handler := slog.NewJSONHandler(config.Output, opts)
	return slog.New(handler)
}
