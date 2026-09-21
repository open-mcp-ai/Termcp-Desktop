// Package logging configures the structured, rotating logs shared by the
// desktop process and the local Core service.
package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	DefaultRetention = 14 * 24 * time.Hour
	DefaultMaxBytes  = int64(10 << 20)
)

var (
	secretValuePattern = regexp.MustCompile(`(?i)(password|passphrase|private[_-]?key|token|secret|authorization)(\s*[:=]\s*)([^\s,;]+)`)
	privateKeyPattern  = regexp.MustCompile(`(?s)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
)

type Config struct {
	DataDir   string
	Component string
	Level     slog.Leveler
	Retention time.Duration
	MaxBytes  int64
	Now       func() time.Time
	PID       int
	Stderr    io.Writer
}

// Runtime owns the file sink installed for the current process.
type Runtime struct {
	Logger *slog.Logger
	Path   string
	writer *rotatingWriter
}

// Configure installs a JSON slog logger and redirects the standard log package
// to the same sink. Debug is the default level so field reports contain enough
// information to reconstruct an interface call without logging request bodies.
func Configure(config Config) (*Runtime, error) {
	writer, err := newRotatingWriter(config)
	if err != nil {
		return nil, err
	}
	output := io.Writer(writer)
	if config.Stderr != nil {
		output = io.MultiWriter(writer, config.Stderr)
	}
	level := config.Level
	if level == nil {
		level = slog.LevelDebug
	}
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	})
	logger := slog.New(handler).With(
		"component", writer.component,
		"pid", writer.pid,
	)
	slog.SetDefault(logger)
	log.SetFlags(0)
	log.SetOutput(output)
	return &Runtime{Logger: logger, Path: writer.path, writer: writer}, nil
}

func (runtime *Runtime) Close() error {
	if runtime == nil || runtime.writer == nil {
		return nil
	}
	return runtime.writer.Close()
}

type rotatingWriter struct {
	mu        sync.Mutex
	directory string
	component string
	retention time.Duration
	maxBytes  int64
	now       func() time.Time
	pid       int
	date      string
	sequence  int
	file      *os.File
	size      int64
	path      string
}

func newRotatingWriter(config Config) (*rotatingWriter, error) {
	dataDir := strings.TrimSpace(config.DataDir)
	if dataDir == "" {
		return nil, fmt.Errorf("log data directory is empty")
	}
	component := safeComponent(config.Component)
	if component == "" {
		component = "termcp-desktop"
	}
	retention := config.Retention
	if retention <= 0 {
		retention = DefaultRetention
	}
	maxBytes := config.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	pid := config.PID
	if pid <= 0 {
		pid = os.Getpid()
	}
	directory := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	writer := &rotatingWriter{
		directory: directory,
		component: component,
		retention: retention,
		maxBytes:  maxBytes,
		now:       now,
		pid:       pid,
	}
	if err := writer.cleanup(now()); err != nil {
		return nil, err
	}
	if err := writer.rotate(now()); err != nil {
		return nil, err
	}
	return writer, nil
}

func (writer *rotatingWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	now := writer.now()
	date := now.Format("2006-01-02")
	if writer.file == nil || writer.date != date || writer.size+int64(len(data)) > writer.maxBytes {
		if writer.date != date {
			writer.sequence = 0
		} else {
			writer.sequence++
		}
		if err := writer.rotate(now); err != nil {
			return 0, err
		}
		if err := writer.cleanup(now); err != nil {
			return 0, err
		}
	}
	written, err := writer.file.Write(data)
	writer.size += int64(written)
	return written, err
}

func (writer *rotatingWriter) rotate(now time.Time) error {
	if writer.file != nil {
		if err := writer.file.Close(); err != nil {
			return err
		}
	}
	writer.date = now.Format("2006-01-02")
	suffix := ""
	if writer.sequence > 0 {
		suffix = fmt.Sprintf("-%02d", writer.sequence)
	}
	name := fmt.Sprintf("%s-%s-%d%s.log", writer.component, writer.date, writer.pid, suffix)
	writer.path = filepath.Join(writer.directory, name)
	file, err := os.OpenFile(writer.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	writer.file = file
	writer.size = info.Size()
	return nil
}

func (writer *rotatingWriter) cleanup(now time.Time) error {
	entries, err := os.ReadDir(writer.directory)
	if err != nil {
		return fmt.Errorf("read log directory: %w", err)
	}
	cutoff := now.Add(-writer.retention)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if removeErr := os.Remove(filepath.Join(writer.directory, entry.Name())); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("remove expired log %s: %w", entry.Name(), removeErr)
		}
	}
	return nil
}

func (writer *rotatingWriter) Close() error {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.file == nil {
		return nil
	}
	err := writer.file.Close()
	writer.file = nil
	return err
}

func safeComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.Join(strings.FieldsFunc(value, func(character rune) bool {
		return (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-'
	}), "-")
}

// WailsLogger adapts slog to Wails' internal logger interface.
type WailsLogger struct{ logger *slog.Logger }

func NewWailsLogger(logger *slog.Logger) *WailsLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return &WailsLogger{logger: logger.With("subsystem", "wails")}
}

func (logger *WailsLogger) Print(message string)   { logger.logger.Info(message) }
func (logger *WailsLogger) Trace(message string)   { logger.logger.Debug(message, "level", "trace") }
func (logger *WailsLogger) Debug(message string)   { logger.logger.Debug(message) }
func (logger *WailsLogger) Info(message string)    { logger.logger.Info(message) }
func (logger *WailsLogger) Warning(message string) { logger.logger.Warn(message) }
func (logger *WailsLogger) Error(message string)   { logger.logger.Error(message) }
func (logger *WailsLogger) Fatal(message string)   { logger.logger.Error(message, "fatal", true) }

// LogFrontend records browser-side failures without accepting arbitrary
// structured fields that could accidentally contain credentials.
func LogFrontend(level, message, details string) {
	message = truncate(Redact(strings.TrimSpace(message)), 4096)
	details = truncate(Redact(strings.TrimSpace(details)), 4096)
	attributes := []any{"source", "frontend"}
	if details != "" {
		attributes = append(attributes, "details", details)
	}
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		slog.Error(message, attributes...)
	case "warn", "warning":
		slog.Warn(message, attributes...)
	case "info":
		slog.Info(message, attributes...)
	default:
		slog.Debug(message, attributes...)
	}
}

func ErrorText(err error) string {
	if err == nil {
		return ""
	}
	return Redact(err.Error())
}

func Redact(value string) string {
	value = privateKeyPattern.ReplaceAllString(value, "[REDACTED PRIVATE KEY]")
	return secretValuePattern.ReplaceAllString(value, "$1$2[REDACTED]")
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}

// Enabled is kept small so callers can avoid assembling expensive debug data.
func Enabled(level slog.Level) bool {
	return slog.Default().Enabled(context.Background(), level)
}
