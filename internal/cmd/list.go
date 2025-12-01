package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/quantmind-br/upkg/internal/ui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// NewListCmd creates the list command
func NewListCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	var (
		jsonOutput  bool
		filterType  string
		filterName  string
		sortBy      string
		showDetails bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed packages",
		Long:  `List all installed packages with filtering and sorting options.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			// Open database
			database, err := db.New(ctx, cfg.Paths.DBFile)
			if err != nil {
				ui.PrintError("failed to open database: %v", err)
				return fmt.Errorf("open database: %w", err)
			}
			defer database.Close()

			// List installs
			installs, err := database.List(ctx)
			if err != nil {
				ui.PrintError("failed to list packages: %v", err)
				return fmt.Errorf("list installs: %w", err)
			}

			// Apply filters
			filtered := filterInstalls(installs, filterType, filterName)

			// Apply sorting
			sortInstalls(filtered, sortBy)

			// JSON output
			if jsonOutput {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(filtered)
			}

			// Check if empty
			if len(filtered) == 0 {
				if filterType != "" || filterName != "" {
					ui.PrintWarning("No packages found matching filters")
				} else {
					ui.PrintInfo("No packages installed")
				}
				return nil
			}

			// Print summary
			printSummary(installs, filtered, filterType, filterName)

			// Table output
			if showDetails {
				printDetailedTable(cmd, filtered)
			} else {
				printCompactTable(cmd, filtered)
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().StringVar(&filterType, "type", "", "filter by package type (appimage, binary, tarball, deb, rpm)")
	cmd.Flags().StringVar(&filterName, "name", "", "filter by package name (partial match)")
	cmd.Flags().StringVar(&sortBy, "sort", "name", "sort by: name, type, date, version")
	cmd.Flags().BoolVarP(&showDetails, "details", "d", false, "show detailed information")

	return cmd
}

// filterInstalls filters installs by type and name
func filterInstalls(installs []db.Install, filterType, filterName string) []db.Install {
	filtered := make([]db.Install, 0)

	for _, install := range installs {
		// Filter by type
		if filterType != "" && !strings.EqualFold(install.PackageType, filterType) {
			continue
		}

		// Filter by name (case-insensitive partial match)
		if filterName != "" && !strings.Contains(strings.ToLower(install.Name), strings.ToLower(filterName)) {
			continue
		}

		filtered = append(filtered, install)
	}

	return filtered
}

// sortInstalls sorts installs by the specified field
func sortInstalls(installs []db.Install, sortBy string) {
	switch strings.ToLower(sortBy) {
	case "name":
		sort.Slice(installs, func(i, j int) bool {
			return strings.ToLower(installs[i].Name) < strings.ToLower(installs[j].Name)
		})
	case "type":
		sort.Slice(installs, func(i, j int) bool {
			if installs[i].PackageType == installs[j].PackageType {
				return strings.ToLower(installs[i].Name) < strings.ToLower(installs[j].Name)
			}
			return installs[i].PackageType < installs[j].PackageType
		})
	case "date":
		sort.Slice(installs, func(i, j int) bool {
			return installs[i].InstallDate.After(installs[j].InstallDate)
		})
	case "version":
		sort.Slice(installs, func(i, j int) bool {
			if installs[i].Version == installs[j].Version {
				return strings.ToLower(installs[i].Name) < strings.ToLower(installs[j].Name)
			}
			return installs[i].Version < installs[j].Version
		})
	default:
		// Default to name
		sort.Slice(installs, func(i, j int) bool {
			return strings.ToLower(installs[i].Name) < strings.ToLower(installs[j].Name)
		})
	}
}

// printSummary prints a summary of installed packages
func printSummary(all, filtered []db.Install, filterType, filterName string) {
	// Count by type
	typeCounts := make(map[string]int)
	for _, install := range all {
		typeCounts[install.PackageType]++
	}

	// Print header
	ui.PrintHeader("Installed Packages")

	// Print total
	fmt.Printf("Total: %d packages", len(all))
	if len(filtered) != len(all) {
		fmt.Printf(" (showing %d filtered)", len(filtered))
	}
	fmt.Println()

	// Print type breakdown
	if len(typeCounts) > 0 && len(filtered) == len(all) {
		fmt.Print("  ")
		first := true
		for pkgType, count := range typeCounts {
			if !first {
				fmt.Print(" | ")
			}
			fmt.Printf("%s: %d", ui.ColorizePackageType(pkgType), count)
			first = false
		}
		fmt.Println()
	}

	// Print active filters
	if filterType != "" || filterName != "" {
		fmt.Println()
		ui.PrintInfo("Active filters:")
		if filterType != "" {
			fmt.Printf("  • Type: %s\n", ui.ColorizePackageType(filterType))
		}
		if filterName != "" {
			fmt.Printf("  • Name: %s\n", filterName)
		}
	}

	fmt.Println()
}

// printCompactTable prints a compact table view
func printCompactTable(cmd *cobra.Command, installs []db.Install) {
	table := tablewriter.NewTable(cmd.OutOrStdout(),
		tablewriter.WithHeader([]string{"Name", "Type", "Version", "Install Date"}),
		tablewriter.WithAlignment(tw.MakeAlign(4, tw.AlignLeft)),
		tablewriter.WithSymbols(tw.NewSymbols(tw.StyleNone)),
	)

	for _, install := range installs {
		version := install.Version
		if version == "" {
			version = "-"
		}

		table.Append(
			install.Name,
			ui.ColorizePackageType(install.PackageType),
			version,
			install.InstallDate.Format("2006-01-02 15:04"),
		)
	}

	table.Render()
}

// printDetailedTable prints a detailed table view
func printDetailedTable(cmd *cobra.Command, installs []db.Install) {
	table := tablewriter.NewTable(cmd.OutOrStdout(),
		tablewriter.WithHeader([]string{"Name", "Type", "Version", "Install Date", "Install ID", "Path"}),
		tablewriter.WithAlignment(tw.MakeAlign(6, tw.AlignLeft)),
		tablewriter.WithSymbols(tw.NewSymbols(tw.StyleLight)),
	)

	for _, install := range installs {
		version := install.Version
		if version == "" {
			version = "-"
		}

		// Truncate path if too long
		path := install.InstallPath
		if len(path) > 40 {
			path = "..." + path[len(path)-37:]
		}

		// Truncate install ID
		installID := install.InstallID
		if len(installID) > 20 {
			installID = installID[:20] + "..."
		}

		table.Append(
			install.Name,
			ui.ColorizePackageType(install.PackageType),
			version,
			install.InstallDate.Format("2006-01-02"),
			installID,
			path,
		)
	}

	table.Render()
}
