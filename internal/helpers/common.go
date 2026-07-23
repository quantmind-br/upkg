package helpers

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/spf13/afero"
)

// NormalizeFilename normalizes a filename by converting to lowercase and replacing special characters
func NormalizeFilename(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// GenerateInstallID generates a unique installation ID from a name
func GenerateInstallID(name string) string {
	return fmt.Sprintf("%s-%s", name, uuid.NewString())
}

// CopyFile copies a file from src to dst with proper error handling and sync
func CopyFile(src, dst string) (err error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if cerr := sourceFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close source file: %w", cerr)
		}
	}()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer func() {
		if cerr := destFile.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("failed to close destination file: %w", cerr)
		}
	}()

	if _, err = io.Copy(destFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Ensure data is synced to disk before returning
	if err = destFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync file to disk: %w", err)
	}

	return nil
}

// RemoveIfExists removes path from fs (recursively when requested), treating an
// empty path and a missing path as success. Any other failure is wrapped with
// the path for context. It centralizes the idempotent-removal policy shared by
// the backends' install rollbacks and uninstall flows.
func RemoveIfExists(fs afero.Fs, path string, recursive bool) error {
	if path == "" {
		return nil
	}
	var err error
	if recursive {
		err = fs.RemoveAll(path)
	} else {
		err = fs.Remove(path)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove %q: %w", path, err)
	}
	return nil
}
