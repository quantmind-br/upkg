package backends

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/quantmind-br/upkg/internal/backends/appimage"
	"github.com/quantmind-br/upkg/internal/backends/binary"
	"github.com/quantmind-br/upkg/internal/backends/deb"
	"github.com/quantmind-br/upkg/internal/backends/flatpak"
	"github.com/quantmind-br/upkg/internal/backends/rpm"
	"github.com/quantmind-br/upkg/internal/backends/tarball"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// Package type constants for detection
const (
	packageTypeTarball = "tarball"
)

// Backend interface that all package installers must implement
type Backend interface {
	// Name returns the backend name
	Name() string

	// Detect checks if this backend can handle the package
	Detect(ctx context.Context, packagePath string) (bool, error)

	// Install installs the package
	Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error)

	// Uninstall removes the installed package
	Uninstall(ctx context.Context, record *core.InstallRecord) error
}

// Registry manages all available backends
type Registry struct {
	backends []Backend
	logger   *zerolog.Logger
}

// NewRegistry creates a backend registry with all backends
func NewRegistry(cfg *config.Config, log *zerolog.Logger) *Registry {
	return NewRegistryWithDeps(cfg, log, afero.NewOsFs(), helpers.NewOSCommandRunner())
}

// NewRegistryWithDeps cria um registry com dependências comuns injetadas.
// O fs será usado conforme os backends forem migrados para DI.
func NewRegistryWithDeps(cfg *config.Config, log *zerolog.Logger, fs afero.Fs, runner helpers.CommandRunner) *Registry {
	registry := &Registry{
		backends: make([]Backend, 0),
		logger:   log,
	}

	// Register backends in priority order
	// 0. Flatpak (App IDs must be detected before file-based formats)
	registry.backends = append(registry.backends, flatpak.NewWithDeps(cfg, log, fs, runner))

	// 1. DEB and RPM (specific format detection)
	registry.backends = append(registry.backends, deb.NewWithDeps(cfg, log, fs, runner))
	registry.backends = append(registry.backends, rpm.NewWithDeps(cfg, log, fs, runner))

	// 2. AppImage must come before Binary (AppImages are also ELF)
	registry.backends = append(registry.backends, appimage.NewWithDeps(cfg, log, fs, runner))

	// 3. Binary (catches standalone ELF binaries)
	registry.backends = append(registry.backends, binary.NewWithDeps(cfg, log, fs, runner))

	// 4. Tarball/Zip (archive formats)
	registry.backends = append(registry.backends, tarball.NewWithDeps(cfg, log, fs, runner))

	return registry
}

// DetectBackend finds the appropriate backend for a package
func (r *Registry) DetectBackend(ctx context.Context, packagePath string) (Backend, error) {
	r.logger.Debug().
		Str("package_path", packagePath).
		Msg("detecting backend for package")

	for _, backend := range r.backends {
		can, err := backend.Detect(ctx, packagePath)
		if err != nil {
			r.logger.Warn().
				Err(err).
				Str("backend", backend.Name()).
				Str("package_path", packagePath).
				Msg("backend detection failed")
			continue
		}

		if can {
			r.logger.Info().
				Str("backend", backend.Name()).
				Str("package_path", packagePath).
				Msg("backend detected")
			return backend, nil
		}
	}

	// Provide detailed error message with file type detection
	return nil, r.createDetectionError(packagePath)
}

// createDetectionError creates a detailed error message for unsupported packages
func (r *Registry) createDetectionError(packagePath string) error {
	// Try to detect file type
	fileType, detectErr := r.detectFileType(packagePath)
	if detectErr != nil {
		r.logger.Debug().Err(detectErr).Str("package_path", packagePath).Msg("failed to detect file type")
		fileType = ""
	}

	errorMsg := fmt.Sprintf("cannot detect package type for: %s", packagePath)

	if fileType != "" {
		errorMsg += fmt.Sprintf("\n\nDetected file type: %s", fileType)
	}

	errorMsg += "\n\nSupported package types:"
	errorMsg += "\n  • AppImage (.AppImage)"
	errorMsg += "\n  • DEB (.deb)"
	errorMsg += "\n  • RPM (.rpm)"
	errorMsg += "\n  • Tarball (.tar.gz, .tar.xz, .tar.bz2, .tgz)"
	errorMsg += "\n  • Zip (.zip)"
	errorMsg += "\n  • ELF Binary (executable files)"

	if fileType == fileTypeDisplayName(helpers.FileTypeScript) {
		errorMsg += "\n\nNote: Shell scripts and text files are not supported as standalone packages."
		errorMsg += "\nWorkaround: Package your script in a tarball (.tar.gz) with any required assets."
	}

	ext := strings.ToLower(filepath.Ext(packagePath))
	switch ext {
	case ".flatpak", ".flatpakref", ".flatpakrepo":
		errorMsg += "\n\nNote: This looks like a Flatpak package."
		errorMsg += "\nUse: flatpak install <file-or-ref>"
	case ".snap":
		errorMsg += "\n\nNote: This looks like a Snap package."
		errorMsg += "\nUse: sudo snap install <file.snap>"
	}

	return errors.New(errorMsg)
}

// detectFileType reports a human-readable file-type label for diagnostics,
// derived from the single content-signature detector so classification matches
// what the backends themselves use. No extension or looser fallback.
func (r *Registry) detectFileType(packagePath string) (string, error) {
	fileType, err := helpers.DetectFileType(packagePath)
	if err != nil {
		return "", err
	}
	if fileType == helpers.FileTypeUnknown {
		return "", fmt.Errorf("unknown file type")
	}
	return fileTypeDisplayName(fileType), nil
}

// fileTypeDisplayName maps a detected FileType to the human-readable label used
// in user-facing diagnostics.
func fileTypeDisplayName(fileType helpers.FileType) string {
	switch fileType {
	case helpers.FileTypeDEB:
		return "deb"
	case helpers.FileTypeRPM:
		return "rpm"
	case helpers.FileTypeAppImage:
		return "AppImage"
	case helpers.FileTypeELF:
		return "ELF binary"
	case helpers.FileTypeScript:
		return "shell script"
	case helpers.FileTypeZip:
		return "ZIP archive"
	case helpers.FileTypeTarGz, helpers.FileTypeTarXz, helpers.FileTypeTarBz2, helpers.FileTypeTar:
		return packageTypeTarball
	default:
		return string(fileType)
	}
}

// GetBackend retrieves a backend by name
func (r *Registry) GetBackend(name string) (Backend, error) {
	for _, backend := range r.backends {
		if backend.Name() == name {
			return backend, nil
		}
	}
	return nil, fmt.Errorf("backend not found: %s", name)
}

// ListBackends returns all registered backends
func (r *Registry) ListBackends() []string {
	names := make([]string, len(r.backends))
	for i, backend := range r.backends {
		names[i] = backend.Name()
	}
	return names
}
