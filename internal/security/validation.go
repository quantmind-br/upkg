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
		"\x00",
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
	// 1. Rejeitar string vazia
	if version == "" {
		return fmt.Errorf("invalid version: version cannot be empty")
	}

	// 2. Limitar comprimento (verificar ANTES de processar conteúdo)
	if len(version) >= 100 {
		return fmt.Errorf("version string too long (max 100 characters)")
	}

	// 3. Detectar null byte
	if strings.Contains(version, "\x00") {
		return fmt.Errorf("invalid version: contains null byte")
	}

	// 4. Detectar caracteres perigosos (path traversal, command injection)
	dangerousPatterns := []string{
		"..", "/", "\\", ";", "&", "|", "`", "$", "\n", "\r",
	}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(version, pattern) {
			return fmt.Errorf("invalid version: contains dangerous pattern: %s", pattern)
		}
	}

	// 5. Validar formato com regex
	if !ValidVersionRegex.MatchString(version) {
		return fmt.Errorf("invalid version format: must be alphanumeric with dots, dashes, or plus signs")
	}

	return nil
}

// ValidateFilePath validates a file path for dangerous patterns
func ValidateFilePath(path string) error {
	// 1. Validar não-vazio
	if path == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	// 2. Limitar comprimento (verificar ANTES de processar conteúdo)
	if len(path) >= 4096 {
		return fmt.Errorf("file path too long (max 4096 characters)")
	}

	// 3. Detectar null byte (ataque de path truncation)
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("file path contains null byte")
	}

	// 4. Limpar e normalizar o caminho
	cleanPath := filepath.Clean(path)

	// 5. Detectar arquivos/diretórios ocultos (iniciam com .)
	parts := strings.Split(cleanPath, string(filepath.Separator))
	for _, part := range parts {
		if part != "" && part != "." && part != ".." && strings.HasPrefix(part, ".") {
			return fmt.Errorf("file path contains hidden file or directory: %s", part)
		}
	}

	// 6. Verificar padrões perigosos (path traversal, command injection)
	for _, pattern := range DangerousPathPatterns {
		if strings.Contains(path, pattern) && pattern != "." && pattern != "-" {
			return fmt.Errorf("file path contains dangerous pattern: %s", pattern)
		}
	}

	// 7. Detectar caminhos sensíveis do sistema
	sensitivePaths := []string{
		"/etc/", "/bin/", "/sbin/", "/usr/sbin/",
		"/root/", "/boot/", "/sys/", "/proc/",
		"/dev/", "/lib/", "/lib64/",
	}

	if filepath.IsAbs(cleanPath) {
		for _, sensitive := range sensitivePaths {
			if strings.HasPrefix(cleanPath, sensitive) {
				return fmt.Errorf("file path points to sensitive system path: %s", sensitive)
			}
		}

		// Permitir caminhos seguros absolutos (para instalações)
		// Note: /usr/bin/ removido para prevenir sobrescrita de binários do sistema
		safeWritePaths := []string{"/home/", "/tmp/", "/var/tmp/", "/usr/local/", "/opt/"}
		isSafe := false
		for _, safe := range safeWritePaths {
			if strings.HasPrefix(cleanPath, safe) {
				isSafe = true
				break
			}
		}

		if !isSafe {
			return fmt.Errorf("suspicious absolute path: %s", cleanPath)
		}
	}

	return nil
}

// SanitizeString removes potentially dangerous characters and normalizes the string
// - Removes null bytes and control characters
// - Replaces spaces and special characters with hyphens
// - Preserves: alphanumeric, underscores, dots, hyphens
// - Normalizes multiple hyphens to single hyphen
// - Trims leading/trailing whitespace and hyphens
func SanitizeString(input string) string {
	// 1. Remove null bytes
	result := strings.ReplaceAll(input, "\x00", "")

	// 2. Remove control characters (except newline, tab, carriage return - which will be handled next)
	result = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1 // Drop character
		}
		return r
	}, result)

	// 3. Trim leading/trailing whitespace
	result = strings.TrimSpace(result)

	// 4. Build sanitized string: replace special chars/spaces with hyphens
	var builder strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			// Keep: alphanumeric, underscore, dot
			builder.WriteRune(r)
		} else if r == '-' {
			// Keep hyphen as-is
			builder.WriteRune(r)
		} else if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			// Replace whitespace with hyphen
			builder.WriteRune('-')
		} else {
			// Replace special characters with hyphen
			// (This includes: @#$%^&*()+=[]{}|;:'",<>?/\)
			builder.WriteRune('-')
		}
	}
	result = builder.String()

	// 5. Normalize multiple consecutive hyphens to single hyphen
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}

	// 6. Trim leading/trailing hyphens
	result = strings.Trim(result, "-")

	return result
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

// IsPathWithinDirectory checks if a target path is within a given base directory
// Parameters:
//   - targetPath: the file/directory path to check (e.g., "/home/user/app/file.txt")
//   - basePath: the base directory to check against (e.g., "/home/user/app")
//
// Returns:
//   - bool: true if targetPath is within basePath
//   - error: non-nil if paths cannot be resolved or if relative paths are used
func IsPathWithinDirectory(targetPath, basePath string) (bool, error) {
	// 1. Validar que ambos os caminhos são absolutos
	if !filepath.IsAbs(targetPath) {
		return false, fmt.Errorf("target path must be absolute, got relative path: %s", targetPath)
	}
	if !filepath.IsAbs(basePath) {
		return false, fmt.Errorf("base path must be absolute, got relative path: %s", basePath)
	}

	// 2. Limpar e normalizar ambos os caminhos
	cleanBase := filepath.Clean(basePath)
	cleanTarget := filepath.Clean(targetPath)

	// 3. Verificar se target começa com base
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil {
		return false, fmt.Errorf("failed to compute relative path: %w", err)
	}

	// 4. Se rel começa com "..", o target está fora do base
	if strings.HasPrefix(rel, "..") {
		return false, nil
	}

	// 5. Se rel é ".", o target é exatamente o base (considerado "dentro")
	return true, nil
}
