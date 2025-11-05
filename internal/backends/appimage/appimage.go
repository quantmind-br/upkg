package appimage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// AppImageBackend handles AppImage installations
type AppImageBackend struct {
	cfg    *config.Config
	logger *zerolog.Logger
}

// New creates a new AppImage backend
func New(cfg *config.Config, log *zerolog.Logger) *AppImageBackend {
	return &AppImageBackend{
		cfg:    cfg,
		logger: log,
	}
}

// Name returns the backend name
func (a *AppImageBackend) Name() string {
	return "appimage"
}

// Detect checks if this backend can handle the package
func (a *AppImageBackend) Detect(ctx context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := os.Stat(packagePath); err != nil {
		return false, nil
	}

	// Check if it's an AppImage
	isAppImage, err := helpers.IsAppImage(packagePath)
	if err != nil {
		return false, err
	}

	return isAppImage, nil
}

// Install installs the AppImage package
func (a *AppImageBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions) (*core.InstallRecord, error) {
	a.logger.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing AppImage package")

	// Validate package exists
	if _, err := os.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	// Make AppImage executable first
	if err := os.Chmod(packagePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to make AppImage executable: %w", err)
	}

	// Create temp directory for extraction
	tmpDir, err := os.MkdirTemp("", "pkgctl-appimage-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract AppImage
	if err := a.extractAppImage(ctx, packagePath, tmpDir); err != nil {
		return nil, fmt.Errorf("failed to extract AppImage: %w", err)
	}

	// Find squashfs-root directory
	squashfsRoot := filepath.Join(tmpDir, "squashfs-root")
	if _, err := os.Stat(squashfsRoot); err != nil {
		return nil, fmt.Errorf("squashfs-root not found after extraction: %w", err)
	}

	// Parse metadata from extracted content
	metadata, err := a.parseAppImageMetadata(squashfsRoot)
	if err != nil {
		a.logger.Warn().Err(err).Msg("failed to parse AppImage metadata, using defaults")
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
		}
	}

	// Normalize name
	binName := helpers.NormalizeFilename(appName)
	installID := helpers.GenerateInstallID(binName)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Copy AppImage to ~/.local/bin/
	binDir := filepath.Join(homeDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bin directory: %w", err)
	}

	destPath := filepath.Join(binDir, binName+".appimage")
	if err := helpers.CopyFile(packagePath, destPath); err != nil {
		return nil, fmt.Errorf("failed to copy AppImage: %w", err)
	}

	// Make destination executable
	if err := os.Chmod(destPath, 0755); err != nil {
		os.Remove(destPath)
		return nil, fmt.Errorf("failed to make AppImage executable: %w", err)
	}

	a.logger.Debug().
		Str("source", packagePath).
		Str("dest", destPath).
		Msg("AppImage copied")

	// Install icons
	iconPaths, err := a.installIcons(squashfsRoot, binName, metadata)
	if err != nil {
		a.logger.Warn().Err(err).Msg("failed to install icons")
	}

	// Create/update desktop file
	var desktopPath string
	if !opts.SkipDesktop {
		desktopPath, err = a.createDesktopFile(squashfsRoot, appName, binName, destPath, metadata, opts)
		if err != nil {
			// Clean up on failure
			os.Remove(destPath)
			a.removeIcons(iconPaths)
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		a.logger.Debug().
			Str("desktop_file", desktopPath).
			Msg("desktop file created")

		// Update caches
		appsDir := filepath.Join(homeDir, ".local", "share", "applications")
		cache.UpdateDesktopDatabase(appsDir, a.logger)

		iconsDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor")
		cache.UpdateIconCache(iconsDir, a.logger)
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
			ExtractedMeta: core.ExtractedMetadata{
				Categories: metadata.categories,
				Comment:    metadata.comment,
			},
		},
	}

	a.logger.Info().
		Str("install_id", installID).
		Str("name", appName).
		Str("path", destPath).
		Msg("AppImage package installed successfully")

	return record, nil
}

// Uninstall removes the installed AppImage package
func (a *AppImageBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	a.logger.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling AppImage package")

	// Remove AppImage
	if record.InstallPath != "" {
		if err := os.Remove(record.InstallPath); err != nil && !os.IsNotExist(err) {
			a.logger.Warn().Err(err).Str("path", record.InstallPath).Msg("failed to remove AppImage")
		}
	}

	// Remove .desktop file
	if record.DesktopFile != "" {
		if err := os.Remove(record.DesktopFile); err != nil && !os.IsNotExist(err) {
			a.logger.Warn().Err(err).Str("path", record.DesktopFile).Msg("failed to remove desktop file")
		}
	}

	// Remove icons
	a.removeIcons(record.Metadata.IconFiles)

	// Update caches
	homeDir, err := os.UserHomeDir()
	if err == nil {
		appsDir := filepath.Join(homeDir, ".local", "share", "applications")
		cache.UpdateDesktopDatabase(appsDir, a.logger)

		iconsDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor")
		cache.UpdateIconCache(iconsDir, a.logger)
	}

	a.logger.Info().
		Str("install_id", record.InstallID).
		Msg("AppImage package uninstalled successfully")

	return nil
}

// extractAppImage extracts an AppImage to a directory
func (a *AppImageBackend) extractAppImage(ctx context.Context, appImagePath, destDir string) error {
	a.logger.Debug().
		Str("appimage", appImagePath).
		Str("dest", destDir).
		Msg("extracting AppImage")

	absAppImagePath, err := filepath.Abs(appImagePath)
	if err != nil {
		return fmt.Errorf("failed to resolve AppImage path: %w", err)
	}

	// Change to destination directory for extraction
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	defer func() {
		_ = os.Chdir(originalDir)
	}()

	if err := os.Chdir(destDir); err != nil {
		return fmt.Errorf("failed to change to destination directory: %w", err)
	}

	// Try --appimage-extract first
	extractCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err = helpers.RunCommand(extractCtx, absAppImagePath, "--appimage-extract")
	if err == nil {
		return nil
	}

	a.logger.Warn().Err(err).Msg("--appimage-extract failed, trying unsquashfs")

	// Fallback to unsquashfs
	if !helpers.CommandExists("unsquashfs") {
		return fmt.Errorf("extraction failed and unsquashfs not found: %w", err)
	}

	_, err = helpers.RunCommand(extractCtx, "unsquashfs", "-d", "squashfs-root", absAppImagePath)
	if err != nil {
		return fmt.Errorf("unsquashfs extraction failed: %w", err)
	}

	return nil
}

// parseAppImageMetadata extracts metadata from extracted AppImage
func (a *AppImageBackend) parseAppImageMetadata(squashfsRoot string) (*appImageMetadata, error) {
	metadata := &appImageMetadata{}

	// Find .desktop file
	desktopFiles, err := filepath.Glob(filepath.Join(squashfsRoot, "*.desktop"))
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
		file, err := os.Open(desktopFiles[0])
		if err == nil {
			defer file.Close()
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

	// Find .DirIcon
	dirIconPath := filepath.Join(squashfsRoot, ".DirIcon")
	if _, err := os.Stat(dirIconPath); err == nil {
		metadata.icon = dirIconPath
	}

	return metadata, nil
}

// installIcons installs all icon files from the AppImage
func (a *AppImageBackend) installIcons(squashfsRoot, binName string, metadata *appImageMetadata) ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	installedIcons := []string{}

	// Discover icons in squashfs-root
	discoveredIcons := icons.DiscoverIcons(squashfsRoot)

	a.logger.Debug().
		Int("count", len(discoveredIcons)).
		Msg("discovered icons in AppImage")

	// Install each icon
	for _, iconFile := range discoveredIcons {
		targetPath, err := icons.InstallIcon(iconFile, binName, homeDir)
		if err != nil {
			a.logger.Warn().
				Err(err).
				Str("icon", iconFile.Path).
				Msg("failed to install icon")
			continue
		}

		installedIcons = append(installedIcons, targetPath)
		a.logger.Debug().
			Str("source", iconFile.Path).
			Str("target", targetPath).
			Msg("icon installed")
	}

	return installedIcons, nil
}

// removeIcons removes installed icons
func (a *AppImageBackend) removeIcons(iconPaths []string) {
	for _, iconPath := range iconPaths {
		if err := os.Remove(iconPath); err != nil && !os.IsNotExist(err) {
			a.logger.Warn().
				Err(err).
				Str("path", iconPath).
				Msg("failed to remove icon")
		}
	}
}

// createDesktopFile creates or updates the .desktop file
func (a *AppImageBackend) createDesktopFile(squashfsRoot, appName, binName, execPath string, metadata *appImageMetadata, opts core.InstallOptions) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	appsDir := filepath.Join(homeDir, ".local", "share", "applications")
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create applications directory: %w", err)
	}

	desktopFilePath := filepath.Join(appsDir, binName+".desktop")

	var entry *core.DesktopEntry

	// Try to use existing .desktop file from AppImage
	if metadata.desktopFile != "" {
		file, err := os.Open(metadata.desktopFile)
		if err == nil {
			defer file.Close()
			entry, _ = desktop.Parse(file)
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
	entry.Exec = execPath + " %U"

	// Set icon (use normalized name for theme compatibility)
	if entry.Icon != "" && !filepath.IsAbs(entry.Icon) {
		entry.Icon = binName
	} else {
		entry.Icon = binName
	}

	// Ensure categories
	if len(entry.Categories) == 0 {
		entry.Categories = []string{"Utility"}
	}

	// Detect Tauri apps (they use WebKitGTK and require specific environment handling)
	isTauriApp := strings.Contains(strings.ToLower(entry.StartupWMClass), "tauri")

	// Inject Wayland environment variables (skip for Tauri apps or if explicitly disabled)
	if a.cfg.Desktop.WaylandEnvVars && !opts.SkipWaylandEnv && !isTauriApp {
		desktop.InjectWaylandEnvVars(entry, a.cfg.Desktop.CustomEnvVars)
	} else if isTauriApp {
		a.logger.Info().
			Str("app", appName).
			Str("wm_class", entry.StartupWMClass).
			Msg("detected Tauri app, skipping Wayland environment injection")
	} else if opts.SkipWaylandEnv {
		a.logger.Info().
			Str("app", appName).
			Msg("skipping Wayland environment injection per user request")
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
			a.logger.Warn().
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
