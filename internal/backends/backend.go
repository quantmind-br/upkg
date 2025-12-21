package backends

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/quantmind-br/upkg/internal/backends/appimage"
	"github.com/quantmind-br/upkg/internal/backends/binary"
	"github.com/quantmind-br/upkg/internal/backends/deb"
	"github.com/quantmind-br/upkg/internal/backends/rpm"
	"github.com/quantmind-br/upkg/internal/backends/tarball"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
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

	if fileType == "shell script" || fileType == "text" {
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

// detectFileType attempts to detect the file type
//
//nolint:gocyclo // file type detection is a set of signature checks.
func (r *Registry) detectFileType(packagePath string) (string, error) {
	// Use the file command to detect type
	// This is a simplified version - could be more sophisticated
	file, err := os.Open(packagePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	// Read first 512 bytes for magic number detection
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil {
		return "", err
	}

	// Check for common file signatures
	if n >= 4 {
		// DEB magic: "!<arch>\ndebian"
		if n >= 16 && bytes.Contains(buf[:60], []byte("debian")) {
			return "deb", nil
		}
		// RPM magic: 0xED 0xAB 0xEE 0xDB
		if buf[0] == 0xED && buf[1] == 0xAB && buf[2] == 0xEE && buf[3] == 0xDB {
			return "rpm", nil
		}
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
		return "tarball", nil
	}

	// Check for bzip2 (tar.bz2)
	if n >= 3 && buf[0] == 'B' && buf[1] == 'Z' && buf[2] == 'h' {
		return "tarball", nil
	}

	// Check for XZ (tar.xz)
	if n >= 6 && buf[0] == 0xfd && buf[1] == '7' && buf[2] == 'z' && buf[3] == 'X' && buf[4] == 'Z' && buf[5] == 0x00 {
		return "tarball", nil
	}

	return "", fmt.Errorf("unknown file type")
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
