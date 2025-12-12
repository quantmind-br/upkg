package binary

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	backendbase "github.com/quantmind-br/upkg/internal/backends/base"
	"github.com/quantmind-br/upkg/internal/cache"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/desktop"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/security"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// BinaryBackend handles standalone ELF binary installations
type BinaryBackend struct {
	*backendbase.BaseBackend
	cacheManager *cache.CacheManager
}

// New creates a new binary backend
func New(cfg *config.Config, log *zerolog.Logger) *BinaryBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), helpers.NewOSCommandRunner())
}

// NewWithRunner creates a new binary backend with a custom command runner
func NewWithRunner(cfg *config.Config, log *zerolog.Logger, runner helpers.CommandRunner) *BinaryBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), runner)
}

// NewWithCacheManager creates a new binary backend with a custom cache manager
func NewWithCacheManager(cfg *config.Config, log *zerolog.Logger, cacheManager *cache.CacheManager) *BinaryBackend {
	backend := New(cfg, log)
	backend.cacheManager = cacheManager
	return backend
}

// NewWithDeps creates a new binary backend with custom dependencies for testing
func NewWithDeps(cfg *config.Config, log *zerolog.Logger, fs afero.Fs, runner helpers.CommandRunner) *BinaryBackend {
	base := backendbase.NewWithDeps(cfg, log, fs, runner)
	return &BinaryBackend{
		BaseBackend:  base,
		cacheManager: cache.NewCacheManagerWithRunner(runner),
	}
}

// Name returns the backend name
func (b *BinaryBackend) Name() string {
	return "binary"
}

// Detect checks if this backend can handle the package
func (b *BinaryBackend) Detect(ctx context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := b.Fs.Stat(packagePath); err != nil {
		return false, nil
	}

	fileType, err := helpers.DetectFileType(packagePath)
	if err != nil {
		return false, err
	}

	// DetectFileType already differentiates AppImage vs plain ELF.
	return fileType == helpers.FileTypeELF, nil
}

// Install installs the binary package
func (b *BinaryBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	b.Log.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing binary package")

	// Validate package exists
	if _, err := b.Fs.Stat(packagePath); err != nil {
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
	if err := security.ValidatePackageName(binName); err != nil {
		return nil, fmt.Errorf("invalid normalized name %q: %w", binName, err)
	}
	installID := helpers.GenerateInstallID(binName)

	// Create ~/.local/bin directory
	binDir := b.Paths.GetBinDir()
	if err := b.Fs.MkdirAll(binDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create bin directory: %w", err)
	}

	// Copy binary to ~/.local/bin/
	destPath := filepath.Join(binDir, binName)
	if _, err := b.Fs.Stat(destPath); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("package already installed at: %s (use --force to reinstall)", destPath)
		}
		if err := b.Fs.Remove(destPath); err != nil {
			return nil, fmt.Errorf("remove existing binary: %w", err)
		}
	}

	if err := b.copyBinary(packagePath, destPath); err != nil {
		return nil, fmt.Errorf("failed to copy binary: %w", err)
	}

	// Make executable
	if err := b.Fs.Chmod(destPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to make executable: %w", err)
	}

	if tx != nil {
		path := destPath
		tx.Add("remove binary", func() error {
			if err := b.Fs.Remove(path); err != nil {
				return err
			}
			return nil
		})
	}

	b.Log.Debug().
		Str("source", packagePath).
		Str("dest", destPath).
		Msg("binary copied and made executable")

	// Create .desktop file if not skipped
	var (
		desktopPath string
		err         error
	)
	if !opts.SkipDesktop {
		if opts.Force {
			appsDir := b.Paths.GetAppsDir()
			_ = b.Fs.Remove(filepath.Join(appsDir, binName+".desktop"))
		}
		desktopPath, err = b.createDesktopFile(appName, binName, destPath, opts)
		if err != nil {
			// Clean up binary on desktop file creation failure
			_ = b.Fs.Remove(destPath)
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		b.Log.Debug().
			Str("desktop_file", desktopPath).
			Msg("desktop file created")

		if tx != nil && desktopPath != "" {
			path := desktopPath
			tx.Add("remove desktop file", func() error {
				if err := b.Fs.Remove(path); err != nil {
					return err
				}
				return nil
			})
		}

		// Update desktop database
		appsDir := b.Paths.GetAppsDir()
		_ = b.cacheManager.UpdateDesktopDatabase(appsDir, b.Log)
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
			InstallMethod:  core.InstallMethodLocal,
		},
	}

	b.Log.Info().
		Str("install_id", installID).
		Str("name", appName).
		Str("path", destPath).
		Msg("binary package installed successfully")

	return record, nil
}

func (b *BinaryBackend) copyBinary(srcPath, destPath string) error {
	srcFile, err := b.Fs.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open source binary: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	dstFile, err := b.Fs.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination binary: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy binary contents: %w", err)
	}

	return nil
}

// Uninstall removes the installed binary package
func (b *BinaryBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	b.Log.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling binary package")

	// Remove binary
	if record.InstallPath != "" {
		if err := b.Fs.Remove(record.InstallPath); err != nil {
			b.Log.Warn().
				Err(err).
				Str("path", record.InstallPath).
				Msg("failed to remove binary")
		} else {
			b.Log.Debug().
				Str("path", record.InstallPath).
				Msg("binary removed")
		}
	}

	// Remove .desktop file(s)
	for _, desktopPath := range record.GetDesktopFiles() {
		if desktopPath == "" {
			continue
		}
		if err := b.Fs.Remove(desktopPath); err != nil {
			b.Log.Warn().
				Err(err).
				Str("path", desktopPath).
				Msg("failed to remove desktop file")
		} else {
			b.Log.Debug().
				Str("path", desktopPath).
				Msg("desktop file removed")
		}
	}

	// Update desktop database
	appsDir := b.Paths.GetAppsDir()
	_ = b.cacheManager.UpdateDesktopDatabase(appsDir, b.Log)

	b.Log.Info().
		Str("install_id", record.InstallID).
		Msg("binary package uninstalled successfully")

	return nil
}

// createDesktopFile creates a .desktop file for the binary
func (b *BinaryBackend) createDesktopFile(appName, binName, execPath string, opts core.InstallOptions) (string, error) {
	appsDir := b.Paths.GetAppsDir()
	if err := b.Fs.MkdirAll(appsDir, 0755); err != nil {
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
	if b.Cfg.Desktop.WaylandEnvVars && !opts.SkipWaylandEnv {
		if err := desktop.InjectWaylandEnvVars(entry, b.Cfg.Desktop.CustomEnvVars); err != nil {
			b.Log.Warn().
				Err(err).
				Str("app", appName).
				Msg("invalid custom Wayland env vars, injecting defaults only")
			_ = desktop.InjectWaylandEnvVars(entry, nil)
		}
	}

	var buf bytes.Buffer
	if err := desktop.Write(&buf, entry); err != nil {
		return "", fmt.Errorf("write desktop entry: %w", err)
	}
	if err := afero.WriteFile(b.Fs, desktopFilePath, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("write desktop file: %w", err)
	}

	// Validate desktop file
	if b.Runner.CommandExists("desktop-file-validate") {
		validateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if _, err := b.Runner.RunCommand(validateCtx, "desktop-file-validate", desktopFilePath); err != nil {
			b.Log.Warn().
				Err(err).
				Str("desktop_file", desktopFilePath).
				Msg("desktop file validation failed")
		}
	}

	return desktopFilePath, nil
}
