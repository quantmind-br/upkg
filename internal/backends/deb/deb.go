package deb

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
	"github.com/quantmind-br/upkg/internal/syspkg"
	"github.com/quantmind-br/upkg/internal/syspkg/arch"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/quantmind-br/upkg/internal/ui"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// DebBackend handles DEB package installations via debtap
//
//nolint:revive // exported backend names are kept for consistency across packages.
type DebBackend struct {
	*backendbase.BaseBackend
	sys          syspkg.Provider
	cacheManager *cache.CacheManager
}

// New creates a new DEB backend
func New(cfg *config.Config, log *zerolog.Logger) *DebBackend {
	base := backendbase.New(cfg, log)
	return &DebBackend{
		BaseBackend:  base,
		sys:          arch.NewPacmanProvider(),
		cacheManager: cache.NewCacheManagerWithRunner(base.Runner),
	}
}

// NewWithRunner creates a new DEB backend with a custom command runner
func NewWithRunner(cfg *config.Config, log *zerolog.Logger, runner helpers.CommandRunner) *DebBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), runner)
}

// NewWithDeps creates a new DEB backend with injected fs and runner.
func NewWithDeps(cfg *config.Config, log *zerolog.Logger, fs afero.Fs, runner helpers.CommandRunner) *DebBackend {
	base := backendbase.NewWithDeps(cfg, log, fs, runner)
	return &DebBackend{
		BaseBackend:  base,
		sys:          arch.NewPacmanProvider(),
		cacheManager: cache.NewCacheManagerWithRunner(runner),
	}
}

// NewWithCacheManager creates a new DEB backend with a custom cache manager
func NewWithCacheManager(cfg *config.Config, log *zerolog.Logger, cacheManager *cache.CacheManager) *DebBackend {
	base := backendbase.New(cfg, log)
	return &DebBackend{
		BaseBackend:  base,
		sys:          arch.NewPacmanProvider(),
		cacheManager: cacheManager,
	}
}

// Name returns the backend name
func (d *DebBackend) Name() string {
	return "deb"
}

// Detect checks if this backend can handle the package
func (d *DebBackend) Detect(_ context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := d.Fs.Stat(packagePath); err != nil {
		return false, nil
	}

	// Check file type
	fileType, err := helpers.DetectFileType(packagePath)
	if err != nil {
		return false, err
	}

	return fileType == helpers.FileTypeDEB, nil
}

// Install installs the DEB package using debtap
//
//nolint:gocyclo // multi-step install with progress, conversion, pacman and desktop integration.
func (d *DebBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	d.Log.Info().
		Str("package_path", packagePath).
		Str("custom_name", opts.CustomName).
		Msg("installing DEB package")

	// Define installation phases with weights
	phases := []ui.InstallationPhase{
		{Name: "Validating package", Weight: 5, Deterministic: true},
		{Name: "Extracting metadata", Weight: 5, Deterministic: true},
		{Name: "Converting DEB to Arch", Weight: 60, Deterministic: false}, // Indeterminate - uses spinner
		{Name: "Fixing dependencies", Weight: 5, Deterministic: true},
		{Name: "Installing with pacman", Weight: 20, Deterministic: false}, // Indeterminate - uses spinner
		{Name: "Configuring desktop", Weight: 5, Deterministic: true},
	}

	// Create progress tracker (enabled unless in quiet mode)
	progressEnabled := d.Log.GetLevel() != zerolog.Disabled && d.Log.GetLevel() <= zerolog.InfoLevel
	progress := ui.NewProgressTracker(phases, "Installing DEB", progressEnabled)
	defer progress.Finish()

	// Phase 1: Validation
	progress.StartPhase(0)

	// Check if debtap is installed
	if err := d.Runner.RequireCommand("debtap"); err != nil {
		return nil, fmt.Errorf("debtap is required for DEB installation: %w\nInstall with: yay -S debtap", err)
	}

	// Check if pacman is available (we're on Arch)
	if err := d.Runner.RequireCommand("pacman"); err != nil {
		return nil, fmt.Errorf("pacman not found - DEB backend requires Arch Linux")
	}

	// Check if debtap is initialized
	if !isDebtapInitialized() {
		return nil, fmt.Errorf("debtap is not initialized\nRun the following command to initialize:\n  sudo debtap -u")
	}

	// Validate package exists
	if _, err := d.Fs.Stat(packagePath); err != nil {
		return nil, fmt.Errorf("package not found: %w", err)
	}

	progress.AdvancePhase()

	// Phase 2: Extract metadata
	progress.StartPhase(1)

	// Determine package name
	pkgName := opts.CustomName
	if pkgName == "" {
		// Try to get official package name from DEB metadata (best practice)
		if name, err := d.queryDebName(ctx, packagePath); err == nil && name != "" {
			pkgName = name
			d.Log.Debug().
				Str("name", name).
				Msg("extracted package name from DEB metadata")
		} else {
			// Fallback: Extract base name from DEB filename
			pkgName = filepath.Base(packagePath)
			pkgName = strings.TrimSuffix(pkgName, filepath.Ext(pkgName))
			d.Log.Debug().
				Str("name", pkgName).
				Msg("extracted package name from filename (dpkg-deb unavailable)")
		}
	}

	normalizedName := helpers.NormalizeFilename(pkgName)
	pacmanPkgName := normalizedName
	var pkgMeta *packageInfo

	d.Log.Debug().
		Str("package_name", pkgName).
		Str("normalized_name", normalizedName).
		Msg("package name determined")

	// Create temp directory for conversion
	tmpDir, err := afero.TempDir(d.Fs, "", "upkg-deb-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := d.Fs.RemoveAll(tmpDir); removeErr != nil {
			d.Log.Debug().Err(removeErr).Str("tmp_dir", tmpDir).Msg("failed to remove temp dir")
		}
	}()

	progress.AdvancePhase()

	// Phase 3: Convert DEB to Arch package (indeterminate phase)
	progress.StartPhase(2)

	archPkgPath, err := d.convertWithDebtapProgress(ctx, packagePath, tmpDir, normalizedName, progress)
	if err != nil {
		return nil, fmt.Errorf("debtap conversion failed: %w", err)
	}

	d.Log.Debug().
		Str("arch_package", archPkgPath).
		Msg("DEB converted to Arch package")

	progress.AdvancePhase()

	// Phase 4: Fix dependencies
	progress.StartPhase(3)

	d.Log.Info().Msg("checking and fixing malformed dependencies...")
	if fixErr := fixMalformedDependencies(archPkgPath, d.Log); fixErr != nil {
		d.Log.Warn().Err(fixErr).Msg("failed to fix malformed dependencies, proceeding anyway")
	}

	// Read package metadata to determine actual pacman package name
	pkgMeta, err = extractPackageInfoFromArchive(archPkgPath)
	if err != nil {
		d.Log.Warn().Err(err).Str("fallback_name", pacmanPkgName).Msg("failed to read package metadata from archive")
	} else if pkgMeta.name != "" {
		pacmanPkgName = pkgMeta.name
		d.Log.Debug().
			Str("package_name", pkgMeta.name).
			Str("normalized_name", normalizedName).
			Msg("resolved pacman package name from archive metadata")
	}

	installID := helpers.GenerateInstallID(pacmanPkgName)

	progress.AdvancePhase()

	// Phase 5: Install with pacman (indeterminate phase)
	progress.StartPhase(4)

	// Need sudo for pacman
	installCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Update progress during pacman installation
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-ticker.C:
				progress.UpdateIndeterminateWithElapsed("Installing with pacman", time.Since(start))
			case <-installCtx.Done():
				return
			}
		}
	}()

	err = d.sys.Install(installCtx, archPkgPath, &syspkg.InstallOptions{Overwrite: opts.Overwrite})
	if err != nil {
		return nil, fmt.Errorf("pacman installation failed: %w", err)
	}
	if tx != nil {
		pkgName := pacmanPkgName
		tx.Add("remove pacman package", func() error {
			removeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			return d.sys.Remove(removeCtx, pkgName)
		})
	}

	d.Log.Info().Msg("package installed successfully via pacman")

	progress.AdvancePhase()

	// Phase 6: Desktop integration
	progress.StartPhase(5)

	// Get package info from pacman
	pkgInfo, err := d.getPackageInfo(ctx, pacmanPkgName)
	if err != nil {
		d.Log.Warn().Err(err).
			Str("package", pacmanPkgName).
			Msg("failed to get package info from pacman")
		fallbackVersion := "unknown"
		if pkgMeta != nil && pkgMeta.version != "" {
			fallbackVersion = pkgMeta.version
		}
		pkgInfo = &packageInfo{
			name:    pacmanPkgName,
			version: fallbackVersion,
		}
	}

	if pkgInfo.version == "" && pkgMeta != nil && pkgMeta.version != "" {
		pkgInfo.version = pkgMeta.version
	}

	// Find installed files
	installedFiles, err := d.findInstalledFiles(ctx, pacmanPkgName)
	if err != nil {
		d.Log.Warn().Err(err).Msg("failed to list installed files")
	}

	// Find desktop files
	desktopFiles := d.findDesktopFiles(installedFiles)

	// Update desktop files with Wayland env vars if needed
	var primaryDesktopFile string
	if len(desktopFiles) > 0 {
		primaryDesktopFile = desktopFiles[0]

		if d.Cfg.Desktop.WaylandEnvVars {
			for _, desktopFile := range desktopFiles {
				if err := d.updateDesktopFileWayland(desktopFile); err != nil {
					d.Log.Warn().
						Err(err).
						Str("desktop_file", desktopFile).
						Msg("failed to update desktop file with Wayland vars")
				}
			}
		}
	}

	// Find icon files
	iconFiles := d.findIconFiles(installedFiles)
	fallbackIcons, fallbackErr := d.installUserIconFallback(iconFiles, primaryDesktopFile)
	if fallbackErr != nil {
		d.Log.Warn().Err(fallbackErr).Msg("failed to install fallback icons")
	} else if len(fallbackIcons) > 0 {
		iconFiles = append(iconFiles, fallbackIcons...)
		iconsDir := d.Paths.GetIconsDir()
		if cacheErr := d.cacheManager.UpdateIconCache(iconsDir, d.Log); cacheErr != nil {
			d.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update user icon cache")
		}
	}

	// Update caches
	if len(desktopFiles) > 0 {
		appsDir := filepath.Dir(desktopFiles[0])
		if cacheErr := d.cacheManager.UpdateDesktopDatabase(appsDir, d.Log); cacheErr != nil {
			d.Log.Warn().Err(cacheErr).Str("apps_dir", appsDir).Msg("failed to update desktop database")
		}
	}

	if len(iconFiles) > 0 {
		// Find hicolor icon directory
		for _, iconFile := range iconFiles {
			if strings.Contains(iconFile, "hicolor") {
				hicolorDir := filepath.Dir(filepath.Dir(filepath.Dir(iconFile)))
				if cacheErr := d.cacheManager.UpdateIconCache(hicolorDir, d.Log); cacheErr != nil {
					d.Log.Warn().Err(cacheErr).Str("icons_dir", hicolorDir).Msg("failed to update icon cache")
				}
				break
			}
		}
	}

	// Create install record
	record := &core.InstallRecord{
		InstallID:    installID,
		PackageType:  core.PackageTypeDeb,
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
			ExtractedMeta: core.ExtractedMetadata{
				Comment: "Installed via debtap/pacman",
			},
		},
	}

	d.Log.Info().
		Str("install_id", installID).
		Str("name", pkgInfo.name).
		Str("version", pkgInfo.version).
		Msg("DEB package installed successfully")

	return record, nil
}

// Uninstall removes the installed DEB package via pacman
func (d *DebBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	d.Log.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling DEB package")

	// Extract package name from InstallPath metadata
	pkgName := record.Name
	normalizedName := helpers.NormalizeFilename(pkgName)

	// Check if package is still installed
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	installed, err := d.sys.IsInstalled(checkCtx, normalizedName)
	if err != nil || !installed {
		d.Log.Warn().
			Str("package", normalizedName).
			Msg("package not found in pacman database")
		return nil // Already uninstalled
	}

	// Uninstall with pacman
	d.Log.Info().Msg("removing package with pacman...")

	uninstallCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	err = d.sys.Remove(uninstallCtx, normalizedName)
	if err != nil {
		return fmt.Errorf("pacman removal failed: %w", err)
	}

	// Update caches
	if cacheErr := d.cacheManager.UpdateDesktopDatabase("/usr/share/applications", d.Log); cacheErr != nil {
		d.Log.Warn().Err(cacheErr).Msg("failed to update desktop database")
	}
	if cacheErr := d.cacheManager.UpdateIconCache("/usr/share/icons/hicolor", d.Log); cacheErr != nil {
		d.Log.Warn().Err(cacheErr).Msg("failed to update icon cache")
	}

	if d.removeUserIcons(record.Metadata.IconFiles) {
		iconsDir := d.Paths.GetIconsDir()
		if cacheErr := d.cacheManager.UpdateIconCache(iconsDir, d.Log); cacheErr != nil {
			d.Log.Warn().Err(cacheErr).Str("icons_dir", iconsDir).Msg("failed to update user icon cache")
		}
	}

	d.Log.Info().
		Str("install_id", record.InstallID).
		Msg("DEB package uninstalled successfully")

	return nil
}

// getPackageInfo gets package info from pacman
func (d *DebBackend) getPackageInfo(ctx context.Context, pkgName string) (*packageInfo, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	info, err := d.sys.GetInfo(queryCtx, pkgName)
	if err != nil {
		return nil, err
	}

	return &packageInfo{
		name:    info.Name,
		version: info.Version,
	}, nil
}

// findInstalledFiles lists all files installed by the package
func (d *DebBackend) findInstalledFiles(ctx context.Context, pkgName string) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	return d.sys.ListFiles(queryCtx, pkgName)
}

// findDesktopFiles filters for .desktop files
func (d *DebBackend) findDesktopFiles(files []string) []string {
	var desktopFiles []string
	for _, file := range files {
		if strings.HasSuffix(file, ".desktop") {
			desktopFiles = append(desktopFiles, file)
		}
	}
	return desktopFiles
}

// findIconFiles filters for icon files
func (d *DebBackend) findIconFiles(files []string) []string {
	var iconFiles []string
	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file))
		if ext == ".png" || ext == ".svg" || ext == ".ico" || ext == ".xpm" {
			if strings.Contains(file, "icons") {
				iconFiles = append(iconFiles, file)
			}
		}
	}
	return iconFiles
}

// updateDesktopFileWayland updates a desktop file with Wayland environment variables
func (d *DebBackend) updateDesktopFileWayland(desktopPath string) error {
	// Read desktop file
	file, err := d.Fs.Open(desktopPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			d.Log.Debug().Err(closeErr).Str("desktop_file", desktopPath).Msg("failed to close desktop file")
		}
	}()

	entry, err := desktop.Parse(file)
	if err != nil {
		return err
	}

	// Validate desktop entry has required fields
	if valErr := desktop.Validate(entry); valErr != nil {
		return fmt.Errorf("invalid desktop entry: %w", valErr)
	}

	// Inject Wayland vars
	if injectErr := desktop.InjectWaylandEnvVars(entry, d.Cfg.Desktop.CustomEnvVars); injectErr != nil {
		d.Log.Warn().
			Err(injectErr).
			Str("desktop_file", desktopPath).
			Msg("invalid custom Wayland env vars, injecting defaults only")
		if err2 := desktop.InjectWaylandEnvVars(entry, nil); err2 != nil {
			return err2
		}
	}

	// Write back (need sudo for system files)
	tmpFile, err := afero.TempFile(d.Fs, "", "upkg-desktop-*.desktop")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	if closeErr := tmpFile.Close(); closeErr != nil {
		return closeErr
	}

	var buf bytes.Buffer
	if writeErr := desktop.Write(&buf, entry); writeErr != nil {
		if removeErr := d.Fs.Remove(tmpPath); removeErr != nil {
			d.Log.Debug().Err(removeErr).Str("path", tmpPath).Msg("failed to remove temp desktop file")
		}
		return writeErr
	}
	if writeErr := afero.WriteFile(d.Fs, tmpPath, buf.Bytes(), 0644); writeErr != nil {
		if removeErr := d.Fs.Remove(tmpPath); removeErr != nil {
			d.Log.Debug().Err(removeErr).Str("path", tmpPath).Msg("failed to remove temp desktop file")
		}
		return writeErr
	}

	// Move with sudo
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = d.Runner.RunCommand(ctx, "sudo", "mv", tmpPath, desktopPath)
	if err != nil {
		if removeErr := d.Fs.Remove(tmpPath); removeErr != nil {
			d.Log.Debug().Err(removeErr).Str("path", tmpPath).Msg("failed to remove temp desktop file")
		}
		return err
	}

	return nil
}

// Helper types and functions

type packageInfo struct {
	name    string
	version string
}

// queryDebName extracts the official package name from DEB metadata using dpkg-deb
// This is the best practice as it uses the authoritative "Package" field from the control file
// instead of parsing the filename which may not match the actual package name.
func (d *DebBackend) queryDebName(ctx context.Context, packagePath string) (string, error) {
	// Check if dpkg-deb command is available
	if !d.Runner.CommandExists("dpkg-deb") {
		return "", fmt.Errorf("dpkg-deb command not found")
	}

	// Convert to absolute path for reliability
	absPath, err := filepath.Abs(packagePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Query DEB metadata for package name using --field Package
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	output, err := d.Runner.RunCommand(queryCtx, "dpkg-deb", "--field", absPath, "Package")
	if err != nil {
		return "", fmt.Errorf("dpkg-deb query failed: %w", err)
	}

	name := strings.TrimSpace(output)
	if name == "" {
		return "", fmt.Errorf("empty package name returned")
	}

	return name, nil
}
