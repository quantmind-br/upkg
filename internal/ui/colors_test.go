package ui

import (
	"bytes"
	"testing"

	"github.com/fatih/color"
)

func TestInitColors(_ *testing.T) {
	// Save original state
	originalNoColor := color.NoColor
	defer func() {
		color.NoColor = originalNoColor
	}()

	// Test that InitColors doesn't crash
	// Note: We can't easily test environment variable behavior
	// without more complex setup, so we just test that it runs
	InitColors()
}

func TestPrintFunctions(t *testing.T) {
	// Enable colors for testing
	color.NoColor = false

	// Test Sprint functions (these don't require output redirection)
	successStr := SprintSuccess("Test success")
	if !bytes.Contains([]byte(successStr), []byte("✓")) {
		t.Error("SprintSuccess should contain checkmark")
	}

	errorStr := SprintError("Test error")
	if !bytes.Contains([]byte(errorStr), []byte("✗")) {
		t.Error("SprintError should contain crossmark")
	}

	warningStr := SprintWarning("Test warning")
	if !bytes.Contains([]byte(warningStr), []byte("Warning:")) {
		t.Error("SprintWarning should contain 'Warning:'")
	}

	infoStr := SprintInfo("Test info")
	if !bytes.Contains([]byte(infoStr), []byte("→")) {
		t.Error("SprintInfo should contain arrow")
	}

	// Test ColorizePackageType
	// Save original state
	originalNoColor := color.NoColor
	color.NoColor = true
	defer func() {
		color.NoColor = originalNoColor
	}()

	tests := []struct {
		name     string
		pkgType  string
		expected string
	}{
		{"AppImage", "appimage", "appimage"},
		{"Binary", "binary", "binary"},
		{"Tarball", "tarball", "tarball"},
		{"DEB", "deb", "deb"},
		{"RPM", "rpm", "rpm"},
		{"Unknown", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ColorizePackageType(tt.pkgType)
			if result != tt.expected {
				t.Errorf("ColorizePackageType(%q) = %q, want %q", tt.pkgType, result, tt.expected)
			}
		})
	}
}

func TestColorManagement(t *testing.T) {
	// Save original state
	originalNoColor := color.NoColor
	defer func() {
		color.NoColor = originalNoColor
	}()

	// Test DisableColors
	color.NoColor = false
	DisableColors()
	if !color.NoColor {
		t.Error("DisableColors() should disable colors")
	}

	// Test EnableColors
	EnableColors()
	if color.NoColor {
		t.Error("EnableColors() should enable colors")
	}

	// Test AreColorsEnabled
	color.NoColor = false
	if !AreColorsEnabled() {
		t.Error("AreColorsEnabled() should return true when colors are enabled")
	}

	color.NoColor = true
	if AreColorsEnabled() {
		t.Error("AreColorsEnabled() should return false when colors are disabled")
	}
}
