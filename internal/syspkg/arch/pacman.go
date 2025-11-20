package arch

import (
	"context"
	"fmt"
	"strings"

	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/syspkg"
)

// PacmanProvider implements the Provider interface for Arch Linux
type PacmanProvider struct{}

// NewPacmanProvider creates a new Pacman provider
func NewPacmanProvider() *PacmanProvider {
	return &PacmanProvider{}
}

func (p *PacmanProvider) Name() string {
	return "pacman"
}

// Install installs a package from a local path using pacman
func (p *PacmanProvider) Install(ctx context.Context, pkgPath string) error {
	_, err := helpers.RunCommand(ctx, "sudo", "pacman", "-U", "--noconfirm", pkgPath)
	if err != nil {
		return fmt.Errorf("pacman installation failed: %w", err)
	}
	return nil
}

// Remove removes a package by name
func (p *PacmanProvider) Remove(ctx context.Context, pkgName string) error {
	_, err := helpers.RunCommand(ctx, "sudo", "pacman", "-R", "--noconfirm", pkgName)
	if err != nil {
		return fmt.Errorf("pacman removal failed: %w", err)
	}
	return nil
}

// IsInstalled checks if a package is installed
func (p *PacmanProvider) IsInstalled(ctx context.Context, pkgName string) (bool, error) {
	_, err := helpers.RunCommand(ctx, "pacman", "-Qi", pkgName)
	if err != nil {
		return false, nil // Not installed (or error, but usually not installed)
	}
	return true, nil
}

// GetInfo retrieves package information
func (p *PacmanProvider) GetInfo(ctx context.Context, pkgName string) (*syspkg.PackageInfo, error) {
	output, err := helpers.RunCommand(ctx, "pacman", "-Qi", pkgName)
	if err != nil {
		return nil, err
	}

	info := &syspkg.PackageInfo{Name: pkgName}
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "Version") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				info.Version = strings.TrimSpace(parts[1])
			}
		}
	}

	return info, nil
}

// ListFiles lists files owned by the package
func (p *PacmanProvider) ListFiles(ctx context.Context, pkgName string) ([]string, error) {
	output, err := helpers.RunCommand(ctx, "pacman", "-Ql", pkgName)
	if err != nil {
		return nil, err
	}

	var files []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		// Format: "pkgname /path/to/file"
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			files = append(files, parts[1])
		}
	}

	return files, nil
}
