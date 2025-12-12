package rpm

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
)

// RpmBackend handles RPM package installations
type RpmBackend struct {
	cfg          *config.Config
	logger       *zerolog.Logger
	scorer       heuristics.Scorer
	sys          syspkg.Provider
	runner       helpers.CommandRunner
	cacheManager *cache.CacheManager
}

// New creates a new RPM backend
func New(cfg *config.Config, log *zerolog.Logger) *RpmBackend {
	return &RpmBackend{
		cfg:          cfg,
		logger:       log,
		scorer:       heuristics.NewScorer(log),
		sys:          arch.NewPacmanProvider(),
		runner:       helpers.NewOSCommandRunner(),
		cacheManager: cache.NewCacheManager(),
	}
}

// NewWithRunner creates a new RPM backend with a custom command runner
func NewWithRunner(cfg *config.Config, log *zerolog.Logger, runner helpers.CommandRunner) *RpmBackend {
	return &RpmBackend{
		cfg:          cfg,
		logger:       log,
		scorer:       heuristics.NewScorer(log),
		sys:          arch.NewPacmanProvider(),
		runner:       runner,
		cacheManager: cache.NewCacheManager(),
	}
}

// NewWithCacheManager creates a new RPM backend with a custom cache manager
func NewWithCacheManager(cfg *config.Config, log *zerolog.Logger, cacheManager *cache.CacheManager) *RpmBackend {
	return &RpmBackend{
		cfg:          cfg,
		logger:       log,
		scorer:       heuristics.NewScorer(log),
		sys:          arch.NewPacmanProvider(),
		runner:       helpers.NewOSCommandRunner(),
		cacheManager: cacheManager,
	}
}

// Name returns the backend name
func (r *RpmBackend) Name() string {
	return "rpm"
}

// Detect checks if this backend can handle the package
func (r *RpmBackend) Detect(ctx context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := os.Stat(packagePath); err != nil {
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
	r.logger.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing RPM package")

	// Validate package exists
	if _, err := os.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	// Determine package name
	pkgName := opts.CustomName
	if pkgName == "" {
		// Try to get official package name from RPM metadata (best practice)
		if name, err := r.queryRpmName(ctx, packagePath); err == nil && name != "" {
			pkgName = name
			r.logger.Debug().
				Str("name", name).
				Msg("extracted package name from RPM metadata")
		} else {
			// Fallback: Extract base name from RPM filename
			pkgName = extractRpmBaseName(filepath.Base(packagePath))
			r.logger.Debug().
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
	if r.runner.CommandExists("rpmextract.sh") {
		return r.installWithExtract(ctx, packagePath, normalizedName, installID, opts, tx)
	}

	// Fallback: check if we can use debtap (on Arch)
	if r.runner.CommandExists("debtap") && r.runner.CommandExists("pacman") {
		r.logger.Info().Msg("using debtap/pacman method for RPM installation")
		return r.installWithDebtap(ctx, packagePath, normalizedName, installID, opts, tx)
	}

	return nil, fmt.Errorf("no suitable RPM installation method found\nInstall either 'rpmextract.sh' or 'debtap' (Arch)")
}

// installWithExtract installs RPM by extracting and manually placing files
func (r *RpmBackend) installWithExtract(ctx context.Context, packagePath, normalizedName, installID string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	r.logger.Info().Msg("extracting RPM package...")

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	// Convert to absolute path before changing directories
	absPackagePath, err := filepath.Abs(packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Create temp directory for extraction
	tmpDir, err := os.MkdirTemp("", "upkg-rpm-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		return nil, fmt.Errorf("failed to change to temp directory: %w", err)
	}

	// Extract RPM (using absolute path)
	extractCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err = r.runner.RunCommand(extractCtx, "rpmextract.sh", absPackagePath)
	if err != nil {
		return nil, fmt.Errorf("rpmextract.sh failed: %w", err)
	}

	r.logger.Debug().Msg("RPM extracted successfully")

	// Create installation directory
	appsDir := filepath.Join(homeDir, ".local", "share", "upkg", "apps")
	installDir := filepath.Join(appsDir, normalizedName)

	if _, err := os.Stat(installDir); err == nil {
		if !opts.Force {
			return nil, fmt.Errorf("package already installed at: %s (use --force to reinstall)", installDir)
		}
		if err := os.RemoveAll(installDir); err != nil {
			return nil, fmt.Errorf("remove existing installation directory: %w", err)
		}
		// Best-effort cleanup of expected wrapper/desktop paths
		binDir := filepath.Join(homeDir, ".local", "bin")
		_ = os.Remove(filepath.Join(binDir, normalizedName))
		appsDbDir := filepath.Join(homeDir, ".local", "share", "applications")
		_ = os.Remove(filepath.Join(appsDbDir, normalizedName+".desktop"))
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create installation directory: %w", err)
	}
	if tx != nil {
		dir := installDir
		tx.Add("remove rpm installation directory", func() error {
			return os.RemoveAll(dir)
		})
	}

	// Move extracted content to installation directory
	// RPMs typically extract to usr/, opt/, etc.
	extractedDirs := []string{"usr", "opt", "etc"}
	for _, dir := range extractedDirs {
		srcDir := filepath.Join(tmpDir, dir)
		if _, err := os.Stat(srcDir); err == nil {
			dstDir := filepath.Join(installDir, dir)
			if err := os.Rename(srcDir, dstDir); err != nil {
				// Try copying if rename fails
				if err := copyDir(srcDir, dstDir); err != nil {
					r.logger.Warn().
						Err(err).
						Str("dir", dir).
						Msg("failed to move directory")
				}
			}
		}
	}

	// Find executables
	executables, err := heuristics.FindExecutables(installDir)
	if err != nil || len(executables) == 0 {
		os.RemoveAll(installDir)
		return nil, fmt.Errorf("no executables found in RPM")
	}

	r.logger.Debug().
		Strs("executables", executables).
		Msg("found executables")

	// Choose primary executable using scoring heuristic (same as tarball backend)
	primaryExec := r.scorer.ChooseBest(executables, normalizedName, installDir)

	// Create wrapper script
	binDir := filepath.Join(homeDir, ".local", "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		os.RemoveAll(installDir)
		return nil, fmt.Errorf("failed to create bin directory: %w", err)
	}

	wrapperPath := filepath.Join(binDir, normalizedName)
	if err := r.createWrapper(wrapperPath, primaryExec); err != nil {
		os.RemoveAll(installDir)
		return nil, fmt.Errorf("failed to create wrapper script: %w", err)
	}
	if tx != nil {
		path := wrapperPath
		tx.Add("remove rpm wrapper script", func() error {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		})
	}

	// Install icons
	iconPaths, err := r.installIcons(installDir, normalizedName)
	if err != nil {
		r.logger.Warn().Err(err).Msg("failed to install icons")
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
			os.RemoveAll(installDir)
			os.Remove(wrapperPath)
			r.removeIcons(iconPaths)
			return nil, fmt.Errorf("failed to create desktop file: %w", err)
		}

		if tx != nil && desktopPath != "" {
			path := desktopPath
			tx.Add("remove rpm desktop file", func() error {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
				return nil
			})
		}

		// Update caches
		appsDbDir := filepath.Join(homeDir, ".local", "share", "applications")
		r.cacheManager.UpdateDesktopDatabase(appsDbDir, r.logger)

		iconsDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor")
		r.cacheManager.UpdateIconCache(iconsDir, r.logger)
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

	r.logger.Info().
		Str("install_id", installID).
		Str("name", normalizedName).
		Str("path", installDir).
		Msg("RPM package installed successfully (extracted)")

	return record, nil
}

// installWithDebtap installs RPM by converting to Arch package via debtap
func (r *RpmBackend) installWithDebtap(ctx context.Context, packagePath, normalizedName, installID string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	r.logger.Info().Msg("converting RPM to Arch package via debtap...")

	// Convert to absolute path before changing directories
	absPackagePath, err := filepath.Abs(packagePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "upkg-rpm-debtap-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory
	originalDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		return nil, fmt.Errorf("failed to change to temp directory: %w", err)
	}

	// Convert RPM with debtap (using absolute path)
	convertCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	_, err = r.runner.RunCommand(convertCtx, "debtap", "-q", "-Q", absPackagePath)
	if err != nil {
		return nil, fmt.Errorf("debtap conversion failed: %w", err)
	}

	// Find generated .pkg.tar.* file
	files, err := filepath.Glob(filepath.Join(tmpDir, "*.pkg.tar.*"))
	if err != nil || len(files) == 0 {
		return nil, fmt.Errorf("no arch package generated by debtap")
	}

	archPkgPath := files[0]

	// Install with pacman
	r.logger.Info().Msg("installing converted package with pacman...")

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
	pkgInfo, _ := r.getPackageInfo(ctx, normalizedName)
	if pkgInfo == nil {
		pkgInfo = &packageInfo{
			name:    normalizedName,
			version: "unknown",
		}
	}

	// Find installed files
	installedFiles, _ := r.findInstalledFiles(ctx, normalizedName)

	// Find desktop and icon files
	desktopFiles := r.findDesktopFiles(installedFiles)
	iconFiles := r.findIconFiles(installedFiles)

	var primaryDesktopFile string
	if len(desktopFiles) > 0 {
		primaryDesktopFile = desktopFiles[0]
	}

	// Update caches
	if len(desktopFiles) > 0 {
		r.cacheManager.UpdateDesktopDatabase("/usr/share/applications", r.logger)
	}
	if len(iconFiles) > 0 {
		r.cacheManager.UpdateIconCache("/usr/share/icons/hicolor", r.logger)
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
		},
	}

	r.logger.Info().
		Str("install_id", installID).
		Str("name", pkgInfo.name).
		Msg("RPM package installed successfully (pacman)")

	return record, nil
}

// Uninstall removes the installed RPM package
func (r *RpmBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	r.logger.Info().
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
		r.logger.Warn().Msg("package not found in pacman database")
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
	r.cacheManager.UpdateDesktopDatabase("/usr/share/applications", r.logger)
	r.cacheManager.UpdateIconCache("/usr/share/icons/hicolor", r.logger)

	return nil
}

// uninstallExtracted removes RPM installed via extraction
func (r *RpmBackend) uninstallExtracted(ctx context.Context, record *core.InstallRecord) error {
	// Remove installation directory
	if record.InstallPath != "" {
		if err := os.RemoveAll(record.InstallPath); err != nil && !os.IsNotExist(err) {
			r.logger.Warn().Err(err).Msg("failed to remove installation directory")
		}
	}

	// Remove wrapper script
	if record.Metadata.WrapperScript != "" {
		if err := os.Remove(record.Metadata.WrapperScript); err != nil && !os.IsNotExist(err) {
			r.logger.Warn().Err(err).Msg("failed to remove wrapper script")
		}
	}

	// Remove desktop file
	if record.DesktopFile != "" {
		if err := os.Remove(record.DesktopFile); err != nil && !os.IsNotExist(err) {
			r.logger.Warn().Err(err).Msg("failed to remove desktop file")
		}
	}

	// Remove icons
	r.removeIcons(record.Metadata.IconFiles)

	// Update caches
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		appsDir := filepath.Join(homeDir, ".local", "share", "applications")
		r.cacheManager.UpdateDesktopDatabase(appsDir, r.logger)

		iconsDir := filepath.Join(homeDir, ".local", "share", "icons", "hicolor")
		r.cacheManager.UpdateIconCache(iconsDir, r.logger)
	}

	return nil
}

// Helper functions

func (r *RpmBackend) createWrapper(wrapperPath, execPath string) error {
	content := fmt.Sprintf(`#!/bin/bash
# upkg wrapper script
exec "%s" "$@"
`, execPath)

	return os.WriteFile(wrapperPath, []byte(content), 0755)
}

func (r *RpmBackend) installIcons(installDir, normalizedName string) ([]string, error) {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return nil, fmt.Errorf("failed to get home directory")
	}

	discoveredIcons := icons.DiscoverIcons(installDir)
	var installedIcons []string

	for _, iconFile := range discoveredIcons {
		targetPath, err := icons.InstallIcon(iconFile, normalizedName, homeDir)
		if err != nil {
			continue
		}
		installedIcons = append(installedIcons, targetPath)
	}

	return installedIcons, nil
}

func (r *RpmBackend) removeIcons(iconPaths []string) {
	for _, iconPath := range iconPaths {
		os.Remove(iconPath)
	}
}

func (r *RpmBackend) createDesktopFile(installDir, normalizedName, wrapperPath string, opts core.InstallOptions) (string, error) {
	homeDir, _ := os.UserHomeDir()
	if homeDir == "" {
		return "", fmt.Errorf("failed to get home directory")
	}

	appsDir := filepath.Join(homeDir, ".local", "share", "applications")
	os.MkdirAll(appsDir, 0755)

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
		matches, _ := filepath.Glob(filepath.Join(searchPath, "*.desktop"))
		if len(matches) > 0 {
			// Found desktop file(s), parse the first one
			file, err := os.Open(matches[0])
			if err == nil {
				defer file.Close()
				entry, err = desktop.Parse(file)
				if err == nil {
					r.logger.Debug().
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
		r.logger.Debug().Msg("no desktop file found in RPM, creating default")

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
	if r.cfg.Desktop.WaylandEnvVars && !opts.SkipWaylandEnv {
		if err := desktop.InjectWaylandEnvVars(entry, r.cfg.Desktop.CustomEnvVars); err != nil {
			r.logger.Warn().
				Err(err).
				Str("app", normalizedName).
				Msg("invalid custom Wayland env vars, injecting defaults only")
			_ = desktop.InjectWaylandEnvVars(entry, nil)
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
	if !r.runner.CommandExists("rpm") {
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

	output, err := r.runner.RunCommand(queryCtx, "rpm", "-qp", "--queryformat", "%{NAME}", absPath)
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

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		// Handle directories
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(path)
			if err != nil {
				// Skip broken symlinks
				return nil
			}

			// Validate symlink doesn't escape destination directory
			linkDir := filepath.Dir(dstPath)
			resolvedTarget := filepath.Join(linkDir, linkTarget)

			// Get absolute paths
			absDst, err := filepath.Abs(dst)
			if err != nil {
				return fmt.Errorf("failed to resolve destination: %w", err)
			}

			absTarget, err := filepath.Abs(resolvedTarget)
			if err != nil {
				return fmt.Errorf("failed to resolve symlink target: %w", err)
			}

			// Check if symlink target is within destination
			rel, err := filepath.Rel(absDst, absTarget)
			if err != nil {
				return fmt.Errorf("failed to compute relative path: %w", err)
			}

			if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
				// Skip symlinks that escape destination
				return nil
			}

			// Create symlink at destination
			return os.Symlink(linkTarget, dstPath)
		}

		// Handle regular files using streaming to avoid loading entire file in memory
		srcFile, err := os.Open(path)
		if err != nil {
			// Skip files that can't be read
			return nil
		}
		defer srcFile.Close()

		dstFile, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
		if err != nil {
			return fmt.Errorf("failed to create destination file: %w", err)
		}
		defer dstFile.Close()

		// Use io.Copy for efficient streaming copy
		if _, err := io.Copy(dstFile, srcFile); err != nil {
			return fmt.Errorf("failed to copy file data: %w", err)
		}

		return nil
	})
}
