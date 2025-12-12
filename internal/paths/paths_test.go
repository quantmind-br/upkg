package paths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
)

func TestNewResolver(t *testing.T) {
	cfg := &config.Config{}
	resolver := NewResolver(cfg)

	if resolver == nil {
		t.Fatal("NewResolver should not return nil")
	}

	homeDir, _ := os.UserHomeDir()
	if resolver.homeDir != homeDir {
		t.Errorf("NewResolver homeDir = %q, want %q", resolver.homeDir, homeDir)
	}
}

func TestNewResolverWithHome(t *testing.T) {
	cfg := &config.Config{}
	customHome := "/custom/home"
	resolver := NewResolverWithHome(cfg, customHome)

	if resolver == nil {
		t.Fatal("NewResolverWithHome should not return nil")
	}

	if resolver.homeDir != customHome {
		t.Errorf("NewResolverWithHome homeDir = %q, want %q", resolver.homeDir, customHome)
	}
}

func TestHomeDir(t *testing.T) {
	cfg := &config.Config{}
	customHome := "/test/home"
	resolver := NewResolverWithHome(cfg, customHome)

	result := resolver.HomeDir()
	if result != customHome {
		t.Errorf("HomeDir() = %q, want %q", result, customHome)
	}
}

func TestGetBinDir(t *testing.T) {
	cfg := &config.Config{}
	resolver := NewResolverWithHome(cfg, "/home/user")

	expected := filepath.Join("/home/user", ".local", "bin")
	result := resolver.GetBinDir()
	if result != expected {
		t.Errorf("GetBinDir() = %q, want %q", result, expected)
	}
}

func TestGetAppsDir(t *testing.T) {
	cfg := &config.Config{}
	resolver := NewResolverWithHome(cfg, "/home/user")

	expected := filepath.Join("/home/user", ".local", "share", "applications")
	result := resolver.GetAppsDir()
	if result != expected {
		t.Errorf("GetAppsDir() = %q, want %q", result, expected)
	}
}

func TestGetIconsDir(t *testing.T) {
	cfg := &config.Config{}
	resolver := NewResolverWithHome(cfg, "/home/user")

	expected := filepath.Join("/home/user", ".local", "share", "icons", "hicolor")
	result := resolver.GetIconsDir()
	if result != expected {
		t.Errorf("GetIconsDir() = %q, want %q", result, expected)
	}
}

func TestGetUpkgAppsDir(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected string
	}{
		{
			name: "Default config",
			cfg: &config.Config{
				Paths: config.PathsConfig{},
			},
			expected: filepath.Join("/home/user", ".local", "share", "upkg", "apps"),
		},
		{
			name: "Custom DataDir",
			cfg: &config.Config{
				Paths: config.PathsConfig{
					DataDir: "/custom/data",
				},
			},
			expected: filepath.Join("/custom/data", "apps"),
		},
		{
			name: "Empty DataDir",
			cfg: &config.Config{
				Paths: config.PathsConfig{
					DataDir: "",
				},
			},
			expected: filepath.Join("/home/user", ".local", "share", "upkg", "apps"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := NewResolverWithHome(tt.cfg, "/home/user")
			result := resolver.GetUpkgAppsDir()
			if result != tt.expected {
				t.Errorf("GetUpkgAppsDir() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetIconSizeDir(t *testing.T) {
	cfg := &config.Config{}
	resolver := NewResolverWithHome(cfg, "/home/user")

	tests := []struct {
		name     string
		size     string
		expected string
	}{
		{"256x256", "256x256", filepath.Join("/home/user", ".local", "share", "icons", "hicolor", "256x256", "apps")},
		{"scalable", "scalable", filepath.Join("/home/user", ".local", "share", "icons", "hicolor", "scalable", "apps")},
		{"48x48", "48x48", filepath.Join("/home/user", ".local", "share", "icons", "hicolor", "48x48", "apps")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.GetIconSizeDir(tt.size)
			if result != tt.expected {
				t.Errorf("GetIconSizeDir(%q) = %q, want %q", tt.size, result, tt.expected)
			}
		})
	}
}

func TestPathConsistency(t *testing.T) {
	cfg := &config.Config{}
	resolver := NewResolverWithHome(cfg, "/home/user")

	// Test that all paths are consistent with home directory
	binDir := resolver.GetBinDir()
	if !strings.HasPrefix(binDir, resolver.homeDir) {
		t.Errorf("GetBinDir() should be under home directory")
	}

	appsDir := resolver.GetAppsDir()
	if !strings.HasPrefix(appsDir, resolver.homeDir) {
		t.Errorf("GetAppsDir() should be under home directory")
	}

	iconsDir := resolver.GetIconsDir()
	if !strings.HasPrefix(iconsDir, resolver.homeDir) {
		t.Errorf("GetIconsDir() should be under home directory")
	}

	upkgAppsDir := resolver.GetUpkgAppsDir()
	if !strings.HasPrefix(upkgAppsDir, resolver.homeDir) {
		t.Errorf("GetUpkgAppsDir() should be under home directory (or custom DataDir)")
	}
}
