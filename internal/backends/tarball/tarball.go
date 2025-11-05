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

	"github.com/diogo/pkgctl/internal/cache"
	"github.com/diogo/pkgctl/internal/config"
	"github.com/diogo/pkgctl/internal/core"
	"github.com/diogo/pkgctl/internal/desktop"
	"github.com/diogo/pkgctl/internal/helpers"
	"github.com/diogo/pkgctl/internal/icons"
	"github.com/rs/zerolog"
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

	// Accept tar.gz, tar, zip
	return fileType == helpers.FileTypeTarGz ||
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
		appName = strings.TrimSuffix(appName, filepath.Ext(appName))
		// Handle .tar.gz
		appName = strings.TrimSuffix(appName, ".tar")
	}

	// Normalize name
	normalizedName := normalizeFilename(appName)
	installID := generateInstallID(normalizedName)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create installation directory in ~/.local/share/pkgctl/apps/
	appsDir := filepath.Join(homeDir, ".local", "share", "pkgctl", "apps")
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
	case "tar.gz", "tar.bz2", "tar.xz":
		return helpers.ExtractTarGz(archivePath, destDir)
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

	// Strong match: filename exactly matches base name
	if filename == normalizedBase || filename == normalizedBase+".exe" {
		score += 100
	}

	// Partial match: filename contains base name
	if strings.Contains(filename, normalizedBase) {
		score += 50
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

// createWrapper creates a wrapper shell script
func (t *TarballBackend) createWrapper(wrapperPath, execPath string) error {
	// Check if this is an Electron app (has .asar file nearby)
	isElectron := t.isElectronApp(execPath)

	var content string
	if isElectron {
		// Electron apps need to run from their own directory
		// and may need --no-sandbox flag
		execDir := filepath.Dir(execPath)
		execName := filepath.Base(execPath)
		content = fmt.Sprintf(`#!/bin/bash
# pkgctl wrapper script for Electron app
cd "%s"
exec "./%s" --no-sandbox "$@"
`, execDir, execName)
	} else {
		// Standard wrapper
		content = fmt.Sprintf(`#!/bin/bash
# pkgctl wrapper script
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

	// Check for *.asar in parent directory
	parentDir := filepath.Dir(execDir)
	matches, _ := filepath.Glob(filepath.Join(parentDir, "**/*.asar"))
	if len(matches) > 0 {
		return true
	}

	return false
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

// extractIconsFromAsar extracts icons from Electron ASAR archives
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

	// Check if npx is available
	if !helpers.CommandExists("npx") {
		t.logger.Debug().Msg("npx not available, skipping asar extraction")
		return nil, fmt.Errorf("npx not available")
	}

	var allIcons []core.IconFile

	for _, asarFile := range asarFiles {
		t.logger.Debug().
			Str("asar", asarFile).
			Msg("attempting to extract icons from asar")

		// Create temporary directory for extraction
		tempDir, err := os.MkdirTemp("", "pkgctl-asar-*")
		if err != nil {
			t.logger.Warn().Err(err).Msg("failed to create temp dir for asar extraction")
			continue
		}
		defer os.RemoveAll(tempDir)

		// Extract ASAR using npx asar
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, extractErr := helpers.RunCommand(ctx, "npx", "--yes", "asar", "extract", asarFile, tempDir)
		if extractErr != nil {
			t.logger.Warn().
				Err(extractErr).
				Str("asar", asarFile).
				Msg("failed to extract asar file")
			continue
		}

		// Discover icons in extracted ASAR
		discoveredIcons := icons.DiscoverIcons(tempDir)

		t.logger.Debug().
			Int("count", len(discoveredIcons)).
			Str("asar", filepath.Base(asarFile)).
			Msg("found icons in asar")

		// Copy icons to a permanent location in installDir before temp cleanup
		for _, icon := range discoveredIcons {
			// Create a subdirectory for asar-extracted icons
			asarIconsDir := filepath.Join(installDir, ".pkgctl-asar-icons")
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
	}

	return allIcons, nil
}

// copyFile is a helper to copy files
func (t *TarballBackend) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
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
	if t.cfg.Desktop.WaylandEnvVars {
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

// Helper functions

func normalizeFilename(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")

	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

func generateInstallID(name string) string {
	return fmt.Sprintf("%s-%d", name, time.Now().Unix())
}
