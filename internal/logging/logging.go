package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// Logger wraps slog.Logger to provide structured JSON logging.
type Logger struct {
	*slog.Logger
}

var (
	defaultLogger *Logger
	defaultMu     sync.RWMutex
)

// New creates a Logger. level is one of "debug","info","warn","error".
// format is "json" or "text". file is a path to a log file; empty string means stdout.
func New(level string, format string, file string) (*Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	var w io.Writer = os.Stdout
	if file != "" {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		w = f
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json", "":
		handler = slog.NewJSONHandler(w, opts)
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("unsupported log format: %s", format)
	}

	return &Logger{Logger: slog.New(handler)}, nil
}

// Init initialises the package-level default logger.
func Init(level, format, file string) error {
	l, err := New(level, format, file)
	if err != nil {
		return err
	}
	defaultMu.Lock()
	defaultLogger = l
	defaultMu.Unlock()
	return nil
}

// Default returns the package-level logger, initialising it to a JSON info
// logger on stdout if Init has not been called.
func Default() *Logger {
	defaultMu.RLock()
	l := defaultLogger
	defaultMu.RUnlock()
	if l != nil {
		return l
	}
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultLogger == nil {
		defaultLogger = &Logger{
			Logger: slog.New(slog.NewJSONHandler(os.Stdout, nil)),
		}
	}
	return defaultLogger
}

// WithComponent returns a child logger with the "component" field set.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{Logger: l.Logger.With("component", component)}
}

// WithFields returns a child logger with additional key-value fields.
func (l *Logger) WithFields(fields ...any) *Logger {
	return &Logger{Logger: l.Logger.With(fields...)}
}

// Info logs at info level.
func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, args...)
}

// Warn logs at warn level.
func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, args...)
}

// Error logs at error level.
func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, args...)
}

// Debug logs at debug level.
func (l *Logger) Debug(msg string, args ...any) {
	l.Logger.Debug(msg, args...)
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unsupported log level: %s", s)
	}
}
