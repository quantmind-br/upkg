package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diogo/pkgctl/internal/backends"
	"github.com/diogo/pkgctl/internal/config"
	"github.com/diogo/pkgctl/internal/core"
	"github.com/diogo/pkgctl/internal/db"
	"github.com/diogo/pkgctl/internal/ui"
	"github.com/fatih/color"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// NewUninstallCmd creates the uninstall command
func NewUninstallCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	var timeoutSecs int

	cmd := &cobra.Command{
		Use:   "uninstall [package-name or install-id]",
		Short: "Uninstall a package",
		Long:  `Uninstall a previously installed package by name or install ID. Run without arguments for an interactive selector.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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

			registry := backends.NewRegistry(cfg, log)

			if len(args) == 0 {
				return runInteractiveUninstall(ctx, database, registry, log)
			}

			identifier := args[0]

			log.Info().
				Str("identifier", identifier).
				Msg("starting uninstallation")

			return runSingleUninstall(ctx, database, registry, log, identifier)
		},
	}

	cmd.Flags().IntVar(&timeoutSecs, "timeout", 600, "uninstallation timeout in seconds")

	return cmd
}

func runInteractiveUninstall(ctx context.Context, database *db.DB, registry *backends.Registry, log *zerolog.Logger) error {
	color.Cyan("→ Loading installed packages...")

	installs, err := database.List(ctx)
	if err != nil {
		color.Red("Error: failed to query database: %v", err)
		return fmt.Errorf("failed to query database: %w", err)
	}

	if len(installs) == 0 {
		color.Yellow("No packages are currently tracked by pkgctl.")
		return nil
	}

	options := make([]string, 0, len(installs))
	optionMap := make(map[string]db.Install, len(installs))

	for _, install := range installs {
		label := fmt.Sprintf("%s (%s) [%s]", install.Name, install.PackageType, install.InstallID)
		options = append(options, label)
		optionMap[label] = install
	}

	selectedLabels, err := ui.MultiSelectPrompt("Select packages to uninstall", options)
	if err != nil {
		color.Yellow("Selection cancelled. No packages were uninstalled.")
		return nil
	}

	if len(selectedLabels) == 0 {
		color.Yellow("No packages selected. Nothing to do.")
		return nil
	}

	color.Cyan("→ Preparing to uninstall %d package(s)...", len(selectedLabels))

	for _, label := range selectedLabels {
		install := optionMap[label]
		record := dbInstallToCore(&install)

		log.Info().
			Str("install_id", record.InstallID).
			Str("name", record.Name).
			Msg("starting uninstallation")

		if err := performUninstall(ctx, registry, database, log, record); err != nil {
			return err
		}
	}

	color.Green("✓ Uninstallation complete for %d package(s)", len(selectedLabels))
	return nil
}

func runSingleUninstall(ctx context.Context, database *db.DB, registry *backends.Registry, log *zerolog.Logger, identifier string) error {
	color.Cyan("→ Looking up package...")

	var dbRecord *db.Install

	dbInstall, err := database.Get(ctx, identifier)
	if err == nil {
		dbRecord = dbInstall
	} else {
		log.Debug().
			Str("identifier", identifier).
			Msg("not found by ID, trying by name")

		allInstalls, err := database.List(ctx)
		if err != nil {
			color.Red("Error: failed to query database: %v", err)
			return fmt.Errorf("failed to query database: %w", err)
		}

		lowerIdentifier := strings.ToLower(identifier)
		for _, install := range allInstalls {
			if strings.ToLower(install.Name) == lowerIdentifier {
				installCopy := install
				dbRecord = &installCopy
				break
			}
		}
	}

	if dbRecord == nil {
		color.Red("Error: package not found: %s", identifier)
		color.Yellow("  Use 'pkgctl list' to see installed packages")
		return fmt.Errorf("package not found")
	}

	color.Green("✓ Found package: %s (%s)", dbRecord.Name, dbRecord.PackageType)

	coreRecord := dbInstallToCore(dbRecord)
	return performUninstall(ctx, registry, database, log, coreRecord)
}

func performUninstall(ctx context.Context, registry *backends.Registry, database *db.DB, log *zerolog.Logger, record *core.InstallRecord) error {
	backend, err := registry.GetBackend(string(record.PackageType))
	if err != nil {
		color.Red("Error: backend not found for type %s", record.PackageType)
		return fmt.Errorf("backend not found: %w", err)
	}

	color.Cyan("→ Uninstalling %s (%s)...", record.Name, record.PackageType)

	if err := backend.Uninstall(ctx, record); err != nil {
		color.Red("Error: uninstallation failed for %s: %v", record.Name, err)
		return fmt.Errorf("uninstallation failed: %w", err)
	}

	if err := database.Delete(ctx, record.InstallID); err != nil {
		color.Yellow("Warning: failed to remove %s from database: %v", record.Name, err)
	} else {
		color.Green("✓ Package uninstalled: %s", record.Name)
	}

	log.Info().
		Str("install_id", record.InstallID).
		Str("name", record.Name).
		Msg("uninstallation completed successfully")

	return nil
}

// dbInstallToCore converts db.Install to core.InstallRecord
func dbInstallToCore(dbRecord *db.Install) *core.InstallRecord {
	record := &core.InstallRecord{
		InstallID:    dbRecord.InstallID,
		PackageType:  core.PackageType(dbRecord.PackageType),
		Name:         dbRecord.Name,
		Version:      dbRecord.Version,
		InstallDate:  dbRecord.InstallDate,
		OriginalFile: dbRecord.OriginalFile,
		InstallPath:  dbRecord.InstallPath,
		DesktopFile:  dbRecord.DesktopFile,
		Metadata:     core.Metadata{},
	}

	if dbRecord.Metadata != nil {
		if iconFiles, ok := dbRecord.Metadata["icon_files"].([]string); ok {
			record.Metadata.IconFiles = iconFiles
		} else if iconFilesInterface, ok := dbRecord.Metadata["icon_files"].([]interface{}); ok {
			for _, item := range iconFilesInterface {
				if str, ok := item.(string); ok {
					record.Metadata.IconFiles = append(record.Metadata.IconFiles, str)
				}
			}
		}

		if wrapperScript, ok := dbRecord.Metadata["wrapper_script"].(string); ok {
			record.Metadata.WrapperScript = wrapperScript
		}

		if waylandSupport, ok := dbRecord.Metadata["wayland_support"].(string); ok {
			record.Metadata.WaylandSupport = waylandSupport
		}
	}

	return record
}
