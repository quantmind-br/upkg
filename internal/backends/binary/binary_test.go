package binary

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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestName(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)
	if backend.Name() != "binary" {
		t.Errorf("Name() = %q, want %q", backend.Name(), "binary")
	}
}

func TestNewWithRunner(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockRunner := &helpers.MockCommandRunner{}

	backend := NewWithRunner(cfg, &logger, mockRunner)

	assert.NotNil(t, backend)
	assert.Equal(t, "binary", backend.Name())
	assert.Equal(t, mockRunner, backend.runner)
}

func TestNewWithCacheManager(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	mockCacheMgr := cache.NewCacheManager()

	backend := NewWithCacheManager(cfg, &logger, mockCacheMgr)

	assert.NotNil(t, backend)
	assert.Equal(t, "binary", backend.Name())
	assert.Equal(t, mockCacheMgr, backend.cacheManager)
}

func TestDetect(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)

	tmpDir := t.TempDir()

	// 1. Non-existent file
	ok, err := backend.Detect(context.Background(), filepath.Join(tmpDir, "nonexistent"))
	if err != nil {
		t.Errorf("Detect failed for nonexistent file: %v", err)
	}
	if ok {
		t.Error("Detect returned true for nonexistent file")
	}

	// 2. Text file (Not ELF)
	txtFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(txtFile, []byte("not an elf"), 0644); err != nil {
		t.Fatal(err)
	}
	ok, err = backend.Detect(context.Background(), txtFile)
	if err != nil {
		t.Errorf("Detect failed for text file: %v", err)
	}
	if ok {
		t.Error("Detect returned true for text file")
	}
}

func TestDetect_ELFMagicButNotValid(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)

	tmpDir := t.TempDir()

	// Create file with ELF magic but invalid content
	elfMagic := []byte{0x7F, 'E', 'L', 'F', 0x00, 0x00, 0x00, 0x00}
	elfFile := filepath.Join(tmpDir, "fake-elf")
	require.NoError(t, os.WriteFile(elfFile, elfMagic, 0755))

	ok, err := backend.Detect(context.Background(), elfFile)
	assert.NoError(t, err)
	assert.False(t, ok, "Should return false for invalid ELF")
}

func TestDetect_ShellScript(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)

	tmpDir := t.TempDir()

	// Create shell script
	script := filepath.Join(tmpDir, "script.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/bash\necho hello"), 0755))

	ok, err := backend.Detect(context.Background(), script)
	assert.NoError(t, err)
	assert.False(t, ok, "Should return false for shell script")
}

func TestInstall_PackageNotFound(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(context.Background(), "/nonexistent/binary", core.InstallOptions{}, tx)

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
	fakeBin := filepath.Join(tmpDir, "test-binary")
	require.NoError(t, os.WriteFile(fakeBin, []byte("fake binary"), 0755))

	// Try to install with an empty custom name after normalization
	// Using a name that normalizes to empty string (all invalid chars)
	record, err := backend.Install(context.Background(), fakeBin, core.InstallOptions{
		CustomName: "///",
	}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
	assert.Nil(t, record)
}

func TestInstall_AlreadyInstalled(t *testing.T) {
	logger := zerolog.New(io.Discard)

	// Create mock runner
	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool { return false },
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}
	backend := &BinaryBackend{
		cfg:          cfg,
		logger:       &logger,
		runner:       mockRunner,
		cacheManager: cacheManager,
	}

	tmpDir := t.TempDir()

	// Set HOME to tmpDir
	origHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHomeDir)

	// Create fake binary
	fakeBin := filepath.Join(tmpDir, "test-binary")
	require.NoError(t, os.WriteFile(fakeBin, []byte("fake binary"), 0755))

	// Pre-create the destination to simulate already installed
	binDir := filepath.Join(tmpDir, ".local", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))
	destPath := filepath.Join(binDir, "test-binary")
	require.NoError(t, os.WriteFile(destPath, []byte("existing"), 0755))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), fakeBin, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already installed")
	assert.Nil(t, record)
}

func TestInstall_ForceReinstall(t *testing.T) {
	logger := zerolog.New(io.Discard)

	// Create mock runner
	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool {
			return name == "desktop-file-validate"
		},
		RunCommandFunc: func(ctx context.Context, name string, args ...string) (string, error) {
			return "", nil
		},
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}
	backend := &BinaryBackend{
		cfg:          cfg,
		logger:       &logger,
		runner:       mockRunner,
		cacheManager: cacheManager,
	}

	tmpDir := t.TempDir()

	// Set HOME to tmpDir
	origHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHomeDir)

	// Create fake binary (valid content for copying)
	fakeBin := filepath.Join(tmpDir, "test-binary")
	require.NoError(t, os.WriteFile(fakeBin, []byte("fake binary content"), 0755))

	// Pre-create the destination
	binDir := filepath.Join(tmpDir, ".local", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))
	destPath := filepath.Join(binDir, "test-binary")
	require.NoError(t, os.WriteFile(destPath, []byte("existing"), 0755))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), fakeBin, core.InstallOptions{
		Force: true,
	}, tx)

	require.NoError(t, err)
	assert.NotNil(t, record)
	assert.Equal(t, "test-binary", record.Name)

	// Verify content was replaced
	content, _ := os.ReadFile(destPath)
	assert.Equal(t, "fake binary content", string(content))
}

func TestInstall_WithCustomName(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool {
			return name == "desktop-file-validate"
		},
		RunCommandFunc: func(ctx context.Context, name string, args ...string) (string, error) {
			return "", nil
		},
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}
	backend := &BinaryBackend{
		cfg:          cfg,
		logger:       &logger,
		runner:       mockRunner,
		cacheManager: cacheManager,
	}

	tmpDir := t.TempDir()

	origHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHomeDir)

	fakeBin := filepath.Join(tmpDir, "original-name")
	require.NoError(t, os.WriteFile(fakeBin, []byte("binary content"), 0755))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), fakeBin, core.InstallOptions{
		CustomName: "CustomApp",
	}, tx)

	require.NoError(t, err)
	assert.NotNil(t, record)
	assert.Equal(t, "CustomApp", record.Name)

	// Verify binary was installed with custom name
	destPath := filepath.Join(tmpDir, ".local", "bin", "customapp")
	assert.FileExists(t, destPath)
}

func TestInstall_SkipDesktop(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool { return false },
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}
	backend := &BinaryBackend{
		cfg:          cfg,
		logger:       &logger,
		runner:       mockRunner,
		cacheManager: cacheManager,
	}

	tmpDir := t.TempDir()

	origHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHomeDir)

	fakeBin := filepath.Join(tmpDir, "test-binary")
	require.NoError(t, os.WriteFile(fakeBin, []byte("binary content"), 0755))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), fakeBin, core.InstallOptions{
		SkipDesktop: true,
	}, tx)

	require.NoError(t, err)
	assert.NotNil(t, record)
	assert.Empty(t, record.DesktopFile, "Desktop file should be empty when skipped")

	// Verify desktop file was NOT created
	desktopPath := filepath.Join(tmpDir, ".local", "share", "applications", "test-binary.desktop")
	assert.NoFileExists(t, desktopPath)
}

func TestInstall_WithTransaction(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool {
			return name == "desktop-file-validate"
		},
		RunCommandFunc: func(ctx context.Context, name string, args ...string) (string, error) {
			return "", nil
		},
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}
	backend := &BinaryBackend{
		cfg:          cfg,
		logger:       &logger,
		runner:       mockRunner,
		cacheManager: cacheManager,
	}

	tmpDir := t.TempDir()

	origHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHomeDir)

	fakeBin := filepath.Join(tmpDir, "test-binary")
	require.NoError(t, os.WriteFile(fakeBin, []byte("binary content"), 0755))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), fakeBin, core.InstallOptions{}, tx)

	require.NoError(t, err)
	assert.NotNil(t, record)

	// Verify files were created
	binPath := filepath.Join(tmpDir, ".local", "bin", "test-binary")
	desktopPath := filepath.Join(tmpDir, ".local", "share", "applications", "test-binary.desktop")
	assert.FileExists(t, binPath)
	assert.FileExists(t, desktopPath)

	// Rollback transaction
	tx.Rollback()

	// Verify files were cleaned up
	assert.NoFileExists(t, binPath, "binary should be removed after rollback")
	assert.NoFileExists(t, desktopPath, "desktop file should be removed after rollback")
}

func TestUninstall(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool { return false },
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}
	backend := &BinaryBackend{
		cfg:          cfg,
		logger:       &logger,
		runner:       mockRunner,
		cacheManager: cacheManager,
	}

	t.Run("uninstalls all files", func(t *testing.T) {
		tmpDir := t.TempDir()

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		// Create fake installation
		binPath := filepath.Join(tmpDir, "bin", "test-app")
		require.NoError(t, os.MkdirAll(filepath.Dir(binPath), 0755))
		require.NoError(t, os.WriteFile(binPath, []byte("fake binary"), 0755))

		desktopPath := filepath.Join(tmpDir, "test.desktop")
		require.NoError(t, os.WriteFile(desktopPath, []byte("[Desktop Entry]"), 0644))

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeBinary,
			InstallPath: binPath,
			DesktopFile: desktopPath,
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)

		assert.NoFileExists(t, binPath)
		assert.NoFileExists(t, desktopPath)
	})

	t.Run("handles missing files gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeBinary,
			InstallPath: "/nonexistent/binary",
			DesktopFile: "/nonexistent/desktop.desktop",
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
	})

	t.Run("handles empty record", func(t *testing.T) {
		tmpDir := t.TempDir()

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-app",
			PackageType: core.PackageTypeBinary,
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
	})
}

func TestCreateDesktopFile(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool {
			return name == "desktop-file-validate"
		},
		RunCommandFunc: func(ctx context.Context, name string, args ...string) (string, error) {
			return "", nil
		},
	}

	t.Run("creates desktop file successfully", func(t *testing.T) {
		tmpDir := t.TempDir()

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: false,
			},
		}

		backend := &BinaryBackend{
			cfg:    cfg,
			logger: &logger,
			runner: mockRunner,
		}

		desktopPath, err := backend.createDesktopFile("Test App", "test-app", "/usr/bin/test-app", core.InstallOptions{})
		require.NoError(t, err)
		assert.NotEmpty(t, desktopPath)

		content, err := os.ReadFile(desktopPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "[Desktop Entry]")
		assert.Contains(t, contentStr, "Name=Test App")
		assert.Contains(t, contentStr, "Exec=/usr/bin/test-app")
		assert.Contains(t, contentStr, "Icon=application-x-executable") // Generic icon
	})

	t.Run("injects wayland environment variables when enabled", func(t *testing.T) {
		tmpDir := t.TempDir()

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: true,
			},
		}

		backend := &BinaryBackend{
			cfg:    cfg,
			logger: &logger,
			runner: mockRunner,
		}

		desktopPath, err := backend.createDesktopFile("Test App", "test-app", "/usr/bin/test-app", core.InstallOptions{})
		require.NoError(t, err)

		content, err := os.ReadFile(desktopPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "env")
	})

	t.Run("skips wayland when option set", func(t *testing.T) {
		tmpDir := t.TempDir()

		origHomeDir := os.Getenv("HOME")
		os.Setenv("HOME", tmpDir)
		defer os.Setenv("HOME", origHomeDir)

		cfg := &config.Config{
			Desktop: config.DesktopConfig{
				WaylandEnvVars: true,
			},
		}

		backend := &BinaryBackend{
			cfg:    cfg,
			logger: &logger,
			runner: mockRunner,
		}

		desktopPath, err := backend.createDesktopFile("Test App", "test-app", "/usr/bin/test-app", core.InstallOptions{
			SkipWaylandEnv: true,
		})
		require.NoError(t, err)

		content, err := os.ReadFile(desktopPath)
		require.NoError(t, err)

		contentStr := string(content)
		assert.Contains(t, contentStr, "Exec=/usr/bin/test-app")
	})
}

func TestInstallRecord(t *testing.T) {
	logger := zerolog.New(io.Discard)

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(name string) bool {
			return name == "desktop-file-validate"
		},
		RunCommandFunc: func(ctx context.Context, name string, args ...string) (string, error) {
			return "", nil
		},
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

	cfg := &config.Config{}
	backend := &BinaryBackend{
		cfg:          cfg,
		logger:       &logger,
		runner:       mockRunner,
		cacheManager: cacheManager,
	}

	tmpDir := t.TempDir()

	origHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHomeDir)

	fakeBin := filepath.Join(tmpDir, "my-app-1.0")
	require.NoError(t, os.WriteFile(fakeBin, []byte("binary content"), 0755))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), fakeBin, core.InstallOptions{}, tx)

	require.NoError(t, err)
	require.NotNil(t, record)

	// Verify record fields
	assert.NotEmpty(t, record.InstallID)
	assert.Equal(t, core.PackageTypeBinary, record.PackageType)
	assert.NotEmpty(t, record.Name)
	assert.NotEmpty(t, record.InstallPath)
	assert.NotEmpty(t, record.DesktopFile)
	assert.Equal(t, core.InstallMethodLocal, record.Metadata.InstallMethod)
	assert.Equal(t, string(core.WaylandUnknown), record.Metadata.WaylandSupport)
}
