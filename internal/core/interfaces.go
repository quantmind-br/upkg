package core

// InstallOptions contains options for package installation
type InstallOptions struct {
	Force          bool   // Force installation even if already installed
	SkipDesktop    bool   // Skip desktop integration
	CustomName     string // Custom application name
	SkipWaylandEnv bool   // Skip Wayland environment variable injection
	Overwrite      bool   // Overwrite conflicting files from other packages (pacman --overwrite)
}
