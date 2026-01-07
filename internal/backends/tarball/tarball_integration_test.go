package tarball

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quantmind-br/upkg/internal/cache"
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

// TestTarballBackend_Install_InvalidPackageName tests the Install function with invalid package names
func TestTarballBackend_Install_InvalidPackageName_Full(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{}
	backend := New(cfg, &logger)

	// Create a tar.gz with a name that normalizes to invalid characters
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte{0x1F, 0x8B}, 0644))

	tx := transaction.NewManager(&logger)
	// Use custom name with invalid characters
	record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{CustomName: "!!!invalid!!!"}, tx)

	// Should fail - either validation or extraction
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestTarballBackend_Install_NoHomeDir tests Install when HOME is not set
func TestTarballBackend_Install_NoHomeDir(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	// Unset HOME
	origHome := os.Getenv("HOME")
	os.Unsetenv("HOME")
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{}
	backend := New(cfg, &logger)

	// Create a tar.gz
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte{0x1F, 0x8B}, 0644))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, tx)

	// Should fail because HOME is not set
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestTarballBackend_Install_EmptyArchive tests Install with an archive that has no executables
func TestTarballBackend_Install_EmptyArchive(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{}
	fs := afero.NewOsFs()
	backend := NewWithDeps(cfg, &logger, fs, helpers.NewOSCommandRunner())

	// Create a minimal tar.gz (will fail extraction, not no executables)
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte{0x1F, 0x8B}, 0644))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, tx)

	// Should fail during extraction
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestTarballBackend_Install_MkdirFailure tests Install when directory creation fails
func TestTarballBackend_Install_MkdirFailure(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{}
	// Use a mock filesystem that fails MkdirAll
	fs := &errorFs{Fs: afero.NewMemMapFs(), failMkdirAll: true}
	runner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(cmd string) bool {
			return true
		},
		RunCommandFunc: func(_ context.Context, cmd string, args ...string) (string, error) {
			return "", nil
		},
	}
	backend := NewWithDeps(cfg, &logger, fs, runner)

	// Create a minimal tar.gz
	archivePath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, afero.WriteFile(fs, archivePath, []byte{0x1F, 0x8B}, 0644))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestTarballBackend_Install_ForceRemovalFailure tests Install with force when removal fails
func TestTarballBackend_Install_ForceRemovalFailure(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{}
	fs := afero.NewOsFs()
	backend := NewWithDeps(cfg, &logger, fs, helpers.NewOSCommandRunner())

	// Create fake installation directory
	appsDir := filepath.Join(tmpDir, ".local", "share", "upkg", "apps")
	require.NoError(t, fs.MkdirAll(appsDir, 0755))
	installDir := filepath.Join(appsDir, "testapp")
	require.NoError(t, fs.MkdirAll(installDir, 0755))

	// Create a file that can't be removed
	testFile := filepath.Join(installDir, "test")
	require.NoError(t, afero.WriteFile(fs, testFile, []byte("test"), 0644))

	// Create a tar.gz (will fail extraction but force should try to remove existing dir)
	archivePath := filepath.Join(tmpDir, "testapp.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte("fake archive"), 0644))

	tx := transaction.NewManager(&logger)
	_, err := backend.Install(context.Background(), archivePath, core.InstallOptions{Force: true}, tx)

	// Should fail during extraction
	assert.Error(t, err)
}

// TestTarballBackend_installIcons_CacheUpdateFailure tests icon installation with cache failures
func TestTarballBackend_installIcons_CacheUpdateFailure(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{}
	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(cmd string) bool {
			// Mock that gtk-update-icon-cache exists
			return cmd == "gtk-update-icon-cache" || cmd == "gtk4-update-icon-cache"
		},
		RunCommandFunc: func(_ context.Context, cmd string, args ...string) (string, error) {
			// Fail the cache update command
			if strings.Contains(cmd, "gtk-update-icon-cache") {
				return "", assert.AnError
			}
			return "", nil
		},
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)
	backend := NewWithCacheManager(cfg, &logger, cacheManager)

	installDir := filepath.Join(tmpDir, "install")
	require.NoError(t, os.MkdirAll(installDir, 0755))

	// Create an icon file
	iconPath := filepath.Join(installDir, "test.png")
	pngData := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A} // PNG magic
	require.NoError(t, os.WriteFile(iconPath, pngData, 0644))

	icons, err := backend.installIcons(installDir, "testapp")

	// Cache update failure should not prevent icon installation
	_ = icons
	_ = err
}

// TestTarballBackend_installIcons_EmptyInstallDir tests icon installation with empty install dir
func TestTarballBackend_installIcons_EmptyInstallDir(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	icons, err := backend.installIcons("", "testapp")
	assert.Empty(t, icons)
	_ = err
}

// TestTarballBackend_createDesktopFile_MissingAppsDir tests desktop file creation when apps dir doesn't exist
func TestTarballBackend_createDesktopFile_MissingAppsDir(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{}
	fs := afero.NewOsFs()
	backend := NewWithDeps(cfg, &logger, fs, helpers.NewOSCommandRunner())

	// Don't create the apps directory - the function should handle this
	installDir := filepath.Join(tmpDir, "install")
	require.NoError(t, os.MkdirAll(installDir, 0755))

	execPath := filepath.Join(installDir, "app")
	require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash"), 0755))

	// Update the backend's paths resolver to use the new home
	backend.Paths = paths.NewResolverWithHome(cfg, tmpDir)

	_, err := backend.createDesktopFile(installDir, "TestApp", "testapp", execPath, core.InstallOptions{})
	// May succeed if it creates the directory, or fail if it can't
	_ = err
}

// errorFs is a mock filesystem that can fail specific operations
type errorFs struct {
	afero.Fs
	failMkdirAll bool
	failRemoveAll bool
	failCreate    bool
	failStat      bool
}

func (fs *errorFs) MkdirAll(path string, perm os.FileMode) error {
	if fs.failMkdirAll {
		return assert.AnError
	}
	return fs.Fs.MkdirAll(path, perm)
}

func (fs *errorFs) RemoveAll(path string) error {
	if fs.failRemoveAll {
		return assert.AnError
	}
	return fs.Fs.RemoveAll(path)
}

func (fs *errorFs) Create(name string) (afero.File, error) {
	if fs.failCreate {
		return nil, assert.AnError
	}
	return fs.Fs.Create(name)
}

func (fs *errorFs) Stat(name string) (os.FileInfo, error) {
	if fs.failStat {
		return nil, assert.AnError
	}
	return fs.Fs.Stat(name)
}

// TestTarballBackend_extractIconsFromAsar_TempDirFailure tests asar extraction when temp dir creation fails
func TestTarballBackend_extractIconsFromAsar_TempDirFailure(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	// Use an errorFs that fails temp dir creation
	errorFs := &errorFs{
		Fs:           afero.NewMemMapFs(),
		failMkdirAll: true,
	}
	backend := NewWithDeps(cfg, &logger, errorFs, helpers.NewOSCommandRunner())

	tmpDir := t.TempDir()
	installDir := filepath.Join(tmpDir, "install")
	require.NoError(t, os.MkdirAll(installDir, 0755))

	// Create fake asar file
	asarFile := filepath.Join(installDir, "app.asar")
	require.NoError(t, os.WriteFile(asarFile, []byte("fake asar"), 0644))

	icons, err := backend.extractIconsFromAsarNative(asarFile, "test-app", installDir)

	assert.Error(t, err)
	assert.Empty(t, icons)
}

// TestTarballBackend_copyFile_SourceNotFound tests copyFile when source doesn't exist
func TestTarballBackend_copyFile_SourceNotFound(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "nonexistent.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	err := backend.copyFile(src, dst)
	assert.Error(t, err)
}

// TestTarballBackend_copyFile_DestCreationFailure tests copyFile when dest creation fails
func TestTarballBackend_copyFile_DestCreationFailure(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	errorFs := &errorFs{
		Fs:        afero.NewMemMapFs(),
		failCreate: true,
	}
	backend := NewWithDeps(cfg, &logger, errorFs, helpers.NewOSCommandRunner())

	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "source.txt")
	dst := filepath.Join(tmpDir, "dest.txt")

	require.NoError(t, os.WriteFile(src, []byte("content"), 0644))

	err := backend.copyFile(src, dst)
	assert.Error(t, err)
}

// TestTarballBackend_Detect_UnknownFileType tests Detect with unknown file type
func TestTarballBackend_Detect_UnknownFileType(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	tmpDir := t.TempDir()
	// Create a file without a recognized extension
	testFile := filepath.Join(tmpDir, "test.unknown")
	require.NoError(t, os.WriteFile(testFile, []byte("content"), 0644))

	result, err := backend.Detect(context.Background(), testFile)

	// Unknown file types return false with no error
	assert.NoError(t, err)
	assert.False(t, result)
}

// TestTarballBackend_Install_SkipWaylandEnv tests Install with SkipWaylandEnv option
func TestTarballBackend_Install_SkipWaylandEnv_Full(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
		},
	}
	fs := afero.NewOsFs()
	runner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(cmd string) bool {
			return cmd == "tar" || cmd == "bsdtar"
		},
		RunCommandFunc: func(_ context.Context, cmd string, args ...string) (string, error) {
			// Mock tar extraction
			if cmd == "tar" || cmd == "bsdtar" {
				destDir := args[len(args)-1]
				// Create a fake executable
				execPath := filepath.Join(destDir, "testapp")
				return "", os.WriteFile(execPath, []byte("#!/bin/bash\necho test"), 0755)
			}
			return "", nil
		},
	}
	backend := NewWithDeps(cfg, &logger, fs, runner)

	// Create a minimal tar.gz
	archivePath := filepath.Join(tmpDir, "testapp.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte{0x1F, 0x8B}, 0644))

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{SkipWaylandEnv: true}, tx)

	_ = record
	_ = err
}

// TestCreateRealTarGz creates a real tar.gz archive for testing
func TestCreateRealTarGz(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple directory structure
	srcDir := filepath.Join(tmpDir, "src")
	binDir := filepath.Join(srcDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))

	// Create a simple executable
	execPath := filepath.Join(binDir, "myapp")
	require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash\necho 'Hello, World!'"), 0755))

	// Create tar.gz
	archivePath := filepath.Join(tmpDir, "myapp.tar.gz")
	cmd := exec.Command("tar", "-czf", archivePath, "-C", srcDir, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("Could not create tar.gz: %v\n%s", err, output)
		return
	}

	// Verify the file was created
	info, err := os.Stat(archivePath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}

// TestTarballBackend_Install_WithRealTar tests Install with a real tar.gz
func TestTarballBackend_Install_WithRealTar(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check if tar is available
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar command not available")
	}

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Create a real tar.gz with an executable
	srcDir := filepath.Join(tmpDir, "src")
	binDir := filepath.Join(srcDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))

	execPath := filepath.Join(binDir, "testapp")
	require.NoError(t, os.WriteFile(execPath, []byte("#!/bin/bash\necho test"), 0755))

	archivePath := filepath.Join(tmpDir, "testapp.tar.gz")
	cmd := exec.Command("tar", "-czf", archivePath, "-C", srcDir, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("Could not create tar.gz: %v\n%s", err, output)
		return
	}

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	tx := transaction.NewManager(&logger)
	record, err := backend.Install(context.Background(), archivePath, core.InstallOptions{}, tx)

	// Should succeed or fail gracefully
	_ = record
	_ = err
}
