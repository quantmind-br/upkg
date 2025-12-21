package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/quantmind-br/upkg/internal/backends"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/quantmind-br/upkg/internal/hyprland"
	"github.com/quantmind-br/upkg/internal/security"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/quantmind-br/upkg/internal/ui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
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
	)

	cmd := &cobra.Command{
		Use:   "install [package]",
		Short: "Install a package",
		Long:  `Install a package from the specified file (AppImage, DEB, RPM, Tarball, or Binary).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			packagePath := args[0]

			absPath, err := filepath.Abs(packagePath)
			if err != nil {
				color.Red("Error: invalid package path: %v", err)
				return fmt.Errorf("invalid package path: %w", err)
			}
			packagePath = absPath

			log.Info().
				Str("package", packagePath).
				Bool("force", force).
				Bool("skip_desktop", skipDesktop).
				Msg("starting installation")

			if validateErr := security.ValidatePath(packagePath); validateErr != nil {
				color.Red("Error: invalid package path: %v", validateErr)
				return fmt.Errorf("invalid package path: %w", validateErr)
			}

			if customName != "" {
				customName = security.SanitizeString(customName)
				if validateErr := security.ValidatePackageName(customName); validateErr != nil {
					color.Red("Error: invalid custom name: %v", validateErr)
					return fmt.Errorf("invalid custom name: %w", validateErr)
				}
			}

			// Validate package exists
			if _, statErr := os.Stat(packagePath); statErr != nil {
				color.Red("Error: package file not found: %s", packagePath)
				return fmt.Errorf("package not found: %w", statErr)
			}

			// Create context with timeout
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSecs)*time.Second)
			defer cancel()

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
				Overwrite:      overwrite,
			}

			record, err := backend.Install(ctx, packagePath, installOpts, tx)
			if err != nil {
				color.Red("Error: installation failed: %v", err)
				return fmt.Errorf("installation failed: %w", err)
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
				Metadata: map[string]interface{}{
					"icon_files":      record.Metadata.IconFiles,
					"wrapper_script":  record.Metadata.WrapperScript,
					"wayland_support": record.Metadata.WaylandSupport,
					"install_method":  record.Metadata.InstallMethod,
					"desktop_files":   record.Metadata.DesktopFiles,
				},
			}

			// Save to database
			if err := database.Create(ctx, dbRecord); err != nil {
				color.Red("Error: failed to save installation record: %v", err)
				// Manual cleanup is handled by transaction rollback (deferred)
				// For legacy/unsupported cleanup, we might still want to try Uninstall
				// but ideally we trust the transaction.
				// Since we haven't fully migrated all cleanup to transaction yet,
				// keeping backend.Uninstall is safer for now as a fallback.
				if cleanupErr := backend.Uninstall(ctx, record); cleanupErr != nil {
					log.Warn().
						Err(cleanupErr).
						Str("install_path", record.InstallPath).
						Msg("failed to cleanup after database save failure")
				}
				return fmt.Errorf("failed to save installation record: %w", err)
			}

			// Commit transaction
			tx.Commit()

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

			// Success!
			color.Green("✓ Package installed successfully")
			color.Green("  Name: %s", record.Name)
			color.Green("  Type: %s", record.PackageType)
			color.Green("  Install ID: %s", record.InstallID)
			if record.InstallPath != "" {
				color.Cyan("  Path: %s", record.InstallPath)
			}
			if record.DesktopFile != "" {
				color.Cyan("  Desktop file: %s", record.DesktopFile)
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
	cmd.Flags().BoolVar(&overwrite, "overwrite", false, "overwrite conflicting files from other packages (DEB/RPM only)")

	return cmd
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
