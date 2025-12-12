package desktop

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/security"
)

// Parse parses a .desktop file from a reader
func Parse(r io.Reader) (*core.DesktopEntry, error) {
	de := &core.DesktopEntry{}
	scanner := bufio.NewScanner(r)
	inDesktopEntry := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for [Desktop Entry] section
		if line == "[Desktop Entry]" {
			inDesktopEntry = true
			continue
		}

		// Parse key-value pairs
		if inDesktopEntry && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "Type":
				de.Type = value
			case "Name":
				de.Name = value
			case "Exec":
				de.Exec = value
			case "Icon":
				de.Icon = value
			case "Comment":
				de.Comment = value
			case "Categories":
				de.Categories = parseSemicolonList(value)
			case "Terminal":
				de.Terminal = value == "true"
			case "StartupWMClass":
				de.StartupWMClass = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan desktop file: %w", err)
	}

	return de, nil
}

// Write writes a .desktop file to a writer
func Write(w io.Writer, de *core.DesktopEntry) error {
	fmt.Fprintln(w, "[Desktop Entry]")
	fmt.Fprintf(w, "Type=%s\n", de.Type)
	fmt.Fprintf(w, "Name=%s\n", de.Name)
	fmt.Fprintf(w, "Exec=%s\n", de.Exec)

	if de.Icon != "" {
		fmt.Fprintf(w, "Icon=%s\n", de.Icon)
	}
	if de.Comment != "" {
		fmt.Fprintf(w, "Comment=%s\n", de.Comment)
	}
	if len(de.Categories) > 0 {
		fmt.Fprintf(w, "Categories=%s\n", strings.Join(de.Categories, ";")+";")
	}
	if de.Terminal {
		fmt.Fprintln(w, "Terminal=true")
	}
	if de.StartupWMClass != "" {
		fmt.Fprintf(w, "StartupWMClass=%s\n", de.StartupWMClass)
	}

	return nil
}

// Validate checks if the desktop entry has required fields
func Validate(de *core.DesktopEntry) error {
	if de.Type == "" {
		return fmt.Errorf("Type field is required")
	}
	if de.Name == "" {
		return fmt.Errorf("Name field is required")
	}
	if de.Exec == "" {
		return fmt.Errorf("Exec field is required")
	}
	return nil
}

// InjectWaylandEnvVars injects Wayland environment variables into the Exec line
func InjectWaylandEnvVars(de *core.DesktopEntry, customVars []string) error {
	envVars := []string{
		"GDK_BACKEND=wayland,x11",
		"QT_QPA_PLATFORM=wayland:xcb",
		"MOZ_ENABLE_WAYLAND=1",
		"ELECTRON_OZONE_PLATFORM_HINT=auto",
	}
	validCustom := make([]string, 0, len(customVars))
	var invalid []string
	for _, raw := range customVars {
		parts := strings.SplitN(raw, "=", 2)
		if len(parts) != 2 {
			invalid = append(invalid, raw)
			continue
		}
		name := parts[0]
		value := parts[1]
		if err := security.ValidateEnvironmentVariable(name, value); err != nil {
			invalid = append(invalid, raw)
			continue
		}
		validCustom = append(validCustom, raw)
	}
	if len(invalid) > 0 {
		return fmt.Errorf("invalid custom env vars: %v", invalid)
	}
	envVars = append(envVars, validCustom...)

	for i, val := range envVars {
		envVars[i] = escapeExecToken(val)
	}

	// Build env prefix
	prefix := "env " + strings.Join(envVars, " ") + " "

	// Prepend to Exec line (if not already present)
	if !strings.HasPrefix(de.Exec, "env ") {
		de.Exec = prefix + de.Exec
	}

	return nil
}

// WriteDesktopFile writes a desktop entry to a file
func WriteDesktopFile(filePath string, de *core.DesktopEntry) error {
	// Validate desktop entry first
	if err := Validate(de); err != nil {
		return fmt.Errorf("invalid desktop entry: %w", err)
	}

	// Create file with proper permissions
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create desktop file: %w", err)
	}
	defer file.Close()

	// Write desktop entry
	return Write(file, de)
}

// parseSemicolonList parses semicolon-separated list
func parseSemicolonList(value string) []string {
	value = strings.TrimSuffix(value, ";")
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ";")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func escapeExecToken(value string) string {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return escapeGenericToken(value, strings.ContainsAny(value, " ;\"'"))
	}

	key, val := parts[0], parts[1]
	needsQuote := strings.ContainsAny(val, " ;\"'")

	escapedVal := strings.ReplaceAll(val, `\`, `\\`)
	escapedVal = strings.ReplaceAll(escapedVal, `"`, `\"`)
	escapedVal = strings.ReplaceAll(escapedVal, `'`, `\'`)

	if needsQuote {
		escapedVal = `"` + escapedVal + `"`
	}

	return key + "=" + escapedVal
}

func escapeGenericToken(token string, quote bool) string {
	escaped := strings.ReplaceAll(token, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)
	if quote {
		escaped = `"` + escaped + `"`
	}
	return escaped
}
