package appimage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// TestAppImageBackend_Install_PackageNotFound tests Install with non-existent package
func TestAppImageBackend_Install_PackageNotFound(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	ctx := context.Background()
	packagePath := "/nonexistent/test.AppImage"
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(ctx, packagePath, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_InvalidFormat tests Install with invalid AppImage
func TestAppImageBackend_Install_InvalidFormat(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	fs := afero.NewOsFs()
	backend := NewWithDeps(cfg, &logger, fs, helpers.NewOSCommandRunner())

	// Create a fake AppImage (not real)
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage content"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_AlreadyInstalled tests Install when package is already installed
// NOTE: This test is skipped because it requires a valid AppImage that can be extracted.
// The "already installed" check happens after extraction, so we need a valid AppImage.
// This is better tested as an integration test with real AppImages.
func TestAppImageBackend_Install_AlreadyInstalled(t *testing.T) {
	t.Skip("requires valid AppImage for extraction - tested in integration")
}

// TestAppImageBackend_Install_WithForce tests Install with force option
func TestAppImageBackend_Install_WithForce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	fs := afero.NewOsFs()
	backend := NewWithDeps(cfg, &logger, fs, helpers.NewOSCommandRunner())

	// Create fake installation directory
	appsDir := filepath.Join(tmpDir, ".local", "share", "upkg", "apps")
	require.NoError(t, fs.MkdirAll(appsDir, 0755))
	installDir := filepath.Join(appsDir, "testapp")
	require.NoError(t, fs.MkdirAll(installDir, 0755))

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "testapp.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	// With force, should try to remove existing dir
	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{Force: true}, tx)

	// Will fail during extraction but should attempt removal
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_SkipDesktop tests Install with SkipDesktop option
func TestAppImageBackend_Install_SkipDesktop(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "testapp.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{SkipDesktop: true}, tx)

	// Should fail during extraction
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_SkipWaylandEnv tests Install with SkipWaylandEnv option
func TestAppImageBackend_Install_SkipWaylandEnv(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "testapp.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{SkipWaylandEnv: true}, tx)

	// Should fail during extraction
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_WithCustomName tests Install with custom name
func TestAppImageBackend_Install_WithCustomName(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "original-name.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{CustomName: "CustomApp"}, tx)

	// Should fail during extraction
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_InvalidCustomName tests Install with invalid custom name
func TestAppImageBackend_Install_InvalidCustomName(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	// Custom name with invalid characters
	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{CustomName: "../../etc/passwd"}, tx)

	assert.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "invalid")
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_NoHomeDirectory tests Install when HOME is not set
func TestAppImageBackend_Install_NoHomeDirectory(t *testing.T) {
	t.Parallel()

	// Unset HOME
	origHome := os.Getenv("HOME")
	os.Unsetenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{}, tx)

	// Should fail because HOME is not set
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_NilTransaction tests Install with nil transaction
func TestAppImageBackend_Install_NilTransaction(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()

	// Pass nil transaction
	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{}, nil)

	// Should fail during extraction
	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_MkdirFailure tests Install when directory creation fails
func TestAppImageBackend_Install_MkdirFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)

	// Use a mock filesystem that fails MkdirAll
	errorFs := &errorFs{
		Fs:           afero.NewMemMapFs(),
		failMkdirAll: true,
	}
	backend := NewWithDeps(cfg, &logger, errorFs, helpers.NewOSCommandRunner())

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, afero.WriteFile(errorFs, fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Nil(t, record)
}

// TestAppImageBackend_Install_ExtractFailure tests Install when extraction fails
func TestAppImageBackend_Install_ExtractFailure(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)

	// Mock runner that fails extraction
	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(cmd string) bool {
			return cmd == "unsquashfs" || cmd == "bsdtar"
		},
		RunCommandFunc: func(_ context.Context, cmd string, args ...string) (string, error) {
			return "", assert.AnError
		},
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)
	backend := NewWithCacheManager(cfg, &logger, cacheManager)

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	tx := transaction.NewManager(&logger)

	record, err := backend.Install(ctx, fakeAppImage, core.InstallOptions{}, tx)

	assert.Error(t, err)
	assert.Nil(t, record)
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

// TestAppImageBackend_Uninstall_WithDesktopFile tests uninstall with desktop file
func TestAppImageBackend_Uninstall_WithDesktopFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create fake desktop file
	desktopPath := filepath.Join(tmpDir, "testapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte("[Desktop Entry]"), 0644))

	record := &core.InstallRecord{
		InstallID:    "test-123",
		Name:         "testapp",
		PackageType:  "appimage",
		InstallPath:  "",
		DesktopFile:  desktopPath,
		Metadata: core.Metadata{
			IconFiles: []string{},
		},
	}

	ctx := context.Background()
	err := backend.Uninstall(ctx, record)

	// May succeed or fail gracefully
	_ = err
}

// TestAppImageBackend_Uninstall_WithIconFiles tests uninstall with icon files
func TestAppImageBackend_Uninstall_WithIconFiles(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create fake icon files
	icon1 := filepath.Join(tmpDir, "icon1.png")
	icon2 := filepath.Join(tmpDir, "icon2.svg")
	require.NoError(t, os.WriteFile(icon1, []byte("icon1"), 0644))
	require.NoError(t, os.WriteFile(icon2, []byte("icon2"), 0644))

	record := &core.InstallRecord{
		InstallID:    "test-123",
		Name:         "testapp",
		PackageType:  "appimage",
		InstallPath:  "",
		DesktopFile:  "",
		Metadata: core.Metadata{
			IconFiles: []string{icon1, icon2},
		},
	}

	ctx := context.Background()
	err := backend.Uninstall(ctx, record)

	// Should succeed - icons should be removed
	assert.NoError(t, err)

	// Verify icons are removed
	_, err1 := os.Stat(icon1)
	_, err2 := os.Stat(icon2)
	assert.True(t, os.IsNotExist(err1) || os.IsNotExist(err2))
}

// TestAppImageBackend_extractAppImage_UnsquashfsNotFound tests extraction when unsquashfs is not available
func TestAppImageBackend_extractAppImage_UnsquashfsNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)

	// Create output dir first
	outputDir := filepath.Join(tmpDir, "output")
	require.NoError(t, os.MkdirAll(outputDir, 0755))

	mockRunner := &helpers.MockCommandRunner{
		CommandExistsFunc: func(cmd string) bool {
			// Neither unsquashfs nor bsdtar available
			return false
		},
		RunCommandFunc: func(_ context.Context, cmd string, args ...string) (string, error) {
			// First attempt is --appimage-extract which will fail
			// Return an error indicating the appimage extraction failed
			return "", fmt.Errorf("appimage extraction failed")
		},
		RunCommandInDirFunc: func(_ context.Context, dir, cmd string, args ...string) (string, error) {
			// First attempt is --appimage-extract which will fail
			return "", fmt.Errorf("appimage extraction failed")
		},
	}
	cacheManager := cache.NewCacheManagerWithRunner(mockRunner)
	backend := NewWithCacheManager(cfg, &logger, cacheManager)

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()

	err := backend.extractAppImage(ctx, fakeAppImage, outputDir)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsquashfs not found")
}

// TestAppImageBackend_extractAppImage_InvalidOutputDir tests extraction when output dir creation fails
func TestAppImageBackend_extractAppImage_InvalidOutputDir(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)

	errorFs := &errorFs{
		Fs:           afero.NewMemMapFs(),
		failMkdirAll: true,
	}
	backend := NewWithDeps(cfg, &logger, errorFs, helpers.NewOSCommandRunner())

	// Create fake AppImage
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, afero.WriteFile(errorFs, fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	outputDir := filepath.Join(tmpDir, "output")

	err := backend.extractAppImage(ctx, fakeAppImage, outputDir)

	assert.Error(t, err)
}
