package flatpak

import (
	"context"

	backendbase "github.com/quantmind-br/upkg/internal/backends/base"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
)

// FlatpakBackend handles Flatpak package installation and management
type FlatpakBackend struct {
	*backendbase.BaseBackend
}

// New creates a new FlatpakBackend with default dependencies
func New(cfg *config.Config, log *zerolog.Logger) *FlatpakBackend {
	return &FlatpakBackend{BaseBackend: backendbase.New(cfg, log)}
}

// NewWithDeps creates a new FlatpakBackend with injected dependencies (for testing)
func NewWithDeps(cfg *config.Config, log *zerolog.Logger, fs afero.Fs, runner helpers.CommandRunner) *FlatpakBackend {
	return &FlatpakBackend{BaseBackend: backendbase.NewWithDeps(cfg, log, fs, runner)}
}

// NewWithRunner creates a new FlatpakBackend with custom command runner
func NewWithRunner(cfg *config.Config, log *zerolog.Logger, runner helpers.CommandRunner) *FlatpakBackend {
	return NewWithDeps(cfg, log, afero.NewOsFs(), runner)
}

// Name returns the backend name
func (f *FlatpakBackend) Name() string {
	return "flatpak"
}

// Detect checks if the input is a Flatpak package
func (f *FlatpakBackend) Detect(ctx context.Context, input string) (bool, error) {
	return Detect(ctx, f.Fs, input)
}

// Install installs a Flatpak package
// TODO: Implement installation logic
func (f *FlatpakBackend) Install(ctx context.Context, input string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	return nil, nil
}

// Uninstall removes a Flatpak package
// TODO: Implement uninstallation logic
func (f *FlatpakBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	return nil
}
