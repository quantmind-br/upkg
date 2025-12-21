package helpers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"lowercase", "MyApp", "myapp"},
		{"spaces to dashes", "My App", "my-app"},
		{"underscores to dashes", "My_App", "my-app"},
		{"special chars removed", "My@App#123", "myapp123"},
		{"keep valid chars", "my-app_123.test", "my-app-123.test"},
		{"empty string", "", ""},
		{"complex", "Test App v1.0 (2024)", "test-app-v1.0-2024"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateInstallID(t *testing.T) {
	t.Run("generates unique IDs", func(t *testing.T) {
		id1 := GenerateInstallID("test")
		time.Sleep(1 * time.Second)
		id2 := GenerateInstallID("test")

		// IDs should be different due to timestamp
		assert.NotEqual(t, id1, id2)
	})

	t.Run("includes name", func(t *testing.T) {
		id := GenerateInstallID("myapp")
		assert.Contains(t, id, "myapp")
	})

	t.Run("format consistency", func(t *testing.T) {
		id := GenerateInstallID("test")
		// Should be in format: name-timestamp
		assert.Contains(t, id, "-")
	})
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("successful copy", func(t *testing.T) {
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		content := []byte("test content")
		require.NoError(t, os.WriteFile(src, content, 0644))

		err := CopyFile(src, dst)
		assert.NoError(t, err)

		// Verify content
		copied, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Equal(t, content, copied)

		// Verify permissions
		srcInfo, _ := os.Stat(src)
		dstInfo, _ := os.Stat(dst)
		// Both should be readable
		assert.Equal(t, srcInfo.Mode().Perm()&0400, dstInfo.Mode().Perm()&0400)
	})

	t.Run("source doesn't exist", func(t *testing.T) {
		src := filepath.Join(tmpDir, "nonexistent.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		err := CopyFile(src, dst)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open source file")
	})

	t.Run("destination directory doesn't exist", func(t *testing.T) {
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "nonexistent", "dest.txt")

		require.NoError(t, os.WriteFile(src, []byte("test"), 0644))

		err := CopyFile(src, dst)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create destination file")
	})

	t.Run("permission denied on destination", func(t *testing.T) {
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "readonly", "dest.txt")

		require.NoError(t, os.WriteFile(src, []byte("test"), 0644))
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "readonly"), 0555))
		defer os.Chmod(filepath.Join(tmpDir, "readonly"), 0755)

		err := CopyFile(src, dst)
		assert.Error(t, err)
	})

	t.Run("large file", func(t *testing.T) {
		src := filepath.Join(tmpDir, "large.txt")
		dst := filepath.Join(tmpDir, "large_copy.txt")

		// Create a 1MB file
		largeContent := make([]byte, 1024*1024)
		for i := range largeContent {
			largeContent[i] = byte(i % 256)
		}
		require.NoError(t, os.WriteFile(src, largeContent, 0644))

		err := CopyFile(src, dst)
		assert.NoError(t, err)

		copied, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Equal(t, largeContent, copied)
	})

	t.Run("empty file", func(t *testing.T) {
		src := filepath.Join(tmpDir, "empty.txt")
		dst := filepath.Join(tmpDir, "empty_copy.txt")

		require.NoError(t, os.WriteFile(src, []byte{}, 0644))

		err := CopyFile(src, dst)
		assert.NoError(t, err)

		copied, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Empty(t, copied)
	})
}
