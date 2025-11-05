package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateExtractPath prevents directory traversal attacks (Zip Slip vulnerability)
// Ensures that the extracted path does not escape the target directory
func ValidateExtractPath(targetDir, extractedPath string) error {
	// Clean the path to resolve . and ..
	cleanPath := filepath.Clean(extractedPath)

	// Check for path traversal attempts
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("path contains ..: %s", extractedPath)
	}

	// Ensure the path doesn't start with /
	if filepath.IsAbs(cleanPath) {
		return fmt.Errorf("absolute path not allowed: %s", extractedPath)
	}

	// Build target path and verify it's under targetDir
	destPath := filepath.Join(targetDir, cleanPath)

	// Get canonical paths for comparison
	cleanDest, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("failed to resolve target directory: %w", err)
	}

	cleanTarget, err := filepath.Abs(destPath)
	if err != nil {
		return fmt.Errorf("failed to resolve destination path: %w", err)
	}

	// Ensure target is under destDir
	if !strings.HasPrefix(cleanTarget, cleanDest+string(filepath.Separator)) &&
		cleanTarget != cleanDest {
		return fmt.Errorf("path escapes destination directory: %s", extractedPath)
	}

	return nil
}

// ValidateSymlink ensures symlinks don't escape the target directory
func ValidateSymlink(targetDir, linkPath, linkTarget string) error {
	// Resolve the symlink target relative to its location
	linkDir := filepath.Dir(linkPath)
	resolvedTarget := filepath.Join(linkDir, linkTarget)

	// Clean and make absolute
	cleanTarget, err := filepath.Abs(resolvedTarget)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink target: %w", err)
	}

	cleanDest, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("failed to resolve target directory: %w", err)
	}

	// Ensure symlink target is under targetDir
	if !strings.HasPrefix(cleanTarget, cleanDest+string(filepath.Separator)) &&
		cleanTarget != cleanDest {
		return fmt.Errorf("symlink target escapes destination: %s -> %s", linkPath, linkTarget)
	}

	return nil
}

// ValidatePath performs general path validation
func ValidatePath(path string) error {
	// Check for null bytes
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("path contains null bytes: %s", path)
	}

	// Check for excessive length
	if len(path) > 4096 {
		return fmt.Errorf("path too long: %d characters", len(path))
	}

	return nil
}

// SanitizePath sanitizes a file path for safe use
func SanitizePath(path string) string {
	// Clean the path
	cleaned := filepath.Clean(path)

	// Remove null bytes
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")

	return cleaned
}

// IsPathSafe checks if a path is safe (doesn't escape target)
func IsPathSafe(basePath, targetPath string) bool {
	err := ValidateExtractPath(basePath, targetPath)
	return err == nil
}
