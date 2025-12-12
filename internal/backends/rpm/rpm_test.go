package rpm

import (
	"context"
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

const rpmName = "rpm"

func TestName(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)
	if backend.Name() != rpmName {
		t.Errorf("Name() = %q, want %q", backend.Name(), rpmName)
	}
}

func TestNewWithRunner(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockRunner := &helpers.MockCommandRunner{}

	backend := NewWithRunner(cfg, &logger, mockRunner)

	assert.NotNil(t, backend)
	assert.Equal(t, rpmName, backend.Name())
	assert.Equal(t, mockRunner, backend.Runner)
}

func TestNewWithCacheManager(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockCacheMgr := cache.NewCacheManager()

	backend := NewWithCacheManager(cfg, &logger, mockCacheMgr)

	assert.NotNil(t, backend)
	assert.Equal(t, rpmName, backend.Name())
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
			name:     "valid .rpm extension with magic",
			filename: "test.rpm",
			content:  []byte{0xED, 0xAB, 0xEE, 0xDB, 0x00, 0x00}, // RPM magic
			expected: true,
		},
		{
			name:     "rpm magic number without extension",
			filename: "package",
			content:  []byte{0xED, 0xAB, 0xEE, 0xDB, 0x00, 0x00, 0x00, 0x00},
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
			filename: "nonexistent.rpm",
			content:  nil,
			expected: false,
		},
		{
			name:     "file with .rpm extension but wrong content",
			filename: "fake.rpm",
			content:  []byte("not an rpm"),
			expected: true, // Detection is by extension, not content validation
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

func TestExtractRpmBaseName(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		// Standard RPM naming
		{"firefox-123.0-1.x86_64.rpm", "firefox"},
		{"google-chrome-stable-120.0.6099.109-1.x86_64.rpm", "google-chrome-stable"},
		{"GitButler_Nightly-0.5.1650-1.x86_64.rpm", "GitButler_Nightly"},

		// Different architectures
		{"package-1.0.0-1.aarch64.rpm", "package"},
		{"package-1.0.0-1.i686.rpm", "package"},
		{"package-1.0.0-1.noarch.rpm", "package"},

		// Simple cases
		{"app-1.0.rpm", "app"},
		{"myapp.rpm", "myapp"},

		// Edge cases
		{"app-beta-2.0-1.x86_64.rpm", "app-beta"},
		{"some-app-with-dashes-1.2.3-4.x86_64.rpm", "some-app-with-dashes"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := extractRpmBaseName(tt.filename)
			assert.Equal(t, tt.expected, result, "extractRpmBaseName(%q) = %q, want %q", tt.filename, result, tt.expected)
		})
	}
}

func TestInstall_PackageNotFound(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(context.Background(), "/nonexistent/package.rpm", core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
	assert.Nil(t, record)
}

func TestInstall_InvalidPackageName(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	tmpDir := t.TempDir()
	fakeRpm := filepath.Join(tmpDir, "test.rpm")
	require.NoError(t, os.WriteFile(fakeRpm, []byte{0xED, 0xAB, 0xEE, 0xDB}, 0644))

	// Try to install with an empty custom name after normalization
	// Using a name that normalizes to empty string (all invalid chars)
	record, err := backend.Install(context.Background(), fakeRpm, core.InstallOptions{
		CustomName: "///",
	}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
	assert.Nil(t, record)
}

func TestInstall_NoInstallationMethod(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(_ string) bool {
			// Neither rpmextract.sh nor debtap/pacman available
			return false
		},
	}

	cfg := &config.Config{}
	backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

	tmpDir := t.TempDir()
	fakeRpm := filepath.Join(tmpDir, "test.rpm")
	require.NoError(t, os.WriteFile(fakeRpm, []byte{0xED, 0xAB, 0xEE, 0xDB}, 0644))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), fakeRpm, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no suitable RPM installation method")
	assert.Nil(t, record)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := backend.findIconFiles(tt.files)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCreateWrapper(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("creates standard wrapper", func(t *testing.T) {
		tmpDir := t.TempDir()
		wrapperPath := filepath.Join(tmpDir, "wrapper")
		execPath := "/path/to/executable"

		err := backend.createWrapper(wrapperPath, execPath)
		require.NoError(t, err)

		content, err := os.ReadFile(wrapperPath)
		require.NoError(t, err)

		assert.Contains(t, string(content), "#!/bin/bash")
		assert.Contains(t, string(content), execPath)
		assert.Contains(t, string(content), "upkg wrapper script")

		// Verify executable permission
		info, err := os.Stat(wrapperPath)
		require.NoError(t, err)
		assert.True(t, info.Mode().Perm()&0111 != 0, "wrapper should be executable")
	})
}

func TestRemoveIcons(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("removes existing icons", func(t *testing.T) {
		tmpDir := t.TempDir()

		icon1 := filepath.Join(tmpDir, "icon1.png")
		icon2 := filepath.Join(tmpDir, "icon2.png")
		require.NoError(t, os.WriteFile(icon1, []byte("fake icon 1"), 0644))
		require.NoError(t, os.WriteFile(icon2, []byte("fake icon 2"), 0644))

		backend.removeIcons([]string{icon1, icon2})

		assert.NoFileExists(t, icon1)
		assert.NoFileExists(t, icon2)
	})

	t.Run("handles non-existent icons gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		nonexistent := filepath.Join(tmpDir, "nonexistent.png")

		// Should not panic
		backend.removeIcons([]string{nonexistent})
	})
}

func TestUninstall_ExtractedMethod(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(_ string) bool { return false },
	}

	cfg := &config.Config{}

	t.Run("uninstalls all files", func(t *testing.T) {
		tmpDir := t.TempDir()

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		// Create fake installation
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(installDir, "app"), []byte("fake app"), 0755))

		wrapperPath := filepath.Join(tmpDir, "wrapper")
		require.NoError(t, os.WriteFile(wrapperPath, []byte("fake wrapper"), 0755))

		desktopPath := filepath.Join(tmpDir, "test.desktop")
		require.NoError(t, os.WriteFile(desktopPath, []byte("[Desktop Entry]"), 0644))

		iconPath := filepath.Join(tmpDir, "icon.png")
		require.NoError(t, os.WriteFile(iconPath, []byte("fake icon"), 0644))

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeRpm,
			InstallPath: installDir,
			DesktopFile: desktopPath,
			Metadata: core.Metadata{
				WrapperScript: wrapperPath,
				IconFiles:     []string{iconPath},
				InstallMethod: core.InstallMethodLocal,
			},
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)

		// Verify all files removed
		assert.NoDirExists(t, installDir)
		assert.NoFileExists(t, wrapperPath)
		assert.NoFileExists(t, desktopPath)
		assert.NoFileExists(t, iconPath)
	})

	t.Run("handles missing files gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeRpm,
			InstallPath: "/nonexistent/path",
			DesktopFile: "/nonexistent/desktop.desktop",
			Metadata: core.Metadata{
				WrapperScript: "/nonexistent/wrapper",
				IconFiles:     []string{"/nonexistent/icon.png"},
				InstallMethod: core.InstallMethodLocal,
			},
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
	})
}

func TestUninstall_PacmanMethod(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(_ string) bool { return false },
	}

	cfg := &config.Config{}

	t.Run("package not in pacman", func(t *testing.T) {
		mockProvider := &mockSyspkgProvider{
			isInstalled: false,
		}

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)
		backend.sys = mockProvider

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-package",
			PackageType: core.PackageTypeRpm,
			Metadata: core.Metadata{
				InstallMethod: core.InstallMethodPacman,
			},
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
	})

	t.Run("package uninstalled successfully", func(t *testing.T) {
		mockProvider := &mockSyspkgProvider{
			isInstalled: true,
			removeErr:   nil,
		}

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)
		backend.sys = mockProvider

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-package",
			PackageType: core.PackageTypeRpm,
			Metadata: core.Metadata{
				InstallMethod: core.InstallMethodPacman,
			},
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
		assert.True(t, mockProvider.removeCalled)
	})
}

func TestQueryRpmName(t *testing.T) {
	logger := zerolog.New(io.Discard)

	t.Run("returns error when rpm not found", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(_ string) bool {
				return false
			},
		}

		cfg := &config.Config{}
		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		tmpDir := t.TempDir()
		fakeRpm := filepath.Join(tmpDir, "test.rpm")
		require.NoError(t, os.WriteFile(fakeRpm, []byte("fake"), 0644))

		name, err := backend.queryRpmName(context.Background(), fakeRpm)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), rpmName)
		assert.Empty(t, name)
	})

	t.Run("returns package name successfully", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(name string) bool {
				return name == rpmName
			},
			RunCommandFunc: func(_ context.Context, name string, _ ...string) (string, error) {
				if name == rpmName {
					return "my-package", nil
				}
				return "", nil
			},
		}

		cfg := &config.Config{}
		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		tmpDir := t.TempDir()
		fakeRpm := filepath.Join(tmpDir, "test.rpm")
		require.NoError(t, os.WriteFile(fakeRpm, []byte("fake"), 0644))

		name, err := backend.queryRpmName(context.Background(), fakeRpm)
		assert.NoError(t, err)
		assert.Equal(t, "my-package", name)
	})
}

func TestCopyDir(t *testing.T) {
	logger := zerolog.New(io.Discard)

	t.Run("copies directory with files", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := t.TempDir()
		dstPath := filepath.Join(dstDir, "copied")

		// Create source structure
		require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644))

		backend := New(&config.Config{}, &logger)
		err := backend.copyDir(srcDir, dstPath)
		require.NoError(t, err)

		// Verify copied files
		content1, err := os.ReadFile(filepath.Join(dstPath, "file1.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content1", string(content1))

		content2, err := os.ReadFile(filepath.Join(dstPath, "subdir", "file2.txt"))
		require.NoError(t, err)
		assert.Equal(t, "content2", string(content2))
	})

	t.Run("handles symlinks safely", func(t *testing.T) {
		srcDir := t.TempDir()
		dstDir := t.TempDir()
		dstPath := filepath.Join(dstDir, "copied")

		// Create source with symlink
		require.NoError(t, os.WriteFile(filepath.Join(srcDir, "target.txt"), []byte("target content"), 0644))
		require.NoError(t, os.Symlink("target.txt", filepath.Join(srcDir, "link.txt")))

		backend := New(&config.Config{}, &logger)
		err := backend.copyDir(srcDir, dstPath)
		require.NoError(t, err)

		// Verify symlink is copied
		linkDst, err := os.Readlink(filepath.Join(dstPath, "link.txt"))
		require.NoError(t, err)
		assert.Equal(t, "target.txt", linkDst)
	})
}

func TestInstallRecord(t *testing.T) {
	// This test verifies the InstallRecord structure is properly created
	// without actually running a full install (which would need system commands)

	record := &core.InstallRecord{
		InstallID:    "test-install-id",
		PackageType:  core.PackageTypeRpm,
		Name:         "test-package",
		InstallPath:  "/home/user/.local/share/upkg/apps/test-package",
		DesktopFile:  "/home/user/.local/share/applications/test-package.desktop",
		OriginalFile: "/path/to/package.rpm",
		Metadata: core.Metadata{
			WrapperScript:  "/home/user/.local/bin/test-package",
			IconFiles:      []string{"/home/user/.local/share/icons/hicolor/256x256/apps/test-package.png"},
			WaylandSupport: string(core.WaylandUnknown),
			InstallMethod:  core.InstallMethodLocal,
		},
	}

	assert.Equal(t, "test-install-id", record.InstallID)
	assert.Equal(t, core.PackageTypeRpm, record.PackageType)
	assert.Equal(t, "test-package", record.Name)
	assert.Equal(t, "/home/user/.local/share/upkg/apps/test-package", record.InstallPath)
	assert.Equal(t, "/home/user/.local/share/applications/test-package.desktop", record.DesktopFile)
	assert.Equal(t, "/path/to/package.rpm", record.OriginalFile)
	assert.Equal(t, core.InstallMethodLocal, record.Metadata.InstallMethod)
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

func (m *mockSyspkgProvider) Install(_ context.Context, _ string) error {
	return nil
}

func (m *mockSyspkgProvider) Remove(_ context.Context, _ string) error {
	m.removeCalled = true
	return m.removeErr
}

func (m *mockSyspkgProvider) IsInstalled(_ context.Context, _ string) (bool, error) {
	return m.isInstalled, nil
}

func (m *mockSyspkgProvider) GetInfo(_ context.Context, packageName string) (*syspkg.PackageInfo, error) {
	return &syspkg.PackageInfo{Name: packageName, Version: "1.0.0"}, nil
}

func (m *mockSyspkgProvider) ListFiles(_ context.Context, _ string) ([]string, error) {
	return []string{}, nil
}
