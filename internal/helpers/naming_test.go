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

func TestGenerateNameVariants(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{
			input:    "firefox-128.0-linux-x86_64",
			expected: []string{"firefox-128.0-linux-x86_64", "firefox-128.0-linux", "firefox-128.0", "firefox", "firefox128.0linuxx86_64", "firefox128.0linux", "firefox128.0"},
		},
		{
			input:    "my-app-v1.0.0",
			expected: []string{"my-app-v1.0.0", "my-app", "myappv1.0.0", "myapp"},
		},
		{
			input:    "simple",
			expected: []string{"simple"},
		},
		{
			input:    "app-beta-1",
			expected: []string{"app-beta-1", "app-beta", "app", "appbeta1", "appbeta"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := GenerateNameVariants(tt.input)
			// Compare as sets since order might vary
			if len(got) != len(tt.expected) {
				t.Errorf("GenerateNameVariants(%q) = %v, want %v (length mismatch)", tt.input, got, tt.expected)
				return
			}
			gotSet := make(map[string]bool)
			for _, v := range got {
				gotSet[v] = true
			}
			for _, v := range tt.expected {
				if !gotSet[v] {
					t.Errorf("GenerateNameVariants(%q) = %v, missing expected variant %q", tt.input, got, v)
				}
			}
		})
	}
}

func TestIsSuffixToken(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"1.0.0", true},
		{"v1.0", true},
		{"x86_64", true},
		{"linux", true},
		{"beta", true},
		{"app", false},
		{"firefox", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isSuffixToken(tt.input)
			if got != tt.expected {
				t.Errorf("isSuffixToken(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsVersionToken(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"1.0.0", true},
		{"v1.0", true},
		{"1", true},
		{"v1", true},
		{"app", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isVersionToken(tt.input)
			if got != tt.expected {
				t.Errorf("isVersionToken(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsArchToken(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"x86_64", true},
		{"amd64", true},
		{"arm64", true},
		{"aarch64", true},
		{"app", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isArchToken(tt.input)
			if got != tt.expected {
				t.Errorf("isArchToken(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsPlatformToken(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"linux", true},
		{"win", true},
		{"mac", true},
		{"appimage", true},
		{"app", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isPlatformToken(tt.input)
			if got != tt.expected {
				t.Errorf("isPlatformToken(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIsReleaseToken(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"beta", true},
		{"alpha", true},
		{"rc", true},
		{"nightly", true},
		{"app", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isReleaseToken(tt.input)
			if got != tt.expected {
				t.Errorf("isReleaseToken(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
