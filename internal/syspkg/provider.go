package syspkg

import (
	"context"
)

// PackageInfo contains basic package metadata
type PackageInfo struct {
	Name    string
	Version string
}

// Provider defines the interface for system package management
type Provider interface {
	// Name returns the provider name (e.g., "pacman", "apt", "dnf")
	Name() string

	// Install installs a package from a local path
	Install(ctx context.Context, pkgPath string) error

	// Remove removes a package by name
	Remove(ctx context.Context, pkgName string) error

	// IsInstalled checks if a package is installed
	IsInstalled(ctx context.Context, pkgName string) (bool, error)

	// GetInfo retrieves package information
	GetInfo(ctx context.Context, pkgName string) (*PackageInfo, error)

	// ListFiles lists files owned by the package
	ListFiles(ctx context.Context, pkgName string) ([]string, error)
}
