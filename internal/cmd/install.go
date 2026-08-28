package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/quantmind-br/upkg/internal/backends"
	"github.com/quantmind-br/upkg/internal/backends/flatpak"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/hyprland"
	"github.com/quantmind-br/upkg/internal/security"
	"github.com/quantmind-br/upkg/internal/source/github"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/quantmind-br/upkg/internal/ui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// Metadata keys persisted in the install record's JSON metadata column.
const (
	metaKeyIconFiles      = "icon_files"
	metaKeyWrapperScript  = "wrapper_script"
	metaKeyWaylandSupport = "wayland_support"
	metaKeyInstallMethod  = "install_method"
	metaKeyDesktopFiles   = "desktop_files"
	metaKeySource         = "source"
)

// NewInstallCmd creates the install command
//
//nolint:gocyclo // command wiring includes validation and multiple optional flows.
func NewInstallCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	var (
		force          bool
		skipDesktop    bool
		customName     string
		timeoutSecs    int
		skipWaylandEnv bool
		skipIconFix    bool
		overwrite      bool
		yes            bool
		ghTag          string
		ghPrerelease   bool
		ghFormat       string
		ghAsset        string
	)

	cmd := &cobra.Command{
		Use:   "install [package|url|owner/repo]",
		Short: "Install a package",
		Long: `Install a package from a local file (AppImage, DEB, RPM, Tarball, or Binary),
a Flatpak app ID, or a GitHub release URL / owner/repo reference.

Examples:
  upkg install ./app.AppImage
  upkg install https://github.com/block/buzz
  upkg install block/buzz --yes
  upkg install block/buzz --format appimage --tag desktop-v0.5.5`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// Validate the timeout flag first: it is independent of the package
			// path, and a non-positive value yields an already-expired context.
			if timeoutSecs <= 0 {
				color.Red("Error: timeout must be greater than zero seconds")
				return fmt.Errorf("timeout must be greater than zero seconds")
			}

			packagePath := args[0]
			var sourceRef *core.SourceRef
			var githubCleanup func()

			isFlatpakAppID := flatpak.IsFlatpakAppID(packagePath) || flatpak.IsFlatpakRemoteRef(packagePath)
			// A bare "owner/repo" is ambiguous with a relative path such as
			// "dist/app.AppImage"; an existing local file always wins.
			isGitHub := !isFlatpakAppID && github.IsGitHubURL(packagePath)

			// Create context with timeout early so GitHub resolution is bounded.
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
			defer cancel()

			if isGitHub {
				if overwrite {
					color.Red("Error: --overwrite is not allowed for remote (GitHub) installs")
					return fmt.Errorf("--overwrite forbidden for remote origin")
				}
				resolved, resErr := resolveGitHubPackage(ctx, packagePath, githubResolveOpts{
					Tag:        ghTag,
					Prerelease: ghPrerelease,
					Yes:        yes,
					Format:     ghFormat,
					AssetRegex: ghAsset,
					DataDir:    cfg.Paths.DataDir,
					AppName:    customName,
				}, log)
				if resErr != nil {
					color.Red("Error: %v", resErr)
					return resErr
				}
				packagePath = resolved.LocalPath
				sourceRef = resolved.Source
				githubCleanup = resolved.Cleanup
				defer func() {
					if githubCleanup != nil {
						githubCleanup()
					}
				}()
				if confErr := confirmRemoteSudo(resolved.Format, yes); confErr != nil {
					return confErr
				}
			} else if !isFlatpakAppID {
				absPath, err := filepath.Abs(packagePath)
				if err != nil {
					color.Red("Error: invalid package path: %v", err)
					return fmt.Errorf("invalid package path: %w", err)
				}
				packagePath = absPath
			}

			log.Info().
				Str("package", packagePath).
				Bool("force", force).
				Bool("skip_desktop", skipDesktop).
				Bool("github", isGitHub).
				Msg("starting installation")

			if !isFlatpakAppID {
				if validateErr := security.ValidatePath(packagePath); validateErr != nil {
					color.Red("Error: invalid package path: %v", validateErr)
					return fmt.Errorf("invalid package path: %w", validateErr)
				}
			}

			if customName != "" {
				customName = security.SanitizeString(customName)
				if validateErr := security.ValidatePackageName(customName); validateErr != nil {
					color.Red("Error: invalid custom name: %v", validateErr)
					return fmt.Errorf("invalid custom name: %w", validateErr)
				}
			}

			if !isFlatpakAppID {
				if _, statErr := os.Stat(packagePath); statErr != nil {
					color.Red("Error: package file not found: %s", packagePath)
					return fmt.Errorf("package not found: %w", statErr)
				}
			}

			// PATH shadowing check on the name that will actually be installed:
			// backends derive it from --name or from the asset filename, never
			// from the repository name.
			if isGitHub && sourceRef != nil {
				derived := customName
				if derived == "" {
					base := filepath.Base(packagePath)
					derived = helpers.NormalizeFilename(helpers.CleanAppName(strings.TrimSuffix(base, filepath.Ext(base))))
				}
				if shErr := confirmPathShadow(derived, yes); shErr != nil {
					color.Red("Error: %v", shErr)
					return shErr
				}
			}

			// Initialize database
			database, err := db.New(ctx, cfg.Paths.DBFile)
			if err != nil {
				color.Red("Error: failed to open database: %v", err)
				return fmt.Errorf("failed to open database: %w", err)
			}
			defer func() { _ = database.Close() }()

			// Create backend registry
			registry := backends.NewRegistry(cfg, log)

			// Detect backend
			color.Cyan("→ Detecting package type...")
			backend, err := registry.DetectBackend(ctx, packagePath)
			if err != nil {
				color.Red("Error: %v", err)
				return fmt.Errorf("failed to detect package type: %w", err)
			}

			color.Green("✓ Detected package type: %s", backend.Name())

			// Initialize transaction manager
			tx := transaction.NewManager(log)
			if githubCleanup != nil {
				// Ensure unverified/temp download is removed on rollback too.
				cleanup := githubCleanup
				tx.Add("remove_github_download", func() error {
					cleanup()
					return nil
				})
			}
			defer func() {
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					log.Warn().Err(rollbackErr).Msg("transaction rollback failed")
					color.Red("Error: rollback failed: %v", rollbackErr)
				}
			}()

			// Install package
			color.Cyan("→ Installing package...")
			installOpts := core.InstallOptions{
				Force:          force,
				SkipDesktop:    skipDesktop,
				CustomName:     customName,
				SkipWaylandEnv: skipWaylandEnv,
				// Remote origin never gets pacman --overwrite '*'.
				Overwrite: overwrite && sourceRef == nil,
			}

			record, err := backend.Install(ctx, packagePath, installOpts, tx)
			if err != nil {
				color.Red("Error: installation failed: %v", err)
				return fmt.Errorf("installation failed: %w", err)
			}

			if sourceRef != nil {
				record.Metadata.Source = sourceRef
				if record.Version == "" && sourceRef.Version != "" {
					record.Version = sourceRef.Version
				}
				// The downloaded artifact is deleted after commit; keep the
				// remote URL instead of a dangling cache path.
				if sourceRef.DownloadURL != "" {
					record.OriginalFile = sourceRef.DownloadURL
				}
			}

			// Convert to db.Install format
			dbRecord := &db.Install{
				InstallID:    record.InstallID,
				PackageType:  string(record.PackageType),
				Name:         record.Name,
				Version:      record.Version,
				InstallDate:  record.InstallDate,
				OriginalFile: record.OriginalFile,
				InstallPath:  record.InstallPath,
				DesktopFile:  record.DesktopFile,
				Metadata:     buildDBMetadata(record),
			}

			// Persist the record. A forced local reinstall replaces any prior row
			// for the same (package_type, install_path) atomically, preserving the
			// canonical ID/date; all other cases insert a fresh row.
			if force && isLocalPackageType(record.PackageType) {
				canonical, replaceErr := database.ReplaceByPath(ctx, dbRecord)
				if replaceErr != nil {
					color.Red("Error: failed to save installation record: %v", replaceErr)
					// Filesystem rollback (deferred) restores any backed-up install.
					return fmt.Errorf("failed to save installation record: %w", replaceErr)
				}
				dbRecord.InstallID = canonical.InstallID
				dbRecord.InstallDate = canonical.InstallDate
				record.InstallID = canonical.InstallID
				record.InstallDate = canonical.InstallDate
			} else {
				if createErr := database.Create(ctx, dbRecord); createErr != nil {
					color.Red("Error: failed to save installation record: %v", createErr)
					// Filesystem rollback (deferred) restores any backed-up install.
					return fmt.Errorf("failed to save installation record: %w", createErr)
				}
			}

			// Commit transaction. A finalizer failure (e.g. leftover backup) does
			// not undo the persisted install; report it as a warning.
			if commitErr := tx.Commit(); commitErr != nil {
				log.Warn().Err(commitErr).Msg("transaction commit cleanup failed")
				color.Yellow("Warning: post-install cleanup failed: %v", commitErr)
			}

			// Try to fix dock icon if we have a desktop file and Hyprland is running
			if record.DesktopFile != "" &&
				!skipIconFix &&
				hyprland.IsHyprlandRunning() &&
				record.Metadata.InstallMethod != core.InstallMethodPacman {
				if newDesktopPath, err := fixDockIcon(ctx, record, dbRecord, database, log); err != nil {
					log.Warn().Err(err).Msg("dock icon fix failed")
				} else if newDesktopPath != "" {
					record.DesktopFile = newDesktopPath
				}
			}

			// Success! Download cleanup is safe after commit (file already consumed).
			if githubCleanup != nil {
				githubCleanup()
				githubCleanup = nil
			}

			color.Green("✓ Package installed successfully")
			color.Green("  Name: %s", record.Name)
			color.Green("  Type: %s", record.PackageType)
			color.Green("  Install ID: %s", record.InstallID)
			if record.Version != "" {
				color.Cyan("  Version: %s", record.Version)
			}
			if record.InstallPath != "" {
				color.Cyan("  Path: %s", record.InstallPath)
			}
			if record.DesktopFile != "" {
				color.Cyan("  Desktop file: %s", record.DesktopFile)
			}
			if sourceRef != nil {
				color.Cyan("  Source: github.com/%s/%s@%s", sourceRef.Owner, sourceRef.Repo, sourceRef.Tag)
			}

			log.Info().
				Str("install_id", record.InstallID).
				Str("name", record.Name).
				Str("type", string(record.PackageType)).
				Msg("installation completed successfully")

			return nil
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "force installation even if already installed")
	cmd.Flags().BoolVar(&skipDesktop, "skip-desktop", false, "skip desktop integration")
	cmd.Flags().StringVarP(&customName, "name", "n", "", "custom application name")
	cmd.Flags().IntVar(&timeoutSecs, "timeout", 600, "installation timeout in seconds")
	cmd.Flags().BoolVar(&skipWaylandEnv, "skip-wayland-env", false, "skip Wayland environment variable injection (recommended for Tauri apps)")
	cmd.Flags().BoolVar(&skipIconFix, "skip-icon-fix", false, "skip dock icon fix (Hyprland initialClass detection)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite conflicting files from other packages (DEB/RPM only; forbidden for GitHub installs)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "non-interactive: pick best GitHub asset / confirm remote sudo")
	cmd.Flags().StringVar(&ghTag, "tag", "", "GitHub release tag (default: latest stable)")
	cmd.Flags().BoolVar(&ghPrerelease, "prerelease", false, "include GitHub prerelease when resolving latest")
	cmd.Flags().StringVar(&ghFormat, "format", "", "filter GitHub assets by format (appimage, deb, rpm, tarball, zip)")
	cmd.Flags().StringVar(&ghAsset, "asset", "", "regex matched against GitHub asset names")

	return cmd
}

// buildDBMetadata flattens core.Metadata into the map stored in SQLite.
func buildDBMetadata(record *core.InstallRecord) map[string]interface{} {
	m := map[string]interface{}{
		metaKeyIconFiles:      record.Metadata.IconFiles,
		metaKeyWrapperScript:  record.Metadata.WrapperScript,
		metaKeyWaylandSupport: record.Metadata.WaylandSupport,
		metaKeyInstallMethod:  record.Metadata.InstallMethod,
		metaKeyDesktopFiles:   record.Metadata.DesktopFiles,
	}
	if record.Metadata.Source != nil {
		m[metaKeySource] = record.Metadata.Source
	}
	return m
}

// isLocalPackageType reports whether a package type installs into the user's
// managed local directories and therefore supports atomic force replacement by
// install path.
func isLocalPackageType(pt core.PackageType) bool {
	switch pt {
	case core.PackageTypeAppImage, core.PackageTypeBinary, core.PackageTypeTarball, core.PackageTypeRpm:
		return true
	default:
		return false
	}
}

// fixDockIcon prompts user to open app, captures initialClass, and renames .desktop file for dock compatibility.
// Returns the new desktop file path if renamed, empty string if not renamed, or error if failed.
//
//nolint:gocyclo // interactive flow with Hyprland probing is naturally branching.
func fixDockIcon(ctx context.Context, record *core.InstallRecord, dbRecord *db.Install, database *db.DB, log *zerolog.Logger) (string, error) {
	// Ask user if they want to fix dock icon
	color.Cyan("\n→ Dock icon fix (Hyprland)")
	color.White("  To display the correct icon in nwg-dock-hyprland, the .desktop file")
	color.White("  must match the application's window class (initialClass).")
	color.White("  This requires briefly opening the application to detect its window class.")

	confirmed, err := ui.ConfirmWithDefault("Open application to detect window class?", true)
	if err != nil || !confirmed {
		color.Yellow("  Skipping dock icon fix")
		return "", nil
	}

	// Get executable from install path or desktop file
	execPath := record.Metadata.WrapperScript
	if execPath == "" {
		execPath = record.InstallPath
	}
	if execPath == "" {
		return "", fmt.Errorf("no executable path available")
	}
	info, statErr := os.Stat(execPath)
	if statErr != nil {
		return "", fmt.Errorf("stat executable path: %w", statErr)
	}
	if info.IsDir() {
		return "", fmt.Errorf("executable path is a directory: %s", execPath)
	}

	// Start the application
	color.Cyan("  Starting application...")
	cmd := exec.CommandContext(ctx, execPath) //nolint:gosec // G204: execPath is from user-installed package
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true, // Create new process group so we can kill it later
	}

	if startErr := cmd.Start(); startErr != nil {
		return "", fmt.Errorf("start application: %w", startErr)
	}

	pid := cmd.Process.Pid
	log.Debug().Int("pid", pid).Str("exec", execPath).Msg("started application for window class detection")

	// Wait for window to appear
	color.Cyan("  Waiting for window to appear (max 10s)...")
	client, err := hyprland.WaitForClient(ctx, pid, 10*time.Second, 500*time.Millisecond)

	// Kill the application regardless of whether we found the window
	defer func() {
		// Kill the entire process group
		if killErr := syscall.Kill(-pid, syscall.SIGTERM); killErr != nil {
			log.Debug().Err(killErr).Int("pid", pid).Msg("failed to kill process group, trying direct kill")
			if directKillErr := cmd.Process.Kill(); directKillErr != nil {
				log.Warn().Err(directKillErr).Int("pid", pid).Msg("failed to kill process")
			}
		}
		// Wait to avoid zombies
		if waitErr := cmd.Wait(); waitErr != nil {
			log.Debug().Err(waitErr).Int("pid", pid).Msg("application wait failed")
		}
	}()

	if err != nil {
		color.Yellow("  Could not detect window class (application may not have a GUI window)")
		return "", nil // Not an error, just skip
	}

	initialClass := client.InitialClass
	if initialClass == "" {
		color.Yellow("  Application has no initialClass set")
		return "", nil
	}

	color.Green("  Detected initialClass: %s", initialClass)

	// Check if rename is needed
	currentDesktopName := filepath.Base(record.DesktopFile)
	expectedDesktopName := initialClass + ".desktop"

	if currentDesktopName == expectedDesktopName {
		color.Green("  Desktop file already matches initialClass")
		return "", nil
	}

	// Rename the desktop file
	desktopDir := filepath.Dir(record.DesktopFile)
	newDesktopPath := filepath.Join(desktopDir, expectedDesktopName)

	color.Cyan("  Renaming: %s → %s", currentDesktopName, expectedDesktopName)

	if err := os.Rename(record.DesktopFile, newDesktopPath); err != nil {
		return "", fmt.Errorf("rename desktop file: %w", err)
	}

	// Update database record
	dbRecord.Metadata["original_desktop_file"] = record.DesktopFile
	dbRecord.DesktopFile = newDesktopPath

	if err := database.Update(ctx, dbRecord); err != nil {
		// Try to rollback the rename
		if rollbackErr := os.Rename(newDesktopPath, record.DesktopFile); rollbackErr != nil {
			log.Error().Err(rollbackErr).Msg("failed to rollback desktop file rename")
		}
		return "", fmt.Errorf("update database: %w", err)
	}

	color.Green("  ✓ Desktop file renamed for dock compatibility")
	return newDesktopPath, nil
}
