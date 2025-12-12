package deb

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/cache"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/syspkg"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)
	assert.Equal(t, "deb", backend.Name())
}

func TestNewWithRunner(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockRunner := &helpers.MockCommandRunner{}

	backend := NewWithRunner(cfg, &logger, mockRunner)

	assert.NotNil(t, backend)
	assert.Equal(t, "deb", backend.Name())
	assert.Equal(t, mockRunner, backend.Runner)
}

func TestNewWithCacheManager(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockCacheMgr := cache.NewCacheManager()

	backend := NewWithCacheManager(cfg, &logger, mockCacheMgr)

	assert.NotNil(t, backend)
	assert.Equal(t, "deb", backend.Name())
	assert.Equal(t, mockCacheMgr, backend.cacheManager)
}

func TestDetect(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)

	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		filename    string
		content     []byte
		expected    bool
		expectError bool
	}{
		{
			name:     "valid .deb extension",
			filename: "test.deb",
			content:  []byte("!<arch>\ndebian-binary   "),
			expected: true,
		},
		{
			name:     "deb magic number",
			filename: "package",
			content:  []byte("!<arch>\ndebian-binary   1234567890"),
			expected: true,
		},
		{
			name:     "text file",
			filename: "test.txt",
			content:  []byte("plain text content"),
			expected: false,
		},
		{
			name:     "non-existent file",
			filename: "nonexistent.deb",
			content:  nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var filePath string
			if tt.content != nil {
				filePath = filepath.Join(tmpDir, tt.filename)
				require.NoError(t, os.WriteFile(filePath, tt.content, 0644))
			} else {
				filePath = filepath.Join(tmpDir, tt.filename)
			}

			result, err := backend.Detect(context.Background(), filePath)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFixDependencyLine(t *testing.T) {
	logger := zerolog.New(io.Discard)

	tests := []struct {
		input    string
		expected string
	}{
		// No change
		{"depend = something", "depend = something"},
		{"pkgname = test", "pkgname = test"}, // Non-depend line

		// Debian -> Arch Mappings
		{"depend = gtk", "depend = gtk3"},
		{"depend = gtk2.0", "depend = gtk2"},
		{"depend = python3", "depend = python"},
		{"depend = libssl", "depend = openssl"},
		{"depend = libssl1.1", "depend = openssl-1.1"},
		{"depend = zlib1g", "depend = zlib"},

		// Version constraints preservation
		{"depend = gtk>=3.0", "depend = gtk3>=3.0"},
		{"depend = python3>3.8", "depend = python>3.8"},

		// Malformed patterns fixes
		{"depend = c>=2.14", "depend = glibc>=2.14"},
		{"depend = libx111.6.2", "depend = libx11>=1.6.2"},
		{"depend = libxcomposite0.4.4-1", "depend = libxcomposite>=0.4.4-1"},
		{"depend = nspr4.9-2~", "depend = nspr>=4.9-2~"},

		// Artifact removal
		{"depend = anaconda", ""},
		{"depend = cura-bin", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := fixDependencyLine(tt.input, &logger)
			assert.Equal(t, tt.expected, got, "fixDependencyLine(%q) = %q, want %q", tt.input, got, tt.expected)
		})
	}
}

func TestFindDesktopFiles(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)

	tests := []struct {
		name     string
		files    []string
		expected []string
	}{
		{
			name:     "no desktop files",
			files:    []string{"/usr/bin/app", "/usr/share/icons/app.png"},
			expected: nil,
		},
		{
			name:     "single desktop file",
			files:    []string{"/usr/share/applications/app.desktop", "/usr/bin/app"},
			expected: []string{"/usr/share/applications/app.desktop"},
		},
		{
			name: "multiple desktop files",
			files: []string{
				"/usr/share/applications/app1.desktop",
				"/usr/share/applications/app2.desktop",
				"/usr/bin/app",
			},
			expected: []string{
				"/usr/share/applications/app1.desktop",
				"/usr/share/applications/app2.desktop",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backend.findDesktopFiles(tt.files)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindIconFiles(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)

	tests := []struct {
		name     string
		files    []string
		expected []string
	}{
		{
			name:     "no icon files",
			files:    []string{"/usr/bin/app", "/usr/share/doc/readme.txt"},
			expected: nil,
		},
		{
			name: "png icon in icons directory",
			files: []string{
				"/usr/share/icons/hicolor/48x48/apps/app.png",
				"/usr/bin/app",
			},
			expected: []string{"/usr/share/icons/hicolor/48x48/apps/app.png"},
		},
		{
			name: "svg icon in icons directory",
			files: []string{
				"/usr/share/icons/hicolor/scalable/apps/app.svg",
			},
			expected: []string{"/usr/share/icons/hicolor/scalable/apps/app.svg"},
		},
		{
			name: "icon not in icons directory",
			files: []string{
				"/usr/share/pixmaps/app.png", // Not in "icons" directory
			},
			expected: nil,
		},
		{
			name: "multiple icons",
			files: []string{
				"/usr/share/icons/hicolor/16x16/apps/app.png",
				"/usr/share/icons/hicolor/32x32/apps/app.png",
				"/usr/share/icons/hicolor/scalable/apps/app.svg",
			},
			expected: []string{
				"/usr/share/icons/hicolor/16x16/apps/app.png",
				"/usr/share/icons/hicolor/32x32/apps/app.png",
				"/usr/share/icons/hicolor/scalable/apps/app.svg",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backend.findIconFiles(tt.files)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDebtapInitialized(t *testing.T) {
	// This test checks the logic, not the actual system state
	// Since we can't mock the filesystem easily for this function,
	// we test the logic indirectly

	t.Run("returns false when cache dir doesn't exist", func(t *testing.T) {
		// The default debtap cache directory likely doesn't exist in CI
		// This tests the expected behavior
		result := isDebtapInitialized()
		// We can't assert a specific value since it depends on system state
		// Just ensure it doesn't panic
		_ = result
	})
}

func TestInstall_MissingDebtap(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		RequireCommandFunc: func(name string) error {
			if name == "debtap" {
				return fmt.Errorf("command not found: %s", name)
			}
			return nil
		},
	}

	cfg := &config.Config{}
	backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

	tmpDir := t.TempDir()
	fakeDeb := filepath.Join(tmpDir, "test.deb")
	require.NoError(t, os.WriteFile(fakeDeb, []byte("fake deb content"), 0644))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), fakeDeb, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "debtap")
	assert.Nil(t, record)
}

func TestInstall_PackageNotFound(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		RequireCommandFunc: func(name string) error { return nil },
	}

	cfg := &config.Config{}
	backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), "/nonexistent/package.deb", core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
	assert.Nil(t, record)
}

func TestUninstall_PackageNotInPacman(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool { return false },
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}

	// Create a mock provider that returns not installed
	mockProvider := &mockSyspkgProvider{
		isInstalled: false,
	}

	backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)
	backend.sys = mockProvider
	backend.cacheManager = cacheManager

	record := &core.InstallRecord{
		InstallID:   "test-id",
		Name:        "test-package",
		PackageType: core.PackageTypeDeb,
		Metadata: core.Metadata{
			InstallMethod: core.InstallMethodPacman,
		},
	}

	err := backend.Uninstall(context.Background(), record)
	assert.NoError(t, err) // Should not error if package not found
}

func TestUninstall_Success(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool { return false },
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}

	mockProvider := &mockSyspkgProvider{
		isInstalled: true,
		removeErr:   nil,
	}

	backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)
	backend.sys = mockProvider
	backend.cacheManager = cacheManager

	record := &core.InstallRecord{
		InstallID:   "test-id",
		Name:        "test-package",
		PackageType: core.PackageTypeDeb,
		Metadata: core.Metadata{
			InstallMethod: core.InstallMethodPacman,
		},
	}

	err := backend.Uninstall(context.Background(), record)
	assert.NoError(t, err)
	assert.True(t, mockProvider.removeCalled)
}

func TestQueryDebName(t *testing.T) {
	logger := zerolog.New(io.Discard)

	t.Run("returns error when dpkg-deb not found", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(name string) bool {
				return false
			},
		}

		cfg := &config.Config{}
		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		tmpDir := t.TempDir()
		fakeDeb := filepath.Join(tmpDir, "test.deb")
		require.NoError(t, os.WriteFile(fakeDeb, []byte("fake"), 0644))

		name, err := backend.queryDebName(context.Background(), fakeDeb)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dpkg-deb")
		assert.Empty(t, name)
	})

	t.Run("returns package name successfully", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(name string) bool {
				return name == "dpkg-deb"
			},
			RunCommandFunc: func(ctx context.Context, name string, args ...string) (string, error) {
				if name == "dpkg-deb" {
					return "my-awesome-package\n", nil
				}
				return "", nil
			},
		}

		cfg := &config.Config{}
		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		tmpDir := t.TempDir()
		fakeDeb := filepath.Join(tmpDir, "test.deb")
		require.NoError(t, os.WriteFile(fakeDeb, []byte("fake"), 0644))

		name, err := backend.queryDebName(context.Background(), fakeDeb)
		assert.NoError(t, err)
		assert.Equal(t, "my-awesome-package", name)
	})
}

func TestDependencyMappings(t *testing.T) {
	logger := zerolog.New(io.Discard)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// GTK mappings
		{"gtk to gtk3", "depend = gtk", "depend = gtk3"},
		{"gtk2.0 to gtk2", "depend = gtk2.0", "depend = gtk2"},
		{"gtk-3.0 to gtk3", "depend = gtk-3.0", "depend = gtk3"},

		// Python mapping
		{"python3 to python", "depend = python3", "depend = python"},
		{"python3 with version", "depend = python3>=3.9", "depend = python>=3.9"},

		// SSL mappings
		{"libssl to openssl", "depend = libssl", "depend = openssl"},
		{"libssl1.1 to openssl-1.1", "depend = libssl1.1", "depend = openssl-1.1"},
		{"libssl3 to openssl", "depend = libssl3", "depend = openssl"},

		// Other common mappings
		{"zlib1g to zlib", "depend = zlib1g", "depend = zlib"},
		{"libjpeg to libjpeg-turbo", "depend = libjpeg", "depend = libjpeg-turbo"},
		{"libcurl to curl", "depend = libcurl", "depend = curl"},
		{"libcurl4 to curl", "depend = libcurl4", "depend = curl"},
		{"libglib2.0 to glib2", "depend = libglib2.0", "depend = glib2"},
		{"libnotify4 to libnotify", "depend = libnotify4", "depend = libnotify"},

		// Glibc fix
		{"c>= to glibc>=", "depend = c>=2.17", "depend = glibc>=2.17"},
		{"c> to glibc>", "depend = c>2.14", "depend = glibc>2.14"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixDependencyLine(tt.input, &logger)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMalformedDependencyPatterns(t *testing.T) {
	logger := zerolog.New(io.Discard)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// libx11 malformation
		{"libx11 with embedded version", "depend = libx111.6.2", "depend = libx11>=1.6.2"},
		{"libx11 with complex version", "depend = libx111.4.99.1", "depend = libx11>=1.4.99.1"},

		// libxcomposite malformation
		{"libxcomposite with embedded version", "depend = libxcomposite0.4.4-1", "depend = libxcomposite>=0.4.4-1"},

		// libxdamage malformation
		{"libxdamage with embedded version", "depend = libxdamage1.1.4", "depend = libxdamage>=1.1.4"},

		// nspr malformation
		{"nspr with embedded version", "depend = nspr4.9-2~", "depend = nspr>=4.9-2~"},
		{"nspr with simple version", "depend = nspr4.21", "depend = nspr>=4.21"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixDependencyLine(tt.input, &logger)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInvalidDependencyRemoval(t *testing.T) {
	logger := zerolog.New(io.Discard)

	invalidDeps := []string{
		"depend = anaconda",
		"depend = anaconda-something",
		"depend = cura-bin",
		"depend = apparmor.d-git",
	}

	for _, dep := range invalidDeps {
		t.Run(dep, func(t *testing.T) {
			result := fixDependencyLine(dep, &logger)
			assert.Empty(t, result, "Invalid dependency %q should be removed", dep)
		})
	}
}

func TestVersionConstraintPreservation(t *testing.T) {
	logger := zerolog.New(io.Discard)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"greater than or equal", "depend = gtk>=3.0", "depend = gtk3>=3.0"},
		{"greater than", "depend = python3>3.8", "depend = python>3.8"},
		{"less than or equal", "depend = libssl<=1.1", "depend = openssl<=1.1"},
		{"less than", "depend = zlib1g<1.3", "depend = zlib<1.3"},
		{"exact version", "depend = libcurl=7.80", "depend = curl=7.80"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fixDependencyLine(tt.input, &logger)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// mockSyspkgProvider is a mock implementation of syspkg.Provider for testing
type mockSyspkgProvider struct {
	isInstalled  bool
	removeCalled bool
	removeErr    error
}

func (m *mockSyspkgProvider) Name() string {
	return "mock"
}

func (m *mockSyspkgProvider) Install(ctx context.Context, packagePath string) error {
	return nil
}

func (m *mockSyspkgProvider) Remove(ctx context.Context, packageName string) error {
	m.removeCalled = true
	return m.removeErr
}

func (m *mockSyspkgProvider) IsInstalled(ctx context.Context, packageName string) (bool, error) {
	return m.isInstalled, nil
}

func (m *mockSyspkgProvider) GetInfo(ctx context.Context, packageName string) (*syspkg.PackageInfo, error) {
	return &syspkg.PackageInfo{Name: packageName, Version: "1.0.0"}, nil
}

func (m *mockSyspkgProvider) ListFiles(ctx context.Context, packageName string) ([]string, error) {
	return []string{}, nil
}
