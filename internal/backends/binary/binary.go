package binary

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quantmind-br/upkg/internal/cache"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/desktop"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// BinaryBackend handles standalone ELF binary installations
type BinaryBackend struct {
	fs     afero.Fs
	cfg    *config.Config
	logger *zerolog.Logger
}

// New creates a new binary backend
func New(cfg *config.Config, log *zerolog.Logger) *BinaryBackend {
	return &BinaryBackend{
		fs:     afero.NewOsFs(),
		cfg:    cfg,
		logger: log,
	}
}

// Name returns the backend name
func (b *BinaryBackend) Name() string {
	return "binary"
}

// Detect checks if this backend can handle the package
func (b *BinaryBackend) Detect(ctx context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := os.Stat(packagePath); err != nil {
		return false, nil
	}

	// Check if it's an ELF binary
	isElf, err := helpers.IsELF(packagePath)
	if err != nil {
		return false, err
	}

	// Check if it's NOT an AppImage (AppImage is also ELF)
	if isElf {
		isAppImage, _ := helpers.IsAppImage(packagePath)
		if isAppImage {
			return false, nil // Let AppImage backend handle it
		}
	}

	return isElf, nil
}

// Install installs the binary package
func (b *BinaryBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	b.logger.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing binary package")

	// Validate package exists
	if _, err := os.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	// Determine application name
	appName := opts.CustomName
	if appName == "" {
		appName = filepath.Base(packagePath)
		appName = strings.TrimSuffix(appName, filepath.Ext(appName))
		appName = helpers.CleanAppName(appName)
	}

	// Normalize name for filesystem
	binName := helpers.NormalizeFilename(appName)
	installID := helpers.GenerateInstallID(binName)

	// Create ~/.local/bin directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	binDir := filepath.Join(homeDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Copy binary to ~/.local/bin/
	destPath := filepath.Join(binDir, binName)
	if err := helpers.CopyFile(packagePath, destPath); err != nil {
		return nil, fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make executable
	if err := os.Chmod(destPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to make executable: %w", err)
	}

	b.logger.Debug().
		Str("source", packagePath).
		Str("dest", destPath).
		Msg("binary copied and made executable")

	// Create .desktop file if not skipped
	var desktopPath string
	if !opts.SkipDesktop {
		desktopPath, err = b.createDesktopFile(appName, binName, destPath, opts)
		if err != nil {
			// Clean up binary on desktop file creation failure
			os.Remove(destPath)
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		b.logger.Debug().
			Str("desktop_file", desktopPath).
			Msg("desktop file created")

		// Update desktop database
		appsDir := filepath.Join(homeDir, ".local", "share", "applications")
		cache.UpdateDesktopDatabase(appsDir, b.logger)
	}

	// Create install record
	record := &core.InstallRecord{
		InstallID:    installID,
		PackageType:  core.PackageTypeBinary,
		Name:         appName,
		InstallDate:  time.Now(),
		OriginalFile: packagePath,
		InstallPath:  destPath,
		DesktopFile:  desktopPath,
		Metadata: core.Metadata{
			WaylandSupport: string(core.WaylandUnknown),
		},
	}

	b.logger.Info().
		Str("install_id", installID).
		Str("name", appName).
		Str("path", destPath).
		Msg("binary package installed successfully")

	return record, nil
}

// Uninstall removes the installed binary package
func (b *BinaryBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	b.logger.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling binary package")

	// Remove binary
	if record.InstallPath != "" {
		if err := os.Remove(record.InstallPath); err != nil && !os.IsNotExist(err) {
			b.logger.Warn().
				Err(err).
				Str("path", record.InstallPath).
				Msg("failed to remove binary")
		} else {
			b.logger.Debug().
				Str("path", record.InstallPath).
				Msg("binary removed")
		}
	}

	// Remove .desktop file
	if record.DesktopFile != "" {
		if err := os.Remove(record.DesktopFile); err != nil && !os.IsNotExist(err) {
			b.logger.Warn().
				Err(err).
				Str("path", record.DesktopFile).
				Msg("failed to remove desktop file")
		} else {
			b.logger.Debug().
				Str("path", record.DesktopFile).
				Msg("desktop file removed")
		}
	}

	// Update desktop database
	homeDir, err := os.UserHomeDir()
	if err == nil {
		appsDir := filepath.Join(homeDir, ".local", "share", "applications")
		cache.UpdateDesktopDatabase(appsDir, b.logger)
	}

	b.logger.Info().
		Str("install_id", record.InstallID).
		Msg("binary package uninstalled successfully")

	return nil
}

// createDesktopFile creates a .desktop file for the binary
func (b *BinaryBackend) createDesktopFile(appName, binName, execPath string, opts core.InstallOptions) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	appsDir := filepath.Join(homeDir, ".local", "share", "applications")
	if err := os.MkdirAll(appsDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create applications directory: %w", err)
	}

	desktopFilePath := filepath.Join(appsDir, binName+".desktop")

	// Create desktop entry
	displayName := helpers.FormatDisplayName(appName)
	entry := &core.DesktopEntry{
		Type:        "Application",
		Version:     "1.5",
		Name:        displayName,
		GenericName: displayName,
		Comment:     fmt.Sprintf("%s application", displayName),
		Icon:        "application-x-executable", // Generic icon
		Exec:        execPath,
		Terminal:    false,
		Categories:  []string{"Utility"},
		Keywords:    []string{appName},
	}

	// Inject Wayland environment variables if enabled
	if b.cfg.Desktop.WaylandEnvVars && !opts.SkipWaylandEnv {
		desktop.InjectWaylandEnvVars(entry, b.cfg.Desktop.CustomEnvVars)
	}

	// Write desktop file
	if err := desktop.WriteDesktopFile(desktopFilePath, entry); err != nil {
		return "", fmt.Errorf("failed to write desktop file: %w", err)
	}

	// Validate desktop file
	if helpers.CommandExists("desktop-file-validate") {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := helpers.RunCommand(ctx, "desktop-file-validate", desktopFilePath); err != nil {
			b.logger.Warn().
				Err(err).
				Str("desktop_file", desktopFilePath).
				Msg("desktop file validation failed")
		}
	}

	return desktopFilePath, nil
}

// No local helper functions - using shared helpers from internal/helpers/common.go
