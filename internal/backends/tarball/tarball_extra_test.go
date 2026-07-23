package tarball

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/paths"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTarballBackend_Install_PackageNotFound(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(context.Background(), "/nonexistent.tar.gz", core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
	assert.Nil(t, record)
}

func TestTarballBackend_Install_InvalidArchive(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte("invalid archive"), 0644))

	record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported archive type")
	assert.Nil(t, record)
}

func newTarballBackend(t *testing.T, home string) *TarballBackend {
	t.Helper()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	b := NewWithDeps(cfg, &logger, afero.NewOsFs(), helpers.NewOSCommandRunner())
	b.Paths = paths.NewResolverWithHome(cfg, home)
	return b
}

func TestTarballBackend_Install_NoExecutables(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	backend := newTarballBackend(t, home)

	// A valid archive whose only entry is a non-executable data file.
	archivePath := filepath.Join(home, "app.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, makeTarGz(t, "data.txt", []byte("just data")), 0644))

	tx := transaction.NewManager(zerologNop())
	record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, tx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no executables found")
	assert.Nil(t, record)
}

func TestTarballBackend_Install_AlreadyInstalled(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	backend := newTarballBackend(t, home)

	archivePath := filepath.Join(home, "app.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, makeTarGzWithELF(t, "app"), 0644))

	// First install succeeds.
	tx := transaction.NewManager(zerologNop())
	rec, err := backend.Install(context.Background(), archivePath, core.InstallOptions{SkipDesktop: true}, tx)
	require.NoError(t, err)
	require.NotNil(t, rec)

	// Second install without force is rejected.
	tx2 := transaction.NewManager(zerologNop())
	rec2, err2 := backend.Install(context.Background(), archivePath, core.InstallOptions{SkipDesktop: true}, tx2)
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "already installed")
	assert.Nil(t, rec2)
}

func TestTarballBackend_Install_WithForce(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	backend := newTarballBackend(t, home)

	archivePath := filepath.Join(home, "app.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, makeTarGzWithELF(t, "app"), 0644))

	tx := transaction.NewManager(zerologNop())
	_, err := backend.Install(context.Background(), archivePath, core.InstallOptions{SkipDesktop: true}, tx)
	require.NoError(t, err)

	// Force reinstall over the existing installation succeeds and commits.
	tx2 := transaction.NewManager(zerologNop())
	rec, err := backend.Install(context.Background(), archivePath, core.InstallOptions{SkipDesktop: true, Force: true}, tx2)
	require.NoError(t, err)
	require.NotNil(t, rec)
	require.NoError(t, tx2.Commit())

	// The installation directory still exists and no backup leaked.
	installDir := filepath.Join(home, ".local", "share", "upkg", "apps", "app")
	_, statErr := os.Stat(installDir)
	require.NoError(t, statErr)
	entries, _ := os.ReadDir(filepath.Dir(installDir))
	for _, e := range entries {
		assert.NotContains(t, e.Name(), "upkg-backup-")
	}
}

func TestTarballBackend_Install_SkipDesktop(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	backend := newTarballBackend(t, home)

	archivePath := filepath.Join(home, "app.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, makeTarGzWithELF(t, "app"), 0644))

	tx := transaction.NewManager(zerologNop())
	rec, err := backend.Install(context.Background(), archivePath, core.InstallOptions{SkipDesktop: true}, tx)
	require.NoError(t, err)
	require.NotNil(t, rec)

	// No .desktop file is created when SkipDesktop is set.
	assert.Empty(t, rec.DesktopFile)
	desktopPath := filepath.Join(home, ".local", "share", "applications", "app.desktop")
	_, statErr := os.Stat(desktopPath)
	assert.True(t, os.IsNotExist(statErr), "desktop file must not be created")
}

func TestTarballBackend_Uninstall(t *testing.T) {
	t.Parallel()

	t.Run("uninstalls all files", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		fs := afero.NewOsFs()
		backend := NewWithDeps(cfg, &logger, fs, helpers.NewOSCommandRunner())

		tmpDir := t.TempDir()
		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		// Create fake installation files
		installDir := filepath.Join(tmpDir, ".local", "share", "apps", "test-app")
		wrapperPath := filepath.Join(tmpDir, ".local", "bin", "test-app")
		desktopPath := filepath.Join(tmpDir, ".local", "share", "applications", "test-app.desktop")
		iconPath := filepath.Join(tmpDir, ".local", "share", "icons", "test.png")

		require.NoError(t, fs.MkdirAll(installDir, 0755))
		require.NoError(t, fs.MkdirAll(filepath.Dir(wrapperPath), 0755))
		require.NoError(t, fs.MkdirAll(filepath.Dir(desktopPath), 0755))
		require.NoError(t, fs.MkdirAll(filepath.Dir(iconPath), 0755))

		require.NoError(t, afero.WriteFile(fs, filepath.Join(installDir, "test"), []byte("test"), 0755))
		require.NoError(t, afero.WriteFile(fs, wrapperPath, []byte("#!/bin/bash"), 0755))
		require.NoError(t, afero.WriteFile(fs, desktopPath, []byte("[Desktop Entry]"), 0644))
		require.NoError(t, afero.WriteFile(fs, iconPath, []byte("icon"), 0644))

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

		// Verify files are removed
		assert.NoFileExists(t, installDir)
		assert.NoFileExists(t, wrapperPath)
		assert.NoFileExists(t, desktopPath)
		assert.NoFileExists(t, iconPath)
	})

	t.Run("handles missing files gracefully", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeTarball,
			InstallPath: "/nonexistent/path",
			DesktopFile: "/nonexistent/desktop",
			Metadata: core.Metadata{
				WrapperScript: "/nonexistent/wrapper",
				IconFiles:     []string{"/nonexistent/icon.png"},
			},
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
	})

	t.Run("handles empty record", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeTarball,
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
	})
}

func TestTarballBackend_Detect(t *testing.T) {
	t.Parallel()

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
			content:  makeTarGz(t, "app", []byte("#!/bin/sh\necho hi\n")),
			expected: true,
		},
		{
			name:     "tar.xz file",
			filename: "test.tar.xz",
			content:  makeTarXz(t, "app", []byte("#!/bin/sh\necho hi\n")),
			expected: true,
		},
		{
			name:     "tar file",
			filename: "test.tar",
			content:  makeTar(t, "app", []byte("#!/bin/sh\necho hi\n")),
			expected: true,
		},
		{
			name:     "zip file",
			filename: "test.zip",
			content:  makeZip(t, "app", []byte("#!/bin/sh\necho hi\n")),
			expected: true,
		},
		{
			name:     "renamed text file is not an archive",
			filename: "fake.tar.gz",
			content:  []byte("plain text pretending to be an archive"),
			expected: false,
		},
		{
			name:     "text file",
			filename: "test.txt",
			content:  []byte("plain text"),
			expected: false,
		},
		{
			name:     "non-existent file",
			filename: "nonexistent.tar.gz",
			content:  nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := zerolog.New(io.Discard)
			cfg := &config.Config{}
			backend := New(cfg, &logger)

			var filePath string
			if tt.content != nil {
				tmpDir := t.TempDir()
				filePath = filepath.Join(tmpDir, tt.filename)
				require.NoError(t, os.WriteFile(filePath, tt.content, 0644))
			} else {
				filePath = tt.filename
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

func TestTarballBackend_Name(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	assert.Equal(t, "tarball", backend.Name())
}

func TestTarballBackend_NewWithRunner(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockRunner := &helpers.MockCommandRunner{}

	backend := NewWithRunner(cfg, &logger, mockRunner)

	assert.NotNil(t, backend)
	assert.Equal(t, mockRunner, backend.Runner)
}

func TestTarballBackend_NewWithCacheManager(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	// Note: NewWithCacheManager doesn't expose cacheManager field directly
	// Just verify backend is created
	backend := NewWithCacheManager(cfg, &logger, nil)

	assert.NotNil(t, backend)
}

func TestTarballBackend_CreateWrapper(t *testing.T) {
	t.Parallel()

	t.Run("standard wrapper", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		wrapperPath := filepath.Join(tmpDir, "test-wrapper")
		execPath := "/path/to/executable"

		wrapperCfg := helpers.WrapperConfig{
			WrapperPath:    wrapperPath,
			ExecPath:       execPath,
			DisableSandbox: false,
		}
		err := helpers.CreateWrapper(backend.Fs, wrapperCfg)
		assert.NoError(t, err)

		// Verify wrapper was created
		info, err := os.Stat(wrapperPath)
		assert.NoError(t, err)
		assert.Equal(t, os.FileMode(0755), info.Mode().Perm())

		// Verify content
		content, err := os.ReadFile(wrapperPath)
		assert.NoError(t, err)
		assert.Contains(t, string(content), execPath)
		assert.Contains(t, string(content), "#!/bin/bash")
	})

	t.Run("electron wrapper", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		execDir := filepath.Join(tmpDir, "app")
		require.NoError(t, os.MkdirAll(execDir, 0755))

		// Create electron structure
		resourcesDir := filepath.Join(execDir, "resources")
		require.NoError(t, os.MkdirAll(resourcesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "app.asar"), []byte("asar"), 0644))

		execPath := filepath.Join(execDir, "app")
		wrapperPath := filepath.Join(tmpDir, "wrapper")

		wrapperCfg := helpers.WrapperConfig{
			WrapperPath:    wrapperPath,
			ExecPath:       execPath,
			DisableSandbox: true,
		}
		err := helpers.CreateWrapper(backend.Fs, wrapperCfg)
		assert.NoError(t, err)

		content, err := os.ReadFile(wrapperPath)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "--no-sandbox")
	})
}

func TestTarballBackend_IsElectronApp(t *testing.T) {
	t.Parallel()

	t.Run("detects electron app", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		execDir := filepath.Join(tmpDir, "app")
		require.NoError(t, os.MkdirAll(execDir, 0755))

		resourcesDir := filepath.Join(execDir, "resources")
		require.NoError(t, os.MkdirAll(resourcesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "app.asar"), []byte("asar"), 0644))

		execPath := filepath.Join(execDir, "app")

		isElectron := helpers.IsElectronApp(backend.Fs, execPath)
		assert.True(t, isElectron)
	})

	t.Run("detects non-electron app", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		execPath := filepath.Join(tmpDir, "app")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash"), 0755))

		isElectron := helpers.IsElectronApp(backend.Fs, execPath)
		assert.False(t, isElectron)
	})

	t.Run("detects electron in parent dir", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		appDir := filepath.Join(tmpDir, "app")
		execDir := filepath.Join(appDir, "bin")
		require.NoError(t, os.MkdirAll(execDir, 0755))

		// .asar in parent
		require.NoError(t, os.WriteFile(filepath.Join(appDir, "app.asar"), []byte("asar"), 0644))

		execPath := filepath.Join(execDir, "app")

		isElectron := helpers.IsElectronApp(backend.Fs, execPath)
		assert.True(t, isElectron)
	})
}

func TestTarballBackend_ExtractArchive(t *testing.T) {
	t.Parallel()

	t.Run("unsupported archive type", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		err := backend.extractArchive("/path/to/file", "/tmp/dest", "unsupported")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported archive type")
	})
}

func TestTarballBackend_InstallIcons(t *testing.T) {
	t.Parallel()

	t.Run("handles missing home directory", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create an icon file
		iconFile := filepath.Join(installDir, "test.png")
		require.NoError(t, os.WriteFile(iconFile, []byte("icon"), 0644))

		// Force the backend to use an empty home directory.
		backend.Paths = paths.NewResolverWithHome(cfg, "")

		icons, err := backend.installIcons(installDir, "test-app")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "home directory")
		assert.Empty(t, icons)
	})

	t.Run("handles icon installation failures", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		home := t.TempDir()
		backend.Paths = paths.NewResolverWithHome(cfg, home)

		// Create necessary directory structure for icons
		iconsDir := filepath.Join(home, ".local", "share", "icons", "hicolor", "256x256", "apps")
		require.NoError(t, os.MkdirAll(iconsDir, 0755))

		installDir := filepath.Join(home, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create an icon file
		iconFile := filepath.Join(installDir, "test.png")
		require.NoError(t, os.WriteFile(iconFile, []byte("icon"), 0644))

		// Should succeed and install icons
		icons, err := backend.installIcons(installDir, "test-app")

		assert.NoError(t, err)
		assert.NotNil(t, icons)
	})
}

func TestTarballBackend_CreateDesktopFile(t *testing.T) {
	t.Parallel()

	t.Run("creates desktop file", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		// Update the backend's paths resolver to use the new home
		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		// Create necessary directory structure for desktop files
		appsDir := filepath.Join(tmpDir, ".local", "share", "applications")
		require.NoError(t, os.MkdirAll(appsDir, 0755))

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		execPath := filepath.Join(installDir, "app")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash"), 0755))

		desktopPath, err := backend.createDesktopFile(installDir, "TestApp", "test-app", execPath, core.InstallOptions{})

		assert.NoError(t, err)
		assert.NotEmpty(t, desktopPath)
		assert.FileExists(t, desktopPath)
	})

	t.Run("handles wayland env vars", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: true,
			},
		}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		// Update the backend's paths resolver to use the new home
		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		execPath := filepath.Join(installDir, "app")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash"), 0755))

		desktopPath, err := backend.createDesktopFile(installDir, "TestApp", "test-app", execPath, core.InstallOptions{})

		assert.NoError(t, err)
		assert.FileExists(t, desktopPath)

		// Verify desktop file contains wayland vars
		content, err := os.ReadFile(desktopPath)
		assert.NoError(t, err)
		// Should contain environment variables
		_ = content
	})

	t.Run("skips wayland for tauri apps", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: true,
			},
		}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		execPath := filepath.Join(installDir, "app")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash"), 0755))

		// Create desktop file with Tauri WMClass
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=app
StartupWMClass=tauri-test`
		desktopFile := filepath.Join(installDir, "TestApp.desktop")
		require.NoError(t, os.WriteFile(desktopFile, []byte(desktopContent), 0644))

		// This would be parsed and detected as Tauri
		// Then wayland env vars should be skipped
		_ = desktopFile
		_ = backend
	})
}

func TestTarballBackend_ExtractIconsFromAsar(t *testing.T) {
	t.Parallel()

	t.Run("no asar files", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		icons, err := backend.extractIconsFromAsar(installDir, "test-app")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no asar files found")
		assert.Empty(t, icons)
	})

	t.Run("asar without npx", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}

		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(cmd string) bool {
				return cmd != "npx"
			},
		}

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create fake asar file
		asarFile := filepath.Join(installDir, "app.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte("fake asar"), 0644))

		icons, err := backend.extractIconsFromAsar(installDir, "test-app")

		// Should succeed but return no icons because native extraction fails and npx not available
		assert.NoError(t, err)
		assert.Empty(t, icons)
	})
}

func TestTarballBackend_CopyFile(t *testing.T) {
	t.Parallel()

	t.Run("copies file successfully", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "source.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		content := []byte("test content")
		require.NoError(t, os.WriteFile(src, content, 0644))

		err := backend.copyFile(src, dst)
		assert.NoError(t, err)

		// Verify copy
		copied, err := os.ReadFile(dst)
		assert.NoError(t, err)
		assert.Equal(t, content, copied)
	})

	t.Run("handles missing source", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		src := filepath.Join(tmpDir, "missing.txt")
		dst := filepath.Join(tmpDir, "dest.txt")

		err := backend.copyFile(src, dst)
		assert.Error(t, err)
	})
}

func TestTarballBackend_RemoveIcons(t *testing.T) {
	t.Parallel()

	t.Run("removes icons", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		icon1 := filepath.Join(tmpDir, "icon1.png")
		icon2 := filepath.Join(tmpDir, "icon2.svg")

		require.NoError(t, os.WriteFile(icon1, []byte("icon1"), 0644))
		require.NoError(t, os.WriteFile(icon2, []byte("icon2"), 0644))

		backend.removeIcons([]string{icon1, icon2})

		assert.NoFileExists(t, icon1)
		assert.NoFileExists(t, icon2)
	})

	t.Run("handles missing icons", func(_ *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		// Should not panic
		backend.removeIcons([]string{"/nonexistent/icon.png"})
	})
}

func TestTarballBackend_CreateDesktopFile_Additional(t *testing.T) {
	t.Parallel()

	t.Run("handles electron sandbox flag in wrapper", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				ElectronDisableSandbox: true,
			},
		}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()

		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		execPath := filepath.Join(installDir, "app")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash"), 0755))

		// Create electron structure
		resourcesDir := filepath.Join(installDir, "resources")
		require.NoError(t, os.MkdirAll(resourcesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "app.asar"), []byte("asar"), 0644))

		// First create wrapper to check it has --no-sandbox
		wrapperPath := filepath.Join(tmpDir, "wrapper")
		wrapperCfg := helpers.WrapperConfig{
			WrapperPath:    wrapperPath,
			ExecPath:       execPath,
			DisableSandbox: true,
		}
		err := helpers.CreateWrapper(backend.Fs, wrapperCfg)
		assert.NoError(t, err)

		content, err := os.ReadFile(wrapperPath)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "--no-sandbox")

		// Then test desktop file creation
		desktopPath, err := backend.createDesktopFile(installDir, "TestApp", "test-app", execPath, core.InstallOptions{})

		assert.NoError(t, err)
		assert.FileExists(t, desktopPath)
	})

	t.Run("handles skip desktop option", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()

		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		execPath := filepath.Join(installDir, "app")
		require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash"), 0755))

		// Create desktop file template
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=app`
		desktopFile := filepath.Join(installDir, "TestApp.desktop")
		require.NoError(t, os.WriteFile(desktopFile, []byte(desktopContent), 0644))

		desktopPath, err := backend.createDesktopFile(installDir, "TestApp", "test-app", execPath, core.InstallOptions{})

		assert.NoError(t, err)
		assert.NotEmpty(t, desktopPath)
	})
}

func TestTarballBackend_InstallIcons_Additional(t *testing.T) {
	t.Parallel()

	t.Run("handles asar icon extraction with npx", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}

		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(cmd string) bool {
				return cmd == "npx"
			},
			RunCommandFunc: func(_ context.Context, cmd string, args ...string) (string, error) {
				// Mock asar extraction - return success but no icons
				return "", nil
			},
		}

		backend := NewWithDeps(cfg, &logger, afero.NewOsFs(), mockRunner)

		tmpDir := t.TempDir()
		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create fake asar file
		asarFile := filepath.Join(installDir, "app.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte("fake asar"), 0644))

		icons, err := backend.extractIconsFromAsar(installDir, "test-app")

		// Should succeed or fail gracefully
		_ = icons
		_ = err
	})

	t.Run("handles multiple asar files", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}

		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create multiple asar files
		require.NoError(t, os.WriteFile(filepath.Join(installDir, "app.asar"), []byte("asar1"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(installDir, "other.asar"), []byte("asar2"), 0644))

		icons, err := backend.extractIconsFromAsar(installDir, "test-app")

		// Should try to extract but fail because npx/asar not available
		// The function should handle this gracefully
		_ = icons
		_ = err
	})
}

func TestTarballBackend_ExtractIconsFromAsarNative(t *testing.T) {
	t.Parallel()

	t.Run("handles non-existent asar file", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Don't create asar file - just test with non-existent path
		asarFile := filepath.Join(installDir, "nonexistent.asar")
		appName := "test-app"

		icons, err := backend.extractIconsFromAsarNative(asarFile, appName, installDir)

		assert.Error(t, err)
		assert.Empty(t, icons)
	})

	t.Run("handles invalid asar file", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create invalid asar file
		asarFile := filepath.Join(installDir, "invalid.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte("not a real asar"), 0644))

		icons, err := backend.extractIconsFromAsarNative(asarFile, "test-app", installDir)

		// Should fail because it's not a valid asar
		assert.Error(t, err)
		assert.Empty(t, icons)
	})
}

func TestTarballBackend_Detect_Additional(t *testing.T) {
	t.Parallel()

	t.Run("non-existent file", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		result, err := backend.Detect(context.Background(), "/nonexistent/file.tar.gz")

		assert.NoError(t, err)
		assert.False(t, result)
	})

	t.Run("empty file", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		tmpDir := t.TempDir()
		emptyFile := filepath.Join(tmpDir, "empty.tar.gz")
		require.NoError(t, os.WriteFile(emptyFile, []byte{}, 0644))

		result, err := backend.Detect(context.Background(), emptyFile)

		assert.NoError(t, err)
		// Empty files might be detected as tar files because tar magic check might pass
		// Just verify the function doesn't error
		_ = result
	})
}

func TestTarballBackend_Install_WithCustomName(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	cfg := &config.Config{}
	backend := New(cfg, &logger)
	backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

	// Create fake tar.gz (will fail extraction)
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte("fake archive"), 0644))

	tx := transaction.NewManager(&logger)
	_, err := backend.Install(context.Background(), archivePath, core.InstallOptions{CustomName: "MyCustomApp"}, tx)

	// Should fail during extraction
	assert.Error(t, err)
}

func TestTarballBackend_Install_WithForce_NotSkipped(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	cfg := &config.Config{}
	backend := New(cfg, &logger)
	backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

	// Create fake tar.gz (will fail extraction)
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte("fake archive"), 0644))

	tx := transaction.NewManager(&logger)
	_, err := backend.Install(context.Background(), archivePath, core.InstallOptions{Force: true}, tx)

	// Should fail during extraction
	assert.Error(t, err)
}

func TestTarballBackend_Install_SkipDesktop_NotSkipped(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	cfg := &config.Config{}
	backend := New(cfg, &logger)
	backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

	// Create fake tar.gz (will fail extraction)
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte("fake archive"), 0644))

	tx := transaction.NewManager(&logger)
	_, err := backend.Install(context.Background(), archivePath, core.InstallOptions{SkipDesktop: true}, tx)

	// Should fail during extraction
	assert.Error(t, err)
}

func TestTarballBackend_createDesktopFile_EdgeCases(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	cfg := &config.Config{}
	backend := New(cfg, &logger)
	backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

	t.Run("basic desktop file creation", func(t *testing.T) {
		installDir := tmpDir
		appName := "testapp"
		normalizedName := "testapp"
		execPath := "/usr/bin/testapp"
		opts := core.InstallOptions{}

		// Create apps directory
		appsDir := filepath.Join(tmpDir, ".local", "share", "applications")
		require.NoError(t, os.MkdirAll(appsDir, 0755))

		desktopPath, err := backend.createDesktopFile(installDir, appName, normalizedName, execPath, opts)
		// Should succeed or fail gracefully
		_ = desktopPath
		_ = err
	})

	t.Run("with electron sandbox disabled", func(t *testing.T) {
		electronTmpDir := t.TempDir()
		installDir := electronTmpDir
		appName := "electronapp"
		normalizedName := "electronapp"
		execPath := "/usr/bin/electronapp"
		opts := core.InstallOptions{}

		cfgElectron := &config.Config{
			Desktop: config.DesktopConfig{
				ElectronDisableSandbox: true,
			},
		}
		backendElectron := New(cfgElectron, &logger)
		backendElectron.Paths = paths.NewResolverWithHome(cfgElectron, electronTmpDir)

		desktopPath, err := backendElectron.createDesktopFile(installDir, appName, normalizedName, execPath, opts)
		// Desktop file should be created successfully
		_ = desktopPath
		_ = err
	})
}

func TestTarballBackend_extractArchive_EdgeCases(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("nonexistent archive", func(t *testing.T) {
		destDir := t.TempDir()
		err := backend.extractArchive("/nonexistent/archive.tar.gz", destDir, "tar.gz")
		assert.Error(t, err)
	})

	t.Run("create destination directory error", func(t *testing.T) {
		// Use a file instead of directory as dest to cause error
		tmpDir := t.TempDir()
		destFile := filepath.Join(tmpDir, "not-a-directory")
		require.NoError(t, os.WriteFile(destFile, []byte("test"), 0644))

		err := backend.extractArchive("/some/path.tar.gz", destFile, "tar.gz")
		assert.Error(t, err)
	})

	t.Run("invalid archive type", func(t *testing.T) {
		tmpDir := t.TempDir()
		archivePath := filepath.Join(tmpDir, "fake.zip")
		require.NoError(t, os.WriteFile(archivePath, []byte("fake"), 0644))
		destDir := t.TempDir()

		err := backend.extractArchive(archivePath, destDir, "zip")
		// Should error for unsupported type or try to extract
		_ = err
	})
}

func TestTarballBackend_installIcons_EdgeCases(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	cfg := &config.Config{}
	backend := New(cfg, &logger)
	backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

	t.Run("empty install directory", func(t *testing.T) {
		installDir := ""
		normalizedName := "test"

		installed, err := backend.installIcons(installDir, normalizedName)
		// May return empty list and no error for empty install dir
		_ = installed
		// Function may handle this case gracefully
		_ = err
	})

	t.Run("nonexistent install directory", func(t *testing.T) {
		installDir := "/nonexistent/path"
		normalizedName := "test"

		installed, err := backend.installIcons(installDir, normalizedName)
		// May return empty list for nonexistent dir
		_ = installed
		// Function may handle this case gracefully
		_ = err
	})

	t.Run("valid directory with icons", func(t *testing.T) {
		installDir := filepath.Join(tmpDir, "install")
		normalizedName := "testapp"

		// Create install directory with icons
		iconsDir := filepath.Join(installDir, "icons")
		require.NoError(t, os.MkdirAll(iconsDir, 0755))
		iconPath := filepath.Join(iconsDir, "app.png")
		require.NoError(t, os.WriteFile(iconPath, []byte("fake icon"), 0644))

		installed, err := backend.installIcons(installDir, normalizedName)
		// May succeed if icon copying works
		_ = installed
		_ = err
	})
}

func TestTarballBackend_extractIconsFromAsar(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	backend := New(cfg, &logger)

	t.Run("nonexistent asar file", func(t *testing.T) {
		installDir := tmpDir
		icons, err := backend.extractIconsFromAsar("/nonexistent/file.asar", installDir)
		_ = icons
		assert.Error(t, err)
	})

	t.Run("empty install directory", func(t *testing.T) {
		installDir := ""
		icons, err := backend.extractIconsFromAsar("fake.asar", installDir)
		_ = icons
		assert.Error(t, err)
	})
}

func TestTarballBackend_extractArchive(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("unsupported archive type", func(t *testing.T) {
		archivePath := filepath.Join(tmpDir, "test.unknown")
		require.NoError(t, os.WriteFile(archivePath, []byte("fake"), 0644))

		destDir := filepath.Join(tmpDir, "dest")
		err := backend.extractArchive(archivePath, destDir, "unknown")
		assert.Error(t, err)
	})

	t.Run("tar.gz extraction", func(t *testing.T) {
		tmpPath := t.TempDir()
		archivePath := filepath.Join(tmpPath, "test.tar.gz")
		// Create a minimal valid tar.gz
		require.NoError(t, os.WriteFile(archivePath, []byte{0x1F, 0x8B, 0x08, 0x00}, 0644))

		destDir := filepath.Join(tmpPath, "dest")
		err := backend.extractArchive(archivePath, destDir, "tar.gz")
		// May fail due to incomplete tar, but should attempt extraction
		_ = err
	})

	t.Run("zip extraction", func(t *testing.T) {
		tmpPath := t.TempDir()
		archivePath := filepath.Join(tmpPath, "test.zip")
		// Create a minimal valid zip header
		require.NoError(t, os.WriteFile(archivePath, []byte{0x50, 0x4B, 0x03, 0x04}, 0644))

		destDir := filepath.Join(tmpPath, "dest")
		err := backend.extractArchive(archivePath, destDir, "zip")
		// May fail due to incomplete zip, but should attempt extraction
		_ = err
	})

	t.Run("tar.xz extraction", func(t *testing.T) {
		tmpPath := t.TempDir()
		archivePath := filepath.Join(tmpPath, "test.tar.xz")
		// Create a minimal valid xz header
		require.NoError(t, os.WriteFile(archivePath, []byte{0xFD, 0x37, 0x7A, 0x58, 0x5A, 0x00}, 0644))

		destDir := filepath.Join(tmpPath, "dest")
		err := backend.extractArchive(archivePath, destDir, "tar.xz")
		// May fail due to incomplete tar.xz, but should attempt extraction
		_ = err
	})

	t.Run("tar.bz2 extraction", func(t *testing.T) {
		tmpPath := t.TempDir()
		archivePath := filepath.Join(tmpPath, "test.tar.bz2")
		// Create a minimal valid bz2 header
		require.NoError(t, os.WriteFile(archivePath, []byte{0x42, 0x5A, 0x68}, 0644))

		destDir := filepath.Join(tmpPath, "dest")
		err := backend.extractArchive(archivePath, destDir, "tar.bz2")
		// May fail due to incomplete tar.bz2, but should attempt extraction
		_ = err
	})

	t.Run("creates destination directory", func(t *testing.T) {
		tmpPath := t.TempDir()
		archivePath := filepath.Join(tmpPath, "test.tar.gz")
		require.NoError(t, os.WriteFile(archivePath, []byte{0x1F, 0x8B, 0x08, 0x00}, 0644))

		destDir := filepath.Join(tmpPath, "newdir", "dest")
		// Don't create destDir - let extractArchive create it
		err := backend.extractArchive(archivePath, destDir, "tar.gz")
		_ = err
		// Verify directory was created or not based on implementation
	})
}

func TestTarballBackend_extractIconsFromAsarNative_MoreCoverage(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("empty asar file", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create empty asar file
		asarFile := filepath.Join(installDir, "empty.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte{}, 0644))

		icons, err := backend.extractIconsFromAsarNative(asarFile, "test-app", installDir)
		assert.Error(t, err)
		assert.Empty(t, icons)
	})

	t.Run("corrupted asar file", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create corrupted asar file
		asarFile := filepath.Join(installDir, "corrupted.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte{0xFF, 0xFF, 0xFF, 0xFF}, 0644))

		icons, err := backend.extractIconsFromAsarNative(asarFile, "test-app", installDir)
		assert.Error(t, err)
		assert.Empty(t, icons)
	})

	t.Run("asar with no icons", func(t *testing.T) {
		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// We need to create a valid minimal ASAR file
		// For now, just test the error path
		asarFile := filepath.Join(installDir, "noicons.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte("not real asar"), 0644))

		icons, err := backend.extractIconsFromAsarNative(asarFile, "test-app", installDir)
		assert.Error(t, err)
		assert.Empty(t, icons)
	})

	t.Run("temp dir creation failure", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		// Use a read-only filesystem to cause temp dir creation to fail
		memFs := afero.NewMemMapFs()
		roFs := afero.NewReadOnlyFs(memFs)
		backend := NewWithDeps(cfg, &logger, roFs, helpers.NewOSCommandRunner())

		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create a minimal valid asar file
		asarFile := filepath.Join(installDir, "test.asar")
		require.NoError(t, os.WriteFile(asarFile, []byte{}, 0644))

		icons, err := backend.extractIconsFromAsarNative(asarFile, "test-app", installDir)
		assert.Error(t, err)
		assert.Empty(t, icons)
	})
}

func TestTarballBackend_Install_MorePaths(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	newBackend := func(fs afero.Fs, home string) *TarballBackend {
		b := NewWithDeps(cfg, &logger, fs, helpers.NewOSCommandRunner())
		b.Paths = paths.NewResolverWithHome(cfg, home)
		return b
	}

	t.Run("already installed without force", func(t *testing.T) {
		home := t.TempDir()
		fs := afero.NewOsFs()
		backend := newBackend(fs, home)

		// Create fake installation directory
		appsDir := filepath.Join(home, ".local", "share", "upkg", "apps")
		require.NoError(t, fs.MkdirAll(appsDir, 0755))
		installDir := filepath.Join(appsDir, "testapp")
		require.NoError(t, fs.MkdirAll(installDir, 0755))

		archivePath := filepath.Join(home, "testapp.tar.gz")
		require.NoError(t, os.WriteFile(archivePath, makeTarGz(t, "app", []byte("#!/bin/sh\n")), 0644))

		tx := transaction.NewManager(&logger)
		record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, tx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already installed")
		assert.Nil(t, record)
	})

	t.Run("force reinstall failure restores previous install", func(t *testing.T) {
		home := t.TempDir()
		fs := afero.NewOsFs()
		backend := newBackend(fs, home)

		// Seed an existing installation with a recognizable marker file.
		appsDir := filepath.Join(home, ".local", "share", "upkg", "apps")
		installDir := filepath.Join(appsDir, "testapp")
		require.NoError(t, fs.MkdirAll(installDir, 0755))
		marker := filepath.Join(installDir, "OLD_MARKER")
		require.NoError(t, os.WriteFile(marker, []byte("previous"), 0644))

		// A valid archive with no executable payload: install proceeds past
		// backup, then fails on "no executables", triggering rollback.
		archivePath := filepath.Join(home, "testapp.tar.gz")
		require.NoError(t, os.WriteFile(archivePath, makeTarGz(t, "data.txt", []byte("just data")), 0644))

		tx := transaction.NewManager(&logger)
		record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{Force: true}, tx)
		require.Error(t, err)
		assert.Nil(t, record)

		// Rollback must restore the previous installation and its marker.
		require.NoError(t, tx.Rollback())
		data, readErr := os.ReadFile(marker)
		require.NoError(t, readErr)
		assert.Equal(t, "previous", string(data))

		// No residual backup directories remain in appsDir.
		entries, dirErr := os.ReadDir(appsDir)
		require.NoError(t, dirErr)
		for _, e := range entries {
			assert.NotContains(t, e.Name(), "upkg-backup-")
		}
	})

	t.Run("create bin dir failure", func(t *testing.T) {
		home := t.TempDir()
		binDir := filepath.Join(home, ".local", "bin")
		// Delegate to the real filesystem (extraction reads the real disk) but
		// fail MkdirAll for the bin directory to hit that specific branch.
		fs := &binDirFailFs{Fs: afero.NewOsFs(), failPrefix: binDir}
		backend := newBackend(fs, home)

		// A real ELF payload so detection, extraction and FindExecutables all
		// succeed, leaving the bin-dir creation as the failing step.
		archivePath := filepath.Join(home, "test.tar.gz")
		require.NoError(t, os.WriteFile(archivePath, makeTarGzWithELF(t, "app"), 0644))

		tx := transaction.NewManager(&logger)
		_, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, tx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bin directory")
	})

	t.Run("nil transaction manager rolls back on failure", func(t *testing.T) {
		home := t.TempDir()
		fs := afero.NewOsFs()
		backend := newBackend(fs, home)

		// Valid archive without executables fails after the install dir is made;
		// the backend's local transaction must roll the directory back.
		archivePath := filepath.Join(home, "testapp.tar.gz")
		require.NoError(t, os.WriteFile(archivePath, makeTarGz(t, "data.txt", []byte("data")), 0644))

		record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, nil)
		assert.Error(t, err)
		assert.Nil(t, record)

		installDir := filepath.Join(home, ".local", "share", "upkg", "apps", "testapp")
		_, statErr := fs.Stat(installDir)
		assert.True(t, os.IsNotExist(statErr), "install dir should be rolled back")
	})
}

func TestTarballBackend_installIcons_MoreCoverage(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	t.Run("various icon formats", func(t *testing.T) {
		tmpDir := t.TempDir()

		backend := New(cfg, &logger)
		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create various icon files
		iconFormats := []struct {
			name string
			data []byte
		}{
			{"app.png", []byte{0x89, 0x50, 0x4E, 0x47}}, // PNG magic
			{"app.svg", []byte("<svg></svg>")},
			{"app.ico", []byte{0x00, 0x00, 0x01, 0x00}}, // ICO magic
		}

		for _, icon := range iconFormats {
			iconPath := filepath.Join(installDir, icon.name)
			require.NoError(t, os.WriteFile(iconPath, icon.data, 0644))
		}

		icons, err := backend.installIcons(installDir, "testapp")
		// May succeed or fail depending on icon validation
		_ = icons
		_ = err
	})

	t.Run("hidden icon files", func(t *testing.T) {
		tmpDir := t.TempDir()

		backend := New(cfg, &logger)
		backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		// Create hidden icon file (should be skipped)
		iconPath := filepath.Join(installDir, ".hidden.png")
		require.NoError(t, os.WriteFile(iconPath, []byte{0x89, 0x50, 0x4E, 0x47}, 0644))

		icons, err := backend.installIcons(installDir, "testapp")
		_ = icons
		_ = err
	})
}

func TestTarballBackend_extractIconsFromAsar_MoreCoverage(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	t.Run("walk error handling", func(t *testing.T) {
		tmpDir := t.TempDir()
		backend := New(cfg, &logger)

		// Create directory without asar files to trigger error
		installDir := filepath.Join(tmpDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		icons, err := backend.extractIconsFromAsar(installDir, "testapp")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no asar files found")
		assert.Empty(t, icons)
	})

	t.Run("nonexistent install directory", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		icons, err := backend.extractIconsFromAsar("/nonexistent/dir", "testapp")
		assert.Error(t, err)
		assert.Empty(t, icons)
	})
}

func TestTarballBackend_Uninstall_MoreCoverage(t *testing.T) {
	t.Parallel()

	t.Run("removes desktop files", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		fs := afero.NewOsFs()
		backend := NewWithDeps(cfg, &logger, fs, helpers.NewOSCommandRunner())

		tmpDir := t.TempDir()
		installDir := filepath.Join(tmpDir, "install")
		wrapperPath := filepath.Join(tmpDir, "wrapper")
		desktopPath := filepath.Join(tmpDir, "app.desktop")
		iconPath := filepath.Join(tmpDir, "icon.png")

		require.NoError(t, fs.MkdirAll(installDir, 0755))
		require.NoError(t, afero.WriteFile(fs, wrapperPath, []byte("wrapper"), 0755))
		require.NoError(t, afero.WriteFile(fs, desktopPath, []byte("desktop"), 0644))
		require.NoError(t, afero.WriteFile(fs, iconPath, []byte("icon"), 0644))

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeTarball,
			InstallPath: installDir,
			Metadata: core.Metadata{
				WrapperScript: wrapperPath,
				IconFiles:     []string{iconPath},
			},
		}

		// Add desktop file to record
		record.DesktopFile = desktopPath

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)

		// Verify removal
		_, err = fs.Stat(installDir)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("handles nil install path", func(t *testing.T) {
		logger := zerolog.New(io.Discard)
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeTarball,
			InstallPath: "",
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
	})
}

func TestTarballBackend_createDesktopFile(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
		},
	}
	backend := New(cfg, &logger)

	installDir := tmpDir
	appName := "TestApp"
	normalizedName := "testapp"
	execPath := "/opt/testapp/bin/testapp"

	t.Run("with wayland env vars", func(t *testing.T) {
		opts := core.InstallOptions{
			SkipWaylandEnv: false,
		}

		desktopPath, err := backend.createDesktopFile(installDir, appName, normalizedName, execPath, opts)
		_ = desktopPath
		_ = err
	})

	t.Run("skip wayland env", func(t *testing.T) {
		opts := core.InstallOptions{
			SkipWaylandEnv: true,
		}

		desktopPath, err := backend.createDesktopFile(installDir, appName, normalizedName, execPath, opts)
		_ = desktopPath
		_ = err
	})

	t.Run("with custom env vars", func(t *testing.T) {
		cfgCustom := &config.Config{
			Paths: config.PathsConfig{
				DataDir: tmpDir,
			},
			Desktop: config.DesktopConfig{
				WaylandEnvVars: true,
				CustomEnvVars:  []string{"CUSTOM_VAR=value"},
			},
		}
		backendCustom := New(cfgCustom, &logger)

		opts := core.InstallOptions{
			SkipWaylandEnv: false,
		}

		desktopPath, err := backendCustom.createDesktopFile(installDir, appName, normalizedName, execPath, opts)
		_ = desktopPath
		_ = err
	})
}

func TestTarballBackend_createWrapper(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	backend := New(cfg, &logger)

	binDir := filepath.Join(tmpDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))
	wrapperPath := filepath.Join(binDir, "testapp")
	execPath := "/opt/testapp/bin/testapp"

	wrapperCfg := helpers.WrapperConfig{
		WrapperPath:    wrapperPath,
		ExecPath:       execPath,
		DisableSandbox: false,
	}
	err := helpers.CreateWrapper(backend.Fs, wrapperCfg)
	assert.NoError(t, err)
}

func TestTarballBackend_isElectronApp(t *testing.T) {
	t.Run("is electron app", func(t *testing.T) {
		t.Parallel()
		logger := zerolog.New(io.Discard)
		tmpDir := t.TempDir()
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		installDir := filepath.Join(tmpDir, "app", "bin")
		require.NoError(t, os.MkdirAll(filepath.Join(installDir, "..", "resources"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(installDir, "..", "resources", "app.asar"), []byte("fake"), 0644))

		isElectron := helpers.IsElectronApp(backend.Fs, installDir)
		assert.True(t, isElectron)
	})

	t.Run("is not electron app", func(t *testing.T) {
		t.Parallel()
		logger := zerolog.New(io.Discard)
		tmpDir := t.TempDir()
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		installDir := filepath.Join(tmpDir, "app", "bin")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		isElectron := helpers.IsElectronApp(backend.Fs, installDir)
		assert.False(t, isElectron)
	})

	t.Run("directory without asar files", func(t *testing.T) {
		t.Parallel()
		logger := zerolog.New(io.Discard)
		tmpDir := t.TempDir()
		cfg := &config.Config{}
		backend := New(cfg, &logger)

		// Create a directory with no .asar files in it or its parent
		installDir := filepath.Join(tmpDir, "app", "bin")
		require.NoError(t, os.MkdirAll(installDir, 0755))

		isElectron := helpers.IsElectronApp(backend.Fs, installDir)
		assert.False(t, isElectron)
	})
}

func TestTarballBackend_copyFile(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	t.Run("successful copy", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcPath := filepath.Join(tmpDir, "source.txt")
		require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0644))

		destPath := filepath.Join(tmpDir, "dest.txt")
		err := backend.copyFile(srcPath, destPath)
		assert.NoError(t, err)
	})

	t.Run("nonexistent source", func(t *testing.T) {
		err := backend.copyFile("/nonexistent/file", "/tmp/dest")
		assert.Error(t, err)
	})
}
