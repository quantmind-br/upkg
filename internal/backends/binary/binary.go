package binary

import (
	"bytes"
	"context"
	"errors"
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
//
//nolint:revive // exported backend names are kept for consistency across packages.
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
func (b *BinaryBackend) Detect(_ context.Context, packagePath string) (bool, error) {
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
//
//nolint:gocyclo // install flow includes optional desktop integration and rollback hooks.
func (b *BinaryBackend) Install(_ context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (record *core.InstallRecord, err error) {
	b.Log.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing binary package")

	// Ensure a non-nil transaction: when none is supplied, manage a local one and
	// finalize it before returning (rollback on error, commit on success). Set up
	// before any filesystem mutation so every mutation is covered.
	if tx == nil {
		localTx := transaction.NewManager(b.Log)
		tx = localTx
		defer func() {
			if err != nil {
				if rbErr := localTx.Rollback(); rbErr != nil {
					b.Log.Error().Err(rbErr).Msg("rollback failed")
				}
				return
			}
			if cErr := localTx.Commit(); cErr != nil {
				b.Log.Warn().Err(cErr).Msg("transaction commit cleanup failed")
			}
		}()
	}

	// Validate package exists
	if _, statErr := b.Fs.Stat(packagePath); statErr != nil {
		return nil, fmt.Errorf("package not found: %w", statErr)
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
	if nameErr := security.ValidatePackageName(binName); nameErr != nil {
		return nil, fmt.Errorf("invalid normalized name %q: %w", binName, nameErr)
	}
	installID := helpers.GenerateInstallID(binName)

	// Create ~/.local/bin directory (shared, not rolled back)
	binDir := b.Paths.GetBinDir()
	if mkErr := b.Fs.MkdirAll(binDir, 0755); mkErr != nil {
		return nil, fmt.Errorf("failed to create bin directory: %w", mkErr)
	}

	destPath := filepath.Join(binDir, binName)

	// Reject an existing installation unless forced.
	if _, statErr := b.Fs.Stat(destPath); statErr == nil && !opts.Force {
		return nil, fmt.Errorf("package already installed at: %s (use --force to reinstall)", destPath)
	}

	appsDir := b.Paths.GetAppsDir()
	desktopTarget := filepath.Join(appsDir, binName+".desktop")

	// With --force, back up the artifacts being replaced BEFORE creating the new
	// ones so a failure restores the previous installation.
	if opts.Force {
		if bErr := tx.BackupForReplace(b.Fs, destPath); bErr != nil {
			return nil, fmt.Errorf("failed to back up existing binary: %w", bErr)
		}
		if bErr := tx.BackupForReplace(b.Fs, desktopTarget); bErr != nil {
			return nil, fmt.Errorf("failed to back up existing desktop file: %w", bErr)
		}
	}

	// Register removal BEFORE the copy so a partial copy is cleaned up.
	tx.Add("remove binary", func() error {
		return helpers.RemoveIfExists(b.Fs, destPath, false)
	})
	if cpErr := b.copyBinary(packagePath, destPath); cpErr != nil {
		return nil, fmt.Errorf("failed to copy binary: %w", cpErr)
	}

	// Make executable
	if chErr := b.Fs.Chmod(destPath, 0755); chErr != nil {
		return nil, fmt.Errorf("failed to make executable: %w", chErr)
	}

	b.Log.Debug().
		Str("source", packagePath).
		Str("dest", destPath).
		Msg("binary copied and made executable")

	// Create .desktop file if not skipped
	var desktopPath string
	if !opts.SkipDesktop {
		// Register removal of the desktop target before it is written.
		tx.Add("remove desktop file", func() error {
			return helpers.RemoveIfExists(b.Fs, desktopTarget, false)
		})
		desktopPath, err = b.createDesktopFile(appName, binName, destPath, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		b.Log.Debug().
			Str("desktop_file", desktopPath).
			Msg("desktop file created")

		// Update desktop database (best-effort)
		if cacheErr := b.cacheManager.UpdateDesktopDatabase(appsDir, b.Log); cacheErr != nil {
			b.Log.Warn().Err(cacheErr).Str("apps_dir", appsDir).Msg("failed to update desktop database")
		}
	}

	// Create install record
	record = &core.InstallRecord{
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
	defer func() {
		if closeErr := srcFile.Close(); closeErr != nil {
			b.Log.Debug().Err(closeErr).Str("path", srcPath).Msg("failed to close source binary")
		}
	}()

	dstFile, err := b.Fs.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination binary: %w", err)
	}
	defer func() {
		if closeErr := dstFile.Close(); closeErr != nil {
			b.Log.Debug().Err(closeErr).Str("path", destPath).Msg("failed to close destination binary")
		}
	}()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("copy binary contents: %w", err)
	}

	return nil
}

// Uninstall removes the installed binary package
func (b *BinaryBackend) Uninstall(_ context.Context, record *core.InstallRecord) error {
	b.Log.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling binary package")

	var errs []error

	// Remove binary
	if err := helpers.RemoveIfExists(b.Fs, record.InstallPath, false); err != nil {
		errs = append(errs, err)
	} else if record.InstallPath != "" {
		b.Log.Debug().Str("path", record.InstallPath).Msg("binary removed")
	}

	// Remove .desktop file(s)
	for _, desktopPath := range record.GetDesktopFiles() {
		if desktopPath == "" {
			continue
		}
		if err := helpers.RemoveIfExists(b.Fs, desktopPath, false); err != nil {
			errs = append(errs, err)
		} else {
			b.Log.Debug().Str("path", desktopPath).Msg("desktop file removed")
		}
	}

	// Update desktop database (best-effort, runs even when removals failed).
	appsDir := b.Paths.GetAppsDir()
	if cacheErr := b.cacheManager.UpdateDesktopDatabase(appsDir, b.Log); cacheErr != nil {
		b.Log.Warn().Err(cacheErr).Str("apps_dir", appsDir).Msg("failed to update desktop database")
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

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
			if fallbackErr := desktop.InjectWaylandEnvVars(entry, nil); fallbackErr != nil {
				b.Log.Warn().Err(fallbackErr).Str("app", appName).Msg("failed to inject default Wayland env vars")
			}
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
