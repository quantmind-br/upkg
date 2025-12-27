package deb

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

// convertWithDebtapProgress converts a DEB package to Arch package with progress tracking
//
//nolint:gocyclo // debtap conversion involves multiple IO streams and search fallbacks.
func (d *DebBackend) convertWithDebtapProgress(ctx context.Context, debPath, outputDir, expectedPkgName string, progress *ui.ProgressTracker) (string, error) {
	// Run debtap with quiet mode (-q) and skip interactive prompts (-Q)
	convertCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Convert to absolute path since we're changing working directory
	absDebPath, err := filepath.Abs(debPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	d.Log.Debug().
		Str("deb_path", absDebPath).
		Str("output_dir", outputDir).
		Msg("running debtap conversion")

	// Execute debtap with explicit working directory
	// Using -Q for fully automated conversion, then fix dependencies afterwards
	cmd := exec.CommandContext(convertCtx, "debtap", "-q", "-Q", absDebPath)
	cmd.Dir = outputDir // Set working directory so debtap creates package here

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to capture debtap stdout: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to capture debtap stderr: %w", err)
	}

	startErr := cmd.Start()
	if startErr != nil {
		return "", fmt.Errorf("failed to start debtap: %w", startErr)
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		reader := io.TeeReader(stdoutPipe, &stdoutBuf)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			d.Log.Debug().
				Str("line", scanner.Text()).
				Msg("debtap stdout")
		}
		if scanErr := scanner.Err(); scanErr != nil {
			d.Log.Warn().Err(scanErr).Msg("failed to read debtap stdout")
		}
	}()

	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		reader := io.TeeReader(stderrPipe, &stderrBuf)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			d.Log.Debug().
				Str("line", scanner.Text()).
				Msg("debtap stderr")
		}
		if scanErr := scanner.Err(); scanErr != nil {
			d.Log.Warn().Err(scanErr).Msg("failed to read debtap stderr")
		}
	}()

	start := time.Now()
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Update progress bar with spinner and elapsed time
				progress.UpdateIndeterminateWithElapsed("Converting DEB to Arch", time.Since(start))
			case <-progressDone:
				return
			}
		}
	}()

	err = cmd.Wait()
	close(progressDone)
	<-stdoutDone
	<-stderrDone

	if err != nil {
		d.Log.Error().
			Err(err).
			Str("stdout", stdoutBuf.String()).
			Str("stderr", stderrBuf.String()).
			Msg("debtap command failed")
		return "", fmt.Errorf("debtap conversion failed: %w\nStderr: %s", err, stderrBuf.String())
	}

	d.Log.Debug().
		Str("stdout", stdoutBuf.String()).
		Str("stderr", stderrBuf.String()).
		Msg("debtap conversion completed")

	// Find generated .pkg.tar.* file
	// Debtap creates package in current working directory, not in temp dir!

	// First try temp dir (in case debtap behavior changes)
	tempPattern := filepath.Join(outputDir, "*.pkg.tar.*")
	files, err := filepath.Glob(tempPattern)

	if err != nil {
		d.Log.Error().
			Err(err).
			Str("pattern", tempPattern).
			Msg("temp dir glob search failed")
		return "", fmt.Errorf("failed to search for generated package: %w", err)
	}

	if len(files) == 0 {
		// Search in the original package directory (where debtap typically creates the output)
		pkgDir := filepath.Dir(debPath)
		pkgPattern := filepath.Join(pkgDir, "*.pkg.tar.*")
		d.Log.Debug().
			Str("pkg_dir", pkgDir).
			Str("pattern", pkgPattern).
			Msg("searching in package directory")

		pkgFiles, globErr := filepath.Glob(pkgPattern)
		if globErr != nil {
			d.Log.Error().Err(globErr).Str("pattern", pkgPattern).Msg("package dir glob search failed")
		} else {
			files = append(files, pkgFiles...)
		}
	}

	if len(files) == 0 {
		// List all files in output directory for debugging
		allFiles, readDirErr := afero.ReadDir(d.Fs, outputDir)
		if readDirErr != nil {
			d.Log.Error().Err(readDirErr).Str("output_dir", outputDir).Msg("failed to list temp dir contents")
		}
		var fileList []string
		for _, f := range allFiles {
			fileList = append(fileList, f.Name())
		}
		d.Log.Error().
			Strs("files_in_dir", fileList).
			Str("output_dir", outputDir).
			Msg("no arch package generated by debtap in any location")
		return "", fmt.Errorf("no arch package generated by debtap (searched temp, working, and pkg dirs)")
	}

	d.Log.Debug().
		Strs("found_files", files).
		Str("expected_pkg_name", expectedPkgName).
		Msg("found generated packages")

	// Filter files to find the one matching expected package name
	var matchingFiles []string
	for _, file := range files {
		baseName := filepath.Base(file)
		// Package files are named like: pkgname-version-release-arch.pkg.tar.*
		if strings.HasPrefix(baseName, expectedPkgName+"-") ||
			strings.HasPrefix(baseName, strings.ReplaceAll(expectedPkgName, "_", "-")+"-") {
			matchingFiles = append(matchingFiles, file)
		}
	}

	// If we found matching files, use the most recent one
	if len(matchingFiles) > 0 {
		// Sort by modification time (most recent first)
		if len(matchingFiles) > 1 {
			// Get file info for sorting
			type fileWithTime struct {
				path    string
				modTime time.Time
			}
			var filesWithTime []fileWithTime
			for _, f := range matchingFiles {
				info, err := d.Fs.Stat(f)
				if err == nil {
					filesWithTime = append(filesWithTime, fileWithTime{path: f, modTime: info.ModTime()})
				}
			}
			// Sort by modification time descending
			for i := 0; i < len(filesWithTime)-1; i++ {
				for j := i + 1; j < len(filesWithTime); j++ {
					if filesWithTime[j].modTime.After(filesWithTime[i].modTime) {
						filesWithTime[i], filesWithTime[j] = filesWithTime[j], filesWithTime[i]
					}
				}
			}
			if len(filesWithTime) > 0 {
				return filesWithTime[0].path, nil
			}
		}
		return matchingFiles[0], nil
	}

	// No exact match found, log warning and use the most recent file
	d.Log.Warn().
		Str("expected", expectedPkgName).
		Strs("found", files).
		Msg("no package matching expected name, using first available")

	return files[0], nil
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

var (
	iconSizePattern = regexp.MustCompile(`(?i)(\d+)x(\d+)`)
	standardSizes   = map[string]struct{}{
		"16x16":    {},
		"22x22":    {},
		"24x24":    {},
		"32x32":    {},
		"48x48":    {},
		"64x64":    {},
		"128x128":  {},
		"256x256":  {},
		"512x512":  {},
		"scalable": {},
	}
)

func iconNameMatches(iconPath, iconName string) bool {
	if iconName == "" {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(iconPath), filepath.Ext(iconPath))
	return strings.EqualFold(base, iconName)
}

func iconSizeFromPath(path string) (string, bool) {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "scalable") {
		return "scalable", true
	}
	matches := iconSizePattern.FindStringSubmatch(lower)
	if len(matches) >= 3 {
		return matches[1] + "x" + matches[2], true
	}
	return "", false
}

func hasStandardIcon(iconFiles []string, iconName string) bool {
	for _, iconFile := range iconFiles {
		if !iconNameMatches(iconFile, iconName) {
			continue
		}
		size, ok := iconSizeFromPath(iconFile)
		if !ok {
			continue
		}
		if _, exists := standardSizes[size]; exists {
			return true
		}
	}
	return false
}

func selectBestIconSource(iconFiles []string, iconName string) string {
	var matches []string
	for _, iconFile := range iconFiles {
		if iconNameMatches(iconFile, iconName) {
			matches = append(matches, iconFile)
		}
	}

	if len(matches) == 0 {
		return ""
	}

	for _, iconFile := range matches {
		if strings.EqualFold(filepath.Ext(iconFile), ".svg") {
			return iconFile
		}
	}

	best := matches[0]
	bestScore := iconPathSizeScore(best)
	for _, iconFile := range matches[1:] {
		if score := iconPathSizeScore(iconFile); score > bestScore {
			best = iconFile
			bestScore = score
		}
	}

	return best
}

func iconPathSizeScore(path string) int {
	size, ok := iconSizeFromPath(path)
	if !ok {
		return 0
	}
	if size == "scalable" {
		return 100000
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	if height > width {
		return height
	}
	return width
}

func (d *DebBackend) installUserIconFallback(iconFiles []string, desktopFile string) ([]string, error) {
	if len(iconFiles) == 0 || desktopFile == "" {
		return nil, nil
	}

	iconName, err := d.iconNameFromDesktopFile(desktopFile)
	if err != nil || iconName == "" {
		return nil, err
	}

	if hasStandardIcon(iconFiles, iconName) {
		return nil, nil
	}

	source := selectBestIconSource(iconFiles, iconName)
	if source == "" {
		return nil, nil
	}
	if pathErr := security.ValidatePath(source); pathErr != nil {
		return nil, fmt.Errorf("invalid icon source path: %w", pathErr)
	}

	homeDir := d.Paths.HomeDir()
	if homeDir == "" {
		return nil, nil
	}
	if homeErr := security.ValidatePath(homeDir); homeErr != nil {
		return nil, fmt.Errorf("invalid home directory: %w", homeErr)
	}

	iconSize := icons.DetectIconSize(source)
	iconDir := filepath.Join(homeDir, ".local", "share", "icons")
	manager := icons.NewManager(d.Fs, iconDir)

	installedPath, err := manager.InstallIcon(source, iconName, iconSize)
	if err != nil {
		return nil, err
	}

	d.Log.Debug().
		Str("source", source).
		Str("target", installedPath).
		Msg("installed fallback icon")

	return []string{installedPath}, nil
}

func (d *DebBackend) iconNameFromDesktopFile(desktopPath string) (string, error) {
	if desktopPath == "" {
		return "", nil
	}
	if err := security.ValidatePath(desktopPath); err != nil {
		return "", fmt.Errorf("invalid desktop file path: %w", err)
	}

	file, err := d.Fs.Open(desktopPath)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			d.Log.Debug().Err(closeErr).Str("desktop_file", desktopPath).Msg("failed to close desktop file")
		}
	}()

	entry, err := desktop.Parse(file)
	if err != nil {
		return "", err
	}

	iconName := strings.TrimSpace(entry.Icon)
	if iconName == "" || filepath.IsAbs(iconName) || strings.ContainsRune(iconName, filepath.Separator) {
		return "", nil
	}
	if err := security.ValidatePath(iconName); err != nil {
		return "", fmt.Errorf("invalid icon name: %w", err)
	}

	iconName = filepath.Base(iconName)
	iconName = strings.TrimSuffix(iconName, filepath.Ext(iconName))
	return iconName, nil
}

func (d *DebBackend) removeUserIcons(iconPaths []string) bool {
	homeDir := d.Paths.HomeDir()
	if homeDir == "" {
		return false
	}
	if err := security.ValidatePath(homeDir); err != nil {
		d.Log.Debug().Err(err).Str("home_dir", homeDir).Msg("invalid home directory")
		return false
	}

	homeDir = filepath.Clean(homeDir)
	removedAny := false

	for _, iconPath := range iconPaths {
		if err := security.ValidatePath(iconPath); err != nil {
			d.Log.Debug().Err(err).Str("path", iconPath).Msg("invalid icon path")
			continue
		}
		cleanPath := filepath.Clean(iconPath)
		if !strings.HasPrefix(cleanPath, homeDir+string(filepath.Separator)) && cleanPath != homeDir {
			continue
		}
		if err := d.Fs.Remove(cleanPath); err != nil {
			d.Log.Warn().Err(err).Str("path", cleanPath).Msg("failed to remove icon")
			continue
		}
		removedAny = true
	}

	return removedAny
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

// isDebtapInitialized checks if debtap has been initialized
func isDebtapInitialized() bool {
	// Debtap stores its database in /var/cache/debtap/
	debtapCacheDir := "/var/cache/debtap"
	fs := afero.NewOsFs()

	// Check if cache directory exists
	if info, err := fs.Stat(debtapCacheDir); err != nil || !info.IsDir() {
		return false
	}

	// Check if essential database files exist (created during initialization)
	essentialFiles := []string{
		"debian-main-packages-files",
		"ubuntu-packages-files",
		"virtual-packages",
	}

	foundCount := 0
	for _, filename := range essentialFiles {
		filePath := filepath.Join(debtapCacheDir, filename)
		if _, err := fs.Stat(filePath); err == nil {
			foundCount++
		}
	}

	// Require at least 2 of the 3 essential files to be present
	return foundCount >= 2
}

// No local helper functions - using shared helpers from internal/helpers/common.go

// extractPackageInfoFromArchive reads .PKGINFO from an Arch package archive
// to discover the package name and version that pacman will register.
func extractPackageInfoFromArchive(pkgPath string) (*packageInfo, error) {
	cmd := exec.Command("bsdtar", "-xOf", pkgPath, ".PKGINFO")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to read .PKGINFO from archive: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	info := &packageInfo{}
	lines := strings.Split(stdout.String(), "\n")
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "pkgname = "):
			info.name = strings.TrimSpace(strings.TrimPrefix(line, "pkgname = "))
		case strings.HasPrefix(line, "pkgver = "):
			info.version = strings.TrimSpace(strings.TrimPrefix(line, "pkgver = "))
		}
	}

	if info.name == "" {
		return nil, fmt.Errorf("pkgname not found in .PKGINFO")
	}

	return info, nil
}

// fixMalformedDependencies corrects common dependency name issues from debtap conversion
// This addresses issues where epoch versions (like 2:1.4.99.1) cause name mangling
func fixMalformedDependencies(pkgPath string, logger *zerolog.Logger) error {
	// Extract the package to a temp directory
	fs := afero.NewOsFs()
	tmpDir, err := afero.TempDir(fs, "", "upkg-fix-deps-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if removeErr := fs.RemoveAll(tmpDir); removeErr != nil {
			logger.Debug().Err(removeErr).Str("tmp_dir", tmpDir).Msg("failed to remove temp dir")
		}
	}()

	// Extract package using bsdtar (Arch standard, auto-detects compression)
	extractCmd := exec.Command("bsdtar", "-xf", pkgPath, "-C", tmpDir) // #nosec G204 -- pkgPath is validated
	var extractStderr bytes.Buffer
	extractCmd.Stderr = &extractStderr
	if extractErr := extractCmd.Run(); extractErr != nil {
		return fmt.Errorf("failed to extract package: %w (stderr: %s)", extractErr, extractStderr.String())
	}

	// Read .PKGINFO
	pkgInfoPath := filepath.Join(tmpDir, ".PKGINFO")
	content, err := afero.ReadFile(fs, pkgInfoPath)
	if err != nil {
		return fmt.Errorf("failed to read .PKGINFO: %w", err)
	}

	// Fix malformed dependencies
	lines := strings.Split(string(content), "\n")
	var fixed []string
	hasChanges := false

	for _, line := range lines {
		if strings.HasPrefix(line, "depend = ") {
			fixedLine := fixDependencyLine(line, logger)
			if fixedLine == "" {
				// Dependency should be removed
				logger.Debug().
					Str("removed_dependency", strings.TrimPrefix(line, "depend = ")).
					Msg("removing invalid dependency")
				hasChanges = true
				continue
			}
			if fixedLine != line {
				logger.Debug().
					Str("original", strings.TrimPrefix(line, "depend = ")).
					Str("fixed", strings.TrimPrefix(fixedLine, "depend = ")).
					Msg("dependency mapping applied")
				hasChanges = true
			}
			fixed = append(fixed, fixedLine)
		} else {
			fixed = append(fixed, line)
		}
	}

	// Only repack if we made changes
	if !hasChanges {
		return nil
	}

	// Write fixed .PKGINFO
	if writeErr := afero.WriteFile(fs, pkgInfoPath, []byte(strings.Join(fixed, "\n")), 0644); writeErr != nil {
		return fmt.Errorf("failed to write fixed .PKGINFO: %w", writeErr)
	}

	// Repack using bsdtar with zstd compression (Arch standard)
	// List files explicitly to avoid ./ prefix that causes "missing metadata" error
	files, err := afero.ReadDir(fs, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to read tmpdir: %w", err)
	}

	// Build list of files without ./ prefix
	var fileList []string
	for _, file := range files {
		fileList = append(fileList, file.Name())
	}

	// Create command with explicit file list: bsdtar --zstd -cf package.tar.zst -C tmpDir file1 file2 ...
	args := []string{"--zstd", "-cf", pkgPath, "-C", tmpDir}
	args = append(args, fileList...)

	repackCmd := exec.Command("bsdtar", args...)
	var repackStderr bytes.Buffer
	repackCmd.Stderr = &repackStderr
	if err := repackCmd.Run(); err != nil {
		return fmt.Errorf("failed to repack package with bsdtar: %w (stderr: %s)", err, repackStderr.String())
	}

	return nil
}

// fixDependencyLine corrects a single dependency line with known malformations
// Returns empty string if dependency should be removed
//
//nolint:gocyclo // dependency normalization is a rule table by nature.
func fixDependencyLine(line string, _ *zerolog.Logger) string {
	// Extract the dependency part after "depend = "
	if !strings.HasPrefix(line, "depend = ") {
		return line
	}

	dep := strings.TrimPrefix(line, "depend = ")

	// Remove completely invalid dependencies (these are artifacts from debtap parsing)
	invalidDeps := []string{
		"anaconda",       // Artifact from libc6 epoch parsing
		"apparmor.d-git", // Artifact
		"cura-bin",       // Artifact from libc6>=2.17
	}

	// Extract just the package name (before any version operator)
	depName := dep
	versionConstraint := ""
	for _, op := range []string{">=", "<=", "=", ">", "<"} {
		if idx := strings.Index(dep, op); idx != -1 {
			depName = dep[:idx]
			versionConstraint = dep[idx:]
			break
		}
	}

	for _, invalid := range invalidDeps {
		if strings.HasPrefix(depName, invalid) {
			return "" // Empty string signals removal
		}
	}

	// Debian/Ubuntu → Arch package name mapping
	// Many Debian packages have different names in Arch repos
	debianToArchMap := map[string]string{
		"gtk":        "gtk3",          // Generic GTK → GTK3 (most compatible)
		"gtk2.0":     "gtk2",          // Debian GTK2 naming
		"gtk-3.0":    "gtk3",          // Debian GTK3 naming variant
		"python3":    "python",        // Arch uses "python" for Python 3
		"nodejs":     "nodejs",        // Same but good to document
		"libssl":     "openssl",       // SSL library naming (v3)
		"libssl1.1":  "openssl-1.1",   // Specific SSL 1.1 version (legacy package)
		"libssl3":    "openssl",       // OpenSSL 3.x
		"libjpeg":    "libjpeg-turbo", // JPEG library
		"libpng":     "libpng",        // Same but documented
		"libpng16":   "libpng",        // Specific version to generic
		"zlib1g":     "zlib",          // Debian zlib naming
		"libcurl":    "curl",          // Curl library
		"libcurl4":   "curl",          // Curl 4.x
		"libglib2.0": "glib2",         // GLib naming difference
		"libnotify4": "libnotify",     // Remove version suffix
	}

	// Apply Debian→Arch mapping if needed
	if archName, exists := debianToArchMap[depName]; exists {
		return "depend = " + archName + versionConstraint
	}

	// Pattern-based fixes
	// Check if dependency name matches regex rules
	replacements := []struct {
		prefix     string
		minLen     int
		versionIdx int // Index where version typically starts
		target     string
	}{
		{prefix: "libx11", minLen: 6, versionIdx: 6, target: "libx11"},
		{prefix: "libxcomposite", minLen: 13, versionIdx: 13, target: "libxcomposite"},
		{prefix: "libxdamage", minLen: 10, versionIdx: 10, target: "libxdamage"},
		{prefix: "libxkbfile", minLen: 10, versionIdx: 10, target: "libxkbfile"},
		{prefix: "nspr", minLen: 4, versionIdx: 4, target: "nspr"},
	}

	// Fix "c>=" → "glibc>=" (but avoid matching "cairo", "curl", etc.)
	if strings.HasPrefix(dep, "c>=") || strings.HasPrefix(dep, "c>") || strings.HasPrefix(dep, "c<") || strings.HasPrefix(dep, "c=") {
		if len(dep) > 1 && (dep[1] == '>' || dep[1] == '<' || dep[1] == '=') {
			return "depend = glibc" + dep[1:]
		}
	}

	// Apply pattern replacements for malformed versions (e.g. libx111.4.99 -> libx11>=1.4.99)
	for _, r := range replacements {
		if strings.HasPrefix(dep, r.prefix) && len(dep) > r.minLen {
			// Check if it's malformed (has digits immediately after prefix)
			if dep[r.versionIdx] >= '0' && dep[r.versionIdx] <= '9' {
				version := dep[r.versionIdx:]
				return "depend = " + r.target + ">=" + version
			}
		}
	}

	return line
}
