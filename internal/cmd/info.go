package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/quantmind-br/upkg/internal/config"
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
		RunE: func(cmd *cobra.Command, args []string) error {
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

			// Display package information
			printPackageInfo(dbRecord)

			log.Info().
				Str("install_id", dbRecord.InstallID).
				Str("name", dbRecord.Name).
				Msg("displayed package info")

			return nil
		},
	}

	return cmd
}

// printPackageInfo displays detailed package information
func printPackageInfo(install *db.Install) {
	ui.PrintHeader(fmt.Sprintf("Package Information: %s", install.Name))
	fmt.Println()

	// Basic information
	ui.PrintKeyValue("Name", install.Name)
	ui.PrintKeyValue("Type", ui.ColorizePackageType(install.PackageType))

	version := install.Version
	if version == "" {
		version = "(not specified)"
	}
	ui.PrintKeyValue("Version", version)

	ui.PrintKeyValue("Install ID", install.InstallID)
	ui.PrintKeyValue("Install Date", install.InstallDate.Format("2006-01-02 15:04:05"))

	fmt.Println()
	ui.PrintSubheader("Paths")

	ui.PrintKeyValue("Install Path", install.InstallPath)
	ui.PrintKeyValue("Original File", install.OriginalFile)

	if install.DesktopFile != "" {
		ui.PrintKeyValue("Desktop File", install.DesktopFile)
	} else {
		ui.PrintKeyValue("Desktop File", "(none)")
	}

	// Metadata section
	if len(install.Metadata) > 0 {
		fmt.Println()
		ui.PrintSubheader("Metadata")

		// Icon files
		if iconFiles, ok := install.Metadata["icon_files"].([]string); ok && len(iconFiles) > 0 {
			ui.PrintKeyValue("Icon Files", "")
			ui.PrintList(iconFiles)
		} else if iconFilesInterface, ok := install.Metadata["icon_files"].([]interface{}); ok && len(iconFilesInterface) > 0 {
			// Handle []interface{} case
			iconStrs := make([]string, 0)
			for _, item := range iconFilesInterface {
				if str, ok := item.(string); ok {
					iconStrs = append(iconStrs, str)
				}
			}
			if len(iconStrs) > 0 {
				ui.PrintKeyValue("Icon Files", "")
				ui.PrintList(iconStrs)
			}
		}

		// Wrapper script
		if wrapperScript, ok := install.Metadata["wrapper_script"].(string); ok && wrapperScript != "" {
			ui.PrintKeyValue("Wrapper Script", wrapperScript)
		}

		// Wayland support
		if waylandSupport, ok := install.Metadata["wayland_support"].(string); ok && waylandSupport != "" {
			ui.PrintKeyValue("Wayland Support", waylandSupport)
		}

		// Display any other metadata
		printOtherMetadata(install.Metadata)
	}

	fmt.Println()
}

// printOtherMetadata displays metadata that isn't handled by specific cases
func printOtherMetadata(metadata map[string]interface{}) {
	// Skip known keys
	knownKeys := map[string]bool{
		"icon_files":      true,
		"wrapper_script":  true,
		"wayland_support": true,
	}

	hasOther := false
	for key := range metadata {
		if !knownKeys[key] {
			hasOther = true
			break
		}
	}

	if hasOther {
		fmt.Println()
		ui.PrintKeyValue("Other Metadata", "")
		for key, value := range metadata {
			if !knownKeys[key] {
				ui.PrintKeyValue("  "+key, fmt.Sprintf("%v", value))
			}
		}
	}
}
