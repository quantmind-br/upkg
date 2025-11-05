package fsops

import (
	"testing"

	"github.com/spf13/afero"
)

func TestCreateTempDir(t *testing.T) {
	fs := afero.NewMemMapFs()

	dir, err := CreateTempDir(fs, "pkgctl-test-")
	if err != nil {
		t.Fatalf("CreateTempDir() error = %v", err)
	}

	if dir == "" {
		t.Error("expected non-empty directory path")
	}

	// Verify directory exists
	if !Exists(fs, dir) {
		t.Error("expected directory to exist")
	}
}

func TestEnsureDir(t *testing.T) {
	fs := afero.NewMemMapFs()

	path := "/test/nested/dir"
	if err := EnsureDir(fs, path, 0755); err != nil {
		t.Fatalf("EnsureDir() error = %v", err)
	}

	if !IsDir(fs, path) {
		t.Error("expected directory to exist and be a directory")
	}
}

func TestExists(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create a test file
	afero.WriteFile(fs, "/test.txt", []byte("test"), 0644)

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"existing file", "/test.txt", true},
		{"non-existing file", "/nonexistent.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Exists(fs, tt.path)
			if got != tt.want {
				t.Errorf("Exists(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestCopyFile(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create source file
	srcContent := []byte("test content")
	afero.WriteFile(fs, "/src.txt", srcContent, 0644)

	// Copy file
	if err := CopyFile(fs, "/src.txt", "/dst.txt"); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	// Verify destination
	dstContent, err := afero.ReadFile(fs, "/dst.txt")
	if err != nil {
		t.Fatalf("failed to read destination: %v", err)
	}

	if string(dstContent) != string(srcContent) {
		t.Errorf("copied content = %q, want %q", dstContent, srcContent)
	}
}
