package flatpak

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	cfg := &config.Config{}
	logger := zerolog.Nop()

	backend := New(cfg, &logger)

	assert.NotNil(t, backend)
	assert.Equal(t, "flatpak", backend.Name())
}

func TestNewWithRunner(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockRunner := &helpers.MockCommandRunner{}

	backend := NewWithRunner(cfg, &logger, mockRunner)

	assert.NotNil(t, backend)
	assert.Equal(t, "flatpak", backend.Name())
	assert.Equal(t, mockRunner, backend.Runner)
}

func TestFlatpakBackend_Install(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		input          string
		opts           core.InstallOptions
		setupMock      func(*helpers.MockCommandRunner)
		setupFS        func(afero.Fs)
		expectError    bool
		errorContains  string
		validateRecord func(*testing.T, *core.InstallRecord)
	}{
		{
			name:  "install app ID from flathub",
			input: "org.mozilla.firefox",
			opts:  core.InstallOptions{},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return name == "flatpak"
				}
				m.RunCommandFunc = func(ctx context.Context, name string, args ...string) (string, error) {
					if name != "flatpak" {
						return "", fmt.Errorf("unexpected command: %s", name)
					}
					expectedArgs := []string{"install", "--user", "--noninteractive", "flathub", "org.mozilla.firefox"}
					if !reflect.DeepEqual(args, expectedArgs) {
						return "", fmt.Errorf("unexpected args: got %v, want %v", args, expectedArgs)
					}
					return "Installation complete.", nil
				}
			},
			setupFS: func(fs afero.Fs) {},
			validateRecord: func(t *testing.T, record *core.InstallRecord) {
				require.NotNil(t, record)
				assert.Equal(t, core.PackageTypeFlatpak, record.PackageType)
				assert.Equal(t, "org.mozilla.firefox", record.Name)
			},
		},
		{
			name:  "install local .flatpak bundle",
			input: "/tmp/app.flatpak",
			opts:  core.InstallOptions{},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return name == "flatpak"
				}
				m.RunCommandFunc = func(ctx context.Context, name string, args ...string) (string, error) {
					if name != "flatpak" {
						return "", fmt.Errorf("unexpected command: %s", name)
					}
					expectedArgs := []string{"install", "--user", "--noninteractive", "/tmp/app.flatpak"}
					if !reflect.DeepEqual(args, expectedArgs) {
						return "", fmt.Errorf("unexpected args: got %v, want %v", args, expectedArgs)
					}
					return "Installation complete.", nil
				}
			},
			setupFS: func(fs afero.Fs) {
				zipMagic := []byte{0x50, 0x4B, 0x03, 0x04}
				require.NoError(t, afero.WriteFile(fs, "/tmp/app.flatpak", zipMagic, 0644))
			},
			validateRecord: func(t *testing.T, record *core.InstallRecord) {
				require.NotNil(t, record)
				assert.Equal(t, core.PackageTypeFlatpak, record.PackageType)
				assert.Equal(t, "/tmp/app.flatpak", record.OriginalFile)
			},
		},
		{
			name:  "install local .flatpakref",
			input: "/tmp/app.flatpakref",
			opts:  core.InstallOptions{},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return name == "flatpak"
				}
				m.RunCommandFunc = func(ctx context.Context, name string, args ...string) (string, error) {
					if name != "flatpak" {
						return "", fmt.Errorf("unexpected command: %s", name)
					}
					expectedArgs := []string{"install", "--user", "--noninteractive", "/tmp/app.flatpakref"}
					if !reflect.DeepEqual(args, expectedArgs) {
						return "", fmt.Errorf("unexpected args: got %v, want %v", args, expectedArgs)
					}
					return "Installation complete.", nil
				}
			},
			setupFS: func(fs afero.Fs) {
				refContent := []byte("[Flatpak Ref]\nName=TestApp\n")
				require.NoError(t, afero.WriteFile(fs, "/tmp/app.flatpakref", refContent, 0644))
			},
			validateRecord: func(t *testing.T, record *core.InstallRecord) {
				require.NotNil(t, record)
				assert.Equal(t, core.PackageTypeFlatpak, record.PackageType)
				assert.Equal(t, "/tmp/app.flatpakref", record.OriginalFile)
			},
		},
		{
			name:  "install with custom remote",
			input: "fedora:org.gnome.Builder",
			opts:  core.InstallOptions{},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return name == "flatpak"
				}
				m.RunCommandFunc = func(ctx context.Context, name string, args ...string) (string, error) {
					if name != "flatpak" {
						return "", fmt.Errorf("unexpected command: %s", name)
					}
					expectedArgs := []string{"install", "--user", "--noninteractive", "fedora", "org.gnome.Builder"}
					if !reflect.DeepEqual(args, expectedArgs) {
						return "", fmt.Errorf("unexpected args: got %v, want %v", args, expectedArgs)
					}
					return "Installation complete.", nil
				}
			},
			setupFS: func(fs afero.Fs) {},
			validateRecord: func(t *testing.T, record *core.InstallRecord) {
				require.NotNil(t, record)
				assert.Equal(t, core.PackageTypeFlatpak, record.PackageType)
				assert.Equal(t, "org.gnome.Builder", record.Name)
			},
		},
		{
			name:  "error: flatpak CLI not found",
			input: "org.mozilla.firefox",
			opts:  core.InstallOptions{},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return false
				}
				m.RequireCommandFunc = func(name string) error {
					return fmt.Errorf("command not found: %s", name)
				}
			},
			setupFS:       func(fs afero.Fs) {},
			expectError:   true,
			errorContains: "command not found",
		},
		{
			name:  "error: remote not configured",
			input: "org.mozilla.firefox",
			opts:  core.InstallOptions{},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return name == "flatpak"
				}
				m.RunCommandFunc = func(ctx context.Context, name string, args ...string) (string, error) {
					return "", fmt.Errorf("error: Remote \"flathub\" not found")
				}
			},
			setupFS:       func(fs afero.Fs) {},
			expectError:   true,
			errorContains: "Remote \"flathub\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{}
			logger := zerolog.New(io.Discard)
			fs := afero.NewMemMapFs()

			mockRunner := &helpers.MockCommandRunner{}
			if tt.setupMock != nil {
				tt.setupMock(mockRunner)
			}

			if tt.setupFS != nil {
				tt.setupFS(fs)
			}

			backend := NewWithDeps(cfg, &logger, fs, mockRunner)
			tx := transaction.NewManager(&logger)

			record, err := backend.Install(context.Background(), tt.input, tt.opts, tx)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			assert.NoError(t, err)
			if tt.validateRecord != nil {
				tt.validateRecord(t, record)
			}
		})
	}
}

func TestFlatpakBackend_Uninstall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		record        *core.InstallRecord
		setupMock     func(*helpers.MockCommandRunner)
		expectError   bool
		errorContains string
	}{
		{
			name: "normal uninstall",
			record: &core.InstallRecord{
				InstallID:   "test-install-1",
				PackageType: core.PackageTypeFlatpak,
				Name:        "org.mozilla.firefox",
			},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return name == "flatpak"
				}
				m.RunCommandFunc = func(ctx context.Context, name string, args ...string) (string, error) {
					if name != "flatpak" {
						return "", fmt.Errorf("unexpected command: %s", name)
					}
					expectedArgs := []string{"uninstall", "--user", "--noninteractive", "org.mozilla.firefox"}
					if !reflect.DeepEqual(args, expectedArgs) {
						return "", fmt.Errorf("unexpected args: got %v, want %v", args, expectedArgs)
					}
					return "Uninstall complete.", nil
				}
			},
		},
		{
			name: "uninstall with delete-data",
			record: &core.InstallRecord{
				InstallID:   "test-install-2",
				PackageType: core.PackageTypeFlatpak,
				Name:        "org.gnome.Builder",
				Metadata: core.Metadata{
					InstallMethod: "delete-data",
				},
			},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return name == "flatpak"
				}
				m.RunCommandFunc = func(ctx context.Context, name string, args ...string) (string, error) {
					if name != "flatpak" {
						return "", fmt.Errorf("unexpected command: %s", name)
					}
					expectedArgs := []string{"uninstall", "--user", "--noninteractive", "--delete-data", "org.gnome.Builder"}
					if !reflect.DeepEqual(args, expectedArgs) {
						return "", fmt.Errorf("unexpected args: got %v, want %v", args, expectedArgs)
					}
					return "Uninstall complete.", nil
				}
			},
		},
		{
			name: "error: flatpak CLI not found",
			record: &core.InstallRecord{
				InstallID:   "test-install-3",
				PackageType: core.PackageTypeFlatpak,
				Name:        "org.mozilla.firefox",
			},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return false
				}
				m.RequireCommandFunc = func(name string) error {
					return fmt.Errorf("command not found: %s", name)
				}
			},
			expectError:   true,
			errorContains: "command not found",
		},
		{
			name: "error: app not installed",
			record: &core.InstallRecord{
				InstallID:   "test-install-4",
				PackageType: core.PackageTypeFlatpak,
				Name:        "org.nonexistent.App",
			},
			setupMock: func(m *helpers.MockCommandRunner) {
				m.CommandExistsFunc = func(name string) bool {
					return name == "flatpak"
				}
				m.RunCommandFunc = func(ctx context.Context, name string, args ...string) (string, error) {
					return "", fmt.Errorf("error: org.nonexistent.App not installed")
				}
			},
			expectError:   true,
			errorContains: "not installed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{}
			logger := zerolog.New(io.Discard)
			fs := afero.NewMemMapFs()

			mockRunner := &helpers.MockCommandRunner{}
			if tt.setupMock != nil {
				tt.setupMock(mockRunner)
			}

			backend := NewWithDeps(cfg, &logger, fs, mockRunner)

			err := backend.Uninstall(context.Background(), tt.record)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}
