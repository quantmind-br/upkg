package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/quantmind-br/upkg/internal/backends"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/fatih/color"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// NewInstallCmd creates the install command
func NewInstallCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	var (
		force          bool
		skipDesktop    bool
		customName     string
		timeoutSecs    int
		skipWaylandEnv bool
	)

	cmd := &cobra.Command{
		Use:   "install [package]",
		Short: "Install a package",
		Long:  `Install a package from the specified file (AppImage, DEB, RPM, Tarball, or Binary).`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			packagePath := args[0]

			log.Info().
				Str("package", packagePath).
				Bool("force", force).
				Bool("skip_desktop", skipDesktop).
				Msg("starting installation")

			// Validate package exists
			if _, err := os.Stat(packagePath); err != nil {
				color.Red("Error: package file not found: %s", packagePath)
				return fmt.Errorf("package not found: %w", err)
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
			defer database.Close()

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

			// Install package
			color.Cyan("→ Installing package...")
			installOpts := core.InstallOptions{
				SkipDesktop:    skipDesktop,
				CustomName:     customName,
				SkipWaylandEnv: skipWaylandEnv,
			}

			record, err := backend.Install(ctx, packagePath, installOpts)
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
				},
			}

			// Save to database
			if err := database.Create(ctx, dbRecord); err != nil {
				color.Red("Error: failed to save installation record: %v", err)
				// Try to clean up
				if cleanupErr := backend.Uninstall(ctx, record); cleanupErr != nil {
					color.Red("Warning: cleanup failed: %v", cleanupErr)
					color.Red("Manual cleanup may be required for: %s", record.InstallPath)
					log.Warn().
						Err(cleanupErr).
						Str("install_path", record.InstallPath).
						Msg("failed to cleanup after database save failure")
				}
				return fmt.Errorf("failed to save installation record: %w", err)
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

	return cmd
}
