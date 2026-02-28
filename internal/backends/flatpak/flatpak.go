package flatpak

import (
	"context"
	"fmt"
	"strings"
	"time"

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

func (f *FlatpakBackend) Install(ctx context.Context, input string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error) {
	if err := f.Runner.RequireCommand("flatpak"); err != nil {
		return nil, err
	}

	var args []string
	var appID string
	var remote string

	if strings.Contains(input, ":") && !strings.Contains(input, "/") {
		parts := strings.SplitN(input, ":", 2)
		remote = parts[0]
		appID = parts[1]
	} else if appIDRegex.MatchString(input) {
		appID = input
		remote = "flathub"
	} else {
		appID = ""
	}

	args = []string{"install", "--user", "--noninteractive", "--or-update"}

	if remote != "" {
		args = append(args, remote, appID)
	} else {
		args = append(args, input)
	}

	f.Log.Info().
		Str("input", input).
		Strs("args", args).
		Msg("Installing flatpak package")

	output, err := f.Runner.RunCommand(ctx, "flatpak", args...)
	if err != nil {
		return nil, fmt.Errorf("flatpak install failed: %w", err)
	}

	f.Log.Debug().Str("output", output).Msg("Flatpak install output")

	if appID == "" {
		appID = extractAppIDFromOutput(output)
		if appID == "" {
			appID = input
		}
	}

	record := &core.InstallRecord{
		InstallID:    helpers.GenerateInstallID(appID),
		PackageType:  core.PackageTypeFlatpak,
		Name:         appID,
		InstallDate:  time.Now(),
		OriginalFile: input,
		InstallPath:  "",
		Metadata:     core.Metadata{},
	}

	return record, nil
}

func extractAppIDFromOutput(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if appIDRegex.MatchString(line) {
			return line
		}
		for _, word := range strings.Fields(line) {
			if appIDRegex.MatchString(word) {
				return word
			}
		}
	}
	return ""
}

func (f *FlatpakBackend) Uninstall(ctx context.Context, record *core.InstallRecord) error {
	if err := f.Runner.RequireCommand("flatpak"); err != nil {
		return err
	}

	args := []string{"uninstall", "--user", "--noninteractive", "-y"}

	if record.Metadata.InstallMethod == "delete-data" {
		args = append(args, "--delete-data")
	}

	args = append(args, record.Name)

	f.Log.Info().
		Str("app_id", record.Name).
		Strs("args", args).
		Msg("Uninstalling flatpak package")

	output, err := f.Runner.RunCommand(ctx, "flatpak", args...)
	if err != nil {
		return fmt.Errorf("flatpak uninstall failed: %w", err)
	}

	f.Log.Debug().Str("output", output).Msg("Flatpak uninstall output")

	return nil
}
