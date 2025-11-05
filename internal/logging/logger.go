package logging

import (
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/pkgerrors"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logger configuration
type Config struct {
	Level   string
	LogFile string
	NoColor bool
}

// NewLogger creates a new zerolog logger with dual output (console + file)
func NewLogger(cfg Config) *zerolog.Logger {
	// Enable stack trace marshaling
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// Determine log level
	level := parseLevel(cfg.Level)

	// Console writer (colored output for TTY)
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "15:04:05",
		NoColor:    cfg.NoColor,
	}

	var writers []io.Writer
	writers = append(writers, consoleWriter)

	// File logger if path provided
	if cfg.LogFile != "" {
		// Ensure directory exists
		dir := filepath.Dir(cfg.LogFile)
		if err := os.MkdirAll(dir, 0755); err == nil {
			fileWriter := &lumberjack.Logger{
				Filename:   cfg.LogFile,
				MaxSize:    10, // MB
				MaxBackups: 3,
				MaxAge:     28, // days
				Compress:   true,
			}
			writers = append(writers, fileWriter)
		}
	}

	// Create multi-writer
	multi := zerolog.MultiLevelWriter(writers...)

	// Create logger
	logger := zerolog.New(multi).
		Level(level).
		With().
		Timestamp().
		Logger()

	return &logger
}

// parseLevel converts string level to zerolog.Level
func parseLevel(level string) zerolog.Level {
	switch level {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// NewTestLogger creates a logger for testing that writes to a buffer
func NewTestLogger(w io.Writer) *zerolog.Logger {
	logger := zerolog.New(w).With().Timestamp().Logger()
	return &logger
}
