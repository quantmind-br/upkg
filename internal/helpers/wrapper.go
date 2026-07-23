package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// WrapperConfig contains configuration for creating a wrapper script
type WrapperConfig struct {
	WrapperPath    string // Path where the wrapper script will be created
	ExecPath       string // Path to the executable to wrap
	DisableSandbox bool   // Whether to add --no-sandbox flag for Electron apps
}

// CreateWrapper creates a wrapper shell script for an executable.
// For Electron apps, it generates a wrapper that runs from the app's directory
// with optional --no-sandbox flag. For regular apps, it creates a simple exec wrapper.
func CreateWrapper(fs afero.Fs, cfg WrapperConfig) error {
	// Check if this is an Electron app (has .asar file nearby)
	isElectron := IsElectronApp(fs, cfg.ExecPath)

	var content string
	if isElectron {
		// Electron apps need to run from their own directory
		execDir := filepath.Dir(cfg.ExecPath)
		execName := filepath.Base(cfg.ExecPath)

		// Only add --no-sandbox if explicitly configured (security risk).
		// The flag stays outside the quoted word.
		sandboxFlag := ""
		if cfg.DisableSandbox {
			sandboxFlag = " --no-sandbox"
		}

		content = fmt.Sprintf(`#!/bin/bash
# upkg wrapper script for Electron app
cd %s
exec %s%s "$@"
`, ShellQuote(execDir), ShellQuote("./"+execName), sandboxFlag)
	} else {
		// Standard wrapper
		content = fmt.Sprintf(`#!/bin/bash
# upkg wrapper script
exec %s "$@"
`, ShellQuote(cfg.ExecPath))
	}

	return afero.WriteFile(fs, cfg.WrapperPath, []byte(content), 0755)
}

// IsElectronApp checks if the executable is part of an Electron app
// by looking for .asar files in the executable's directory structure
func IsElectronApp(fs afero.Fs, execPath string) bool {
	execDir := filepath.Dir(execPath)

	// Check for resources/app.asar (typical Electron structure)
	asarPath := filepath.Join(execDir, "resources", "app.asar")
	if _, err := fs.Stat(asarPath); err == nil {
		return true
	}

	// Check for *.asar in parent directory and subdirectories using the injected
	// filesystem (never the host) so detection is deterministic and testable.
	parentDir := filepath.Dir(execDir)
	var asarFound bool
	// afero.Walk propagates filepath.SkipAll as a non-nil error (unlike
	// filepath.Walk), so the walk result is ignored: detection is best-effort and
	// asarFound already captures a positive match.
	_ = afero.Walk(fs, parentDir, func(path string, info os.FileInfo, entryErr error) error { //nolint:errcheck // best-effort detection; SkipAll surfaces as non-nil and asarFound holds the result
		if entryErr != nil {
			return nil // Continue on errors
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".asar") {
			asarFound = true
			return filepath.SkipAll // Found one, stop walking
		}
		return nil
	})
	return asarFound
}

// ShellQuote returns value as a single Bash word wrapped in single quotes,
// replacing each embedded single quote with the '"'"' sequence so the result
// is safe to interpolate into a generated shell script.
func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}
