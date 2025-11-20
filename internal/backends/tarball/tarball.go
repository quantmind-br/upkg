package tarball

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/quantmind-br/upkg/internal/cache"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/desktop"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/icons"
	"github.com/rs/zerolog"
	"layeh.com/asar"
)

// TarballBackend handles tarball and zip archive installations
type TarballBackend struct {
	cfg    *config.Config
	logger *zerolog.Logger
}

// New creates a new tarball backend
func New(cfg *config.Config, log *zerolog.Logger) *TarballBackend {
	return &TarballBackend{
		cfg:    cfg,
		logger: log,
	}
}

// Name returns the backend name
func (t *TarballBackend) Name() string {
	return "tarball"
}

// Detect checks if this backend can handle the package
func (t *TarballBackend) Detect(ctx context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := os.Stat(packagePath); err != nil {
		return false, nil
	}

	// Check file type
	fileType, err := helpers.DetectFileType(packagePath)
	if err != nil {
		return false, err
	}

	// Accept tar.gz, tar.xz, tar.bz2, tar, zip
	return fileType == helpers.FileTypeTarGz ||
		fileType == helpers.FileTypeTarXz ||
		fileType == helpers.FileTypeTarBz2 ||
		fileType == helpers.FileTypeTar ||
		fileType == helpers.FileTypeZip, nil
}

// Install installs the tarball/zip package
func (t *TarballBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions) (*core.InstallRecord, error) {
	t.logger.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing tarball/zip package")

	// Validate package exists
	if _, err := os.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	// Detect archive type
	archiveType := helpers.GetArchiveType(packagePath)
	if archiveType == "" {
		return nil, fmt.Errorf("unsupported archive type: %s", packagePath)
	}

	t.logger.Debug().
		Str("archive_type", archiveType).
		Msg("detected archive type")

	// Determine application name
	appName := opts.CustomName
	if appName == "" {
		appName = filepath.Base(packagePath)
		// Remove all extensions
		for {
			ext := filepath.Ext(appName)
			if ext == "" {
				break
			}
			appName = strings.TrimSuffix(appName, ext)
		}

		// Clean up version numbers, arch, etc.
		appName = helpers.CleanAppName(appName)

		// Title case for better presentation
		appName = helpers.FormatDisplayName(appName)
	}

	// Normalize name
	normalizedName := helpers.NormalizeFilename(appName)
	installID := helpers.GenerateInstallID(normalizedName)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create installation directory in ~/.local/share/upkg/apps/
	appsDir := filepath.Join(homeDir, ".local", "share", "upkg", "apps")
	installDir := filepath.Join(appsDir, normalizedName)

	// Check if already exists
	if _, err := os.Stat(installDir); err == nil {
		return nil, fmt.Errorf("package already installed at: %s", installDir)
	}

	// Create installation directory
	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create installation directory: %w", err)
	}

	// Extract archive
	t.logger.Debug().
		Str("archive", packagePath).
		Str("dest", installDir).
		Msg("extracting archive")

	if err := t.extractArchive(packagePath, installDir, archiveType); err != nil {
		os.RemoveAll(installDir)
		return nil, fmt.Errorf("failed to extract archive: %w", err)
	}

	// Find executable(s)
	executables, err := t.findExecutables(installDir)
	if err != nil || len(executables) == 0 {
		os.RemoveAll(installDir)
		return nil, fmt.Errorf("no executables found in archive")
	}

	t.logger.Debug().
		Strs("executables", executables).
		Msg("found executables")

	// Choose primary executable using scoring heuristic
	primaryExec := t.chooseBestExecutable(executables, normalizedName, installDir)

	t.logger.Debug().
		Str("primary_executable", primaryExec).
		Int("total_candidates", len(executables)).
		Msg("selected primary executable")

	// Create wrapper script in ~/.local/bin/
	binDir := filepath.Join(homeDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		os.RemoveAll(installDir)
		return nil, fmt.Errorf("failed to create bin directory: %w", err)
	}

	wrapperPath := filepath.Join(binDir, normalizedName)
	if err := t.createWrapper(wrapperPath, primaryExec); err != nil {
		os.RemoveAll(installDir)
		return nil, fmt.Errorf("failed to create wrapper script: %w", err)
	}

	t.logger.Debug().
		Str("wrapper", wrapperPath).
		Msg("created wrapper script")

	// Install icons (if any)
	iconPaths, err := t.installIcons(installDir, normalizedName)
	if err != nil {
		t.logger.Warn().Err(err).Msg("failed to install icons")
	}

	// Create .desktop file
	var desktopPath string
	if !opts.SkipDesktop {
		desktopPath, err = t.createDesktopFile(installDir, appName, normalizedName, wrapperPath, opts)
		if err != nil {
			// Clean up on failure
			os.RemoveAll(installDir)
			os.Remove(wrapperPath)
			t.removeIcons(iconPaths)
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		t.logger.Debug().
			Str("desktop_file", desktopPath).
			Msg("desktop file created")

		// Update caches
		appsDbDir := filepath.Join(homeDir, ".local", "share", "applications")
		cache.UpdateDesktopDatabase(appsDbDir, t.logger)

		iconsDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor")
		cache.UpdateIconCache(iconsDir, t.logger)
	}

	// Create install record
	record := &core.InstallRecord{
		InstallID:    installID,
		PackageType:  core.PackageTypeTarball,
		Name:         appName,
		InstallDate:  time.Now(),
		OriginalFile: packagePath,
		InstallPath:  installDir,
		DesktopFile:  desktopPath,
		Metadata: core.Metadata{
			IconFiles:      iconPaths,
			WrapperScript:  wrapperPath,
			WaylandSupport: string(core.WaylandUnknown),
		},
	}

	t.logger.Info().
		Str("install_id", installID).
		Str("name", appName).
		Str("path", installDir).
		Msg("tarball/zip package installed successfully")

	return record, nil
}

// Uninstall removes the installed tarball/zip package
func (t *TarballBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	t.logger.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling tarball/zip package")

	// Remove installation directory
	if record.InstallPath != "" {
		if err := os.RemoveAll(record.InstallPath); err != nil && !os.IsNotExist(err) {
			t.logger.Warn().Err(err).Str("path", record.InstallPath).Msg("failed to remove installation directory")
		}
	}

	// Remove wrapper script
	if record.Metadata.WrapperScript != "" {
		if err := os.Remove(record.Metadata.WrapperScript); err != nil && !os.IsNotExist(err) {
			t.logger.Warn().Err(err).Str("path", record.Metadata.WrapperScript).Msg("failed to remove wrapper script")
		}
	}

	// Remove .desktop file
	if record.DesktopFile != "" {
		if err := os.Remove(record.DesktopFile); err != nil && !os.IsNotExist(err) {
			t.logger.Warn().Err(err).Str("path", record.DesktopFile).Msg("failed to remove desktop file")
		}
	}

	// Remove icons
	t.removeIcons(record.Metadata.IconFiles)

	// Update caches
	homeDir, err := os.UserHomeDir()
	if err == nil {
		appsDir := filepath.Join(homeDir, ".local", "share", "applications")
		cache.UpdateDesktopDatabase(appsDir, t.logger)

		iconsDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor")
		cache.UpdateIconCache(iconsDir, t.logger)
	}

	t.logger.Info().
		Str("install_id", record.InstallID).
		Msg("tarball/zip package uninstalled successfully")

	return nil
}

// extractArchive extracts an archive to a directory
func (t *TarballBackend) extractArchive(archivePath, destDir, archiveType string) error {
	switch archiveType {
	case "tar.gz":
		return helpers.ExtractTarGz(archivePath, destDir)
	case "tar.xz":
		return helpers.ExtractTarXz(archivePath, destDir)
	case "tar.bz2":
		return helpers.ExtractTarBz2(archivePath, destDir)
	case "tar":
		return helpers.ExtractTar(archivePath, destDir)
	case "zip":
		return helpers.ExtractZip(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive type: %s", archiveType)
	}
}

// findExecutables finds all executable files in a directory
func (t *TarballBackend) findExecutables(dir string) ([]string, error) {
	var executables []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Check if file is executable
		if info.Mode()&0111 != 0 {
			// Exclude shared libraries (.so files)
			// .so, .so.X, .so.X.Y, .so.X.Y.Z patterns
			baseName := filepath.Base(path)
			if strings.HasSuffix(baseName, ".so") || strings.Contains(baseName, ".so.") {
				return nil
			}

			// Check if it's an ELF binary
			isElf, _ := helpers.IsELF(path)
			if isElf {
				executables = append(executables, path)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return executables, nil
}

// execCandidate represents an executable with its score
type execCandidate struct {
	path  string
	score int
}

// chooseBestExecutable selects the best executable using a scoring heuristic
func (t *TarballBackend) chooseBestExecutable(executables []string, baseName, installDir string) string {
	if len(executables) == 0 {
		return ""
	}
	if len(executables) == 1 {
		return executables[0]
	}

	candidates := make([]execCandidate, 0, len(executables))

	for _, exe := range executables {
		score := t.scoreExecutable(exe, baseName, installDir)
		candidates = append(candidates, execCandidate{path: exe, score: score})

		t.logger.Debug().
			Str("executable", exe).
			Int("score", score).
			Msg("scored executable candidate")
	}

	// Sort by score descending (highest score first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return candidates[0].path
}

// scoreExecutable assigns a score to an executable based on various heuristics
func (t *TarballBackend) scoreExecutable(execPath, baseName, installDir string) int {
	score := 0
	filename := strings.ToLower(filepath.Base(execPath))
	normalizedBase := strings.ToLower(baseName)
	nameVariants := helpers.GenerateNameVariants(normalizedBase)

	// Calculate relative path and depth
	relPath := strings.TrimPrefix(execPath, installDir)
	relPath = strings.Trim(relPath, "/")
	depth := len(strings.Split(relPath, "/"))

	// Prefer shallow depth (executables in root or first level)
	// Depth 1: +50, Depth 2: +40, Depth 3: +30, etc.
	score += (11 - depth) * 10
	if depth > 10 {
		score -= 50 // Very deep, probably not the main executable
	}

	// Strong match: filename exactly matches any base variant
exactMatchLoop:
	for _, variant := range nameVariants {
		if variant == "" {
			continue
		}
		if filename == variant || filename == variant+".exe" {
			score += 120
			break exactMatchLoop
		}
	}

	// Partial match: filename contains any of the variants
partialMatchLoop:
	for _, variant := range nameVariants {
		if variant == "" || len(variant) < 3 {
			continue
		}
		if strings.Contains(filename, variant) {
			score += 60
			break partialMatchLoop
		}
	}

	// Bonus for known main executable patterns
	bonusPatterns := []string{
		"^wine$", "^wine64$", "^run$", "^start$", "^launch$",
		"^main$", "^app$", "^game$", "^application$",
	}
	for _, pattern := range bonusPatterns {
		matched, _ := regexp.MatchString(pattern, filename)
		if matched {
			score += 80
		}
	}

	// Penalize known helper/utility executables
	penaltyPatterns := []string{
		"chrome-sandbox", "crashpad", "minidump",
		"update", "uninstall", "helper", "crash",
		"debugger", "sandbox", "nacl", "xdg",
		"installer", "setup", "config", "daemon",
		"service", "agent", "monitor", "reporter",
		"dump", "winedump", "windump", "objdump",
		"winedbg", "wineboot", "winecfg", "wineconsole",
		"wineserver", "widl", "wmc", "wrc", "winebuild",
		"winegcc", "wineg++", "winecpp", "winemaker",
		"winefile", "winemine", "winepath",
	}
	for _, pattern := range penaltyPatterns {
		if strings.Contains(filename, pattern) {
			score -= 200 // Heavy penalty for utility executables
		}
	}

	// Strongly penalize shared libraries and lib-prefixed files that slip through
	if strings.HasPrefix(filename, "lib") {
		score -= 80
	}
	if strings.HasSuffix(filename, ".so") || strings.Contains(filename, ".so.") ||
		strings.HasSuffix(filename, ".dylib") || strings.HasSuffix(filename, ".dll") {
		score -= 400
	}

	// Check file size (main executables are usually larger)
	if info, err := os.Stat(execPath); err == nil {
		fileSize := info.Size()

		if fileSize > 10*1024*1024 { // > 10MB
			score += 30 // Likely a main application
		} else if fileSize > 1*1024*1024 { // 1-10MB
			score += 10 // Reasonable size
		} else if fileSize < 100*1024 { // < 100KB
			score -= 20 // Too small, probably a helper

			// Extra penalty for tiny executables (< 1KB) - likely wrapper scripts
			if fileSize < 1024 {
				score -= 50 // Very small, probably a wrapper script
			}
		}
	}

	// Bonus for executables in "bin" directory
	if strings.Contains(strings.ToLower(relPath), "/bin/") {
		score += 20
	}

	// Additional check: penalize if executable is a shell script with invalid references
	if t.isInvalidWrapperScript(execPath, installDir) {
		score -= 300 // Heavy penalty for wrapper scripts pointing to invalid paths
	}

	return score
}

// isInvalidWrapperScript checks if file is a wrapper script with invalid path references
func (t *TarballBackend) isInvalidWrapperScript(execPath, installDir string) bool {
	// Only check small files (< 10KB) that might be scripts
	info, err := os.Stat(execPath)
	if err != nil || info.Size() > 10*1024 {
		return false
	}

	// Read first 1KB to check for invalid paths
	file, err := os.Open(execPath)
	if err != nil {
		return false
	}
	defer file.Close()

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}
	if n == 0 {
		return false
	}

	content := string(buf[:n])

	// Check for shebang (shell script indicator)
	if !strings.HasPrefix(content, "#!") {
		return false // Not a shell script
	}

	// Check for absolute paths that don't exist or point outside installDir
	// Common patterns: /home/runner/, /tmp/build/, /opt/build/, etc.
	invalidPatterns := []string{
		"/home/runner/",
		"/home/builder/",
		"/tmp/build/",
		"/opt/build/",
		"/workspace/",
		"/build/",
	}

	for _, pattern := range invalidPatterns {
		if strings.Contains(content, pattern) {
			t.logger.Debug().
				Str("executable", execPath).
				Str("invalid_pattern", pattern).
				Msg("detected wrapper script with invalid build path")
			return true
		}
	}

	return false
}

// cleanAppName removes version numbers, architecture, and platform suffixes
// MOVED TO INTERNAL/HELPERS

// generateNameVariants produces different normalized variants for matching executable names
// MOVED TO INTERNAL/HELPERS

// createWrapper creates a wrapper shell script
func (t *TarballBackend) createWrapper(wrapperPath, execPath string) error {
	// Check if this is an Electron app (has .asar file nearby)
	isElectron := t.isElectronApp(execPath)

	var content string
	if isElectron {
		// Electron apps need to run from their own directory
		execDir := filepath.Dir(execPath)
		execName := filepath.Base(execPath)

		// Only add --no-sandbox if explicitly configured (security risk)
		sandboxFlag := ""
		if t.cfg.Desktop.ElectronDisableSandbox {
			sandboxFlag = " --no-sandbox"
		}

		content = fmt.Sprintf(`#!/bin/bash
# upkg wrapper script for Electron app
cd "%s"
exec "./%s"%s "$@"
`, execDir, execName, sandboxFlag)
	} else {
		// Standard wrapper
		content = fmt.Sprintf(`#!/bin/bash
# upkg wrapper script
exec "%s" "$@"
`, execPath)
	}

	if err := os.WriteFile(wrapperPath, []byte(content), 0755); err != nil {
		return err
	}

	return nil
}

// isElectronApp checks if the executable is part of an Electron app
func (t *TarballBackend) isElectronApp(execPath string) bool {
	execDir := filepath.Dir(execPath)

	// Check for resources/app.asar (typical Electron structure)
	asarPath := filepath.Join(execDir, "resources", "app.asar")
	if _, err := os.Stat(asarPath); err == nil {
		return true
	}

	// Check for *.asar in parent directory and subdirectories
	parentDir := filepath.Dir(execDir)
	var asarFound bool
	filepath.Walk(parentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
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

// installIcons installs icons from the extracted directory
func (t *TarballBackend) installIcons(installDir, normalizedName string) ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	installedIcons := []string{}

	// Discover icons from regular filesystem
	discoveredIcons := icons.DiscoverIcons(installDir)

	t.logger.Debug().
		Int("count", len(discoveredIcons)).
		Msg("discovered icons in filesystem")

	// Try to extract icons from ASAR archives (Electron apps)
	asarIcons, err := t.extractIconsFromAsar(installDir, normalizedName)
	if err != nil {
		t.logger.Debug().Err(err).Msg("asar icon extraction failed or not applicable")
	} else if len(asarIcons) > 0 {
		t.logger.Info().
			Int("count", len(asarIcons)).
			Msg("extracted icons from ASAR archive")
		discoveredIcons = append(discoveredIcons, asarIcons...)
	}

	// Install each icon
	for _, iconFile := range discoveredIcons {
		targetPath, err := icons.InstallIcon(iconFile, normalizedName, homeDir)
		if err != nil {
			t.logger.Warn().
				Err(err).
				Str("icon", iconFile.Path).
				Msg("failed to install icon")
			continue
		}

		installedIcons = append(installedIcons, targetPath)
	}

	return installedIcons, nil
}

// extractIconsFromAsarNative extracts icons using native Go ASAR library
// This is significantly faster than spawning npx for each ASAR file
// Returns extracted icons and any error encountered
func (t *TarballBackend) extractIconsFromAsarNative(asarPath, installDir, normalizedName string) ([]core.IconFile, error) {
	t.logger.Debug().
		Str("asar", asarPath).
		Msg("extracting icons using native Go ASAR library")

	// Open ASAR file
	f, err := os.Open(asarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ASAR: %w", err)
	}
	defer f.Close()

	// Decode ASAR archive
	archive, err := asar.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ASAR: %w", err)
	}

	// Create temporary directory for extracted icons
	tempDir, err := os.MkdirTemp("", "upkg-asar-icons-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Track extracted icon files
	var extractedPaths []string

	// Walk ASAR and extract only icon files
	walkErr := archive.Walk(func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Filter for icon files only
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".png" && ext != ".ico" && ext != ".svg" && ext != ".jpg" && ext != ".jpeg" {
			return nil
		}

		// Skip very small files (likely not real icons, < 100 bytes)
		if info.Size() < 100 {
			return nil
		}

		t.logger.Debug().
			Str("path", path).
			Int64("size", info.Size()).
			Msg("extracting icon from ASAR")

		// Find the entry to read its contents
		pathParts := strings.Split(strings.Trim(path, "/"), "/")
		entry := archive.Find(pathParts...)
		if entry == nil {
			t.logger.Warn().Str("path", path).Msg("entry not found")
			return nil
		}

		// Create target path in temp directory
		targetPath := filepath.Join(tempDir, filepath.Base(path))

		// Handle duplicate filenames by appending path component
		if _, err := os.Stat(targetPath); err == nil {
			// File exists, make unique by adding directory name
			dirName := filepath.Base(filepath.Dir(path))
			if dirName != "." && dirName != "/" {
				targetPath = filepath.Join(tempDir, dirName+"_"+filepath.Base(path))
			}
		}

		// Open entry for reading
		reader := entry.Open()
		if reader == nil {
			t.logger.Warn().Str("path", path).Msg("failed to open entry")
			return nil
		}

		// Create output file
		outFile, err := os.Create(targetPath)
		if err != nil {
			t.logger.Warn().Err(err).Str("path", path).Msg("failed to create icon file")
			return nil // Continue on error
		}
		defer outFile.Close()

		// Copy contents
		if _, err := io.Copy(outFile, reader); err != nil {
			t.logger.Warn().Err(err).Str("path", path).Msg("failed to write icon file")
			return nil // Continue on error
		}

		extractedPaths = append(extractedPaths, targetPath)
		return nil
	})

	if walkErr != nil {
		return nil, fmt.Errorf("failed to walk ASAR: %w", walkErr)
	}

	if len(extractedPaths) == 0 {
		return nil, nil // No icons found, not an error
	}

	t.logger.Info().
		Int("count", len(extractedPaths)).
		Str("asar", filepath.Base(asarPath)).
		Msg("extracted icons using native ASAR library")

	// Use DiscoverIcons to get properly formatted IconFile structs
	discoveredIcons := icons.DiscoverIcons(tempDir)

	// Copy icons to permanent location
	var allIcons []core.IconFile
	for _, icon := range discoveredIcons {
		// Create subdirectory for asar-extracted icons
		asarIconsDir := filepath.Join(installDir, ".upkg-asar-icons")
		if err := os.MkdirAll(asarIconsDir, 0755); err != nil {
			continue
		}

		// Copy icon to permanent location
		iconName := filepath.Base(icon.Path)
		permanentPath := filepath.Join(asarIconsDir, iconName)

		if err := t.copyFile(icon.Path, permanentPath); err != nil {
			t.logger.Warn().
				Err(err).
				Str("icon", icon.Path).
				Msg("failed to copy icon from ASAR")
			continue
		}

		// Update icon path to permanent location
		icon.Path = permanentPath
		allIcons = append(allIcons, icon)
	}

	return allIcons, nil
}

// extractIconsFromAsar extracts icons from Electron ASAR archives
// Uses native Go ASAR library with fallback to npx if needed
func (t *TarballBackend) extractIconsFromAsar(installDir, normalizedName string) ([]core.IconFile, error) {
	// Find .asar files recursively
	var asarFiles []string
	err := filepath.Walk(installDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue on errors
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".asar") {
			// Filter out .asar.unpacked directories and only get .asar files
			if !strings.Contains(path, ".asar.unpacked") {
				asarFiles = append(asarFiles, path)
			}
		}
		return nil
	})

	if err != nil || len(asarFiles) == 0 {
		return nil, fmt.Errorf("no asar files found")
	}

	var allIcons []core.IconFile

	for _, asarFile := range asarFiles {
		// OPTIMIZATION: Try native Go ASAR extraction first (80-95% faster)
		extractedIcons, nativeErr := t.extractIconsFromAsarNative(asarFile, installDir, normalizedName)
		if nativeErr == nil && len(extractedIcons) > 0 {
			t.logger.Debug().
				Str("asar", filepath.Base(asarFile)).
				Int("icons", len(extractedIcons)).
				Msg("successfully extracted icons using native Go ASAR library")
			allIcons = append(allIcons, extractedIcons...)
			continue
		}

		// Log native extraction failure
		if nativeErr != nil {
			t.logger.Debug().
				Err(nativeErr).
				Str("asar", filepath.Base(asarFile)).
				Msg("native ASAR extraction failed, falling back to npx")
		}

		// Fallback to npx method if native extraction fails
		if !helpers.CommandExists("npx") {
			t.logger.Warn().
				Str("asar", filepath.Base(asarFile)).
				Msg("native ASAR failed and npx not available, skipping")
			continue
		}

		t.logger.Debug().
			Str("asar", asarFile).
			Msg("attempting to extract icons using npx fallback")

		// Create temporary directory for extraction
		tempDir, err := os.MkdirTemp("", "upkg-asar-*")
		if err != nil {
			t.logger.Warn().Err(err).Msg("failed to create temp dir for asar extraction")
			continue
		}

		// Extract ASAR using npx asar
		// OPTIMIZATION: Use streaming variant to avoid buffering large outputs
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		extractErr := helpers.RunCommandStreaming(ctx, nil, nil, "npx", "--yes", "asar", "extract", asarFile, tempDir)
		if extractErr != nil {
			t.logger.Warn().
				Err(extractErr).
				Str("asar", asarFile).
				Msg("failed to extract asar file with npx")
			// OPTIMIZATION: Release resources immediately after failure
			cancel()
			os.RemoveAll(tempDir)
			continue
		}

		// Discover icons in extracted ASAR
		discoveredIcons := icons.DiscoverIcons(tempDir)

		t.logger.Debug().
			Int("count", len(discoveredIcons)).
			Str("asar", filepath.Base(asarFile)).
			Msg("found icons in asar using npx")

		// Copy icons to a permanent location in installDir before temp cleanup
		for _, icon := range discoveredIcons {
			// Create a subdirectory for asar-extracted icons
			asarIconsDir := filepath.Join(installDir, ".upkg-asar-icons")
			if err := os.MkdirAll(asarIconsDir, 0755); err != nil {
				continue
			}

			// Copy icon to permanent location
			iconName := filepath.Base(icon.Path)
			permanentPath := filepath.Join(asarIconsDir, iconName)

			if err := t.copyFile(icon.Path, permanentPath); err != nil {
				t.logger.Warn().
					Err(err).
					Str("icon", icon.Path).
					Msg("failed to copy icon from asar")
				continue
			}

			// Update icon path to permanent location
			icon.Path = permanentPath
			allIcons = append(allIcons, icon)
		}

		// OPTIMIZATION: Release resources at end of each iteration instead of defer
		// This frees disk space and cancels timers immediately
		cancel()
		os.RemoveAll(tempDir)
	}

	return allIcons, nil
}

// copyFile is a helper to copy files
func (t *TarballBackend) copyFile(src, dst string) (err error) {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := sourceFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := destFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}

// removeIcons removes installed icons
func (t *TarballBackend) removeIcons(iconPaths []string) {
	for _, iconPath := range iconPaths {
		if err := os.Remove(iconPath); err != nil && !os.IsNotExist(err) {
			t.logger.Warn().
				Err(err).
				Str("path", iconPath).
				Msg("failed to remove icon")
		}
	}
}

// createDesktopFile creates a .desktop file
func (t *TarballBackend) createDesktopFile(installDir, appName, normalizedName, execPath string, opts core.InstallOptions) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	appsDir := filepath.Join(homeDir, ".local", "share", "applications")
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create applications directory: %w", err)
	}

	desktopFilePath := filepath.Join(appsDir, normalizedName+".desktop")

	// Try to find existing .desktop file in installDir
	var entry *core.DesktopEntry
	desktopFiles, _ := filepath.Glob(filepath.Join(installDir, "*.desktop"))
	if len(desktopFiles) > 0 {
		file, err := os.Open(desktopFiles[0])
		if err == nil {
			defer file.Close()
			entry, _ = desktop.Parse(file)
		}
	}

	// Create default entry if not found
	if entry == nil {
		entry = &core.DesktopEntry{
			Type:    "Application",
			Version: "1.5",
			Name:    appName,
			Comment: fmt.Sprintf("%s application", appName),
			Icon:    normalizedName,
		}
	}

	// Update Exec to point to wrapper
	entry.Exec = execPath + " %U"

	// Set icon
	entry.Icon = normalizedName

	// Ensure categories
	if len(entry.Categories) == 0 {
		entry.Categories = []string{"Utility"}
	}

	// Inject Wayland environment variables
	if t.cfg.Desktop.WaylandEnvVars && !opts.SkipWaylandEnv {
		desktop.InjectWaylandEnvVars(entry, t.cfg.Desktop.CustomEnvVars)
	}

	// Write desktop file
	if err := desktop.WriteDesktopFile(desktopFilePath, entry); err != nil {
		return "", err
	}

	// Validate
	if helpers.CommandExists("desktop-file-validate") {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := helpers.RunCommand(ctx, "desktop-file-validate", desktopFilePath); err != nil {
			t.logger.Warn().
				Err(err).
				Str("desktop_file", desktopFilePath).
				Msg("desktop file validation failed")
		}
	}

	return desktopFilePath, nil
}

// No local helper functions - using shared helpers from internal/helpers/common.go
