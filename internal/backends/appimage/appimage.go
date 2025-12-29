package appimage

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	backendbase "github.com/quantmind-br/upkg/internal/backends/base"
	"github.com/quantmind-br/upkg/internal/cache"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/desktop"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/icons"
	"github.com/quantmind-br/upkg/internal/security"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// AppImageBackend handles AppImage installations
//
//nolint:revive // exported backend names are kept for consistency across packages.
type AppImageBackend struct {
	*backendbase.BaseBackend
	cacheManager *cache.CacheManager
}

// New creates a new AppImage backend
func New(cfg *config.Config, log *zerolog.Logger) *AppImageBackend {
	base := backendbase.New(cfg, log)
	return &AppImageBackend{
		BaseBackend:  base,
		cacheManager: cache.NewCacheManagerWithRunner(base.Runner),
	}
}

// NewWithRunner creates a new AppImage backend with a custom command runner
func NewWithRunner(cfg *config.Config, log *zerolog.Logger, runner helpers.CommandRunner) *AppImageBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), runner)
}

// NewWithDeps creates a new AppImage backend with injected fs and runner.
func NewWithDeps(cfg *config.Config, log *zerolog.Logger, fs afero.Fs, runner helpers.CommandRunner) *AppImageBackend {
	base := backendbase.NewWithDeps(cfg, log, fs, runner)
	return &AppImageBackend{
		BaseBackend:  base,
		cacheManager: cache.NewCacheManagerWithRunner(runner),
	}
}

// NewWithCacheManager creates a new AppImage backend with a custom cache manager
func NewWithCacheManager(cfg *config.Config, log *zerolog.Logger, cacheManager *cache.CacheManager) *AppImageBackend {
	base := backendbase.New(cfg, log)
	return &AppImageBackend{
		BaseBackend:  base,
		cacheManager: cacheManager,
	}
}

// Name returns the backend name
func (a *AppImageBackend) Name() string {
	return "appimage"
}

// Detect checks if this backend can handle the package
func (a *AppImageBackend) Detect(_ context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := a.Fs.Stat(packagePath); err != nil {
		return false, nil
	}

	// Check extension first for quick detection
	// This allows .AppImage files to be detected even if they're not fully valid
	// (useful for testing and edge cases)
	if strings.HasSuffix(strings.ToLower(packagePath), ".appimage") {
		// Verify it has ELF magic (fast check, doesn't require full ELF validity)
		file, err := a.Fs.Open(packagePath)
		if err != nil {
			return false, nil
		}
		defer file.Close()

		magic := make([]byte, 4)
		n, err := file.Read(magic)
		if err != nil || n < 4 {
			return false, nil
		}

		// Check for ELF magic: 0x7F 'E' 'L' 'F'
		//nolint:gosec // bounds checked by n < 4 above
		if magic[0] == 0x7F && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F' {
			return true, nil
		}
		return false, nil
	}

	// Check if it's an AppImage with embedded squashfs
	isAppImage, err := helpers.IsAppImage(packagePath)
	if err != nil {
		return false, err
	}

	return isAppImage, nil
}

// Install installs the AppImage package
//
//nolint:gocyclo // install flow is inherently branching (metadata, icons, desktop, tx).
func (a *AppImageBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	a.Log.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing AppImage package")

	// Validate package exists
	if _, err := a.Fs.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	// Make AppImage executable first
	if err := a.Fs.Chmod(packagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to make AppImage executable: %w", err)
	}

	// Create temp directory for extraction
	tmpDir, err := afero.TempDir(a.Fs, "", "upkg-appimage-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := a.Fs.RemoveAll(tmpDir); removeErr != nil {
			a.Log.Debug().Err(removeErr).Str("tmp_dir", tmpDir).Msg("failed to remove temp dir")
		}
	}()

	// Extract AppImage
	if extractErr := a.extractAppImage(ctx, packagePath, tmpDir); extractErr != nil {
		return nil, fmt.Errorf("failed to extract AppImage: %w", extractErr)
	}

	// Find squashfs-root directory
	squashfsRoot := filepath.Join(tmpDir, "squashfs-root")
	if _, statErr := a.Fs.Stat(squashfsRoot); statErr != nil {
		return nil, fmt.Errorf("squashfs-root not found after extraction: %w", statErr)
	}

	// Parse metadata from extracted content
	metadata, err := a.parseAppImageMetadata(squashfsRoot)
	if err != nil {
		a.Log.Warn().Err(err).Msg("failed to parse AppImage metadata, using defaults")
		metadata = &appImageMetadata{
			appName: opts.CustomName,
		}
	}

	// Determine application name
	appName := opts.CustomName
	if appName == "" {
		if metadata.appName != "" {
			appName = metadata.appName
		} else {
			appName = filepath.Base(packagePath)
			appName = strings.TrimSuffix(appName, filepath.Ext(appName))
			appName = helpers.CleanAppName(appName)
		}
		appName = helpers.FormatDisplayName(appName)
	}

	// Normalize name
	binName := helpers.NormalizeFilename(appName)
	if validateErr := security.ValidatePackageName(binName); validateErr != nil {
		return nil, fmt.Errorf("invalid normalized name %q: %w", binName, validateErr)
	}
	installID := helpers.GenerateInstallID(binName)

	if a.Paths.HomeDir() == "" {
		return nil, fmt.Errorf("failed to get home directory")
	}

	// Copy AppImage to ~/.local/bin/
	binDir := a.Paths.GetBinDir()
	if mkdirErr := a.Fs.MkdirAll(binDir, 0755); mkdirErr != nil {
		return nil, fmt.Errorf("failed to create bin directory: %w", mkdirErr)
	}

	destPath := filepath.Join(binDir, binName+".appimage")
	if _, statErr := a.Fs.Stat(destPath); statErr == nil {
		if !opts.Force {
			return nil, fmt.Errorf("package already installed at: %s (use --force to reinstall)", destPath)
		}
		if removeErr := a.Fs.Remove(destPath); removeErr != nil {
			return nil, fmt.Errorf("remove existing AppImage: %w", removeErr)
		}
	}

	content, err := afero.ReadFile(a.Fs, packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read AppImage: %w", err)
	}
	if writeErr := afero.WriteFile(a.Fs, destPath, content, 0755); writeErr != nil {
		return nil, fmt.Errorf("failed to copy AppImage: %w", writeErr)
	}

	if chmodErr := a.Fs.Chmod(destPath, 0755); chmodErr != nil {
		if removeErr := a.Fs.Remove(destPath); removeErr != nil {
			a.Log.Warn().Err(removeErr).Str("path", destPath).Msg("failed to remove AppImage after chmod error")
		}
		return nil, fmt.Errorf("failed to make AppImage executable: %w", chmodErr)
	}

	if tx != nil {
		path := destPath
		tx.Add("remove appimage binary", func() error {
			return a.Fs.Remove(path)
		})
	}

	a.Log.Debug().
		Str("source", packagePath).
		Str("dest", destPath).
		Msg("AppImage copied")

	// Install icons
	discoveredIcons := icons.DiscoverIcons(squashfsRoot)
	a.Log.Debug().
		Int("count", len(discoveredIcons)).
		Msg("discovered icons in AppImage")
	for i, icon := range discoveredIcons {
		a.Log.Debug().
			Int("index", i).
			Str("path", icon.Path).
			Str("size", icon.Size).
			Msg("icon discovered")
	}

	iconPaths, err := a.installIcons(squashfsRoot, binName, metadata)
	if err != nil {
		a.Log.Warn().Err(err).Msg("failed to install icons")
	}
	if tx != nil && len(iconPaths) > 0 {
		paths := append([]string(nil), iconPaths...)
		tx.Add("remove appimage icons", func() error {
			a.removeIcons(paths)
			return nil
		})
	}

	// Create/update desktop file
	var desktopPath string
	if !opts.SkipDesktop {
		if opts.Force {
			appsDir := a.Paths.GetAppsDir()
			oldDesktopPath := filepath.Join(appsDir, binName+".desktop")
			if removeErr := a.Fs.Remove(oldDesktopPath); removeErr != nil {
				a.Log.Debug().Err(removeErr).Str("desktop_file", oldDesktopPath).Msg("failed to remove existing desktop file")
			}
		}
		desktopPath, err = a.createDesktopFile(squashfsRoot, appName, binName, destPath, metadata, opts)
		if err != nil {
			// Clean up on failure
			if removeErr := a.Fs.Remove(destPath); removeErr != nil {
				a.Log.Warn().Err(removeErr).Str("path", destPath).Msg("failed to remove AppImage after desktop file error")
			}
			a.removeIcons(iconPaths)
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		a.Log.Debug().
			Str("desktop_file", desktopPath).
			Msg("desktop file created")

		if tx != nil && desktopPath != "" {
			path := desktopPath
			tx.Add("remove desktop file", func() error {
				return a.Fs.Remove(path)
			})
		}

		// Update caches
		appsDir := a.Paths.GetAppsDir()
		if cacheErr := a.cacheManager.UpdateDesktopDatabase(appsDir, a.Log); cacheErr != nil {
			a.Log.Warn().Err(cacheErr).Str("apps_dir", appsDir).Msg("failed to update desktop database")
		}

		iconsDir := a.Paths.GetIconsDir()
		if cacheErr := a.cacheManager.UpdateIconCache(iconsDir, a.Log); cacheErr != nil {
			a.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update icon cache")
		}
	}

	// Create install record
	record := &core.InstallRecord{
		InstallID:    installID,
		PackageType:  core.PackageTypeAppImage,
		Name:         appName,
		Version:      metadata.version,
		InstallDate:  time.Now(),
		OriginalFile: packagePath,
		InstallPath:  destPath,
		DesktopFile:  desktopPath,
		Metadata: core.Metadata{
			IconFiles:      iconPaths,
			WaylandSupport: string(core.WaylandUnknown),
			InstallMethod:  core.InstallMethodLocal,
			ExtractedMeta: core.ExtractedMetadata{
				Categories: metadata.categories,
				Comment:    metadata.comment,
			},
		},
	}

	a.Log.Info().
		Str("install_id", installID).
		Str("name", appName).
		Str("path", destPath).
		Msg("AppImage package installed successfully")

	return record, nil
}

// Uninstall removes the installed AppImage package
func (a *AppImageBackend) Uninstall(_ context.Context, record *core.InstallRecord) error {
	a.Log.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling AppImage package")

	// Remove AppImage
	if record.InstallPath != "" {
		if err := a.Fs.Remove(record.InstallPath); err != nil {
			a.Log.Warn().Err(err).Str("path", record.InstallPath).Msg("failed to remove AppImage")
		}
	}

	// Remove .desktop file(s)
	for _, desktopPath := range record.GetDesktopFiles() {
		if desktopPath == "" {
			continue
		}
		if err := a.Fs.Remove(desktopPath); err != nil {
			a.Log.Warn().Err(err).Str("path", desktopPath).Msg("failed to remove desktop file")
		}
	}

	// Remove icons
	a.removeIcons(record.Metadata.IconFiles)

	// Update caches
	appsDir := a.Paths.GetAppsDir()
	if cacheErr := a.cacheManager.UpdateDesktopDatabase(appsDir, a.Log); cacheErr != nil {
		a.Log.Warn().Err(cacheErr).Str("apps_dir", appsDir).Msg("failed to update desktop database")
	}

	iconsDir := a.Paths.GetIconsDir()
	if cacheErr := a.cacheManager.UpdateIconCache(iconsDir, a.Log); cacheErr != nil {
		a.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update icon cache")
	}

	a.Log.Info().
		Str("install_id", record.InstallID).
		Msg("AppImage package uninstalled successfully")

	return nil
}

// extractAppImage extracts an AppImage to a directory
func (a *AppImageBackend) extractAppImage(ctx context.Context, appImagePath, destDir string) error {
	a.Log.Debug().
		Str("appimage", appImagePath).
		Str("dest", destDir).
		Msg("extracting AppImage")

	absAppImagePath, err := filepath.Abs(appImagePath)
	if err != nil {
		return fmt.Errorf("failed to resolve AppImage path: %w", err)
	}

	// Try --appimage-extract first (runs in destDir)
	extractCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err = a.Runner.RunCommandInDir(extractCtx, destDir, absAppImagePath, "--appimage-extract")
	if err == nil {
		return nil
	}

	a.Log.Warn().Err(err).Msg("--appimage-extract failed, trying unsquashfs")

	// Fallback to unsquashfs
	if !a.Runner.CommandExists("unsquashfs") {
		return fmt.Errorf("extraction failed and unsquashfs not found: %w", err)
	}

	_, err = a.Runner.RunCommand(extractCtx, "unsquashfs", "-d", "squashfs-root", absAppImagePath)
	if err != nil {
		return fmt.Errorf("unsquashfs extraction failed: %w", err)
	}

	return nil
}

// parseAppImageMetadata extracts metadata from extracted AppImage
func (a *AppImageBackend) parseAppImageMetadata(squashfsRoot string) (*appImageMetadata, error) {
	metadata := &appImageMetadata{}

	// Find .desktop file
	desktopFiles, err := afero.Glob(a.Fs, filepath.Join(squashfsRoot, "*.desktop"))
	if err != nil {
		return metadata, err
	}

	if len(desktopFiles) > 0 {
		// Use desktop file FILENAME as app name (not the Name field!)
		// Per AppImageSpec and freedesktop.org: the filename is the application ID,
		// while the Name field is the human-readable display name
		desktopFilename := filepath.Base(desktopFiles[0])
		metadata.appName = strings.TrimSuffix(desktopFilename, ".desktop")

		// Parse first .desktop file found for additional metadata
		file, err := a.Fs.Open(desktopFiles[0])
		if err == nil {
			defer func() { _ = file.Close() }()
			entry, err := desktop.Parse(file)
			if err == nil {
				// Store display name and other metadata (but don't use Name as appName!)
				metadata.comment = entry.Comment
				metadata.icon = entry.Icon
				metadata.categories = entry.Categories
				metadata.desktopFile = desktopFiles[0]
			}
		}
	}

	// Find .DirIcon (only use as fallback if no icon in .desktop file)
	if metadata.icon == "" {
		dirIconPath := filepath.Join(squashfsRoot, ".DirIcon")
		if _, statErr := a.Fs.Stat(dirIconPath); statErr == nil {
			// .DirIcon is a symlink to the actual icon file
			// We need to read the symlink target to extract the icon name
			// .DirIcon typically points to: "usr/share/icons/hicolor/4096x4096/apps/auto-claude-ui.png"
			// We need just the base name without extension: "auto-claude-ui"
			if lr, ok := a.Fs.(afero.LinkReader); ok {
				target, readlinkErr := lr.ReadlinkIfPossible(dirIconPath)
				if readlinkErr == nil {
					iconName := filepath.Base(target)
					// Remove file extension if present
					ext := filepath.Ext(iconName)
					if ext != "" {
						iconName = strings.TrimSuffix(iconName, ext)
					}
					metadata.icon = iconName
				}
			}
		}
	}

	return metadata, nil
}

// installIcons installs all icon files from the AppImage
func (a *AppImageBackend) installIcons(squashfsRoot, binName string, metadata *appImageMetadata) ([]string, error) {
	homeDir := a.Paths.HomeDir()
	if homeDir == "" {
		return nil, fmt.Errorf("failed to get home directory")
	}

	installedIcons := []string{}

	// Discover icons in squashfs-root
	discoveredIcons := icons.DiscoverIcons(squashfsRoot)

	a.Log.Debug().
		Int("count", len(discoveredIcons)).
		Msg("discovered icons in AppImage")

	// Use icon name from .desktop file if available, otherwise use binName
	iconName := metadata.icon
	if iconName == "" {
		iconName = binName
	}

	// Install each icon
	for _, iconFile := range discoveredIcons {
		targetPath, err := icons.InstallIcon(iconFile, iconName, homeDir)
		if err != nil {
			a.Log.Warn().
				Err(err).
				Str("icon", iconFile.Path).
				Msg("failed to install icon")
			continue
		}

		installedIcons = append(installedIcons, targetPath)
		a.Log.Debug().
			Str("source", iconFile.Path).
			Str("target", targetPath).
			Msg("icon installed")
	}

	return installedIcons, nil
}

// removeIcons removes installed icons
func (a *AppImageBackend) removeIcons(iconPaths []string) {
	for _, iconPath := range iconPaths {
		if err := a.Fs.Remove(iconPath); err != nil {
			a.Log.Warn().
				Err(err).
				Str("path", iconPath).
				Msg("failed to remove icon")
		}
	}
}

// createDesktopFile creates or updates the .desktop file
//
//nolint:gocyclo // desktop generation handles multiple formats and environment cases.
func (a *AppImageBackend) createDesktopFile(squashfsRoot, appName, binName, execPath string, metadata *appImageMetadata, opts core.InstallOptions) (string, error) {
	appsDir := a.Paths.GetAppsDir()
	if err := a.Fs.MkdirAll(appsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create applications directory: %w", err)
	}

	desktopFilePath := filepath.Join(appsDir, binName+".desktop")

	var entry *core.DesktopEntry

	// Try to use existing .desktop file from AppImage
	if metadata.desktopFile != "" {
		file, err := a.Fs.Open(metadata.desktopFile)
		if err == nil {
			defer func() {
				if closeErr := file.Close(); closeErr != nil {
					a.Log.Debug().Err(closeErr).Str("desktop_file", metadata.desktopFile).Msg("failed to close desktop file")
				}
			}()
			if parsed, parseErr := desktop.Parse(file); parseErr == nil {
				entry = parsed
			} else {
				a.Log.Debug().Err(parseErr).Str("desktop_file", metadata.desktopFile).Msg("failed to parse desktop file from AppImage")
			}
		}
	}

	// Create default entry if parsing failed
	if entry == nil {
		entry = &core.DesktopEntry{
			Type:    "Application",
			Version: "1.5",
			Name:    appName,
		}
	}

	// Update Exec to point to installed AppImage
	entry.Exec = execPath

	// Check for Electron structure to apply sandbox fix if needed
	isElectron := false
	if _, err := a.Fs.Stat(filepath.Join(squashfsRoot, "resources", "app.asar")); err == nil {
		isElectron = true
	}

	if a.Cfg.Desktop.ElectronDisableSandbox && isElectron {
		entry.Exec += " --no-sandbox"
	}

	entry.Exec += " %U"

	// Set icon (use icon name from embedded .desktop file if available, otherwise binName)
	iconName := metadata.icon
	if iconName == "" {
		iconName = binName
	}
	entry.Icon = iconName

	// Ensure categories
	if len(entry.Categories) == 0 {
		entry.Categories = []string{"Utility"}
	}

	// Detect Tauri apps (they use WebKitGTK and require specific environment handling)
	isTauriApp := strings.Contains(strings.ToLower(entry.StartupWMClass), "tauri")

	// Inject Wayland environment variables (skip for Tauri apps or if explicitly disabled)
	if a.Cfg.Desktop.WaylandEnvVars && !opts.SkipWaylandEnv && !isTauriApp {
		if err := desktop.InjectWaylandEnvVars(entry, a.Cfg.Desktop.CustomEnvVars); err != nil {
			a.Log.Warn().
				Err(err).
				Str("app", appName).
				Msg("invalid custom Wayland env vars, injecting defaults only")
			if fallbackErr := desktop.InjectWaylandEnvVars(entry, nil); fallbackErr != nil {
				a.Log.Warn().Err(fallbackErr).Str("app", appName).Msg("failed to inject default Wayland env vars")
			}
		}
	} else if isTauriApp {
		a.Log.Info().
			Str("app", appName).
			Str("wm_class", entry.StartupWMClass).
			Msg("detected Tauri app, skipping Wayland environment injection")
	} else if opts.SkipWaylandEnv {
		a.Log.Info().
			Str("app", appName).
			Msg("skipping Wayland environment injection per user request")
	}

	var buf bytes.Buffer
	if err := desktop.Write(&buf, entry); err != nil {
		return "", err
	}
	if err := afero.WriteFile(a.Fs, desktopFilePath, buf.Bytes(), 0644); err != nil {
		return "", err
	}

	// Validate
	if a.Runner.CommandExists("desktop-file-validate") {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := a.Runner.RunCommand(ctx, "desktop-file-validate", desktopFilePath); err != nil {
			a.Log.Warn().
				Err(err).
				Str("desktop_file", desktopFilePath).
				Msg("desktop file validation failed")
		}
	}

	return desktopFilePath, nil
}

// Helper types

type appImageMetadata struct {
	appName     string
	version     string
	comment     string
	icon        string
	categories  []string
	desktopFile string
}
