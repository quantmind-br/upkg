package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestNewLogger(t *testing.T) {
	t.Run("creates logger with console writer", func(t *testing.T) {
		cfg := Config{
			Level:   "info",
			NoColor: true,
		}

		logger := NewLogger(cfg)
		assert.NotNil(t, logger)
	})

	t.Run("creates logger with file writer", func(t *testing.T) {
		tmpDir := t.TempDir()
		logFile := filepath.Join(tmpDir, "test.log")

		cfg := Config{
			Level:   "info",
			LogFile: logFile,
			NoColor: true,
		}

		logger := NewLogger(cfg)
		assert.NotNil(t, logger)

		// Log something
		logger.Info().Msg("test")

		// Verify file was created
		_, err := os.Stat(logFile)
		assert.NoError(t, err)
	})

	t.Run("with NO_COLOR environment", func(t *testing.T) {
		os.Setenv("NO_COLOR", "1")
		defer os.Unsetenv("NO_COLOR")

		cfg := Config{
			Level:   "info",
			NoColor: false,
		}

		logger := NewLogger(cfg)
		assert.NotNil(t, logger)
	})

	t.Run("with TERM=dumb", func(t *testing.T) {
		os.Setenv("TERM", "dumb")
		defer os.Unsetenv("TERM")

		cfg := Config{
			Level:   "info",
			NoColor: false,
		}

		logger := NewLogger(cfg)
		assert.NotNil(t, logger)
	})
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
		{"invalid", "info"}, // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level := parseLevel(tt.input)
			if level.String() != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, level, tt.want)
			}
		})
	}
}

func TestLoggerOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := NewTestLogger(&buf)

	logger.Info().Str("test", "value").Msg("test message")

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected log output to contain 'test message', got: %s", output)
	}
	if !strings.Contains(output, "test") {
		t.Errorf("expected log output to contain 'test' field, got: %s", output)
	}
}

func TestProgressSafeWriter(t *testing.T) {
	t.Run("write normal line", func(t *testing.T) {
		var buf bytes.Buffer
		writer := newProgressSafeWriter(&buf)

		n, err := writer.Write([]byte("line1\n"))
		assert.NoError(t, err)
		assert.Equal(t, 6, n)
		assert.Contains(t, buf.String(), "line1")
	})

	t.Run("write partial line", func(t *testing.T) {
		var buf bytes.Buffer
		writer := newProgressSafeWriter(&buf)

		writer.Write([]byte("partial"))
		assert.Contains(t, buf.String(), "partial")
	})

	t.Run("write multiple lines", func(t *testing.T) {
		var buf bytes.Buffer
		writer := newProgressSafeWriter(&buf)

		writer.Write([]byte("line1\n"))
		writer.Write([]byte("line2\n"))

		output := buf.String()
		assert.Contains(t, output, "line1")
		assert.Contains(t, output, "line2")
	})

	t.Run("concurrent writes", func(_ *testing.T) {
		var buf bytes.Buffer
		writer := newProgressSafeWriter(&buf)

		done := make(chan bool)
		for range 10 {
			go func() {
				writer.Write([]byte("concurrent\n"))
				done <- true
			}()
		}

		for range 10 {
			<-done
		}
	})
}

func TestLoggerLevels(t *testing.T) {
	var buf bytes.Buffer

	// Create logger with buffer
	zerologLogger := zerolog.New(&buf).Level(zerolog.DebugLevel)

	// Test different log levels
	zerologLogger.Trace().Msg("trace message")
	zerologLogger.Debug().Msg("debug message")
	zerologLogger.Info().Msg("info message")
	zerologLogger.Warn().Msg("warn message")
	zerologLogger.Error().Msg("error message")

	output := buf.String()
	assert.Contains(t, output, "debug message")
	assert.Contains(t, output, "info message")
	assert.Contains(t, output, "warn message")
	assert.Contains(t, output, "error message")
}
