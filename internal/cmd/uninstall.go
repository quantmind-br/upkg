package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/quantmind-br/upkg/internal/backends"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/quantmind-br/upkg/internal/ui"
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
	color.Cyan("🔍 Loading installed packages...")

	installs, err := database.List(ctx)
	if err != nil {
		color.Red("Error: failed to query database: %v", err)
		return fmt.Errorf("failed to query database: %w", err)
	}

	if len(installs) == 0 {
		color.Yellow("No packages are currently tracked by upkg.")
		return nil
	}

	color.Green("✓ Found %d installed packages\n", len(installs))
	color.Cyan("📦 Use fuzzy search to filter packages (type to search)")
	color.Cyan("   Press '/' to start searching, ↑↓ to navigate, Enter to select\n")

	options := make([]string, 0, len(installs))
	optionMap := make(map[string]db.Install, len(installs))

	for _, install := range installs {
		// Don't calculate size here to avoid UI delay
		// Size will be calculated only for selected packages
		dateStr := install.InstallDate.Format("2006-01-02")
		label := fmt.Sprintf("%s (%s) - %s",
			install.Name,
			install.PackageType,
			dateStr,
		)
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

	// Show summary and confirmation
	fmt.Println()
	color.Cyan("📋 Selected %d package(s) for uninstallation:", len(selectedLabels))
	color.Cyan("📏 Calculating sizes...")

	// Calculate sizes only for selected packages
	var totalSize int64
	sizeMap := make(map[string]int64, len(selectedLabels))
	for _, label := range selectedLabels {
		install := optionMap[label]
		size := int64(0)
		if install.InstallPath != "" {
			size, _ = calculatePackageSize(install.InstallPath)
		}
		sizeMap[label] = size
		totalSize += size
	}

	fmt.Println()
	for _, label := range selectedLabels {
		install := optionMap[label]
		size := sizeMap[label]
		fmt.Printf("   • %s (%s) - %s\n", install.Name, install.PackageType, formatBytes(size))
	}
	fmt.Printf("\n💾 Total space to free: %s\n\n", formatBytes(totalSize))

	// Confirmation
	color.Yellow("⚠️  This action cannot be undone!")
	confirmed, err := ui.ConfirmPrompt("Are you sure you want to uninstall these packages?")
	if err != nil {
		color.Yellow("Confirmation cancelled. No packages were uninstalled.")
		return nil
	}
	if !confirmed {
		color.Yellow("Uninstallation cancelled by user.")
		return nil
	}

	// Uninstall packages
	fmt.Println()
	color.Cyan("🚀 Starting uninstallation...\n")

	successCount := 0
	failureCount := 0

	for i, label := range selectedLabels {
		install := optionMap[label]
		record := dbInstallToCore(&install)

		fmt.Printf("[%d/%d] ", i+1, len(selectedLabels))

		log.Info().
			Str("install_id", record.InstallID).
			Str("name", record.Name).
			Msg("starting uninstallation")

		if err := performUninstall(ctx, registry, database, log, record); err != nil {
			failureCount++
			continue
		}
		successCount++
	}

	// Summary
	fmt.Println()
	if failureCount > 0 {
		color.Yellow("⚠️  Uninstallation completed with errors:")
		color.Green("   ✓ Successful: %d", successCount)
		color.Red("   ✗ Failed: %d", failureCount)
	} else {
		color.Green("✓ Successfully uninstalled all %d package(s)!", successCount)
	}

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
		color.Yellow("  Use 'upkg list' to see installed packages")
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

		if originalDesktopFile, ok := dbRecord.Metadata["original_desktop_file"].(string); ok {
			record.Metadata.OriginalDesktopFile = originalDesktopFile
		}
	}

	return record
}

// formatBytes formats a byte size in human readable format
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// calculatePackageSize calculates the total size and file count of a package
func calculatePackageSize(installPath string) (int64, int) {
	var totalSize int64
	var fileCount int

	// Check if path exists
	info, err := os.Stat(installPath)
	if err != nil {
		return 0, 0
	}

	// If it's a single file
	if !info.IsDir() {
		return info.Size(), 1
	}

	// If it's a directory, walk through it
	filepath.Walk(installPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			totalSize += info.Size()
			fileCount++
		}
		return nil
	})

	return totalSize, fileCount
}
