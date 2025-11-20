package helpers

import (
	"testing"
)

func TestCleanAppName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"firefox-128.0", "firefox"},
		{"firefox-128.0-linux-x86_64", "firefox"},
		{"my-app-v1.0.0", "my-app"},
		{"discord-0.0.1", "discord"},
		{"visual-studio-code", "visual-studio-code"},
		{"Obsidian-1.4.13", "Obsidian"},
		{"app-x86_64.AppImage", "app-x86_64.AppImage"}, // CleanAppName assumes extensions stripped
		{"app-x86_64", "app"},
		{"app-linux-amd64", "app"},
		{"my-cool-app-beta-1", "my-cool-app"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := CleanAppName(tt.input)
			if got != tt.expected {
				t.Errorf("CleanAppName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"git-butler-nightly", "Git Butler Nightly"},
		{"cursor", "Cursor"},
		{"firefox-esr", "Firefox ESR"},
		{"visual-studio-code", "Visual Studio Code"},
		{"gimp", "Gimp"}, // GIMP is not in the hardcoded list, so Title Case
		{"api-client", "API Client"},
		{"my-xml-parser", "My XML Parser"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := FormatDisplayName(tt.input)
			if got != tt.expected {
				t.Errorf("FormatDisplayName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
