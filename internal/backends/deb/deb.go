package deb

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/diogo/upkg/internal/cache"
	"github.com/diogo/upkg/internal/config"
	"github.com/diogo/upkg/internal/core"
	"github.com/diogo/upkg/internal/desktop"
	"github.com/diogo/upkg/internal/helpers"
	"github.com/diogo/upkg/internal/ui"
	"github.com/rs/zerolog"
)

// DebBackend handles DEB package installations via debtap
type DebBackend struct {
	cfg    *config.Config
	logger *zerolog.Logger
}

// New creates a new DEB backend
func New(cfg *config.Config, log *zerolog.Logger) *DebBackend {
	return &DebBackend{
		cfg:    cfg,
		logger: log,
	}
}

// Name returns the backend name
func (d *DebBackend) Name() string {
	return "deb"
}

// Detect checks if this backend can handle the package
func (d *DebBackend) Detect(ctx context.Context, packagePath string) (bool, error) {
	// Check if file exists
	if _, err := os.Stat(packagePath); err != nil {
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
func (d *DebBackend) Install(ctx context.Context, packagePath string, opts core.InstallOptions) (*core.InstallRecord, error) {
	d.logger.Info().
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
	progressEnabled := d.logger.GetLevel() != zerolog.Disabled && d.logger.GetLevel() <= zerolog.InfoLevel
	progress := ui.NewProgressTracker(phases, "Installing DEB", progressEnabled)
	defer progress.Finish()

	// Phase 1: Validation
	progress.StartPhase(0)

	// Check if debtap is installed
	if err := helpers.RequireCommand("debtap"); err != nil {
		return nil, fmt.Errorf("debtap is required for DEB installation: %w\nInstall with: yay -S debtap", err)
	}

	// Check if pacman is available (we're on Arch)
	if err := helpers.RequireCommand("pacman"); err != nil {
		return nil, fmt.Errorf("pacman not found - DEB backend requires Arch Linux")
	}

	// Check if debtap is initialized
	if !isDebtapInitialized() {
		return nil, fmt.Errorf("debtap is not initialized\nRun the following command to initialize:\n  sudo debtap -u")
	}

	// Validate package exists
	if _, err := os.Stat(packagePath); err != nil {
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
			d.logger.Debug().
				Str("name", name).
				Msg("extracted package name from DEB metadata")
		} else {
			// Fallback: Extract base name from DEB filename
			pkgName = filepath.Base(packagePath)
			pkgName = strings.TrimSuffix(pkgName, filepath.Ext(pkgName))
			d.logger.Debug().
				Str("name", pkgName).
				Msg("extracted package name from filename (dpkg-deb unavailable)")
		}
	}

	normalizedName := helpers.NormalizeFilename(pkgName)
	pacmanPkgName := normalizedName
	var pkgMeta *packageInfo

	d.logger.Debug().
		Str("package_name", pkgName).
		Str("normalized_name", normalizedName).
		Msg("package name determined")

	// Create temp directory for conversion
	tmpDir, err := os.MkdirTemp("", "pkgctl-deb-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	progress.AdvancePhase()

	// Phase 3: Convert DEB to Arch package (indeterminate phase)
	progress.StartPhase(2)

	archPkgPath, err := d.convertWithDebtapProgress(ctx, packagePath, tmpDir, progress)
	if err != nil {
		return nil, fmt.Errorf("debtap conversion failed: %w", err)
	}

	d.logger.Debug().
		Str("arch_package", archPkgPath).
		Msg("DEB converted to Arch package")

	progress.AdvancePhase()

	// Phase 4: Fix dependencies
	progress.StartPhase(3)

	d.logger.Info().Msg("checking and fixing malformed dependencies...")
	if err := fixMalformedDependencies(archPkgPath, d.logger); err != nil {
		d.logger.Warn().Err(err).Msg("failed to fix malformed dependencies, proceeding anyway")
	}

	// Read package metadata to determine actual pacman package name
	pkgMeta, err = extractPackageInfoFromArchive(archPkgPath)
	if err != nil {
		d.logger.Warn().Err(err).Str("fallback_name", pacmanPkgName).Msg("failed to read package metadata from archive")
	} else if pkgMeta.name != "" {
		pacmanPkgName = pkgMeta.name
		d.logger.Debug().
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

	_, err = helpers.RunCommand(installCtx, "sudo", "pacman", "-U", "--noconfirm", archPkgPath)
	if err != nil {
		return nil, fmt.Errorf("pacman installation failed: %w", err)
	}

	d.logger.Info().Msg("package installed successfully via pacman")

	progress.AdvancePhase()

	// Phase 6: Desktop integration
	progress.StartPhase(5)

	// Get package info from pacman
	pkgInfo, err := d.getPackageInfo(ctx, pacmanPkgName)
	if err != nil {
		d.logger.Warn().Err(err).
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
		d.logger.Warn().Err(err).Msg("failed to list installed files")
	}

	// Find desktop files
	desktopFiles := d.findDesktopFiles(installedFiles)

	// Update desktop files with Wayland env vars if needed
	var primaryDesktopFile string
	if len(desktopFiles) > 0 {
		primaryDesktopFile = desktopFiles[0]

		if d.cfg.Desktop.WaylandEnvVars {
			for _, desktopFile := range desktopFiles {
				if err := d.updateDesktopFileWayland(desktopFile); err != nil {
					d.logger.Warn().
						Err(err).
						Str("desktop_file", desktopFile).
						Msg("failed to update desktop file with Wayland vars")
				}
			}
		}
	}

	// Find icon files
	iconFiles := d.findIconFiles(installedFiles)

	// Update caches
	if len(desktopFiles) > 0 {
		appsDir := filepath.Dir(desktopFiles[0])
		cache.UpdateDesktopDatabase(appsDir, d.logger)
	}

	if len(iconFiles) > 0 {
		// Find hicolor icon directory
		for _, iconFile := range iconFiles {
			if strings.Contains(iconFile, "hicolor") {
				hicolorDir := filepath.Dir(filepath.Dir(filepath.Dir(iconFile)))
				cache.UpdateIconCache(hicolorDir, d.logger)
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
		InstallPath:  fmt.Sprintf("/usr (managed by pacman: %s)", pacmanPkgName),
		DesktopFile:  primaryDesktopFile,
		Metadata: core.Metadata{
			IconFiles:      iconFiles,
			WaylandSupport: string(core.WaylandUnknown),
			ExtractedMeta: core.ExtractedMetadata{
				Comment: fmt.Sprintf("Installed via debtap/pacman"),
			},
		},
	}

	d.logger.Info().
		Str("install_id", installID).
		Str("name", pkgInfo.name).
		Str("version", pkgInfo.version).
		Msg("DEB package installed successfully")

	return record, nil
}

// Uninstall removes the installed DEB package via pacman
func (d *DebBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	d.logger.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstalling DEB package")

	// Extract package name from InstallPath metadata
	pkgName := record.Name
	normalizedName := helpers.NormalizeFilename(pkgName)

	// Check if package is still installed
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := helpers.RunCommand(checkCtx, "pacman", "-Qi", normalizedName)
	if err != nil {
		d.logger.Warn().
			Str("package", normalizedName).
			Msg("package not found in pacman database")
		return nil // Already uninstalled
	}

	// Uninstall with pacman
	d.logger.Info().Msg("removing package with pacman...")

	uninstallCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_, err = helpers.RunCommand(uninstallCtx, "sudo", "pacman", "-R", "--noconfirm", normalizedName)
	if err != nil {
		return fmt.Errorf("pacman removal failed: %w", err)
	}

	// Update caches
	cache.UpdateDesktopDatabase("/usr/share/applications", d.logger)
	cache.UpdateIconCache("/usr/share/icons/hicolor", d.logger)

	d.logger.Info().
		Str("install_id", record.InstallID).
		Msg("DEB package uninstalled successfully")

	return nil
}

// convertWithDebtapProgress converts a DEB package to Arch package with progress tracking
func (d *DebBackend) convertWithDebtapProgress(ctx context.Context, debPath, outputDir string, progress *ui.ProgressTracker) (string, error) {
	// Run debtap with quiet mode (-q) and skip interactive prompts (-Q)
	convertCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Convert to absolute path since we're changing working directory
	absDebPath, err := filepath.Abs(debPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	d.logger.Debug().
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

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start debtap: %w", err)
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		reader := io.TeeReader(stdoutPipe, &stdoutBuf)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			d.logger.Debug().
				Str("line", scanner.Text()).
				Msg("debtap stdout")
		}
		if err := scanner.Err(); err != nil {
			d.logger.Warn().Err(err).Msg("failed to read debtap stdout")
		}
	}()

	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		reader := io.TeeReader(stderrPipe, &stderrBuf)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			d.logger.Debug().
				Str("line", scanner.Text()).
				Msg("debtap stderr")
		}
		if err := scanner.Err(); err != nil {
			d.logger.Warn().Err(err).Msg("failed to read debtap stderr")
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
		d.logger.Error().
			Err(err).
			Str("stdout", stdoutBuf.String()).
			Str("stderr", stderrBuf.String()).
			Msg("debtap command failed")
		return "", fmt.Errorf("debtap conversion failed: %w\nStderr: %s", err, stderrBuf.String())
	}

	d.logger.Debug().
		Str("stdout", stdoutBuf.String()).
		Str("stderr", stderrBuf.String()).
		Msg("debtap conversion completed")

	// Find generated .pkg.tar.* file
	// Debtap creates package in current working directory, not in temp dir!

	// First try temp dir (in case debtap behavior changes)
	tempPattern := filepath.Join(outputDir, "*.pkg.tar.*")
	files, err := filepath.Glob(tempPattern)

	if err != nil {
		d.logger.Error().
			Err(err).
			Str("pattern", tempPattern).
			Msg("temp dir glob search failed")
		return "", fmt.Errorf("failed to search for generated package: %w", err)
	}

	if len(files) == 0 {
		// Search in current working directory (debtap default behavior)
		wd, _ := os.Getwd()
		wdPattern := filepath.Join(wd, "*.pkg.tar.*")
		d.logger.Debug().
			Str("working_dir", wd).
			Str("pattern", wdPattern).
			Msg("searching in working directory for debtap package")

		wdFiles, _ := filepath.Glob(wdPattern)

		if len(wdFiles) > 0 {
			// Filter for files matching our package name
			for _, file := range wdFiles {
				if strings.Contains(filepath.Base(file), "goose") ||
					strings.Contains(filepath.Base(file), "cursor") {
					files = append(files, file)
				}
			}
		}
	}

	if len(files) == 0 {
		// Try searching in the original package directory
		pkgDir := filepath.Dir(debPath)
		pkgPattern := filepath.Join(pkgDir, "*.pkg.tar.*")
		d.logger.Debug().
			Str("pkg_dir", pkgDir).
			Str("pattern", pkgPattern).
			Msg("searching in package directory")

		pkgFiles, _ := filepath.Glob(pkgPattern)
		files = append(files, pkgFiles...)
	}

	if len(files) == 0 {
		// List all files in output directory for debugging
		allFiles, _ := os.ReadDir(outputDir)
		var fileList []string
		for _, f := range allFiles {
			fileList = append(fileList, f.Name())
		}
		d.logger.Error().
			Strs("files_in_dir", fileList).
			Str("output_dir", outputDir).
			Msg("no arch package generated by debtap in any location")
		return "", fmt.Errorf("no arch package generated by debtap (searched temp, working, and pkg dirs)")
	}

	d.logger.Debug().
		Strs("found_files", files).
		Msg("found generated packages")

	return files[0], nil
}

// getPackageInfo gets package info from pacman
func (d *DebBackend) getPackageInfo(ctx context.Context, pkgName string) (*packageInfo, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	output, err := helpers.RunCommand(queryCtx, "pacman", "-Qi", pkgName)
	if err != nil {
		return nil, err
	}

	info := &packageInfo{name: pkgName}

	// Parse output
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Version") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.version = strings.TrimSpace(parts[1])
			}
		}
	}

	return info, nil
}

// findInstalledFiles lists all files installed by the package
func (d *DebBackend) findInstalledFiles(ctx context.Context, pkgName string) ([]string, error) {
	queryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	output, err := helpers.RunCommand(queryCtx, "pacman", "-Ql", pkgName)
	if err != nil {
		return nil, err
	}

	var files []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Format: "pkgname /path/to/file"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			files = append(files, parts[1])
		}
	}

	return files, nil
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
	file, err := os.Open(desktopPath)
	if err != nil {
		return err
	}
	defer file.Close()

	entry, err := desktop.Parse(file)
	if err != nil {
		return err
	}

	// Inject Wayland vars
	if err := desktop.InjectWaylandEnvVars(entry, d.cfg.Desktop.CustomEnvVars); err != nil {
		return err
	}

	// Write back (need sudo for system files)
	tmpFile, err := os.CreateTemp("", "pkgctl-desktop-*.desktop")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	if err := desktop.WriteDesktopFile(tmpPath, entry); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Move with sudo
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = helpers.RunCommand(ctx, "sudo", "mv", tmpPath, desktopPath)
	if err != nil {
		os.Remove(tmpPath)
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
	if !helpers.CommandExists("dpkg-deb") {
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

	output, err := helpers.RunCommand(queryCtx, "dpkg-deb", "--field", absPath, "Package")
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

	// Check if cache directory exists
	if info, err := os.Stat(debtapCacheDir); err != nil || !info.IsDir() {
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
		if _, err := os.Stat(filePath); err == nil {
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
	tmpDir, err := os.MkdirTemp("", "pkgctl-fix-deps-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Extract package using bsdtar (Arch standard, auto-detects compression)
	extractCmd := exec.Command("bsdtar", "-xf", pkgPath, "-C", tmpDir)
	var extractStderr bytes.Buffer
	extractCmd.Stderr = &extractStderr
	if err := extractCmd.Run(); err != nil {
		return fmt.Errorf("failed to extract package: %w (stderr: %s)", err, extractStderr.String())
	}

	// Read .PKGINFO
	pkgInfoPath := filepath.Join(tmpDir, ".PKGINFO")
	content, err := os.ReadFile(pkgInfoPath)
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
	if err := os.WriteFile(pkgInfoPath, []byte(strings.Join(fixed, "\n")), 0644); err != nil {
		return fmt.Errorf("failed to write fixed .PKGINFO: %w", err)
	}

	// Repack using bsdtar with zstd compression (Arch standard)
	// List files explicitly to avoid ./ prefix that causes "missing metadata" error
	files, err := os.ReadDir(tmpDir)
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
func fixDependencyLine(line string, logger *zerolog.Logger) string {
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

	// Check if this dependency should be removed entirely
	depName := dep
	versionConstraint := ""
	// Extract just the package name (before any version operator)
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
		"libssl":     "openssl",       // SSL library naming
		"libssl1.1":  "openssl",       // Specific SSL version
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
	// Fix "c>=" → "glibc>=" (but avoid matching "cairo", "curl", etc.)
	if strings.HasPrefix(dep, "c>=") || strings.HasPrefix(dep, "c>") || strings.HasPrefix(dep, "c<") || strings.HasPrefix(dep, "c=") {
		if len(dep) > 1 && (dep[1] == '>' || dep[1] == '<' || dep[1] == '=') {
			return "depend = glibc" + dep[1:]
		}
	}

	// Fix malformed libx11 variants (libx111.4.99.1 → libx11>=1.4.99.1)
	if strings.HasPrefix(dep, "libx11") && len(dep) > 6 {
		// Check if it's malformed (has digits immediately after libx11)
		if len(dep) > 6 && dep[6] >= '0' && dep[6] <= '9' {
			// Extract version part
			version := dep[6:]
			return "depend = libx11>=" + version
		}
	}

	// Fix malformed libxcomposite variants (libxcomposite0.4.4-1 → libxcomposite>=0.4.4-1)
	if strings.HasPrefix(dep, "libxcomposite") && len(dep) > 13 {
		if len(dep) > 13 && dep[13] >= '0' && dep[13] <= '9' {
			version := dep[13:]
			return "depend = libxcomposite>=" + version
		}
	}

	// Fix malformed libxdamage variants
	if strings.HasPrefix(dep, "libxdamage") && len(dep) > 10 {
		if len(dep) > 10 && dep[10] >= '0' && dep[10] <= '9' {
			version := dep[10:]
			return "depend = libxdamage>=" + version
		}
	}

	// Fix malformed libxkbfile variants
	if strings.HasPrefix(dep, "libxkbfile") && len(dep) > 10 {
		if len(dep) > 10 && dep[10] >= '0' && dep[10] <= '9' {
			version := dep[10:]
			return "depend = libxkbfile>=" + version
		}
	}

	// Fix malformed nspr variants (nspr4.9-2~ → nspr>=4.9-2~)
	if strings.HasPrefix(dep, "nspr") && len(dep) > 4 {
		if len(dep) > 4 && dep[4] >= '0' && dep[4] <= '9' {
			version := dep[4:]
			return "depend = nspr>=" + version
		}
	}

	return line
}
