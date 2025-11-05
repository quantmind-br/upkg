package helpers

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
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
	return fmt.Sprintf("%s-%d", name, time.Now().Unix())
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
