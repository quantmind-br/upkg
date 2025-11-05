package security

import (
	"testing"
)

func TestValidatePackageName(t *testing.T) {
	tests := []struct {
		name    string
		pkgName string
		wantErr bool
	}{
		{
			name:    "valid simple name",
			pkgName: "appname",
			wantErr: false,
		},
		{
			name:    "valid with dashes",
			pkgName: "my-app-name",
			wantErr: false,
		},
		{
			name:    "valid with underscores",
			pkgName: "my_app_name",
			wantErr: false,
		},
		{
			name:    "valid with dots",
			pkgName: "app.name-1.0",
			wantErr: false,
		},
		{
			name:    "empty name",
			pkgName: "",
			wantErr: true,
		},
		{
			name:    "name with spaces",
			pkgName: "app name",
			wantErr: true,
		},
		{
			name:    "name with path traversal",
			pkgName: "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "name with absolute path",
			pkgName: "/usr/bin/app",
			wantErr: true,
		},
		{
			name:    "name with tilde",
			pkgName: "~/.ssh/id_rsa",
			wantErr: true,
		},
		{
			name:    "null byte injection",
			pkgName: "app\x00bad",
			wantErr: true,
		},
		{
			name:    "very long name",
			pkgName: string(make([]byte, 300)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePackageName(tt.pkgName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePackageName() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name      string
		version   string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid simple version",
			version: "1.0.0",
			wantErr: false,
		},
		{
			name:    "valid with prerelease",
			version: "1.0.0-beta",
			wantErr: false,
		},
		{
			name:    "valid semantic version",
			version: "2.1.3-alpha.1",
			wantErr: false,
		},
		{
			name:      "empty version",
			version:   "",
			wantErr:   true,
			errSubstr: "invalid version",
		},
		{
			name:      "version with path traversal",
			version:   "../../etc/passwd",
			wantErr:   true,
			errSubstr: "dangerous pattern",
		},
		{
			name:      "version with null byte",
			version:   "1.0\x00bad",
			wantErr:   true,
			errSubstr: "null byte",
		},
		{
			name:      "version too long",
			version:   string(make([]byte, 100)),
			wantErr:   true,
			errSubstr: "too long",
		},
		{
			name:      "version with script injection",
			version:   "1.0; rm -rf /",
			wantErr:   true,
			errSubstr: "dangerous pattern",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errSubstr != "" {
				if !contains(err.Error(), tt.errSubstr) {
					t.Errorf("ValidateVersion() error = %v, expected to contain %v", err.Error(), tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid simple path",
			path:    "app.bin",
			wantErr: false,
		},
		{
			name:    "valid path with subdirectory",
			path:    "bin/app",
			wantErr: false,
		},
		{
			name:    "valid absolute path",
			path:    "/usr/bin/app",
			wantErr: false,
		},
		{
			name:      "path traversal attempt",
			path:      "../../../etc/passwd",
			wantErr:   true,
			errSubstr: "dangerous pattern",
		},
		{
			name:      "absolute path to sensitive system",
			path:      "/etc/passwd",
			wantErr:   true,
			errSubstr: "sensitive system path",
		},
		{
			name:      "null byte injection",
			path:      "app\x00/etc/passwd",
			wantErr:   true,
			errSubstr: "null byte",
		},
		{
			name:      "path too long",
			path:      string(make([]byte, 1000)),
			wantErr:   true,
			errSubstr: "too long",
		},
		{
			name:      "shell injection",
			path:      "app; rm -rf /",
			wantErr:   true,
			errSubstr: "dangerous pattern",
		},
		{
			name:      "hidden directory attempt",
			path:      ".ssh/id_rsa",
			wantErr:   true,
			errSubstr: "hidden file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFilePath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errSubstr != "" {
				if !contains(err.Error(), tt.errSubstr) {
					t.Errorf("ValidateFilePath() error = %v, expected to contain %v", err.Error(), tt.errSubstr)
				}
			}
		})
	}
}

func TestIsPathWithinDirectory(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		directory    string
		wantResult   bool
		wantErr      bool
	}{
		{
			name:       "file within directory",
			path:       "/home/user/app/file.txt",
			directory:  "/home/user/app",
			wantResult: true,
		},
		{
			name:       "file is the directory itself",
			path:       "/home/user/app",
			directory:  "/home/user/app",
			wantResult: true,
		},
		{
			name:       "file outside directory",
			path:       "/home/user/other/file.txt",
			directory:  "/home/user/app",
			wantResult: false,
		},
		{
			name:       "path traversal attempt",
			path:       "/home/user/app/../other/file.txt",
			directory:  "/home/user/app",
			wantResult: false,
		},
		{
			name:       "relative path within",
			path:       "subdir/file.txt",
			directory:  "/home/user/app",
			wantResult: false, // relative paths not supported
			wantErr:    true,
		},
		{
			name:       "sibling directory",
			path:       "/home/user/other",
			directory:  "/home/user/app",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := IsPathWithinDirectory(tt.path, tt.directory)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsPathWithinDirectory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && result != tt.wantResult {
				t.Errorf("IsPathWithinDirectory() = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean string",
			input:    "appname",
			expected: "appname",
		},
		{
			name:     "string with spaces",
			input:    "app name",
			expected: "app-name",
		},
		{
			name:     "string with multiple spaces",
			input:    "app   name  test",
			expected: "app-name-test",
		},
		{
			name:     "string with special chars",
			input:    "app@#$%name",
			expected: "app-name",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "string with leading/trailing spaces",
			input:    "  appname  ",
			expected: "appname",
		},
		{
			name:     "string with underscores preserved",
			input:    "app_name_test",
			expected: "app_name_test",
		},
		{
			name:     "string with dots preserved",
			input:    "app.name.test",
			expected: "app.name.test",
		},
		{
			name:     "string with dashes preserved",
			input:    "app-name-test",
			expected: "app-name-test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeString(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeString() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}