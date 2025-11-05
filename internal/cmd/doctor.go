package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/diogo/pkgctl/internal/config"
	"github.com/diogo/pkgctl/internal/db"
	"github.com/diogo/pkgctl/internal/ui"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
)

// NewDoctorCmd creates the doctor command
func NewDoctorCmd(cfg *config.Config, log *zerolog.Logger) *cobra.Command {
	var verbose bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check system dependencies and integrity",
		Long:  `Check system dependencies, configuration, database integrity, and installed packages.`,
		RunE: func(cmd *cobra.Command, args []string) error {
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
				if checkDirectory(dir.path, dir.name) {
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
				defer database.Close()

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
							for _, install := range brokenInstalls {
								fmt.Printf("  • %s (%s)\n", install.Name, install.InstallID)
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

	return cmd
}

// checkDependency checks if a dependency is available
func checkDependency(command, name, purpose string, required bool) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// checkDirectory checks if a directory exists and is writable
func checkDirectory(path, name string) bool {
	info, err := os.Stat(path)
	if err != nil {
		// Try to create if it doesn't exist
		if os.IsNotExist(err) {
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
	testFile := filepath.Join(path, ".pkgctl-test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return false
	}
	os.Remove(testFile)

	return true
}

// checkPackageIntegrity checks if installed packages have their files intact
func checkPackageIntegrity(installs []db.Install) []db.Install {
	var broken []db.Install

	for _, install := range installs {
		// Check if install path exists
		if install.InstallPath != "" {
			if _, err := os.Stat(install.InstallPath); os.IsNotExist(err) {
				broken = append(broken, install)
				continue
			}
		}

		// Check if desktop file exists (if specified)
		if install.DesktopFile != "" {
			if _, err := os.Stat(install.DesktopFile); os.IsNotExist(err) {
				broken = append(broken, install)
			}
		}
	}

	return broken
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
