package backends

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/diogo/upkg/internal/backends/appimage"
	"github.com/diogo/upkg/internal/backends/binary"
	"github.com/diogo/upkg/internal/backends/deb"
	"github.com/diogo/upkg/internal/backends/rpm"
	"github.com/diogo/upkg/internal/backends/tarball"
	"github.com/diogo/upkg/internal/config"
	"github.com/diogo/upkg/internal/core"
	"github.com/rs/zerolog"
)

// Backend interface that all package installers must implement
type Backend interface {
	// Name returns the backend name
	Name() string

	// Detect checks if this backend can handle the package
	Detect(ctx context.Context, packagePath string) (bool, error)

	// Install installs the package
	Install(ctx context.Context, packagePath string, opts core.InstallOptions) (*core.InstallRecord, error)

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
	registry := &Registry{
		backends: make([]Backend, 0),
		logger:   log,
	}

	// Register backends in priority order
	// 1. DEB and RPM (specific format detection)
	registry.backends = append(registry.backends, deb.New(cfg, log))
	registry.backends = append(registry.backends, rpm.New(cfg, log))

	// 2. AppImage must come before Binary (AppImages are also ELF)
	registry.backends = append(registry.backends, appimage.New(cfg, log))

	// 3. Binary (catches standalone ELF binaries)
	registry.backends = append(registry.backends, binary.New(cfg, log))

	// 4. Tarball/Zip (archive formats)
	registry.backends = append(registry.backends, tarball.New(cfg, log))

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
	fileType, _ := r.detectFileType(packagePath)

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

	if fileType == "shell script" || fileType == "text" {
		errorMsg += "\n\nNote: Shell scripts and text files are not supported as standalone packages."
		errorMsg += "\nWorkaround: Package your script in a tarball (.tar.gz) with any required assets."
	}

	return errors.New(errorMsg)
}

// detectFileType attempts to detect the file type
func (r *Registry) detectFileType(packagePath string) (string, error) {
	// Use the file command to detect type
	// This is a simplified version - could be more sophisticated
	file, err := os.Open(packagePath)
	if err != nil {
		return "unknown", err
	}
	defer file.Close()

	// Read first 512 bytes for magic number detection
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil {
		return "unknown", err
	}

	// Check for common file signatures
	if n >= 4 {
		// ELF binary
		if buf[0] == 0x7f && buf[1] == 0x45 && buf[2] == 0x4c && buf[3] == 0x46 {
			return "ELF binary", nil
		}
		// Shell script
		if buf[0] == '#' && buf[1] == '!' {
			return "shell script", nil
		}
		// Zip/JAR
		if buf[0] == 'P' && buf[1] == 'K' {
			return "ZIP archive", nil
		}
	}

	// Check for gzip (tar.gz)
	if n >= 2 && buf[0] == 0x1f && buf[1] == 0x8b {
		return "gzip compressed (likely tar.gz)", nil
	}

	// Check for bzip2 (tar.bz2)
	if n >= 3 && buf[0] == 'B' && buf[1] == 'Z' && buf[2] == 'h' {
		return "bzip2 compressed (likely tar.bz2)", nil
	}

	// Check for XZ (tar.xz)
	if n >= 6 && buf[0] == 0xfd && buf[1] == '7' && buf[2] == 'z' && buf[3] == 'X' && buf[4] == 'Z' && buf[5] == 0x00 {
		return "XZ compressed (likely tar.xz)", nil
	}

	return "unknown", nil
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
