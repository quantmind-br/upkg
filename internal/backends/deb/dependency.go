package deb

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// fixMalformedDependencies corrects common dependency name issues from debtap conversion
// This addresses issues where epoch versions (like 2:1.4.99.1) cause name mangling
func fixMalformedDependencies(pkgPath string, logger *zerolog.Logger) error {
	// Extract the package to a temp directory
	fs := afero.NewOsFs()
	tmpDir, err := afero.TempDir(fs, "", "upkg-fix-deps-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := fs.RemoveAll(tmpDir); removeErr != nil {
			logger.Debug().Err(removeErr).Str("tmp_dir", tmpDir).Msg("failed to remove temp dir")
		}
	}()

	// Extract package using bsdtar (Arch standard, auto-detects compression)
	extractCmd := exec.Command("bsdtar", "-xf", pkgPath, "-C", tmpDir) // #nosec G204 -- pkgPath is validated
	var extractStderr bytes.Buffer
	extractCmd.Stderr = &extractStderr
	if extractErr := extractCmd.Run(); extractErr != nil {
		return fmt.Errorf("failed to extract package: %w (stderr: %s)", extractErr, extractStderr.String())
	}

	// Read .PKGINFO
	pkgInfoPath := filepath.Join(tmpDir, ".PKGINFO")
	content, err := afero.ReadFile(fs, pkgInfoPath)
	if err != nil {
		return fmt.Errorf("failed to read .PKGINFO: %w", err)
	}

	// Fix malformed dependencies
	lines := strings.Split(string(content), "\n")
	var fixed []string
	hasChanges := false

	for _, line := range lines {
		if strings.HasPrefix(line, "depend = ") {
			fixedLine := fixDependencyLine(line, logger)
			if fixedLine == "" {
				// Dependency should be removed
				logger.Debug().
					Str("removed_dependency", strings.TrimPrefix(line, "depend = ")).
					Msg("removing invalid dependency")
				hasChanges = true
				continue
			}
			if fixedLine != line {
				logger.Debug().
					Str("original", strings.TrimPrefix(line, "depend = ")).
					Str("fixed", strings.TrimPrefix(fixedLine, "depend = ")).
					Msg("dependency mapping applied")
				hasChanges = true
			}
			fixed = append(fixed, fixedLine)
		} else {
			fixed = append(fixed, line)
		}
	}

	// Only repack if we made changes
	if !hasChanges {
		return nil
	}

	// Write fixed .PKGINFO
	if writeErr := afero.WriteFile(fs, pkgInfoPath, []byte(strings.Join(fixed, "\n")), 0644); writeErr != nil {
		return fmt.Errorf("failed to write fixed .PKGINFO: %w", writeErr)
	}

	// Repack using bsdtar with zstd compression (Arch standard)
	// List files explicitly to avoid ./ prefix that causes "missing metadata" error
	files, err := afero.ReadDir(fs, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to read tmpdir: %w", err)
	}

	// Build list of files without ./ prefix
	var fileList []string
	for _, file := range files {
		fileList = append(fileList, file.Name())
	}

	// Create command with explicit file list: bsdtar --zstd -cf package.tar.zst -C tmpDir file1 file2 ...
	args := []string{"--zstd", "-cf", pkgPath, "-C", tmpDir}
	args = append(args, fileList...)

	repackCmd := exec.Command("bsdtar", args...)
	var repackStderr bytes.Buffer
	repackCmd.Stderr = &repackStderr
	if err := repackCmd.Run(); err != nil {
		return fmt.Errorf("failed to repack package with bsdtar: %w (stderr: %s)", err, repackStderr.String())
	}

	return nil
}

// fixDependencyLine corrects a single dependency line with known malformations
// Returns empty string if dependency should be removed
//
//nolint:gocyclo // dependency normalization is a rule table by nature.
func fixDependencyLine(line string, _ *zerolog.Logger) string {
	// Extract the dependency part after "depend = "
	if !strings.HasPrefix(line, "depend = ") {
		return line
	}

	dep := strings.TrimPrefix(line, "depend = ")

	// Remove completely invalid dependencies (these are artifacts from debtap parsing)
	invalidDeps := []string{
		"anaconda",       // Artifact from libc6 epoch parsing
		"apparmor.d-git", // Artifact
		"cura-bin",       // Artifact from libc6>=2.17
	}

	// Extract just the package name (before any version operator)
	depName := dep
	versionConstraint := ""
	for _, op := range []string{">=", "<=", "=", ">", "<"} {
		if idx := strings.Index(dep, op); idx != -1 {
			depName = dep[:idx]
			versionConstraint = dep[idx:]
			break
		}
	}

	for _, invalid := range invalidDeps {
		if strings.HasPrefix(depName, invalid) {
			return "" // Empty string signals removal
		}
	}

	// Debian/Ubuntu → Arch package name mapping
	// Many Debian packages have different names in Arch repos
	debianToArchMap := map[string]string{
		"gtk":        "gtk3",          // Generic GTK → GTK3 (most compatible)
		"gtk2.0":     "gtk2",          // Debian GTK2 naming
		"gtk-3.0":    "gtk3",          // Debian GTK3 naming variant
		"python3":    "python",        // Arch uses "python" for Python 3
		"nodejs":     "nodejs",        // Same but good to document
		"libssl":     "openssl",       // SSL library naming (v3)
		"libssl1.1":  "openssl-1.1",   // Specific SSL 1.1 version (legacy package)
		"libssl3":    "openssl",       // OpenSSL 3.x
		"libjpeg":    "libjpeg-turbo", // JPEG library
		"libpng":     "libpng",        // Same but documented
		"libpng16":   "libpng",        // Specific version to generic
		"zlib1g":     "zlib",          // Debian zlib naming
		"libcurl":    "curl",          // Curl library
		"libcurl4":   "curl",          // Curl 4.x
		"libglib2.0": "glib2",         // GLib naming difference
		"libnotify4": "libnotify",     // Remove version suffix
	}

	// Apply Debian→Arch mapping if needed
	if archName, exists := debianToArchMap[depName]; exists {
		return "depend = " + archName + versionConstraint
	}

	// Pattern-based fixes
	// Check if dependency name matches regex rules
	replacements := []struct {
		prefix     string
		minLen     int
		versionIdx int // Index where version typically starts
		target     string
	}{
		{prefix: "libx11", minLen: 6, versionIdx: 6, target: "libx11"},
		{prefix: "libxcomposite", minLen: 13, versionIdx: 13, target: "libxcomposite"},
		{prefix: "libxdamage", minLen: 10, versionIdx: 10, target: "libxdamage"},
		{prefix: "libxkbfile", minLen: 10, versionIdx: 10, target: "libxkbfile"},
		{prefix: "nspr", minLen: 4, versionIdx: 4, target: "nspr"},
	}

	// Fix "c>=" → "glibc>=" (but avoid matching "cairo", "curl", etc.)
	if strings.HasPrefix(dep, "c>=") || strings.HasPrefix(dep, "c>") || strings.HasPrefix(dep, "c<") || strings.HasPrefix(dep, "c=") {
		if len(dep) > 1 && (dep[1] == '>' || dep[1] == '<' || dep[1] == '=') {
			return "depend = glibc" + dep[1:]
		}
	}

	// Apply pattern replacements for malformed versions (e.g. libx111.4.99 -> libx11>=1.4.99)
	for _, r := range replacements {
		if strings.HasPrefix(dep, r.prefix) && len(dep) > r.minLen {
			// Check if it's malformed (has digits immediately after prefix)
			if dep[r.versionIdx] >= '0' && dep[r.versionIdx] <= '9' {
				version := dep[r.versionIdx:]
				return "depend = " + r.target + ">=" + version
			}
		}
	}

	return line
}
