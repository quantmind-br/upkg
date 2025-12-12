package tarball

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	backendbase "github.com/quantmind-br/upkg/internal/backends/base"
	"github.com/quantmind-br/upkg/internal/cache"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/desktop"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/heuristics"
	"github.com/quantmind-br/upkg/internal/icons"
	"github.com/quantmind-br/upkg/internal/security"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"layeh.com/asar"
)

// TarballBackend handles tarball and zip archive installations
//
//nolint:revive // exported backend names are kept for consistency across packages.
type TarballBackend struct {
	*backendbase.BaseBackend
	scorer       heuristics.Scorer
	cacheManager *cache.CacheManager
}

// New creates a new tarball backend
func New(cfg *config.Config, log *zerolog.Logger) *TarballBackend {
	base := backendbase.New(cfg, log)
	return &TarballBackend{
		BaseBackend:  base,
		scorer:       heuristics.NewScorer(log),
		cacheManager: cache.NewCacheManagerWithRunner(base.Runner),
	}
}

// NewWithRunner creates a new tarball backend with a custom command runner
func NewWithRunner(cfg *config.Config, log *zerolog.Logger, runner helpers.CommandRunner) *TarballBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), runner)
}

// NewWithDeps creates a new tarball backend with injected fs and runner.
func NewWithDeps(cfg *config.Config, log *zerolog.Logger, fs afero.Fs, runner helpers.CommandRunner) *TarballBackend {
	base := backendbase.NewWithDeps(cfg, log, fs, runner)
	return &TarballBackend{
		BaseBackend:  base,
		scorer:       heuristics.NewScorer(log),
		cacheManager: cache.NewCacheManagerWithRunner(runner),
	}
}

// NewWithCacheManager creates a new tarball backend with a custom cache manager
func NewWithCacheManager(cfg *config.Config, log *zerolog.Logger, cacheManager *cache.CacheManager) *TarballBackend {
	base := backendbase.New(cfg, log)
	return &TarballBackend{
		BaseBackend:  base,
		scorer:       heuristics.NewScorer(log),
		cacheManager: cacheManager,
	}
}

// Name returns the backend name
func (t *TarballBackend) Name() string {
	return "tarball"
}

// Detect checks if this backend can handle the package
func (t *TarballBackend) Detect(_ context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := t.Fs.Stat(packagePath); err != nil {
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
//
//nolint:gocyclo // archive install handles multiple formats, icons, desktop and rollback.
func (t *TarballBackend) Install(_ context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	t.Log.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing tarball/zip package")

	// Validate package exists
	if _, err := t.Fs.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	// Detect archive type
	archiveType := helpers.GetArchiveType(packagePath)
	if archiveType == "" {
		return nil, fmt.Errorf("unsupported archive type: %s", packagePath)
	}

	t.Log.Debug().
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
	if err := security.ValidatePackageName(normalizedName); err != nil {
		return nil, fmt.Errorf("invalid normalized name %q: %w", normalizedName, err)
	}
	installID := helpers.GenerateInstallID(normalizedName)

	if t.Paths.HomeDir() == "" {
		return nil, fmt.Errorf("failed to get home directory")
	}

	// Create installation directory in ~/.local/share/upkg/apps/
	appsDir := t.Paths.GetUpkgAppsDir()
	installDir := filepath.Join(appsDir, normalizedName)

	// Check if already exists
	if _, err := t.Fs.Stat(installDir); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("package already installed at: %s (use --force to reinstall)", installDir)
		}
		if err := t.Fs.RemoveAll(installDir); err != nil {
			return nil, fmt.Errorf("remove existing installation directory: %w", err)
		}
		// Best-effort cleanup of expected wrapper/desktop paths
		binDir := t.Paths.GetBinDir()
		oldWrapper := filepath.Join(binDir, normalizedName)
		if removeErr := t.Fs.Remove(oldWrapper); removeErr != nil {
			t.Log.Debug().Err(removeErr).Str("path", oldWrapper).Msg("failed to remove existing wrapper")
		}
		appsDbDir := t.Paths.GetAppsDir()
		oldDesktop := filepath.Join(appsDbDir, normalizedName+".desktop")
		if removeErr := t.Fs.Remove(oldDesktop); removeErr != nil {
			t.Log.Debug().Err(removeErr).Str("desktop_file", oldDesktop).Msg("failed to remove existing desktop file")
		}
	}

	// Create installation directory
	if err := t.Fs.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create installation directory: %w", err)
	}
	if tx != nil {
		dir := installDir
		tx.Add("remove installation directory", func() error {
			return t.Fs.RemoveAll(dir)
		})
	}

	// Extract archive
	t.Log.Debug().
		Str("archive", packagePath).
		Str("dest", installDir).
		Msg("extracting archive")

	if extractErr := t.extractArchive(packagePath, installDir, archiveType); extractErr != nil {
		if removeErr := t.Fs.RemoveAll(installDir); removeErr != nil {
			t.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after extract error")
		}
		return nil, fmt.Errorf("failed to extract archive: %w", extractErr)
	}

	// Find executable(s)
	executables, err := heuristics.FindExecutables(installDir)
	if err != nil || len(executables) == 0 {
		if removeErr := t.Fs.RemoveAll(installDir); removeErr != nil {
			t.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after no executables")
		}
		return nil, fmt.Errorf("no executables found in archive")
	}

	t.Log.Debug().
		Strs("executables", executables).
		Msg("found executables")

	// Choose primary executable using scoring heuristic
	primaryExec := t.scorer.ChooseBest(executables, normalizedName, installDir)

	t.Log.Debug().
		Str("primary_executable", primaryExec).
		Int("total_candidates", len(executables)).
		Msg("selected primary executable")

	// Create wrapper script in ~/.local/bin/
	binDir := t.Paths.GetBinDir()
	if mkdirErr := t.Fs.MkdirAll(binDir, 0755); mkdirErr != nil {
		if removeErr := t.Fs.RemoveAll(installDir); removeErr != nil {
			t.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after mkdir error")
		}
		return nil, fmt.Errorf("failed to create bin directory: %w", mkdirErr)
	}

	wrapperPath := filepath.Join(binDir, normalizedName)
	if wrapperErr := t.createWrapper(wrapperPath, primaryExec); wrapperErr != nil {
		if removeErr := t.Fs.RemoveAll(installDir); removeErr != nil {
			t.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after wrapper error")
		}
		return nil, fmt.Errorf("failed to create wrapper script: %w", wrapperErr)
	}
	if tx != nil {
		path := wrapperPath
		tx.Add("remove wrapper script", func() error {
			return t.Fs.Remove(path)
		})
	}

	t.Log.Debug().
		Str("wrapper", wrapperPath).
		Msg("created wrapper script")

	// Install icons (if any)
	iconPaths, err := t.installIcons(installDir, normalizedName)
	if err != nil {
		t.Log.Warn().Err(err).Msg("failed to install icons")
	}
	if tx != nil && len(iconPaths) > 0 {
		paths := append([]string(nil), iconPaths...)
		tx.Add("remove tarball icons", func() error {
			t.removeIcons(paths)
			return nil
		})
	}

	// Create .desktop file
	var desktopPath string
	if !opts.SkipDesktop {
		desktopPath, err = t.createDesktopFile(installDir, appName, normalizedName, wrapperPath, opts)
		if err != nil {
			// Clean up on failure
			if removeErr := t.Fs.RemoveAll(installDir); removeErr != nil {
				t.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after desktop error")
			}
			if removeErr := t.Fs.Remove(wrapperPath); removeErr != nil {
				t.Log.Debug().Err(removeErr).Str("path", wrapperPath).Msg("failed to cleanup wrapper after desktop error")
			}
			t.removeIcons(iconPaths)
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		t.Log.Debug().
			Str("desktop_file", desktopPath).
			Msg("desktop file created")

		if tx != nil && desktopPath != "" {
			path := desktopPath
			tx.Add("remove desktop file", func() error {
				return t.Fs.Remove(path)
			})
		}

		// Update caches
		appsDbDir := t.Paths.GetAppsDir()
		if cacheErr := t.cacheManager.UpdateDesktopDatabase(appsDbDir, t.Log); cacheErr != nil {
			t.Log.Warn().Err(cacheErr).Str("apps_dir", appsDbDir).Msg("failed to update desktop database")
		}

		iconsDir := t.Paths.GetIconsDir()
		if cacheErr := t.cacheManager.UpdateIconCache(iconsDir, t.Log); cacheErr != nil {
			t.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update icon cache")
		}
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
			InstallMethod:  core.InstallMethodLocal,
		},
	}

	t.Log.Info().
		Str("install_id", installID).
		Str("name", appName).
		Str("path", installDir).
		Msg("tarball/zip package installed successfully")

	return record, nil
}

// Uninstall removes the installed tarball/zip package
func (t *TarballBackend) Uninstall(_ context.Context, record *core.InstallRecord) error {
	t.Log.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling tarball/zip package")

	// Remove installation directory
	if record.InstallPath != "" {
		if err := t.Fs.RemoveAll(record.InstallPath); err != nil {
			t.Log.Warn().Err(err).Str("path", record.InstallPath).Msg("failed to remove installation directory")
		}
	}

	// Remove wrapper script
	if record.Metadata.WrapperScript != "" {
		if err := t.Fs.Remove(record.Metadata.WrapperScript); err != nil {
			t.Log.Warn().Err(err).Str("path", record.Metadata.WrapperScript).Msg("failed to remove wrapper script")
		}
	}

	// Remove .desktop file(s)
	for _, desktopPath := range record.GetDesktopFiles() {
		if desktopPath == "" {
			continue
		}
		if err := t.Fs.Remove(desktopPath); err != nil {
			t.Log.Warn().Err(err).Str("path", desktopPath).Msg("failed to remove desktop file")
		}
	}

	// Remove icons
	t.removeIcons(record.Metadata.IconFiles)

	// Update caches
	appsDir := t.Paths.GetAppsDir()
	if cacheErr := t.cacheManager.UpdateDesktopDatabase(appsDir, t.Log); cacheErr != nil {
		t.Log.Warn().Err(cacheErr).Str("apps_dir", appsDir).Msg("failed to update desktop database")
	}

	iconsDir := t.Paths.GetIconsDir()
	if cacheErr := t.cacheManager.UpdateIconCache(iconsDir, t.Log); cacheErr != nil {
		t.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update icon cache")
	}

	t.Log.Info().
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
		if t.Cfg.Desktop.ElectronDisableSandbox {
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

	return afero.WriteFile(t.Fs, wrapperPath, []byte(content), 0755)
}

// isElectronApp checks if the executable is part of an Electron app
func (t *TarballBackend) isElectronApp(execPath string) bool {
	execDir := filepath.Dir(execPath)

	// Check for resources/app.asar (typical Electron structure)
	asarPath := filepath.Join(execDir, "resources", "app.asar")
	if _, err := t.Fs.Stat(asarPath); err == nil {
		return true
	}

	// Check for *.asar in parent directory and subdirectories
	parentDir := filepath.Dir(execDir)
	var asarFound bool
	if walkErr := filepath.Walk(parentDir, func(path string, info fs.FileInfo, entryErr error) error {
		if entryErr != nil {
			return nil // Continue on errors
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".asar") {
			asarFound = true
			return filepath.SkipAll // Found one, stop walking
		}
		return nil
	}); walkErr != nil {
		t.Log.Debug().Err(walkErr).Str("dir", parentDir).Msg("failed walking for asar detection")
	}
	return asarFound
}

// installIcons installs icons from the extracted directory
func (t *TarballBackend) installIcons(installDir, normalizedName string) ([]string, error) {
	homeDir := t.Paths.HomeDir()
	if homeDir == "" {
		return nil, fmt.Errorf("failed to get home directory")
	}

	installedIcons := []string{}

	// Discover icons from regular filesystem
	discoveredIcons := icons.DiscoverIcons(installDir)

	t.Log.Debug().
		Int("count", len(discoveredIcons)).
		Msg("discovered icons in filesystem")

	// Try to extract icons from ASAR archives (Electron apps)
	asarIcons, err := t.extractIconsFromAsar(installDir, normalizedName)
	if err != nil {
		t.Log.Debug().Err(err).Msg("asar icon extraction failed or not applicable")
	} else if len(asarIcons) > 0 {
		t.Log.Info().
			Int("count", len(asarIcons)).
			Msg("extracted icons from ASAR archive")
		discoveredIcons = append(discoveredIcons, asarIcons...)
	}

	// Install each icon
	for _, iconFile := range discoveredIcons {
		targetPath, err := icons.InstallIcon(iconFile, normalizedName, homeDir)
		if err != nil {
			t.Log.Warn().
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
//
//nolint:gocyclo // ASAR extraction handles multiple filesystem and naming cases.
func (t *TarballBackend) extractIconsFromAsarNative(asarPath, installDir, _ string) ([]core.IconFile, error) {
	t.Log.Debug().
		Str("asar", asarPath).
		Msg("extracting icons using native Go ASAR library")

	// Open ASAR file
	f, err := t.Fs.Open(asarPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open ASAR: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Log.Debug().Err(closeErr).Str("asar", asarPath).Msg("failed to close ASAR file")
		}
	}()

	// Decode ASAR archive
	archive, err := asar.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ASAR: %w", err)
	}

	// Create temporary directory for extracted icons
	tempDir, err := afero.TempDir(t.Fs, "", "upkg-asar-icons-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		if removeErr := t.Fs.RemoveAll(tempDir); removeErr != nil {
			t.Log.Debug().Err(removeErr).Str("temp_dir", tempDir).Msg("failed to remove temp dir")
		}
	}()

	// Track extracted icon files
	var extractedPaths []string

	// Walk ASAR and extract only icon files
	walkErr := archive.Walk(func(path string, info fs.FileInfo, entryErr error) error {
		if entryErr != nil {
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

		t.Log.Debug().
			Str("path", path).
			Int64("size", info.Size()).
			Msg("extracting icon from ASAR")

		// Find the entry to read its contents
		pathParts := strings.Split(strings.Trim(path, "/"), "/")
		entry := archive.Find(pathParts...)
		if entry == nil {
			t.Log.Warn().Str("path", path).Msg("entry not found")
			return nil
		}

		// Create target path in temp directory
		targetPath := filepath.Join(tempDir, filepath.Base(path))

		// Handle duplicate filenames by appending path component
		if _, statErr := t.Fs.Stat(targetPath); statErr == nil {
			// File exists, make unique by adding directory name
			dirName := filepath.Base(filepath.Dir(path))
			if dirName != "." && dirName != "/" {
				targetPath = filepath.Join(tempDir, dirName+"_"+filepath.Base(path))
			}
		}

		// Open entry for reading
		reader := entry.Open()
		if reader == nil {
			t.Log.Warn().Str("path", path).Msg("failed to open entry")
			return nil
		}

		// Create output file
		outFile, err := t.Fs.Create(targetPath)
		if err != nil {
			t.Log.Warn().Err(err).Str("path", path).Msg("failed to create icon file")
			return nil // Continue on error
		}
		defer func() {
			if closeErr := outFile.Close(); closeErr != nil {
				t.Log.Debug().Err(closeErr).Str("path", targetPath).Msg("failed to close icon file")
			}
		}()

		// Copy contents
		if _, err := io.Copy(outFile, reader); err != nil {
			t.Log.Warn().Err(err).Str("path", path).Msg("failed to write icon file")
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

	t.Log.Info().
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
		if err := t.Fs.MkdirAll(asarIconsDir, 0755); err != nil {
			continue
		}

		// Copy icon to permanent location
		iconName := filepath.Base(icon.Path)
		permanentPath := filepath.Join(asarIconsDir, iconName)

		if err := t.copyFile(icon.Path, permanentPath); err != nil {
			t.Log.Warn().
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
//
//nolint:gocyclo // ASAR scanning and dual extraction paths are inherently branching.
func (t *TarballBackend) extractIconsFromAsar(installDir, normalizedName string) ([]core.IconFile, error) {
	// Find .asar files recursively
	var asarFiles []string
	err := filepath.Walk(installDir, func(path string, info fs.FileInfo, err error) error {
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
			t.Log.Debug().
				Str("asar", filepath.Base(asarFile)).
				Int("icons", len(extractedIcons)).
				Msg("successfully extracted icons using native Go ASAR library")
			allIcons = append(allIcons, extractedIcons...)
			continue
		}

		// Log native extraction failure
		if nativeErr != nil {
			t.Log.Debug().
				Err(nativeErr).
				Str("asar", filepath.Base(asarFile)).
				Msg("native ASAR extraction failed, falling back to npx")
		}

		// Fallback to npx method if native extraction fails
		if !t.Runner.CommandExists("npx") {
			t.Log.Warn().
				Str("asar", filepath.Base(asarFile)).
				Msg("native ASAR failed and npx not available, skipping")
			continue
		}

		t.Log.Debug().
			Str("asar", asarFile).
			Msg("attempting to extract icons using npx fallback")

		// Create temporary directory for extraction
		tempDir, err := afero.TempDir(t.Fs, "", "upkg-asar-*")
		if err != nil {
			t.Log.Warn().Err(err).Msg("failed to create temp dir for asar extraction")
			continue
		}

		// Extract ASAR using npx asar
		// OPTIMIZATION: Use streaming variant to avoid buffering large outputs
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		extractErr := t.Runner.RunCommandStreaming(ctx, nil, nil, "npx", "--yes", "asar", "extract", asarFile, tempDir)
		if extractErr != nil {
			t.Log.Warn().
				Err(extractErr).
				Str("asar", asarFile).
				Msg("failed to extract asar file with npx")
			// OPTIMIZATION: Release resources immediately after failure
			cancel()
			if removeErr := t.Fs.RemoveAll(tempDir); removeErr != nil {
				t.Log.Debug().Err(removeErr).Str("temp_dir", tempDir).Msg("failed to remove temp dir after asar failure")
			}
			continue
		}

		// Discover icons in extracted ASAR
		discoveredIcons := icons.DiscoverIcons(tempDir)

		t.Log.Debug().
			Int("count", len(discoveredIcons)).
			Str("asar", filepath.Base(asarFile)).
			Msg("found icons in asar using npx")

		// Copy icons to a permanent location in installDir before temp cleanup
		for _, icon := range discoveredIcons {
			// Create a subdirectory for asar-extracted icons
			asarIconsDir := filepath.Join(installDir, ".upkg-asar-icons")
			if err := t.Fs.MkdirAll(asarIconsDir, 0755); err != nil {
				continue
			}

			// Copy icon to permanent location
			iconName := filepath.Base(icon.Path)
			permanentPath := filepath.Join(asarIconsDir, iconName)

			if err := t.copyFile(icon.Path, permanentPath); err != nil {
				t.Log.Warn().
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
		if removeErr := t.Fs.RemoveAll(tempDir); removeErr != nil {
			t.Log.Debug().Err(removeErr).Str("temp_dir", tempDir).Msg("failed to remove temp dir after asar extraction")
		}
	}

	return allIcons, nil
}

// copyFile is a helper to copy files
func (t *TarballBackend) copyFile(src, dst string) (err error) {
	sourceFile, err := t.Fs.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := sourceFile.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	destFile, err := t.Fs.Create(dst)
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
		if err := t.Fs.Remove(iconPath); err != nil {
			t.Log.Warn().
				Err(err).
				Str("path", iconPath).
				Msg("failed to remove icon")
		}
	}
}

// createDesktopFile creates a .desktop file
//
//nolint:gocyclo // desktop generation handles multiple discovery and environment cases.
func (t *TarballBackend) createDesktopFile(installDir, appName, normalizedName, execPath string, opts core.InstallOptions) (string, error) {
	appsDir := t.Paths.GetAppsDir()
	if err := t.Fs.MkdirAll(appsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create applications directory: %w", err)
	}

	desktopFilePath := filepath.Join(appsDir, normalizedName+".desktop")

	// Try to find existing .desktop file in installDir
	var entry *core.DesktopEntry
	desktopFiles, globErr := afero.Glob(t.Fs, filepath.Join(installDir, "*.desktop"))
	if globErr != nil {
		t.Log.Debug().Err(globErr).Str("dir", installDir).Msg("failed to glob desktop files")
	}
	if len(desktopFiles) > 0 {
		file, err := t.Fs.Open(desktopFiles[0])
		if err == nil {
			defer func() {
				if closeErr := file.Close(); closeErr != nil {
					t.Log.Debug().Err(closeErr).Str("desktop_file", desktopFiles[0]).Msg("failed to close desktop file")
				}
			}()
			if parsed, parseErr := desktop.Parse(file); parseErr == nil {
				entry = parsed
			} else {
				t.Log.Debug().Err(parseErr).Str("desktop_file", desktopFiles[0]).Msg("failed to parse desktop file")
			}
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
	if t.Cfg.Desktop.WaylandEnvVars && !opts.SkipWaylandEnv {
		if err := desktop.InjectWaylandEnvVars(entry, t.Cfg.Desktop.CustomEnvVars); err != nil {
			t.Log.Warn().
				Err(err).
				Str("app", appName).
				Msg("invalid custom Wayland env vars, injecting defaults only")
			if fallbackErr := desktop.InjectWaylandEnvVars(entry, nil); fallbackErr != nil {
				t.Log.Warn().Err(fallbackErr).Str("app", appName).Msg("failed to inject default Wayland env vars")
			}
		}
	}

	var buf bytes.Buffer
	if err := desktop.Write(&buf, entry); err != nil {
		return "", err
	}
	if err := afero.WriteFile(t.Fs, desktopFilePath, buf.Bytes(), 0644); err != nil {
		return "", err
	}

	// Validate
	if t.Runner.CommandExists("desktop-file-validate") {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := t.Runner.RunCommand(ctx, "desktop-file-validate", desktopFilePath); err != nil {
			t.Log.Warn().
				Err(err).
				Str("desktop_file", desktopFilePath).
				Msg("desktop file validation failed")
		}
	}

	return desktopFilePath, nil
}

// No local helper functions - using shared helpers from internal/helpers/common.go
