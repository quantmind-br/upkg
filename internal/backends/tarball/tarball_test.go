package tarball

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
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTarballBackend(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	if backend == nil {
		t.Fatal("expected backend to be created")
	}

	if backend.Name() != "tarball" {
		t.Errorf("expected name 'tarball', got %s", backend.Name())
	}
}

func TestNewWithRunner(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockRunner := &helpers.MockCommandRunner{}

	backend := NewWithRunner(cfg, &logger, mockRunner)

	assert.NotNil(t, backend)
	assert.Equal(t, "tarball", backend.Name())
	assert.Equal(t, mockRunner, backend.Runner)
}

func TestNewWithCacheManager(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockCacheMgr := cache.NewCacheManager()

	backend := NewWithCacheManager(cfg, &logger, mockCacheMgr)

	assert.NotNil(t, backend)
	assert.Equal(t, "tarball", backend.Name())
	assert.Equal(t, mockCacheMgr, backend.cacheManager)
}

func TestDetect(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		filename    string
		content     []byte
		expected    bool
		expectError bool
	}{
		{
			name:     "tar.gz file",
			filename: "test.tar.gz",
			content:  []byte{0x1F, 0x8B, 0x08, 0x00}, // gzip magic
			expected: true,
		},
		{
			name:     "tar.xz file",
			filename: "test.tar.xz",
			content:  []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}, // xz magic
			expected: true,
		},
		{
			name:     "tar.bz2 file",
			filename: "test.tar.bz2",
			content:  []byte{'B', 'Z', 'h', '9'}, // bzip2 magic
			expected: true,
		},
		{
			name:     "zip file",
			filename: "test.zip",
			content:  []byte{'P', 'K', 0x03, 0x04}, // zip magic
			expected: true,
		},
		{
			name:     "plain tar file",
			filename: "test.tar",
			content:  createMinimalTar(),
			expected: true,
		},
		{
			name:     "text file",
			filename: "test.txt",
			content:  []byte("plain text file"),
			expected: false,
		},
		{
			name:        "non-existent file",
			filename:    "nonexistent.tar.gz",
			content:     nil, // Will not create file
			expected:    false,
			expectError: false,
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

func TestExtractArchive(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("unsupported archive type", func(t *testing.T) {
		tmpDir := t.TempDir()
		err := backend.extractArchive("/some/path", tmpDir, "unsupported")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported archive type")
	})
}

func TestCreateWrapper(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("standard wrapper", func(t *testing.T) {
		tmpDir := t.TempDir()
		wrapperPath := filepath.Join(tmpDir, "test-wrapper")
		execPath := "/path/to/executable"

		err := backend.createWrapper(wrapperPath, execPath)
		require.NoError(t, err)

		// Verify wrapper was created
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

	t.Run("electron app wrapper", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create fake Electron structure with .asar file
		resourcesDir := filepath.Join(tmpDir, "resources")
		require.NoError(t, os.MkdirAll(resourcesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "app.asar"), []byte("fake asar"), 0644))

		execPath := filepath.Join(tmpDir, "electron-app")
		require.NoError(t, os.WriteFile(execPath, []byte("fake exec"), 0755))

		wrapperPath := filepath.Join(tmpDir, "wrapper")
		err := backend.createWrapper(wrapperPath, execPath)
		require.NoError(t, err)

		content, err := os.ReadFile(wrapperPath)
		require.NoError(t, err)

		assert.Contains(t, string(content), "Electron app")
		assert.Contains(t, string(content), "cd")
	})

	t.Run("electron app with sandbox disabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create fake Electron structure
		resourcesDir := filepath.Join(tmpDir, "resources")
		require.NoError(t, os.MkdirAll(resourcesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "app.asar"), []byte("fake asar"), 0644))

		execPath := filepath.Join(tmpDir, "electron-app")
		require.NoError(t, os.WriteFile(execPath, []byte("fake exec"), 0755))

		// Enable sandbox disable in config
		cfgWithSandbox := &config.Config{
			Desktop: config.DesktopConfig{
				ElectronDisableSandbox: true,
			},
		}
		backendWithSandbox := New(cfgWithSandbox, &logger)

		wrapperPath := filepath.Join(tmpDir, "wrapper")
		err := backendWithSandbox.createWrapper(wrapperPath, execPath)
		require.NoError(t, err)

		content, err := os.ReadFile(wrapperPath)
		require.NoError(t, err)

		assert.Contains(t, string(content), "--no-sandbox")
	})
}

func TestIsElectronApp(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("standard binary is not electron", func(t *testing.T) {
		tmpDir := t.TempDir()
		execPath := filepath.Join(tmpDir, "standard-binary")
		require.NoError(t, os.WriteFile(execPath, []byte("fake exec"), 0755))

		isElectron := backend.isElectronApp(execPath)
		assert.False(t, isElectron)
	})

	t.Run("binary with resources/app.asar is electron", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create Electron structure
		resourcesDir := filepath.Join(tmpDir, "resources")
		require.NoError(t, os.MkdirAll(resourcesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "app.asar"), []byte("fake asar"), 0644))

		execPath := filepath.Join(tmpDir, "electron-app")
		require.NoError(t, os.WriteFile(execPath, []byte("fake exec"), 0755))

		isElectron := backend.isElectronApp(execPath)
		assert.True(t, isElectron)
	})

	t.Run("binary with .asar in parent dir is electron", func(t *testing.T) {
		tmpDir := t.TempDir()
		binDir := filepath.Join(tmpDir, "bin")
		require.NoError(t, os.MkdirAll(binDir, 0755))

		// Create .asar file in parent directory
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "app.asar"), []byte("fake asar"), 0644))

		execPath := filepath.Join(binDir, "electron-app")
		require.NoError(t, os.WriteFile(execPath, []byte("fake exec"), 0755))

		isElectron := backend.isElectronApp(execPath)
		assert.True(t, isElectron)
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

	t.Run("handles empty list", func(_ *testing.T) {
		// Should not panic
		backend.removeIcons([]string{})
	})
}

func TestInstall_PackageNotFound(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(context.Background(), "/nonexistent/package.tar.gz", core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
	assert.Nil(t, record)
}

func TestInstall_UnsupportedArchiveType(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	tmpDir := t.TempDir()
	fakePkg := filepath.Join(tmpDir, "test.unknown")
	require.NoError(t, os.WriteFile(fakePkg, []byte("fake content"), 0644))

	record, err := backend.Install(context.Background(), fakePkg, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported archive type")
	assert.Nil(t, record)
}

func TestInstall_InvalidPackageName(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	tmpDir := t.TempDir()
	// Create a tar.gz with an invalid custom name
	fakePkg := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(fakePkg, []byte{0x1F, 0x8B, 0x08, 0x00}, 0644))

	// Try to install with an empty custom name after normalization
	// Using a name that normalizes to empty string (all invalid chars)
	record, err := backend.Install(context.Background(), fakePkg, core.InstallOptions{
		CustomName: "///",
	}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
	assert.Nil(t, record)
}

func TestUninstall(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	// Create mock cache manager to avoid real cache updates
	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(_ string) bool { return false },
	}
	backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

	t.Run("uninstalls all files", func(t *testing.T) {
		tmpDir := t.TempDir()

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
			PackageType: core.PackageTypeTarball,
			InstallPath: installDir,
			DesktopFile: desktopPath,
			Metadata: core.Metadata{
				WrapperScript: wrapperPath,
				IconFiles:     []string{iconPath},
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
		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeTarball,
			InstallPath: "/nonexistent/path",
			DesktopFile: "/nonexistent/desktop.desktop",
			Metadata: core.Metadata{
				WrapperScript: "/nonexistent/wrapper",
				IconFiles:     []string{"/nonexistent/icon.png"},
			},
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err) // Should not error on missing files
	})

	t.Run("handles empty record", func(t *testing.T) {
		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeTarball,
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
	})
}

func TestCopyFile(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("copies file successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "source.txt")
		dstPath := filepath.Join(tmpDir, "dest.txt")

		content := []byte("test content for copy")
		require.NoError(t, os.WriteFile(srcPath, content, 0644))

		err := backend.copyFile(srcPath, dstPath)
		require.NoError(t, err)

		// Verify content was copied
		copied, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, content, copied)
	})

	t.Run("returns error for non-existent source", func(t *testing.T) {
		tmpDir := t.TempDir()
		dstPath := filepath.Join(tmpDir, "dest.txt")

		err := backend.copyFile("/nonexistent/source.txt", dstPath)
		assert.Error(t, err)
	})

	t.Run("returns error for invalid destination", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "source.txt")
		require.NoError(t, os.WriteFile(srcPath, []byte("test"), 0644))

		err := backend.copyFile(srcPath, "/nonexistent/dir/dest.txt")
		assert.Error(t, err)
	})
}

func TestCreateDesktopFile(t *testing.T) {
	logger := zerolog.New(io.Discard)

	// Create mock runner to avoid real command execution
	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool {
			return name == "desktop-file-validate"
		},
		RunCommandFunc: func(_ context.Context, _ string, _ ...string) (string, error) {
			return "", nil
		},
	}

	t.Run("creates desktop file without existing template", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := tmpDir

		// Temporarily override UserHomeDir
		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", homeDir)
		defer os.Setenv("HOME", origHomeDir)

		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: false,
			},
		}

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		desktopPath, err := backend.createDesktopFile(installDir, "Test App", "test-app", "/usr/bin/test-app", core.InstallOptions{})
		require.NoError(t, err)
		assert.NotEmpty(t, desktopPath)

		// Verify desktop file was created
		content, err := os.ReadFile(desktopPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "[Desktop Entry]")
		assert.Contains(t, contentStr, "Name=Test App")
		assert.Contains(t, contentStr, "Exec=/usr/bin/test-app %U")
		assert.Contains(t, contentStr, "Icon=test-app")
	})

	t.Run("uses existing desktop template from archive", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := tmpDir

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", homeDir)
		defer os.Setenv("HOME", origHomeDir)

		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: false,
			},
		}

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create existing desktop file in archive
		existingDesktop := `[Desktop Entry]
Type=Application
Name=My Custom App
Comment=A custom application
Exec=/old/path
Icon=custom-icon
Categories=Development;IDE;
`
		require.NoError(t, os.WriteFile(filepath.Join(installDir, "app.desktop"), []byte(existingDesktop), 0644))

		desktopPath, err := backend.createDesktopFile(installDir, "Test App", "test-app", "/usr/bin/test-app", core.InstallOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(desktopPath)
		require.NoError(t, err)

		contentStr := string(content)
		// Should use template values but override Exec and Icon
		assert.Contains(t, contentStr, "Comment=A custom application")
		assert.Contains(t, contentStr, "Exec=/usr/bin/test-app %U")
		assert.Contains(t, contentStr, "Icon=test-app") // Icon gets overridden
	})

	t.Run("injects wayland environment variables when enabled", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := tmpDir

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", homeDir)
		defer os.Setenv("HOME", origHomeDir)

		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: true,
			},
		}

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		desktopPath, err := backend.createDesktopFile(installDir, "Test App", "test-app", "/usr/bin/test-app", core.InstallOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(desktopPath)
		require.NoError(t, err)

		// Should contain Wayland environment variables in Exec
		contentStr := string(content)
		assert.Contains(t, contentStr, "env")
	})

	t.Run("skips wayland injection when option is set", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := tmpDir

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", homeDir)
		defer os.Setenv("HOME", origHomeDir)

		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: true,
			},
		}

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		desktopPath, err := backend.createDesktopFile(installDir, "Test App", "test-app", "/usr/bin/test-app", core.InstallOptions{
			SkipWaylandEnv: true,
		})
		require.NoError(t, err)

		content, err := os.ReadFile(desktopPath)
		require.NoError(t, err)

		// Should NOT contain env prefix since SkipWaylandEnv is true
		contentStr := string(content)
		assert.Contains(t, contentStr, "Exec=/usr/bin/test-app")
	})
}

func TestTransactionRollback(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	tmpDir := t.TempDir()

	// Create a valid tar.gz for testing
	// Note: This will fail during extraction since it's not a real tar.gz,
	// but we're testing the transaction rollback behavior
	fakePkg := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(fakePkg, []byte{0x1F, 0x8B, 0x08, 0x00}, 0644))

	// Override HOME to use tmpDir
	origHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHomeDir)

	_, err := backend.Install(context.Background(), fakePkg, core.InstallOptions{}, tx)
	assert.Error(t, err) // Should fail during extraction

	// Transaction should be rolled back
	tx.Rollback()

	// Verify install directory was cleaned up
	installDir := filepath.Join(tmpDir, ".local", "share", "upkg", "apps")
	entries, _ := os.ReadDir(installDir)
	assert.Empty(t, entries, "install directory should be empty after rollback")
}

func TestBaseBackend_Embedded(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	// Check that BaseBackend is properly embedded
	assert.NotNil(t, backend.BaseBackend)
	assert.Equal(t, cfg, backend.Cfg)
	assert.Equal(t, &logger, backend.Log)
	assert.NotNil(t, backend.Fs)
	assert.NotNil(t, backend.Runner)
	assert.NotNil(t, backend.Paths)
}

func TestBaseBackend_NewWithDeps(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewMemMapFs()
	runner := &helpers.MockCommandRunner{}

	backend := NewWithDeps(cfg, &logger, fs, runner)

	// Check that BaseBackend is properly initialized with dependencies
	assert.NotNil(t, backend.BaseBackend)
	assert.Equal(t, cfg, backend.Cfg)
	assert.Equal(t, &logger, backend.Log)
	assert.Equal(t, fs, backend.Fs)
	assert.Equal(t, runner, backend.Runner)
	assert.NotNil(t, backend.Paths)
}

func TestBaseBackend_NewWithNilConfig(t *testing.T) {
	logger := zerolog.New(io.Discard)
	backend := New(nil, &logger)

	// Check that BaseBackend handles nil config
	assert.NotNil(t, backend.BaseBackend)
	assert.Nil(t, backend.Cfg)
	assert.Equal(t, &logger, backend.Log)
	assert.NotNil(t, backend.Fs)
	assert.NotNil(t, backend.Runner)
	assert.NotNil(t, backend.Paths)
}

func TestBaseBackend_NewWithNilLogger(t *testing.T) {
	cfg := &config.Config{}
	backend := New(cfg, nil)

	// Check that BaseBackend handles nil logger
	assert.NotNil(t, backend.BaseBackend)
	assert.Equal(t, cfg, backend.Cfg)
	assert.Nil(t, backend.Log)
	assert.NotNil(t, backend.Fs)
	assert.NotNil(t, backend.Runner)
	assert.NotNil(t, backend.Paths)
}

// createMinimalTar creates a minimal valid tar archive header for testing
func createMinimalTar() []byte {
	// Minimal tar header: 512 bytes with "ustar" magic at offset 257
	header := make([]byte, 512)
	copy(header[257:262], []byte("ustar"))
	return header
}

func TestInstallIcons(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("installs icons successfully", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create icon files
		iconFile := filepath.Join(installDir, "test-icon.png")
		require.NoError(t, os.WriteFile(iconFile, []byte("fake icon"), 0644))

		// Mock home directory
		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		installedIcons, err := backend.installIcons(installDir, "test-app")
		assert.NoError(t, err)
		assert.NotNil(t, installedIcons)
	})

	t.Run("handles missing home directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Mock missing home directory
		origHomeDir := os.Getenv("HOME")
		os.Unsetenv("HOME")
		defer os.Setenv("HOME", origHomeDir)

		installedIcons, err := backend.installIcons(installDir, "test-app")
		assert.Error(t, err)
		assert.Empty(t, installedIcons)
	})

	t.Run("handles icon installation failures gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create icon file
		iconFile := filepath.Join(installDir, "test-icon.png")
		require.NoError(t, os.WriteFile(iconFile, []byte("fake icon"), 0644))

		// Mock home directory
		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		// Test should complete without panic even if icon installation fails
		installedIcons, err := backend.installIcons(installDir, "test-app")
		assert.NoError(t, err)
		assert.NotNil(t, installedIcons)
	})
}

func TestExtractIconsFromAsarNative(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("extracts icons from asar archive", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create fake asar file
		asarFile := filepath.Join(installDir, "app.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte("fake asar content"), 0644))

		// Mock home directory
		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		icons, err := backend.extractIconsFromAsarNative(asarFile, installDir, "test-app")
		// Should not panic even if asar extraction fails
		assert.NoError(t, err)
		assert.NotNil(t, icons)
	})

	t.Run("handles missing asar file", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Mock home directory
		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		icons, err := backend.extractIconsFromAsarNative("/nonexistent/app.asar", installDir, "test-app")
		assert.NoError(t, err)
		assert.NotNil(t, icons)
	})

	t.Run("handles missing home directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create fake asar file
		asarFile := filepath.Join(installDir, "app.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte("fake asar content"), 0644))

		// Mock missing home directory
		origHomeDir := os.Getenv("HOME")
		os.Unsetenv("HOME")
		defer os.Setenv("HOME", origHomeDir)

		icons, err := backend.extractIconsFromAsarNative(asarFile, installDir, "test-app")
		assert.NoError(t, err)
		assert.NotNil(t, icons)
	})
}

func TestExtractIconsFromAsar(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("extracts icons from asar directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create icon files in install directory
		iconFile := filepath.Join(installDir, "test-icon.png")
		require.NoError(t, os.WriteFile(iconFile, []byte("fake icon"), 0644))

		// Mock home directory
		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		icons, err := backend.extractIconsFromAsar(installDir, "test-app")
		assert.NoError(t, err)
		assert.NotNil(t, icons)
	})

	t.Run("handles missing icon files", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Mock home directory
		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		icons, err := backend.extractIconsFromAsar(installDir, "test-app")
		assert.NoError(t, err)
		assert.NotNil(t, icons)
	})

	t.Run("handles missing home directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create icon files in install directory
		iconFile := filepath.Join(installDir, "test-icon.png")
		require.NoError(t, os.WriteFile(iconFile, []byte("fake icon"), 0644))

		// Mock missing home directory
		origHomeDir := os.Getenv("HOME")
		os.Unsetenv("HOME")
		defer os.Setenv("HOME", origHomeDir)

		icons, err := backend.extractIconsFromAsar(installDir, "test-app")
		assert.NoError(t, err)
		assert.NotNil(t, icons)
	})
}
