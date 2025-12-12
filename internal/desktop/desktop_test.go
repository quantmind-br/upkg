package desktop

import (
	"os"
	"strings"
	"testing"

	"github.com/quantmind-br/upkg/internal/core"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantEntry *core.DesktopEntry
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid desktop entry",
			input: `[Desktop Entry]
Type=Application
Name=Firefox
Exec=firefox %u
Icon=firefox
Comment=Web Browser
Categories=Network;WebBrowser;
Terminal=false
StartupWMClass=firefox`,
			wantEntry: &core.DesktopEntry{
				Type:           "Application",
				Name:           "Firefox",
				Exec:           "firefox %u",
				Icon:           "firefox",
				Comment:        "Web Browser",
				Categories:     []string{"Network", "WebBrowser"},
				Terminal:       false,
				StartupWMClass: "firefox",
			},
			wantErr: false,
		},
		{
			name: "minimal desktop entry",
			input: `[Desktop Entry]
Type=Application
Name=Test
Exec=test`,
			wantEntry: &core.DesktopEntry{
				Type: "Application",
				Name: "Test",
				Exec: "test",
			},
			wantErr: false,
		},
		{
			name:      "empty desktop entry",
			input:     ``,
			wantEntry: &core.DesktopEntry{},
			wantErr:   false,
		},
		{
			name: "desktop entry with comments",
			input: `# This is a comment
[Desktop Entry]
# Another comment
Type=Application
Name=Test
Exec=test
# Final comment`,
			wantEntry: &core.DesktopEntry{
				Type: "Application",
				Name: "Test",
				Exec: "test",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := Parse(strings.NewReader(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errSubstr != "" {
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Parse() error = %v, expected to contain %v", err.Error(), tt.errSubstr)
				}
				return
			}
			if !tt.wantErr {
				if entry.Type != tt.wantEntry.Type {
					t.Errorf("Parse() Type = %v, want %v", entry.Type, tt.wantEntry.Type)
				}
				if entry.Name != tt.wantEntry.Name {
					t.Errorf("Parse() Name = %v, want %v", entry.Name, tt.wantEntry.Name)
				}
				if entry.Exec != tt.wantEntry.Exec {
					t.Errorf("Parse() Exec = %v, want %v", entry.Exec, tt.wantEntry.Exec)
				}
				if entry.Icon != tt.wantEntry.Icon {
					t.Errorf("Parse() Icon = %v, want %v", entry.Icon, tt.wantEntry.Icon)
				}
				if entry.Comment != tt.wantEntry.Comment {
					t.Errorf("Parse() Comment = %v, want %v", entry.Comment, tt.wantEntry.Comment)
				}
				if !compareStringSlices(entry.Categories, tt.wantEntry.Categories) {
					t.Errorf("Parse() Categories = %v, want %v", entry.Categories, tt.wantEntry.Categories)
				}
				if entry.Terminal != tt.wantEntry.Terminal {
					t.Errorf("Parse() Terminal = %v, want %v", entry.Terminal, tt.wantEntry.Terminal)
				}
				if entry.StartupWMClass != tt.wantEntry.StartupWMClass {
					t.Errorf("Parse() StartupWMClass = %v, want %v", entry.StartupWMClass, tt.wantEntry.StartupWMClass)
				}
			}
		})
	}
}

func TestWrite(t *testing.T) {
	tests := []struct {
		name      string
		entry     *core.DesktopEntry
		wantErr   bool
		errSubstr string
	}{
		{
			name: "complete desktop entry",
			entry: &core.DesktopEntry{
				Type:           "Application",
				Name:           "Firefox",
				Exec:           "firefox %u",
				Icon:           "firefox",
				Comment:        "Web Browser",
				Categories:     []string{"Network", "WebBrowser"},
				Terminal:       false,
				StartupWMClass: "firefox",
			},
			wantErr: false,
		},
		{
			name: "minimal desktop entry",
			entry: &core.DesktopEntry{
				Type: "Application",
				Name: "Test",
				Exec: "test",
			},
			wantErr: false,
		},
		{
			name:    "empty desktop entry",
			entry:   &core.DesktopEntry{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf strings.Builder
			err := Write(&buf, tt.entry)
			if (err != nil) != tt.wantErr {
				t.Errorf("Write() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errSubstr != "" {
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Write() error = %v, expected to contain %v", err.Error(), tt.errSubstr)
				}
				return
			}
			if !tt.wantErr {
				// Verify that the output is valid by parsing it back
				parsedEntry, err := Parse(strings.NewReader(buf.String()))
				if err != nil {
					t.Errorf("Write() produced invalid output: %v", err)
					return
				}
				// Compare key fields
				if parsedEntry.Type != tt.entry.Type {
					t.Errorf("Write() Type mismatch: got %v, want %v", parsedEntry.Type, tt.entry.Type)
				}
				if parsedEntry.Name != tt.entry.Name {
					t.Errorf("Write() Name mismatch: got %v, want %v", parsedEntry.Name, tt.entry.Name)
				}
				if parsedEntry.Exec != tt.entry.Exec {
					t.Errorf("Write() Exec mismatch: got %v, want %v", parsedEntry.Exec, tt.entry.Exec)
				}
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		entry     *core.DesktopEntry
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid desktop entry",
			entry: &core.DesktopEntry{
				Type: "Application",
				Name: "Firefox",
				Exec: "firefox",
			},
			wantErr: false,
		},
		{
			name: "missing Type",
			entry: &core.DesktopEntry{
				Name: "Firefox",
				Exec: "firefox",
			},
			wantErr:   true,
			errSubstr: "type field is required",
		},
		{
			name: "missing Name",
			entry: &core.DesktopEntry{
				Type: "Application",
				Exec: "firefox",
			},
			wantErr:   true,
			errSubstr: "name field is required",
		},
		{
			name: "missing Exec",
			entry: &core.DesktopEntry{
				Type: "Application",
				Name: "Firefox",
			},
			wantErr:   true,
			errSubstr: "exec field is required",
		},
		{
			name:    "empty desktop entry",
			entry:   &core.DesktopEntry{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.entry)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errSubstr != "" {
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("Validate() error = %v, expected to contain %v", err.Error(), tt.errSubstr)
				}
			}
		})
	}
}

func TestInjectWaylandEnvVars(t *testing.T) {
	tests := []struct {
		name       string
		entry      *core.DesktopEntry
		customVars []string
		wantErr    bool
		errSubstr  string
		checkExec  func(string) bool
	}{
		{
			name:       "default injection",
			entry:      &core.DesktopEntry{Exec: "myapp"},
			customVars: nil,
			wantErr:    false,
			checkExec: func(exec string) bool {
				return strings.HasPrefix(exec, "env ") &&
					strings.Contains(exec, "GDK_BACKEND=") &&
					strings.Contains(exec, "QT_QPA_PLATFORM=")
			},
		},
		{
			name:       "with custom valid vars",
			entry:      &core.DesktopEntry{Exec: "myapp"},
			customVars: []string{"CUSTOM_VAR=value", "ANOTHER=test"},
			wantErr:    false,
			checkExec: func(exec string) bool {
				return strings.Contains(exec, "CUSTOM_VAR=") &&
					strings.Contains(exec, "ANOTHER=")
			},
		},
		{
			name:       "with invalid custom vars",
			entry:      &core.DesktopEntry{Exec: "myapp"},
			customVars: []string{"INVALID", "ANOTHER=test"},
			wantErr:    true,
			errSubstr:  "invalid custom env vars",
		},
		{
			name:       "already has env prefix",
			entry:      &core.DesktopEntry{Exec: "env VAR=val myapp"},
			customVars: nil,
			wantErr:    false,
			checkExec: func(exec string) bool {
				// Should not duplicate env prefix
				return strings.HasPrefix(exec, "env ") &&
					!strings.HasPrefix(exec, "env env ")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InjectWaylandEnvVars(tt.entry, tt.customVars)
			if (err != nil) != tt.wantErr {
				t.Errorf("InjectWaylandEnvVars() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errSubstr != "" {
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("InjectWaylandEnvVars() error = %v, expected to contain %v", err.Error(), tt.errSubstr)
				}
				return
			}
			if tt.checkExec != nil && !tt.checkExec(tt.entry.Exec) {
				t.Errorf("InjectWaylandEnvVars() Exec = %v, check failed", tt.entry.Exec)
			}
		})
	}
}

func TestWriteDesktopFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		entry     *core.DesktopEntry
		wantErr   bool
		errSubstr string
	}{
		{
			name: "valid desktop entry",
			entry: &core.DesktopEntry{
				Type: "Application",
				Name: "TestApp",
				Exec: "testapp",
			},
			wantErr: false,
		},
		{
			name: "invalid desktop entry (missing required fields)",
			entry: &core.DesktopEntry{
				Name: "TestApp",
			},
			wantErr:   true,
			errSubstr: "invalid desktop entry",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := tmpDir + "/test.desktop"
			err := WriteDesktopFile(filePath, tt.entry)
			if (err != nil) != tt.wantErr {
				t.Errorf("WriteDesktopFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errSubstr != "" {
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("WriteDesktopFile() error = %v, expected to contain %v", err.Error(), tt.errSubstr)
				}
				return
			}
			if !tt.wantErr {
				// Verify file was created and can be read back
				file, err := os.Open(filePath)
				if err != nil {
					t.Errorf("WriteDesktopFile() failed to create file: %v", err)
					return
				}
				defer file.Close()

				parsedEntry, err := Parse(file)
				if err != nil {
					t.Errorf("WriteDesktopFile() created invalid file: %v", err)
					return
				}

				// Verify key fields match
				if parsedEntry.Type != tt.entry.Type {
					t.Errorf("WriteDesktopFile() Type mismatch: got %v, want %v", parsedEntry.Type, tt.entry.Type)
				}
				if parsedEntry.Name != tt.entry.Name {
					t.Errorf("WriteDesktopFile() Name mismatch: got %v, want %v", parsedEntry.Name, tt.entry.Name)
				}
				if parsedEntry.Exec != tt.entry.Exec {
					t.Errorf("WriteDesktopFile() Exec mismatch: got %v, want %v", parsedEntry.Exec, tt.entry.Exec)
				}
			}
		})
	}
}

func TestParseSemicolonList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "normal list",
			input:    "Network;WebBrowser;",
			expected: []string{"Network", "WebBrowser"},
		},
		{
			name:     "list with spaces",
			input:    "Network; WebBrowser; ",
			expected: []string{"Network", "WebBrowser"},
		},
		{
			name:     "empty list",
			input:    ";",
			expected: nil,
		},
		{
			name:     "single item",
			input:    "Network;",
			expected: []string{"Network"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSemicolonList(tt.input)
			if !compareStringSlices(got, tt.expected) {
				t.Errorf("parseSemicolonList() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEscapeExecToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple token",
			input:    "VAR=value",
			expected: "VAR=value",
		},
		{
			name:     "token with spaces",
			input:    "VAR=value with spaces",
			expected: `VAR="value with spaces"`,
		},
		{
			name:     "token with quotes",
			input:    `VAR="quoted"`,
			expected: `VAR="\"quoted\""`,
		},
		{
			name:     "token with backslash",
			input:    `VAR=value\with\backslash`,
			expected: `VAR=value\\with\\backslash`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeExecToken(tt.input)
			if got != tt.expected {
				t.Errorf("escapeExecToken() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// Helper functions
func compareStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
