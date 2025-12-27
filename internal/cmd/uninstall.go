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
	"golang.org/x/term"
)

// uninstallOptions holds command flags
type uninstallOptions struct {
	yes        bool
	dryRun     bool
	all        bool
	timeoutSec int
}

// UninstallResult tracks the outcome of a single uninstall operation
type UninstallResult struct {
	Name    string
	Success bool
	Error   error
}

// NewUninstallCmd creates the uninstall command
func NewUninstallCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	opts := &uninstallOptions{}

	cmd := &cobra.Command{
		Use:   "uninstall [package-name...] [flags]",
		Short: "Uninstall one or more packages",
		Long: `Uninstall previously installed packages by name or install ID.

Examples:
  upkg uninstall firefox              # Uninstall single package
  upkg uninstall pkg1 pkg2 pkg3       # Uninstall multiple packages
  upkg uninstall pkg1 --yes           # Skip confirmation prompt
  upkg uninstall pkg1 --dry-run       # Preview without removing
  upkg uninstall --all --yes          # Uninstall all packages
  upkg uninstall                      # Interactive mode (select from list)`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			return runUninstallCmd(cfg, log, opts, args)
		},
	}

	cmd.Flags().BoolVarP(&opts.yes, "yes", "y", false, "skip confirmation prompts (required for non-interactive environments)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "preview what would be uninstalled without making changes")
	cmd.Flags().BoolVar(&opts.all, "all", false, "uninstall all tracked packages")
	cmd.Flags().IntVar(&opts.timeoutSec, "timeout", 600, "uninstallation timeout in seconds")

	return cmd
}

func runUninstallCmd(cfg *config.Config, log *zerolog.Logger, opts *uninstallOptions, args []string) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(opts.timeoutSec)*time.Second)
	defer cancel()

	// Initialize database
	database, err := db.New(ctx, cfg.Paths.DBFile)
	if err != nil {
		color.Red("Error: failed to open database: %v", err)
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = database.Close() }()

	registry := backends.NewRegistry(cfg, log)

	// Determine the mode of operation
	switch {
	case opts.all:
		return runUninstallAll(ctx, database, registry, log, opts)
	case len(args) == 0:
		return runInteractiveUninstall(ctx, database, registry, log, opts)
	case len(args) == 1:
		return runSingleUninstall(ctx, database, registry, log, opts, args[0])
	default:
		return runBulkUninstall(ctx, database, registry, log, opts, args)
	}
}

// isInteractive checks if stdin is a terminal
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// requireInteractiveOrYes ensures we're either in a TTY or have --yes flag
func requireInteractiveOrYes(opts *uninstallOptions) error {
	if !isInteractive() && !opts.yes {
		return fmt.Errorf("non-interactive mode requires --yes flag")
	}
	return nil
}

// runUninstallAll uninstalls all tracked packages
func runUninstallAll(ctx context.Context, database *db.DB, registry *backends.Registry, log *zerolog.Logger, opts *uninstallOptions) error {
	if err := requireInteractiveOrYes(opts); err != nil {
		color.Red("Error: %v", err)
		return err
	}

	installs, err := database.List(ctx)
	if err != nil {
		color.Red("Error: failed to query database: %v", err)
		return fmt.Errorf("failed to query database: %w", err)
	}

	if len(installs) == 0 {
		color.Yellow("No packages are currently tracked by upkg.")
		return nil
	}

	// Convert to records
	records := make([]*core.InstallRecord, 0, len(installs))
	for i := range installs {
		records = append(records, dbInstallToCore(&installs[i]))
	}

	color.Yellow("⚠️  WARNING: This will uninstall ALL %d packages!", len(records))

	return executeUninstall(ctx, registry, database, log, opts, records)
}

// runInteractiveUninstall handles the interactive multi-select mode
func runInteractiveUninstall(ctx context.Context, database *db.DB, registry *backends.Registry, log *zerolog.Logger, opts *uninstallOptions) error {
	if !isInteractive() {
		color.Red("Error: interactive mode requires a TTY")
		color.Yellow("  Use 'upkg uninstall <package>' or 'upkg uninstall --all --yes'")
		return fmt.Errorf("interactive mode requires a TTY")
	}

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
		dateStr := install.InstallDate.Format("2006-01-02")
		shortID := install.InstallID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}
		label := fmt.Sprintf("%s (%s) - %s [id:%s]",
			install.Name,
			install.PackageType,
			dateStr,
			shortID,
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

	// Convert selected labels to records
	records := make([]*core.InstallRecord, 0, len(selectedLabels))
	for _, label := range selectedLabels {
		install := optionMap[label]
		records = append(records, dbInstallToCore(&install))
	}

	return executeUninstall(ctx, registry, database, log, opts, records)
}

// runSingleUninstall handles uninstalling a single package by name or ID
func runSingleUninstall(ctx context.Context, database *db.DB, registry *backends.Registry, log *zerolog.Logger, opts *uninstallOptions, identifier string) error {
	if err := requireInteractiveOrYes(opts); err != nil && !opts.dryRun {
		// Allow dry-run without --yes in non-interactive mode
		if !opts.dryRun {
			color.Red("Error: %v", err)
			return err
		}
	}

	color.Cyan("→ Looking up package...")

	record, err := lookupPackage(ctx, database, log, identifier)
	if err != nil {
		return err
	}

	color.Green("✓ Found package: %s (%s)", record.Name, record.PackageType)

	log.Info().
		Str("identifier", identifier).
		Str("name", record.Name).
		Msg("starting uninstallation")

	return executeUninstall(ctx, registry, database, log, opts, []*core.InstallRecord{record})
}

// runBulkUninstall handles uninstalling multiple packages specified as arguments
func runBulkUninstall(ctx context.Context, database *db.DB, registry *backends.Registry, log *zerolog.Logger, opts *uninstallOptions, identifiers []string) error {
	if err := requireInteractiveOrYes(opts); err != nil && !opts.dryRun {
		color.Red("Error: %v", err)
		return err
	}

	color.Cyan("🔍 Looking up %d packages...", len(identifiers))

	records := make([]*core.InstallRecord, 0, len(identifiers))
	notFound := make([]string, 0)

	for _, identifier := range identifiers {
		record, err := lookupPackage(ctx, database, log, identifier)
		if err != nil {
			notFound = append(notFound, identifier)
			continue
		}
		records = append(records, record)
	}

	if len(notFound) > 0 {
		color.Yellow("⚠️  The following packages were not found:")
		for _, name := range notFound {
			fmt.Printf("   • %s\n", name)
		}
		fmt.Println()
	}

	if len(records) == 0 {
		color.Red("Error: no valid packages to uninstall")
		return fmt.Errorf("no valid packages found")
	}

	color.Green("✓ Found %d package(s)\n", len(records))

	return executeUninstall(ctx, registry, database, log, opts, records)
}

// lookupPackage finds a package by ID or name
func lookupPackage(ctx context.Context, database *db.DB, log *zerolog.Logger, identifier string) (*core.InstallRecord, error) {
	// Try by install ID first
	dbInstall, err := database.Get(ctx, identifier)
	if err == nil {
		return dbInstallToCore(dbInstall), nil
	}

	log.Debug().
		Str("identifier", identifier).
		Msg("not found by ID, trying by name")

	// Try by name
	allInstalls, err := database.List(ctx)
	if err != nil {
		color.Red("Error: failed to query database: %v", err)
		return nil, fmt.Errorf("failed to query database: %w", err)
	}

	lowerIdentifier := strings.ToLower(identifier)
	for _, install := range allInstalls {
		if strings.ToLower(install.Name) == lowerIdentifier {
			return dbInstallToCore(&install), nil
		}
	}

	color.Red("Error: package not found: %s", identifier)
	color.Yellow("  Use 'upkg list' to see installed packages")
	return nil, fmt.Errorf("package not found: %s", identifier)
}

// executeUninstall is the unified execution path for all uninstall modes
func executeUninstall(ctx context.Context, registry *backends.Registry, database *db.DB, log *zerolog.Logger, opts *uninstallOptions, records []*core.InstallRecord) error {
	// Calculate sizes and show summary
	fmt.Println()
	color.Cyan("📋 %d package(s) for uninstallation:", len(records))

	if !opts.dryRun {
		color.Cyan("📏 Calculating sizes...")
	}

	var totalSize int64
	sizes := make(map[string]int64, len(records))

	for _, record := range records {
		size := int64(0)
		if record.InstallPath != "" {
			size, _ = calculatePackageSize(record.InstallPath)
		}
		sizes[record.InstallID] = size
		totalSize += size
	}

	fmt.Println()
	for _, record := range records {
		size := sizes[record.InstallID]
		fmt.Printf("   • %s (%s) - %s\n", record.Name, record.PackageType, formatBytes(size))
	}
	fmt.Printf("\n💾 Total space to free: %s\n\n", formatBytes(totalSize))

	// Dry-run mode: show detailed breakdown and exit
	if opts.dryRun {
		return showDryRunDetails(records, sizes)
	}

	// Confirmation (skip if --yes)
	if !opts.yes {
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
	}

	// Execute uninstallation
	fmt.Println()
	color.Cyan("🚀 Starting uninstallation...\n")

	results := make([]UninstallResult, 0, len(records))

	for i, record := range records {
		fmt.Printf("[%d/%d] ", i+1, len(records))

		log.Info().
			Str("install_id", record.InstallID).
			Str("name", record.Name).
			Msg("starting uninstallation")

		err := performUninstall(ctx, registry, database, log, record)
		results = append(results, UninstallResult{
			Name:    record.Name,
			Success: err == nil,
			Error:   err,
		})
	}

	// Summary
	return printUninstallSummary(results)
}

// showDryRunDetails displays what would be removed without actually removing
func showDryRunDetails(records []*core.InstallRecord, sizes map[string]int64) error {
	color.Cyan("🔍 [DRY-RUN] The following would be removed:\n")

	for _, record := range records {
		size := sizes[record.InstallID]
		fmt.Printf("📦 %s (%s) - %s\n", record.Name, record.PackageType, formatBytes(size))

		if record.InstallPath != "" {
			fmt.Printf("   📁 Install path: %s\n", record.InstallPath)
		}
		if record.DesktopFile != "" {
			fmt.Printf("   🖥️  Desktop file: %s\n", record.DesktopFile)
		}
		if len(record.Metadata.IconFiles) > 0 {
			fmt.Printf("   🎨 Icon files: %d file(s)\n", len(record.Metadata.IconFiles))
			for _, icon := range record.Metadata.IconFiles {
				fmt.Printf("      • %s\n", icon)
			}
		}
		if record.Metadata.WrapperScript != "" {
			fmt.Printf("   📜 Wrapper script: %s\n", record.Metadata.WrapperScript)
		}
		if len(record.Metadata.DesktopFiles) > 0 {
			fmt.Printf("   🖥️  Additional desktop files: %d\n", len(record.Metadata.DesktopFiles))
		}
		fmt.Println()
	}

	color.Green("✓ [DRY-RUN] No changes were made.")
	return nil
}

// printUninstallSummary prints the final summary of the uninstall operation
func printUninstallSummary(results []UninstallResult) error {
	var successCount, failureCount int
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	fmt.Println()
	if failureCount > 0 {
		color.Yellow("⚠️  Uninstallation completed with errors:")
		color.Green("   ✓ Successful: %d", successCount)
		color.Red("   ✗ Failed: %d", failureCount)

		// Show failed packages
		fmt.Println()
		color.Red("Failed packages:")
		for _, r := range results {
			if !r.Success {
				fmt.Printf("   • %s: %v\n", r.Name, r.Error)
			}
		}
		return fmt.Errorf("%d package(s) failed to uninstall", failureCount)
	}

	color.Green("✓ Successfully uninstalled all %d package(s)!", successCount)
	return nil
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
//
//nolint:gocyclo // metadata decoding handles several legacy shapes.
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

		if installMethod, ok := dbRecord.Metadata["install_method"].(string); ok && installMethod != "" {
			record.Metadata.InstallMethod = installMethod
		}

		if desktopFiles, ok := dbRecord.Metadata["desktop_files"].([]string); ok {
			record.Metadata.DesktopFiles = desktopFiles
		} else if desktopFilesInterface, ok := dbRecord.Metadata["desktop_files"].([]interface{}); ok {
			for _, item := range desktopFilesInterface {
				if str, ok := item.(string); ok {
					record.Metadata.DesktopFiles = append(record.Metadata.DesktopFiles, str)
				}
			}
		}
	}

	if record.Metadata.InstallMethod == "" {
		record.Metadata.InstallMethod = core.InstallMethodLocal
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
	if walkErr := filepath.Walk(installPath, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			totalSize += info.Size()
			fileCount++
		}
		return nil
	}); walkErr != nil {
		// Best-effort size calculation; ignore walk errors.
		_ = walkErr
	}

	return totalSize, fileCount
}
