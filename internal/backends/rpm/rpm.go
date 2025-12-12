package rpm

import (
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
	"github.com/quantmind-br/upkg/internal/syspkg"
	"github.com/quantmind-br/upkg/internal/syspkg/arch"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// RpmBackend handles RPM package installations
//
//nolint:revive // exported backend names are kept for consistency across packages.
type RpmBackend struct {
	*backendbase.BaseBackend
	scorer       heuristics.Scorer
	sys          syspkg.Provider
	cacheManager *cache.CacheManager
}

// New creates a new RPM backend
func New(cfg *config.Config, log *zerolog.Logger) *RpmBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), helpers.NewOSCommandRunner())
}

// NewWithRunner creates a new RPM backend with a custom command runner
func NewWithRunner(cfg *config.Config, log *zerolog.Logger, runner helpers.CommandRunner) *RpmBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), runner)
}

// NewWithCacheManager creates a new RPM backend with a custom cache manager
func NewWithCacheManager(cfg *config.Config, log *zerolog.Logger, cacheManager *cache.CacheManager) *RpmBackend {
	backend := New(cfg, log)
	backend.cacheManager = cacheManager
	return backend
}

// NewWithDeps creates a new RPM backend with injected dependencies.
func NewWithDeps(cfg *config.Config, log *zerolog.Logger, fs afero.Fs, runner helpers.CommandRunner) *RpmBackend {
	base := backendbase.NewWithDeps(cfg, log, fs, runner)
	return &RpmBackend{
		BaseBackend:  base,
		scorer:       heuristics.NewScorer(log),
		sys:          arch.NewPacmanProviderWithRunner(runner),
		cacheManager: cache.NewCacheManagerWithRunner(runner),
	}
}

// Name returns the backend name
func (r *RpmBackend) Name() string {
	return "rpm"
}

// Detect checks if this backend can handle the package
func (r *RpmBackend) Detect(_ context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := r.Fs.Stat(packagePath); err != nil {
		return false, nil
	}

	// Check file type
	fileType, err := helpers.DetectFileType(packagePath)
	if err != nil {
		return false, err
	}

	return fileType == helpers.FileTypeRPM, nil
}

// Install installs the RPM package
func (r *RpmBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	r.Log.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing RPM package")

	// Validate package exists
	if _, err := r.Fs.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	// Determine package name
	pkgName := opts.CustomName
	if pkgName == "" {
		// Try to get official package name from RPM metadata (best practice)
		if name, err := r.queryRpmName(ctx, packagePath); err == nil && name != "" {
			pkgName = name
			r.Log.Debug().
				Str("name", name).
				Msg("extracted package name from RPM metadata")
		} else {
			// Fallback: Extract base name from RPM filename
			pkgName = extractRpmBaseName(filepath.Base(packagePath))
			r.Log.Debug().
				Str("name", pkgName).
				Msg("extracted package name from filename (rpm query unavailable)")
		}
	}

	normalizedName := helpers.NormalizeFilename(pkgName)
	if err := security.ValidatePackageName(normalizedName); err != nil {
		return nil, fmt.Errorf("invalid normalized name %q: %w", normalizedName, err)
	}
	installID := helpers.GenerateInstallID(normalizedName)

	// Check if rpmextract.sh is available (preferred method)
	if r.Runner.CommandExists("rpmextract.sh") {
		return r.installWithExtract(ctx, packagePath, normalizedName, installID, opts, tx)
	}

	// Fallback: check if we can use debtap (on Arch)
	if r.Runner.CommandExists("debtap") && r.Runner.CommandExists("pacman") {
		r.Log.Info().Msg("using debtap/pacman method for RPM installation")
		return r.installWithDebtap(ctx, packagePath, normalizedName, installID, opts, tx)
	}

	return nil, fmt.Errorf("no suitable RPM installation method found\nInstall either 'rpmextract.sh' or 'debtap' (Arch)")
}

// installWithExtract installs RPM by extracting and manually placing files
//
//nolint:gocyclo // extraction install handles multiple fallbacks and integrations.
func (r *RpmBackend) installWithExtract(ctx context.Context, packagePath, normalizedName, installID string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	r.Log.Info().Msg("extracting RPM package...")

	homeDir := r.Paths.HomeDir()
	if homeDir == "" {
		return nil, fmt.Errorf("failed to get home directory")
	}

	// Convert to absolute path for rpmextract.sh reliability
	absPackagePath, err := filepath.Abs(packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Create temp directory for extraction
	tmpDir, err := afero.TempDir(r.Fs, "", "upkg-rpm-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := r.Fs.RemoveAll(tmpDir); removeErr != nil {
			r.Log.Debug().Err(removeErr).Str("tmp_dir", tmpDir).Msg("failed to remove temp dir")
		}
	}()

	// Extract RPM (in temp directory) using absolute path
	extractCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err = r.Runner.RunCommandInDir(extractCtx, tmpDir, "rpmextract.sh", absPackagePath)
	if err != nil {
		return nil, fmt.Errorf("rpmextract.sh failed: %w", err)
	}

	r.Log.Debug().Msg("RPM extracted successfully")

	// Create installation directory
	appsDir := r.Paths.GetUpkgAppsDir()
	installDir := filepath.Join(appsDir, normalizedName)

	if _, statErr := r.Fs.Stat(installDir); statErr == nil {
		if !opts.Force {
			return nil, fmt.Errorf("package already installed at: %s (use --force to reinstall)", installDir)
		}
		if removeErr := r.Fs.RemoveAll(installDir); removeErr != nil {
			return nil, fmt.Errorf("remove existing installation directory: %w", removeErr)
		}
		// Best-effort cleanup of expected wrapper/desktop paths
		binDir := r.Paths.GetBinDir()
		oldWrapper := filepath.Join(binDir, normalizedName)
		if removeErr := r.Fs.Remove(oldWrapper); removeErr != nil {
			r.Log.Debug().Err(removeErr).Str("path", oldWrapper).Msg("failed to remove existing wrapper")
		}
		appsDbDir := r.Paths.GetAppsDir()
		oldDesktop := filepath.Join(appsDbDir, normalizedName+".desktop")
		if removeErr := r.Fs.Remove(oldDesktop); removeErr != nil {
			r.Log.Debug().Err(removeErr).Str("desktop_file", oldDesktop).Msg("failed to remove existing desktop file")
		}
	}

	if mkdirErr := r.Fs.MkdirAll(installDir, 0755); mkdirErr != nil {
		return nil, fmt.Errorf("failed to create installation directory: %w", mkdirErr)
	}
	if tx != nil {
		dir := installDir
		tx.Add("remove rpm installation directory", func() error {
			return r.Fs.RemoveAll(dir)
		})
	}

	// Move extracted content to installation directory
	// RPMs typically extract to usr/, opt/, etc.
	extractedDirs := []string{"usr", "opt", "etc"}
	for _, dir := range extractedDirs {
		srcDir := filepath.Join(tmpDir, dir)
		if _, statErr := r.Fs.Stat(srcDir); statErr == nil {
			dstDir := filepath.Join(installDir, dir)
			if renameErr := r.Fs.Rename(srcDir, dstDir); renameErr != nil {
				// Try copying if rename fails
				if copyErr := r.copyDir(srcDir, dstDir); copyErr != nil {
					r.Log.Warn().
						Err(copyErr).
						Str("dir", dir).
						Msg("failed to move directory")
				}
			}
		}
	}

	// Find executables
	executables, err := heuristics.FindExecutables(installDir)
	if err != nil || len(executables) == 0 {
		if removeErr := r.Fs.RemoveAll(installDir); removeErr != nil {
			r.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after no executables")
		}
		return nil, fmt.Errorf("no executables found in RPM")
	}

	r.Log.Debug().
		Strs("executables", executables).
		Msg("found executables")

	// Choose primary executable using scoring heuristic (same as tarball backend)
	primaryExec := r.scorer.ChooseBest(executables, normalizedName, installDir)

	// Create wrapper script
	binDir := r.Paths.GetBinDir()
	if mkdirErr := r.Fs.MkdirAll(binDir, 0755); mkdirErr != nil {
		if removeErr := r.Fs.RemoveAll(installDir); removeErr != nil {
			r.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after mkdir error")
		}
		return nil, fmt.Errorf("failed to create bin directory: %w", mkdirErr)
	}

	wrapperPath := filepath.Join(binDir, normalizedName)
	if wrapperErr := r.createWrapper(wrapperPath, primaryExec); wrapperErr != nil {
		if removeErr := r.Fs.RemoveAll(installDir); removeErr != nil {
			r.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after wrapper error")
		}
		return nil, fmt.Errorf("failed to create wrapper script: %w", wrapperErr)
	}
	if tx != nil {
		path := wrapperPath
		tx.Add("remove rpm wrapper script", func() error {
			return r.Fs.Remove(path)
		})
	}

	// Install icons
	iconPaths, err := r.installIcons(installDir, normalizedName)
	if err != nil {
		r.Log.Warn().Err(err).Msg("failed to install icons")
	}
	if tx != nil && len(iconPaths) > 0 {
		paths := append([]string(nil), iconPaths...)
		tx.Add("remove rpm icons", func() error {
			r.removeIcons(paths)
			return nil
		})
	}

	// Create .desktop file
	var desktopPath string
	if !opts.SkipDesktop {
		desktopPath, err = r.createDesktopFile(installDir, normalizedName, wrapperPath, opts)
		if err != nil {
			// Clean up on failure
			if removeErr := r.Fs.RemoveAll(installDir); removeErr != nil {
				r.Log.Debug().Err(removeErr).Str("install_dir", installDir).Msg("failed to cleanup install dir after desktop error")
			}
			if removeErr := r.Fs.Remove(wrapperPath); removeErr != nil {
				r.Log.Debug().Err(removeErr).Str("path", wrapperPath).Msg("failed to cleanup wrapper after desktop error")
			}
			r.removeIcons(iconPaths)
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		if tx != nil && desktopPath != "" {
			path := desktopPath
			tx.Add("remove rpm desktop file", func() error {
				return r.Fs.Remove(path)
			})
		}

		// Update caches
		appsDbDir := r.Paths.GetAppsDir()
		if cacheErr := r.cacheManager.UpdateDesktopDatabase(appsDbDir, r.Log); cacheErr != nil {
			r.Log.Warn().Err(cacheErr).Str("apps_dir", appsDbDir).Msg("failed to update desktop database")
		}

		iconsDir := r.Paths.GetIconsDir()
		if cacheErr := r.cacheManager.UpdateIconCache(iconsDir, r.Log); cacheErr != nil {
			r.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update icon cache")
		}
	}

	// Create install record
	record := &core.InstallRecord{
		InstallID:    installID,
		PackageType:  core.PackageTypeRpm,
		Name:         normalizedName,
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

	r.Log.Info().
		Str("install_id", installID).
		Str("name", normalizedName).
		Str("path", installDir).
		Msg("RPM package installed successfully (extracted)")

	return record, nil
}

// installWithDebtap installs RPM by converting to Arch package via debtap
//
//nolint:gocyclo // pacman-based RPM install has multiple fallbacks and integrations.
func (r *RpmBackend) installWithDebtap(ctx context.Context, packagePath, normalizedName, installID string, _ core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	r.Log.Info().Msg("converting RPM to Arch package via debtap...")

	// Convert to absolute path for reliability
	absPackagePath, err := filepath.Abs(packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Create temp directory
	tmpDir, err := afero.TempDir(r.Fs, "", "upkg-rpm-debtap-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := r.Fs.RemoveAll(tmpDir); removeErr != nil {
			r.Log.Debug().Err(removeErr).Str("tmp_dir", tmpDir).Msg("failed to remove temp dir")
		}
	}()

	// Convert RPM with debtap (using absolute path)
	convertCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	_, err = r.Runner.RunCommandInDir(convertCtx, tmpDir, "debtap", "-q", "-Q", absPackagePath)
	if err != nil {
		return nil, fmt.Errorf("debtap conversion failed: %w", err)
	}

	// Find generated .pkg.tar.* file
	files, err := afero.Glob(r.Fs, filepath.Join(tmpDir, "*.pkg.tar.*"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no arch package generated by debtap")
	}

	archPkgPath := files[0]

	// Install with pacman
	r.Log.Info().Msg("installing converted package with pacman...")

	installCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	err = r.sys.Install(installCtx, archPkgPath)
	if err != nil {
		return nil, fmt.Errorf("pacman installation failed: %w", err)
	}
	if tx != nil {
		pkgName := normalizedName
		tx.Add("remove pacman package", func() error {
			removeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			return r.sys.Remove(removeCtx, pkgName)
		})
	}

	// Get package info
	pkgInfo, infoErr := r.getPackageInfo(ctx, normalizedName)
	if infoErr != nil {
		r.Log.Warn().Err(infoErr).Str("package", normalizedName).Msg("failed to get package info from pacman")
	}
	if pkgInfo == nil {
		pkgInfo = &packageInfo{
			name:    normalizedName,
			version: "unknown",
		}
	}

	// Find installed files
	installedFiles, listErr := r.findInstalledFiles(ctx, normalizedName)
	if listErr != nil {
		r.Log.Warn().Err(listErr).Str("package", normalizedName).Msg("failed to list installed files")
	}

	// Find desktop and icon files
	desktopFiles := r.findDesktopFiles(installedFiles)
	iconFiles := r.findIconFiles(installedFiles)

	var primaryDesktopFile string
	if len(desktopFiles) > 0 {
		primaryDesktopFile = desktopFiles[0]
	}

	// Update caches
	if len(desktopFiles) > 0 {
		if cacheErr := r.cacheManager.UpdateDesktopDatabase("/usr/share/applications", r.Log); cacheErr != nil {
			r.Log.Warn().Err(cacheErr).Msg("failed to update desktop database")
		}
	}
	if len(iconFiles) > 0 {
		if cacheErr := r.cacheManager.UpdateIconCache("/usr/share/icons/hicolor", r.Log); cacheErr != nil {
			r.Log.Warn().Err(cacheErr).Msg("failed to update icon cache")
		}
	}

	// Create install record
	record := &core.InstallRecord{
		InstallID:    installID,
		PackageType:  core.PackageTypeRpm,
		Name:         pkgInfo.name,
		Version:      pkgInfo.version,
		InstallDate:  time.Now(),
		OriginalFile: packagePath,
		InstallPath:  "",
		DesktopFile:  primaryDesktopFile,
		Metadata: core.Metadata{
			IconFiles:      iconFiles,
			WaylandSupport: string(core.WaylandUnknown),
			InstallMethod:  core.InstallMethodPacman,
			DesktopFiles:   desktopFiles,
		},
	}

	r.Log.Info().
		Str("install_id", installID).
		Str("name", pkgInfo.name).
		Msg("RPM package installed successfully (pacman)")

	return record, nil
}

// Uninstall removes the installed RPM package
func (r *RpmBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	r.Log.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling RPM package")

	// Check if it was installed via pacman or extracted
	if record.Metadata.InstallMethod == core.InstallMethodPacman ||
		strings.Contains(record.InstallPath, "pacman") { // backward compatibility
		// Installed via pacman
		return r.uninstallPacman(ctx, record)
	}

	// Installed via extraction
	return r.uninstallExtracted(ctx, record)
}

// uninstallPacman removes RPM installed via pacman
func (r *RpmBackend) uninstallPacman(ctx context.Context, record *core.InstallRecord) error {
	normalizedName := helpers.NormalizeFilename(record.Name)

	// Check if still installed
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	installed, err := r.sys.IsInstalled(checkCtx, normalizedName)
	if err != nil || !installed {
		r.Log.Warn().Msg("package not found in pacman database")
		return nil
	}

	// Uninstall with pacman
	uninstallCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	err = r.sys.Remove(uninstallCtx, normalizedName)
	if err != nil {
		return fmt.Errorf("pacman removal failed: %w", err)
	}

	// Update caches
	if cacheErr := r.cacheManager.UpdateDesktopDatabase("/usr/share/applications", r.Log); cacheErr != nil {
		r.Log.Warn().Err(cacheErr).Msg("failed to update desktop database")
	}
	if cacheErr := r.cacheManager.UpdateIconCache("/usr/share/icons/hicolor", r.Log); cacheErr != nil {
		r.Log.Warn().Err(cacheErr).Msg("failed to update icon cache")
	}

	return nil
}

// uninstallExtracted removes RPM installed via extraction
func (r *RpmBackend) uninstallExtracted(_ context.Context, record *core.InstallRecord) error {
	// Remove installation directory
	if record.InstallPath != "" {
		if err := r.Fs.RemoveAll(record.InstallPath); err != nil {
			r.Log.Warn().Err(err).Msg("failed to remove installation directory")
		}
	}

	// Remove wrapper script
	if record.Metadata.WrapperScript != "" {
		if err := r.Fs.Remove(record.Metadata.WrapperScript); err != nil {
			r.Log.Warn().Err(err).Msg("failed to remove wrapper script")
		}
	}

	// Remove desktop file(s)
	for _, desktopPath := range record.GetDesktopFiles() {
		if desktopPath == "" {
			continue
		}
		if err := r.Fs.Remove(desktopPath); err != nil {
			r.Log.Warn().Err(err).Str("path", desktopPath).Msg("failed to remove desktop file")
		}
	}

	// Remove icons
	r.removeIcons(record.Metadata.IconFiles)

	// Update caches
	appsDir := r.Paths.GetAppsDir()
	if cacheErr := r.cacheManager.UpdateDesktopDatabase(appsDir, r.Log); cacheErr != nil {
		r.Log.Warn().Err(cacheErr).Str("apps_dir", appsDir).Msg("failed to update desktop database")
	}

	iconsDir := r.Paths.GetIconsDir()
	if cacheErr := r.cacheManager.UpdateIconCache(iconsDir, r.Log); cacheErr != nil {
		r.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update icon cache")
	}

	return nil
}

// Helper functions

func (r *RpmBackend) createWrapper(wrapperPath, execPath string) error {
	content := fmt.Sprintf(`#!/bin/bash
# upkg wrapper script
exec "%s" "$@"
`, execPath)

	return afero.WriteFile(r.Fs, wrapperPath, []byte(content), 0755)
}

func (r *RpmBackend) installIcons(installDir, normalizedName string) ([]string, error) {
	homeDir := r.Paths.HomeDir()
	if homeDir == "" {
		return nil, fmt.Errorf("failed to get home directory")
	}

	iconBaseDir := filepath.Join(homeDir, ".local", "share", "icons")
	iconManager := icons.NewManager(r.Fs, iconBaseDir)

	discoveredIcons, err := iconManager.DiscoverIcons(installDir)
	if err != nil {
		return nil, err
	}

	var installedIcons []string

	for _, iconFile := range discoveredIcons {
		targetPath, err := iconManager.InstallIcon(iconFile.Path, normalizedName, iconFile.Size)
		if err != nil {
			continue
		}
		installedIcons = append(installedIcons, targetPath)
	}

	return installedIcons, nil
}

func (r *RpmBackend) removeIcons(iconPaths []string) {
	for _, iconPath := range iconPaths {
		if removeErr := r.Fs.Remove(iconPath); removeErr != nil {
			r.Log.Debug().Err(removeErr).Str("path", iconPath).Msg("failed to remove icon")
		}
	}
}

func (r *RpmBackend) createDesktopFile(installDir, normalizedName, wrapperPath string, opts core.InstallOptions) (string, error) {
	homeDir := r.Paths.HomeDir()
	if homeDir == "" {
		return "", fmt.Errorf("failed to get home directory")
	}

	appsDir := r.Paths.GetAppsDir()
	if mkdirErr := r.Fs.MkdirAll(appsDir, 0755); mkdirErr != nil {
		return "", fmt.Errorf("failed to create applications directory: %w", mkdirErr)
	}

	desktopFilePath := filepath.Join(appsDir, normalizedName+".desktop")

	// Try to find existing .desktop file in extracted RPM (similar to tarball backend)
	var entry *core.DesktopEntry

	// Common locations for .desktop files in RPMs
	desktopSearchPaths := []string{
		filepath.Join(installDir, "usr", "share", "applications"),
		filepath.Join(installDir, "usr", "local", "share", "applications"),
		filepath.Join(installDir, "opt", "*", "share", "applications"),
	}

	for _, searchPath := range desktopSearchPaths {
		matches, globErr := afero.Glob(r.Fs, filepath.Join(searchPath, "*.desktop"))
		if globErr != nil {
			continue
		}
		if len(matches) > 0 {
			// Found desktop file(s), parse the first one
			file, err := r.Fs.Open(matches[0])
			if err == nil {
				defer func() {
					if closeErr := file.Close(); closeErr != nil {
						r.Log.Debug().Err(closeErr).Str("desktop_file", matches[0]).Msg("failed to close desktop file")
					}
				}()
				entry, err = desktop.Parse(file)
				if err == nil {
					r.Log.Debug().
						Str("desktop_file", matches[0]).
						Str("name", entry.Name).
						Msg("using desktop file from RPM package")
					break
				}
			}
		}
	}

	// Create default entry if not found in RPM
	if entry == nil {
		r.Log.Debug().Msg("no desktop file found in RPM, creating default")

		// Try to create a better display name from the original package name
		// Example: "git-butler-nightly" -> "Git Butler Nightly"
		displayName := helpers.FormatDisplayName(normalizedName)

		entry = &core.DesktopEntry{
			Type:    "Application",
			Version: "1.5",
			Name:    displayName,
			Icon:    normalizedName,
			Exec:    wrapperPath + " %U",
		}
	} else {
		// Update Exec to point to our wrapper
		entry.Exec = wrapperPath + " %U"

		// Ensure icon uses normalized name for consistency
		entry.Icon = normalizedName
	}

	// Inject Wayland vars
	if r.Cfg.Desktop.WaylandEnvVars && !opts.SkipWaylandEnv {
		if err := desktop.InjectWaylandEnvVars(entry, r.Cfg.Desktop.CustomEnvVars); err != nil {
			r.Log.Warn().
				Err(err).
				Str("app", normalizedName).
				Msg("invalid custom Wayland env vars, injecting defaults only")
			if fallbackErr := desktop.InjectWaylandEnvVars(entry, nil); fallbackErr != nil {
				r.Log.Warn().Err(fallbackErr).Str("app", normalizedName).Msg("failed to inject default Wayland env vars")
			}
		}
	}

	return desktopFilePath, desktop.WriteDesktopFile(desktopFilePath, entry)
}

func (r *RpmBackend) getPackageInfo(ctx context.Context, pkgName string) (*packageInfo, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	info, err := r.sys.GetInfo(queryCtx, pkgName)
	if err != nil {
		return nil, err
	}

	return &packageInfo{
		name:    info.Name,
		version: info.Version,
	}, nil
}

func (r *RpmBackend) findInstalledFiles(ctx context.Context, pkgName string) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return r.sys.ListFiles(queryCtx, pkgName)
}

func (r *RpmBackend) findDesktopFiles(files []string) []string {
	var desktopFiles []string
	for _, file := range files {
		if strings.HasSuffix(file, ".desktop") {
			desktopFiles = append(desktopFiles, file)
		}
	}
	return desktopFiles
}

func (r *RpmBackend) findIconFiles(files []string) []string {
	var iconFiles []string
	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file))
		if (ext == ".png" || ext == ".svg" || ext == ".ico" || ext == ".xpm") && strings.Contains(file, "icons") {
			iconFiles = append(iconFiles, file)
		}
	}
	return iconFiles
}

// Helper types and functions

type packageInfo struct {
	name    string
	version string
}

// queryRpmName extracts the official package name from RPM metadata using rpm -qp
// This is the best practice as it uses the authoritative NAME field from the RPM header
// instead of parsing the filename which may not match the actual package name.
func (r *RpmBackend) queryRpmName(ctx context.Context, packagePath string) (string, error) {
	// Check if rpm command is available
	if !r.Runner.CommandExists("rpm") {
		return "", fmt.Errorf("rpm command not found")
	}

	// Convert to absolute path for reliability
	absPath, err := filepath.Abs(packagePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Query RPM metadata for package name using %{NAME} tag
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	output, err := r.Runner.RunCommand(queryCtx, "rpm", "-qp", "--queryformat", "%{NAME}", absPath)
	if err != nil {
		return "", fmt.Errorf("rpm query failed: %w", err)
	}

	name := strings.TrimSpace(output)
	if name == "" {
		return "", fmt.Errorf("empty package name returned")
	}

	return name, nil
}

// extractRpmBaseName extracts the base package name from an RPM filename
// Examples:
//   - GitButler_Nightly-0.5.1650-1.x86_64.rpm -> GitButler_Nightly
//   - firefox-123.0-1.x86_64.rpm -> firefox
//   - google-chrome-stable-120.0.6099.109-1.x86_64.rpm -> google-chrome-stable
func extractRpmBaseName(filename string) string {
	// Remove .rpm extension
	name := strings.TrimSuffix(filename, ".rpm")

	// Remove known architecture suffixes
	knownArchs := []string{".x86_64", ".aarch64", ".i686", ".i386", ".noarch", ".armv7hl", ".ppc64le", ".s390x"}
	for _, arch := range knownArchs {
		name = strings.TrimSuffix(name, arch)
	}

	// Split by hyphens to find version-release pattern
	parts := strings.Split(name, "-")
	if len(parts) < 2 {
		return name
	}

	// Find the first part that looks like a version number (starts with digit)
	// Everything before that is the package name
	for i := len(parts) - 1; i > 0; i-- {
		if len(parts[i]) > 0 && parts[i][0] >= '0' && parts[i][0] <= '9' {
			// This looks like version or release, keep searching backwards
			continue
		}
		// Found a non-numeric part, this is likely still part of the name
		return strings.Join(parts[:i+1], "-")
	}

	// If all parts after first are numeric, just return first part
	return parts[0]
}

// No local helper functions - using shared helpers from internal/helpers/common.go

//nolint:gocyclo // safe recursive copy with symlink handling is inherently branching.
func (r *RpmBackend) copyDir(src, dst string) error {
	return afero.Walk(r.Fs, src, func(path string, info fs.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}

		dstPath := filepath.Join(dst, relPath)

		// Handle directories
		if info.IsDir() {
			if validateErr := security.ValidateExtractPath(dst, relPath); validateErr != nil {
				return nil
			}
			return r.Fs.MkdirAll(dstPath, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&fs.ModeSymlink != 0 {
			linkReader, ok := r.Fs.(afero.LinkReader)
			if !ok {
				return nil
			}
			linkTarget, readlinkErr := linkReader.ReadlinkIfPossible(path)
			if readlinkErr != nil {
				// Skip broken symlinks
				return nil
			}

			if validateErr := security.ValidateSymlink(dst, dstPath, linkTarget); validateErr != nil {
				return nil
			}
			// Create symlink at destination
			if mkdirErr := r.Fs.MkdirAll(filepath.Dir(dstPath), 0755); mkdirErr != nil {
				return nil
			}
			linker, ok := r.Fs.(afero.Linker)
			if !ok {
				return nil
			}
			return linker.SymlinkIfPossible(linkTarget, dstPath)
		}

		// Handle regular files using streaming to avoid loading entire file in memory
		if validateErr := security.ValidateExtractPath(dst, relPath); validateErr != nil {
			return nil
		}

		srcFile, openErr := r.Fs.Open(path)
		if openErr != nil {
			// Skip files that can't be read
			return nil
		}
		defer func() {
			if closeErr := srcFile.Close(); closeErr != nil {
				r.Log.Debug().Err(closeErr).Str("path", path).Msg("failed to close source file")
			}
		}()

		if mkdirErr := r.Fs.MkdirAll(filepath.Dir(dstPath), 0755); mkdirErr != nil {
			return nil
		}
		dstFile, createErr := r.Fs.Create(dstPath)
		if createErr != nil {
			return fmt.Errorf("failed to create destination file: %w", createErr)
		}
		defer func() {
			if closeErr := dstFile.Close(); closeErr != nil {
				r.Log.Debug().Err(closeErr).Str("path", dstPath).Msg("failed to close destination file")
			}
		}()

		// Use io.Copy for efficient streaming copy
		if _, copyErr := io.Copy(dstFile, srcFile); copyErr != nil {
			return fmt.Errorf("failed to copy file data: %w", copyErr)
		}

		// Preserve original permissions
		if chmodErr := r.Fs.Chmod(dstPath, info.Mode()); chmodErr != nil {
			r.Log.Debug().Err(chmodErr).Str("path", dstPath).Msg("failed to preserve file permissions")
		}

		return nil
	})
}
