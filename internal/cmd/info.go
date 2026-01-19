package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/quantmind-br/upkg/internal/ui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// NewInfoCmd creates the info command
func NewInfoCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info [package-name or install-id]",
		Short: "Show package information",
		Long:  `Show detailed information about an installed package.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			identifier := args[0]
			ctx := context.Background()

			// Open database
			database, err := db.New(ctx, cfg.Paths.DBFile)
			if err != nil {
				ui.PrintError("failed to open database: %v", err)
				return fmt.Errorf("open database: %w", err)
			}
			defer func() { _ = database.Close() }()

			// Try to find install record (by ID or name)
			var dbRecord *db.Install

			// Try as install ID first
			dbRecord, err = database.Get(ctx, identifier)
			if err != nil {
				// Try finding by name
				log.Debug().
					Str("identifier", identifier).
					Msg("not found by ID, trying by name")

				// List all and find by name
				allInstalls, err := database.List(ctx)
				if err != nil {
					ui.PrintError("failed to query database: %v", err)
					return fmt.Errorf("failed to query database: %w", err)
				}

				// Find by name (case-insensitive)
				lowerIdentifier := strings.ToLower(identifier)
				for _, install := range allInstalls {
					if strings.ToLower(install.Name) == lowerIdentifier {
						installCopy := install
						dbRecord = &installCopy
						break
					}
				}

				if dbRecord == nil {
					ui.PrintError("package not found: %s", identifier)
					ui.PrintInfo("Use 'upkg list' to see installed packages")
					return fmt.Errorf("package not found")
				}
			}

			// Convert to core.InstallRecord
			record := db.ToInstallRecord(dbRecord)

			// Display package information
			printPackageInfo(record)

			log.Info().
				Str("install_id", record.InstallID).
				Str("name", record.Name).
				Msg("displayed package info")

			return nil
		},
	}

	return cmd
}

// printPackageInfo displays detailed package information
func printPackageInfo(record *core.InstallRecord) {
	ui.PrintHeader(fmt.Sprintf("Package Information: %s", record.Name))
	fmt.Println()

	// Basic information
	ui.PrintKeyValue("Name", record.Name)
	ui.PrintKeyValue("Type", ui.ColorizePackageType(string(record.PackageType)))

	version := record.Version
	if version == "" {
		version = "(not specified)"
	}
	ui.PrintKeyValue("Version", version)

	ui.PrintKeyValue("Install ID", record.InstallID)
	ui.PrintKeyValue("Install Date", record.InstallDate.Format("2006-01-02 15:04:05"))

	fmt.Println()
	ui.PrintSubheader("Paths")

	ui.PrintKeyValue("Install Path", record.InstallPath)
	ui.PrintKeyValue("Original File", record.OriginalFile)

	if record.DesktopFile != "" {
		ui.PrintKeyValue("Desktop File", record.DesktopFile)
	} else {
		ui.PrintKeyValue("Desktop File", "(none)")
	}

	// Metadata section
	fmt.Println()
	ui.PrintSubheader("Metadata")

	// Icon files
	if len(record.Metadata.IconFiles) > 0 {
		ui.PrintKeyValue("Icon Files", "")
		ui.PrintList(record.Metadata.IconFiles)
	}

	// Wrapper script
	if record.Metadata.WrapperScript != "" {
		ui.PrintKeyValue("Wrapper Script", record.Metadata.WrapperScript)
	}

	// Wayland support
	if record.Metadata.WaylandSupport != "" {
		ui.PrintKeyValue("Wayland Support", record.Metadata.WaylandSupport)
	}

	// Desktop files
	if len(record.Metadata.DesktopFiles) > 0 {
		ui.PrintKeyValue("Desktop Files", "")
		ui.PrintList(record.Metadata.DesktopFiles)
	}

	// Original desktop file
	if record.Metadata.OriginalDesktopFile != "" {
		ui.PrintKeyValue("Original Desktop File", record.Metadata.OriginalDesktopFile)
	}

	// Install method
	if record.Metadata.InstallMethod != "" {
		ui.PrintKeyValue("Install Method", record.Metadata.InstallMethod)
	}

	fmt.Println()
}
