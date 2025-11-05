package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Test loading config (will use defaults if file doesn't exist)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg == nil {
		t.Fatal("expected config, got nil")
	}

	// Verify defaults are set
	if cfg.Logging.Level == "" {
		t.Error("expected default log level, got empty")
	}

	if cfg.Paths.DataDir == "" {
		t.Error("expected default data_dir, got empty")
	}
}

func TestExpandPath(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty path",
			input: "",
			want:  "",
		},
		{
			name:  "absolute path",
			input: "/usr/local/bin",
			want:  "/usr/local/bin",
		},
		{
			name:  "home expansion",
			input: "~/test",
			want:  filepath.Join(homeDir, "test"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandPath(tt.input)
			if got != tt.want {
				t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetDefaults(t *testing.T) {
	setDefaults()

	// Verify defaults were set (via viper)
	// This is tested indirectly through Load()
}
