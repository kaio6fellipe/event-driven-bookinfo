// Package logging provides structured JSON logging utilities and HTTP middleware.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type contextKey struct{}

// New creates a JSON slog.Logger with the given level and service name.
// Output is written to os.Stdout.
func New(level string, serviceName string) *slog.Logger {
	return NewWithWriter(level, serviceName, os.Stdout)
}

// NewWithWriter creates a JSON slog.Logger that writes to the given writer.
// This is useful for testing where output needs to be captured.
func NewWithWriter(level string, serviceName string, w io.Writer) *slog.Logger {
	lvl := parseLevel(level)

	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: lvl,
	})

	return slog.New(handler).With(
		slog.String("service", serviceName),
	)
}

// FromContext retrieves the request-scoped logger from the context.
// If no logger is found, a default logger is returned.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// WithContext stores a logger in the context.
func WithContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
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
