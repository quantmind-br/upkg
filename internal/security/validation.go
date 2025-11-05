package security

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ValidPackageNameRegex allows alphanumeric, dash, underscore, and dot
	ValidPackageNameRegex = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

	// ValidVersionRegex allows standard version formats
	ValidVersionRegex = regexp.MustCompile(`^[a-zA-Z0-9._+-]+$`)

	// DangerousPathPatterns contains patterns that should not appear in paths
	DangerousPathPatterns = []string{
		"..",
		"~",
		"$",
		"`",
		"|",
		"&",
		";",
		"\n",
		"\r",
	}
)

// ValidatePackageName validates a package name for safety
func ValidatePackageName(name string) error {
	if name == "" {
		return fmt.Errorf("package name cannot be empty")
	}

	if len(name) > 255 {
		return fmt.Errorf("package name too long (max 255 characters)")
	}

	if !ValidPackageNameRegex.MatchString(name) {
		return fmt.Errorf("invalid package name: must contain only alphanumeric, dash, underscore, or dot characters")
	}

	// Check for suspicious patterns
	lowerName := strings.ToLower(name)
	suspiciousPatterns := []string{
		"../",
		"..\\",
		"~/",
		"/etc/",
		"/bin/",
		"/sbin/",
		"/usr/bin/",
		"/usr/sbin/",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerName, pattern) {
			return fmt.Errorf("package name contains suspicious pattern: %s", pattern)
		}
	}

	return nil
}

// ValidateVersion validates a version string
func ValidateVersion(version string) error {
	if version == "" {
		// Empty version is allowed (optional field)
		return nil
	}

	if len(version) > 100 {
		return fmt.Errorf("version string too long (max 100 characters)")
	}

	if !ValidVersionRegex.MatchString(version) {
		return fmt.Errorf("invalid version format")
	}

	return nil
}

// ValidateFilePath validates a file path for dangerous patterns
func ValidateFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	// Clean the path first
	cleanPath := filepath.Clean(path)

	// Check for dangerous patterns
	for _, pattern := range DangerousPathPatterns {
		if strings.Contains(path, pattern) && pattern != "." && pattern != "-" {
			return fmt.Errorf("file path contains dangerous pattern: %s", pattern)
		}
	}

	// Ensure no absolute paths in user input (except for install operations where it's expected)
	// This check is contextual and should be used carefully
	if filepath.IsAbs(cleanPath) && !strings.HasPrefix(cleanPath, "/home/") && !strings.HasPrefix(cleanPath, "/tmp/") {
		// Allow /home/ and /tmp/ but warn about other absolute paths
		return fmt.Errorf("suspicious absolute path: %s", cleanPath)
	}

	return nil
}

// SanitizeString removes potentially dangerous characters from a string
func SanitizeString(input string) string {
	// Remove null bytes
	result := strings.ReplaceAll(input, "\x00", "")

	// Remove other control characters
	result = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1 // Drop character
		}
		return r
	}, result)

	return strings.TrimSpace(result)
}

// ValidateCommandArg validates a command-line argument for safety
func ValidateCommandArg(arg string) error {
	if strings.Contains(arg, "\x00") {
		return fmt.Errorf("argument contains null byte")
	}

	// Check for command injection patterns
	dangerousChars := []string{
		";", "&", "|", "`", "$", "(", ")", "<", ">", "\n", "\r",
	}

	for _, char := range dangerousChars {
		if strings.Contains(arg, char) {
			return fmt.Errorf("argument contains dangerous character: %s", char)
		}
	}

	return nil
}

// ValidateEnvironmentVariable validates an environment variable name and value
func ValidateEnvironmentVariable(name, value string) error {
	if name == "" {
		return fmt.Errorf("environment variable name cannot be empty")
	}

	// Variable names should be alphanumeric + underscore
	if !regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`).MatchString(name) {
		return fmt.Errorf("invalid environment variable name: %s", name)
	}

	// Values should not contain null bytes or control characters
	if strings.Contains(value, "\x00") {
		return fmt.Errorf("environment variable value contains null byte")
	}

	return nil
}

// ValidateInstallID validates an install ID format
func ValidateInstallID(id string) error {
	if id == "" {
		return fmt.Errorf("install ID cannot be empty")
	}

	// Install IDs should be alphanumeric with dashes
	if !regexp.MustCompile(`^[a-zA-Z0-9-]+$`).MatchString(id) {
		return fmt.Errorf("invalid install ID format")
	}

	if len(id) > 100 {
		return fmt.Errorf("install ID too long")
	}

	return nil
}

// IsPathWithinDirectory checks if a path is within a given directory (additional validation)
func IsPathWithinDirectory(basePath, targetPath string) (bool, error) {
	// Clean both paths
	cleanBase, err := filepath.Abs(filepath.Clean(basePath))
	if err != nil {
		return false, fmt.Errorf("failed to resolve base path: %w", err)
	}

	cleanTarget, err := filepath.Abs(filepath.Clean(targetPath))
	if err != nil {
		return false, fmt.Errorf("failed to resolve target path: %w", err)
	}

	// Check if target starts with base
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return false, err
	}

	// If rel starts with "..", it's outside the directory
	if strings.HasPrefix(rel, "..") {
		return false, nil
	}

	return true, nil
}
