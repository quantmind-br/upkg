package rpm

import (
	"context"
	"errors"
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
func (r *RpmBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (record *core.InstallRecord, err error) {
	r.Log.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing RPM package")

	// Ensure a non-nil transaction before any filesystem mutation.
	if tx == nil {
		localTx := transaction.NewManager(r.Log)
		tx = localTx
		defer func() {
			if err != nil {
				if rbErr := localTx.Rollback(); rbErr != nil {
					r.Log.Error().Err(rbErr).Msg("rollback failed")
				}
				return
			}
			if cErr := localTx.Commit(); cErr != nil {
				r.Log.Warn().Err(cErr).Msg("transaction commit cleanup failed")
			}
		}()
	}

	// Validate package exists
	if _, statErr := r.Fs.Stat(packagePath); statErr != nil {
		return nil, fmt.Errorf("package not found: %w", statErr)
	}

	// Determine package name
	pkgName := opts.CustomName
	if pkgName == "" {
		// Try to get official package name from RPM metadata (best practice)
		if name, nameErr := r.queryRpmName(ctx, packagePath); nameErr == nil && name != "" {
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
	if validateErr := security.ValidatePackageName(normalizedName); validateErr != nil {
		return nil, fmt.Errorf("invalid normalized name %q: %w", normalizedName, validateErr)
	}
	installID := helpers.GenerateInstallID(normalizedName)

	// Check if rpmextract.sh or bsdtar is available
	if r.Runner.CommandExists("rpmextract.sh") || r.Runner.CommandExists("bsdtar") {
		return r.installWithExtract(ctx, packagePath, normalizedName, installID, opts, tx)
	}

	return nil, fmt.Errorf("no suitable RPM extraction tool found\nInstall 'rpmextract' or 'bsdtar'")
}

// installWithExtract installs RPM by extracting and manually placing files
//
//nolint:gocyclo // extraction install handles multiple fallbacks and integrations.
func (r *RpmBackend) installWithExtract(ctx context.Context, packagePath, normalizedName, installID string, opts core.InstallOptions, tx *transaction.Manager) (record *core.InstallRecord, err error) {
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

	// Use rpmextract.sh if available, otherwise bsdtar
	cmd := "rpmextract.sh"
	args := []string{absPackagePath}
	if !r.Runner.CommandExists("rpmextract.sh") {
		cmd = "bsdtar"
		args = []string{"-xf", absPackagePath}
	}

	_, err = r.Runner.RunCommandInDir(extractCtx, tmpDir, cmd, args...)
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", cmd, err)
	}

	r.Log.Debug().Msg("RPM extracted successfully")

	// Create installation directory
	appsDir := r.Paths.GetUpkgAppsDir()
	installDir := filepath.Join(appsDir, normalizedName)
	binDir := r.Paths.GetBinDir()
	wrapperPath := filepath.Join(binDir, normalizedName)
	appsDbDir := r.Paths.GetAppsDir()
	desktopTarget := filepath.Join(appsDbDir, normalizedName+".desktop")

	// Reject an existing installation unless forced.
	if _, statErr := r.Fs.Stat(installDir); statErr == nil && !opts.Force {
		return nil, fmt.Errorf("package already installed at: %s (use --force to reinstall)", installDir)
	}

	// With --force, back up the replaced artifacts BEFORE creating new ones.
	if opts.Force {
		if bErr := tx.BackupForReplace(r.Fs, installDir); bErr != nil {
			return nil, fmt.Errorf("failed to back up existing installation: %w", bErr)
		}
		if bErr := tx.BackupForReplace(r.Fs, wrapperPath); bErr != nil {
			return nil, fmt.Errorf("failed to back up existing wrapper: %w", bErr)
		}
		if bErr := tx.BackupForReplace(r.Fs, desktopTarget); bErr != nil {
			return nil, fmt.Errorf("failed to back up existing desktop file: %w", bErr)
		}
	}

	// Create installation directory (rollback registered before mutation).
	tx.Add("remove rpm installation directory", func() error {
		return helpers.RemoveIfExists(r.Fs, installDir, true)
	})
	if mkdirErr := r.Fs.MkdirAll(installDir, 0755); mkdirErr != nil {
		return nil, fmt.Errorf("failed to create installation directory: %w", mkdirErr)
	}

	// Move extracted content to installation directory
	// RPMs typically extract to usr/, opt/, etc.
	extractedDirs := []string{"usr", "opt", "etc"}
	for _, dir := range extractedDirs {
		srcDir := filepath.Join(tmpDir, dir)
		if _, statErr := r.Fs.Stat(srcDir); statErr == nil {
			dstDir := filepath.Join(installDir, dir)
			if renameErr := r.Fs.Rename(srcDir, dstDir); renameErr != nil {
				// Try copying if rename fails; a copy failure aborts so the
				// transaction rolls back and restores any previous installation.
				if copyErr := r.copyDir(srcDir, dstDir); copyErr != nil {
					return nil, fmt.Errorf("failed to move extracted directory %q: %w", dir, copyErr)
				}
			}
		}
	}

	// Find executables
	executables, execErr := heuristics.FindExecutables(installDir)
	if execErr != nil || len(executables) == 0 {
		return nil, fmt.Errorf("no executables found in RPM")
	}

	r.Log.Debug().
		Strs("executables", executables).
		Msg("found executables")

	// Choose primary executable using scoring heuristic (same as tarball backend)
	primaryExec := r.scorer.ChooseBest(executables, normalizedName, installDir)

	// Create wrapper script
	if mkdirErr := r.Fs.MkdirAll(binDir, 0755); mkdirErr != nil {
		return nil, fmt.Errorf("failed to create bin directory: %w", mkdirErr)
	}

	wrapperCfg := helpers.WrapperConfig{
		WrapperPath:    wrapperPath,
		ExecPath:       primaryExec,
		DisableSandbox: r.Cfg.Desktop.ElectronDisableSandbox,
	}
	// Register wrapper removal before creating it.
	tx.Add("remove rpm wrapper script", func() error {
		return helpers.RemoveIfExists(r.Fs, wrapperPath, false)
	})
	if wrapperErr := helpers.CreateWrapper(r.Fs, wrapperCfg); wrapperErr != nil {
		return nil, fmt.Errorf("failed to create wrapper script: %w", wrapperErr)
	}

	// Install icons
	iconPaths, iconErr := r.installIcons(installDir, normalizedName)
	if iconErr != nil {
		r.Log.Warn().Err(iconErr).Msg("failed to install icons")
	}
	if len(iconPaths) > 0 {
		paths := append([]string(nil), iconPaths...)
		tx.Add("remove rpm icons", func() error {
			return r.removeIcons(paths)
		})
	}

	// Create .desktop file
	var desktopPath string
	if !opts.SkipDesktop {
		// Register removal of the desktop target before it is written.
		tx.Add("remove rpm desktop file", func() error {
			return helpers.RemoveIfExists(r.Fs, desktopTarget, false)
		})
		desktopPath, err = r.createDesktopFile(installDir, normalizedName, wrapperPath, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		// Update caches (best-effort)
		if cacheErr := r.cacheManager.UpdateDesktopDatabase(appsDbDir, r.Log); cacheErr != nil {
			r.Log.Warn().Err(cacheErr).Str("apps_dir", appsDbDir).Msg("failed to update desktop database")
		}

		iconsDir := r.Paths.GetIconsDir()
		if cacheErr := r.cacheManager.UpdateIconCache(iconsDir, r.Log); cacheErr != nil {
			r.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update icon cache")
		}
	}

	// Create install record
	record = &core.InstallRecord{
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
func (r *RpmBackend) installWithDebtap(ctx context.Context, packagePath, normalizedName, installID string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	return nil, fmt.Errorf("debtap installation is deprecated/unsupported for RPMs")
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
	var errs []error

	// Remove installation directory
	if err := helpers.RemoveIfExists(r.Fs, record.InstallPath, true); err != nil {
		errs = append(errs, err)
	}

	// Remove wrapper script
	if err := helpers.RemoveIfExists(r.Fs, record.Metadata.WrapperScript, false); err != nil {
		errs = append(errs, err)
	}

	// Remove desktop file(s)
	for _, desktopPath := range record.GetDesktopFiles() {
		if desktopPath == "" {
			continue
		}
		if err := helpers.RemoveIfExists(r.Fs, desktopPath, false); err != nil {
			errs = append(errs, err)
		}
	}

	// Remove icons
	if err := r.removeIcons(record.Metadata.IconFiles); err != nil {
		errs = append(errs, err)
	}

	// Update caches (best-effort, runs even when removals failed).
	appsDir := r.Paths.GetAppsDir()
	if cacheErr := r.cacheManager.UpdateDesktopDatabase(appsDir, r.Log); cacheErr != nil {
		r.Log.Warn().Err(cacheErr).Str("apps_dir", appsDir).Msg("failed to update desktop database")
	}

	iconsDir := r.Paths.GetIconsDir()
	if cacheErr := r.cacheManager.UpdateIconCache(iconsDir, r.Log); cacheErr != nil {
		r.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update icon cache")
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
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

func (r *RpmBackend) removeIcons(iconPaths []string) error {
	var errs []error
	for _, iconPath := range iconPaths {
		if removeErr := helpers.RemoveIfExists(r.Fs, iconPath, false); removeErr != nil {
			errs = append(errs, removeErr)
		}
	}
	return errors.Join(errs...)
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
