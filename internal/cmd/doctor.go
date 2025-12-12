package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/quantmind-br/upkg/internal/ui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

// NewDoctorCmd creates the doctor command
//
//nolint:gocyclo // diagnostics command performs many sequential checks.
func NewDoctorCmd(cfg *config.Config, _ *zerolog.Logger) *cobra.Command {
	var verbose bool
	var fix bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check system dependencies and integrity",
		Long:  `Check system dependencies, configuration, database integrity, and installed packages.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			ui.PrintHeader("System Diagnostics")
			fmt.Println()

			var issues []string
			var warnings []string

			// 1. Check required dependencies
			ui.PrintSubheader("Required Dependencies")
			requiredDeps := []struct {
				name    string
				command string
				purpose string
			}{
				{"tar", "tar", "Extract tarball packages"},
				{"unsquashfs", "unsquashfs", "Extract AppImage packages"},
			}

			for _, dep := range requiredDeps {
				if checkDependency(dep.command, dep.name, dep.purpose, true) {
					ui.PrintSuccess("%s: found", dep.name)
				} else {
					ui.PrintError("%s: NOT FOUND", dep.name)
					issues = append(issues, fmt.Sprintf("Missing required dependency: %s (%s)", dep.name, dep.purpose))
				}
			}

			fmt.Println()

			// 2. Check optional dependencies
			ui.PrintSubheader("Optional Dependencies")
			optionalDeps := []struct {
				name    string
				command string
				purpose string
			}{
				{"debtap", "debtap", "Install DEB packages"},
				{"rpmextract.sh", "rpmextract.sh", "Install RPM packages"},
				{"gtk4-update-icon-cache", "gtk4-update-icon-cache", "Update icon cache"},
				{"update-desktop-database", "update-desktop-database", "Update desktop database"},
				{"desktop-file-validate", "desktop-file-validate", "Validate desktop files"},
			}

			for _, dep := range optionalDeps {
				if checkDependency(dep.command, dep.name, dep.purpose, false) {
					ui.PrintSuccess("%s: found", dep.name)
				} else {
					ui.PrintWarning("%s: not found (optional - %s)", dep.name, dep.purpose)
					warnings = append(warnings, fmt.Sprintf("Optional dependency missing: %s", dep.name))
				}
			}

			fmt.Println()

			// 3. Check directory structure
			ui.PrintSubheader("Directory Structure")
			dirs := []struct {
				path string
				name string
			}{
				{cfg.Paths.DataDir, "Data directory"},
				{filepath.Dir(cfg.Paths.DBFile), "Database directory"},
				{filepath.Dir(cfg.Paths.LogFile), "Log directory"},
			}

			for _, dir := range dirs {
				if checkDirectory(dir.path, dir.name, fix) {
					ui.PrintSuccess("%s: %s", dir.name, dir.path)
				} else {
					ui.PrintError("%s: NOT ACCESSIBLE (%s)", dir.name, dir.path)
					issues = append(issues, fmt.Sprintf("Directory not accessible: %s", dir.path))
				}
			}

			fmt.Println()

			// 4. Check database
			ui.PrintSubheader("Database")
			ctx := context.Background()
			database, err := db.New(ctx, cfg.Paths.DBFile)
			if err != nil {
				ui.PrintError("Database: NOT ACCESSIBLE")
				issues = append(issues, fmt.Sprintf("Cannot open database: %v", err))
			} else {
				ui.PrintSuccess("Database: accessible (%s)", cfg.Paths.DBFile)
				defer func() { _ = database.Close() }()

				// Check installed packages
				installs, err := database.List(ctx)
				if err != nil {
					ui.PrintWarning("Cannot list installed packages: %v", err)
					warnings = append(warnings, "Cannot list installed packages")
				} else {
					ui.PrintInfo("Installed packages: %d", len(installs))

					if verbose {
						// Check integrity of installed packages
						brokenInstalls := checkPackageIntegrity(installs)
						if len(brokenInstalls) > 0 {
							ui.PrintWarning("Found %d packages with missing files:", len(brokenInstalls))
							for _, broken := range brokenInstalls {
								fmt.Printf("  • %s (%s)\n", broken.install.Name, broken.install.InstallID)
								for _, missing := range broken.missing {
									fmt.Printf("      - %s\n", missing)
								}
							}
							warnings = append(warnings, fmt.Sprintf("%d packages have missing files", len(brokenInstalls)))
						} else {
							ui.PrintSuccess("All installed packages have intact files")
						}
					}
				}
			}

			fmt.Println()

			// 5. Check environment
			ui.PrintSubheader("Environment")
			checkEnvironment()

			fmt.Println()

			// Summary
			ui.PrintHeader("Summary")
			fmt.Println()

			if len(issues) == 0 {
				ui.PrintSuccess("All critical checks passed!")
			} else {
				ui.PrintError("Found %d issue(s):", len(issues))
				ui.PrintList(issues)
				fmt.Println()
			}

			if len(warnings) > 0 {
				ui.PrintWarning("Found %d warning(s):", len(warnings))
				ui.PrintList(warnings)
			}

			fmt.Println()

			if len(issues) > 0 {
				return fmt.Errorf("system check failed with %d issue(s)", len(issues))
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "verbose output with integrity checks")
	cmd.Flags().BoolVar(&fix, "fix", false, "create missing directories and try to fix permissions")

	return cmd
}

// checkDependency checks if a dependency is available
func checkDependency(command, _, _ string, _ bool) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// checkDirectory checks if a directory exists and is writable
func checkDirectory(path, _ string, fix bool) bool {
	info, err := os.Stat(path)
	if err != nil {
		// Try to create if it doesn't exist
		if os.IsNotExist(err) {
			if !fix {
				return false
			}
			if err := os.MkdirAll(path, 0755); err != nil {
				return false
			}
			return true
		}
		return false
	}

	if !info.IsDir() {
		return false
	}

	// Check if writable
	if fix {
		testFile := filepath.Join(path, ".upkg-test")
		if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
			return false
		}
		if removeErr := os.Remove(testFile); removeErr != nil {
			return false
		}
		return true
	}

	if err := unix.Access(path, unix.W_OK); err != nil {
		return false
	}

	return true
}

type brokenInstall struct {
	install db.Install
	missing []string
}

// checkPackageIntegrity checks if installed packages have their files intact
//
//nolint:gocyclo // integrity check aggregates multiple missing-file heuristics.
func checkPackageIntegrity(installs []db.Install) []brokenInstall {
	var broken []brokenInstall

	for _, install := range installs {
		if isSystemManagedInstall(install) {
			continue
		}

		var missing []string

		// Check if install path exists
		if install.InstallPath != "" {
			if _, err := os.Stat(install.InstallPath); os.IsNotExist(err) {
				missing = append(missing, install.InstallPath)
			}
		}

		// Check desktop files (plural or singular)
		for _, desktopPath := range getDesktopFilesFromDB(install) {
			if desktopPath == "" {
				continue
			}
			if _, err := os.Stat(desktopPath); os.IsNotExist(err) {
				missing = append(missing, desktopPath)
			}
		}

		// Check wrapper script
		if install.Metadata != nil {
			if wrapper, ok := install.Metadata["wrapper_script"].(string); ok && wrapper != "" {
				if _, err := os.Stat(wrapper); os.IsNotExist(err) {
					missing = append(missing, wrapper)
				}
			}

			// Check icon files
			var iconFiles []string
			if iconsSlice, ok := install.Metadata["icon_files"].([]string); ok {
				iconFiles = iconsSlice
			} else if iconsInterface, ok := install.Metadata["icon_files"].([]interface{}); ok {
				for _, item := range iconsInterface {
					if str, ok := item.(string); ok {
						iconFiles = append(iconFiles, str)
					}
				}
			}
			for _, iconPath := range iconFiles {
				if iconPath == "" {
					continue
				}
				if _, err := os.Stat(iconPath); os.IsNotExist(err) {
					missing = append(missing, iconPath)
				}
			}
		}

		if len(missing) > 0 {
			broken = append(broken, brokenInstall{install: install, missing: missing})
		}
	}

	return broken
}

func getDesktopFilesFromDB(install db.Install) []string {
	var desktopFiles []string

	if install.Metadata != nil {
		if files, ok := install.Metadata["desktop_files"].([]string); ok {
			desktopFiles = append(desktopFiles, files...)
		} else if filesInterface, ok := install.Metadata["desktop_files"].([]interface{}); ok {
			for _, item := range filesInterface {
				if str, ok := item.(string); ok {
					desktopFiles = append(desktopFiles, str)
				}
			}
		}
	}

	if len(desktopFiles) == 0 && install.DesktopFile != "" {
		desktopFiles = []string{install.DesktopFile}
	}

	return desktopFiles
}

func isSystemManagedInstall(install db.Install) bool {
	if install.Metadata != nil {
		if method, ok := install.Metadata["install_method"].(string); ok && method != "" {
			return method == core.InstallMethodPacman
		}
	}

	// Backward compatibility: older records used a descriptive InstallPath.
	return strings.Contains(install.InstallPath, "pacman")
}

// checkEnvironment checks environment variables
func checkEnvironment() {
	envVars := []struct {
		name   string
		needed bool
	}{
		{"XDG_DATA_HOME", false},
		{"XDG_CONFIG_HOME", false},
		{"XDG_CACHE_HOME", false},
		{"WAYLAND_DISPLAY", false},
		{"HYPRLAND_INSTANCE_SIGNATURE", false},
	}

	for _, env := range envVars {
		value := os.Getenv(env.name)
		if value != "" {
			ui.PrintSuccess("%s: %s", env.name, value)
		} else {
			if env.needed {
				ui.PrintWarning("%s: not set", env.name)
			} else {
				ui.PrintInfo("%s: not set (using defaults)", env.name)
			}
		}
	}
}
