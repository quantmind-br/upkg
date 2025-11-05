package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "info level logger",
			config: Config{
				Level:   "info",
				NoColor: true,
			},
			wantErr: false,
		},
		{
			name: "debug level logger",
			config: Config{
				Level:   "debug",
				NoColor: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.config)
			if logger == nil {
				t.Fatal("expected logger, got nil")
			}
		})
	}
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
