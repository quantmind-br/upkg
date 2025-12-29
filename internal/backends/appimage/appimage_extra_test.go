package appimage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppImageBackend_createDesktopFile_WithExistingDesktop(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create squashfs root with .desktop file
	squashfsRoot := filepath.Join(tmpDir, "squashfs")
	applicationsDir := filepath.Join(squashfsRoot, "usr", "share", "applications")
	require.NoError(t, os.MkdirAll(applicationsDir, 0755))

	desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
Icon=testapp
Categories=Utility;`
	desktopPath := filepath.Join(applicationsDir, "testapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

	metadata := &appImageMetadata{
		desktopFile: desktopPath,
		icon:        "testapp",
	}

	appName := "TestApp"
	binName := "testapp"
	execPath := "/opt/testapp.TestImage"
	opts := core.InstallOptions{}

	resultPath, err := backend.createDesktopFile(squashfsRoot, appName, binName, execPath, metadata, opts)
	_ = resultPath
	_ = err
}

func TestAppImageBackend_createDesktopFile_WithoutDesktop(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create squashfs root without .desktop file
	squashfsRoot := tmpDir

	metadata := &appImageMetadata{
		desktopFile: "",
		icon:        "",
	}

	appName := "TestApp"
	binName := "testapp"
	execPath := "/opt/testapp.TestImage"
	opts := core.InstallOptions{}

	resultPath, err := backend.createDesktopFile(squashfsRoot, appName, binName, execPath, metadata, opts)
	_ = resultPath
	_ = err
}

func TestAppImageBackend_createDesktopFile_WithElectron(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
		Desktop: config.DesktopConfig{
			ElectronDisableSandbox: true,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create squashfs root with Electron structure
	squashfsRoot := tmpDir
	resourcesDir := filepath.Join(squashfsRoot, "resources")
	require.NoError(t, os.MkdirAll(resourcesDir, 0755))
	// Create app.asar to indicate Electron app
	require.NoError(t, os.WriteFile(filepath.Join(resourcesDir, "app.asar"), []byte("fake"), 0644))

	metadata := &appImageMetadata{
		desktopFile: "",
		icon:        "testapp",
	}

	appName := "ElectronApp"
	binName := "electronapp"
	execPath := "/opt/electronapp.AppImage"
	opts := core.InstallOptions{}

	resultPath, err := backend.createDesktopFile(squashfsRoot, appName, binName, execPath, metadata, opts)
	_ = resultPath
	_ = err
}

func TestAppImageBackend_createDesktopFile_WithWaylandEnv(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	squashfsRoot := tmpDir

	metadata := &appImageMetadata{
		desktopFile: "",
		icon:        "testapp",
	}

	appName := "TestApp"
	binName := "testapp"
	execPath := "/opt/testapp.AppImage"
	opts := core.InstallOptions{
		SkipWaylandEnv: false,
	}

	resultPath, err := backend.createDesktopFile(squashfsRoot, appName, binName, execPath, metadata, opts)
	_ = resultPath
	_ = err
}

func TestAppImageBackend_createDesktopFile_SkipWaylandEnv(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	squashfsRoot := tmpDir

	metadata := &appImageMetadata{
		desktopFile: "",
		icon:        "testapp",
	}

	appName := "TestApp"
	binName := "testapp"
	execPath := "/opt/testapp.AppImage"
	opts := core.InstallOptions{
		SkipWaylandEnv: true, // Skip Wayland env
	}

	resultPath, err := backend.createDesktopFile(squashfsRoot, appName, binName, execPath, metadata, opts)
	_ = resultPath
	_ = err
}

func TestAppImageBackend_createDesktopFile_WithTauriApp(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create squashfs root with .desktop file for Tauri app
	squashfsRoot := tmpDir
	applicationsDir := filepath.Join(squashfsRoot, "usr", "share", "applications")
	require.NoError(t, os.MkdirAll(applicationsDir, 0755))

	desktopContent := `[Desktop Entry]
Type=Application
Name=TauriApp
StartupWMClass=tauri-app
Exec=tauriapp`
	desktopPath := filepath.Join(applicationsDir, "tauriapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

	metadata := &appImageMetadata{
		desktopFile: desktopPath,
		icon:        "tauriapp",
	}

	appName := "TauriApp"
	binName := "tauriapp"
	execPath := "/opt/tauriapp.AppImage"
	opts := core.InstallOptions{}

	resultPath, err := backend.createDesktopFile(squashfsRoot, appName, binName, execPath, metadata, opts)
	_ = resultPath
	_ = err
}

func TestAppImageBackend_createDesktopFile_WithCustomEnvVars(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
			CustomEnvVars:  []string{"CUSTOM_VAR=value"},
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	squashfsRoot := tmpDir

	metadata := &appImageMetadata{
		desktopFile: "",
		icon:        "testapp",
	}

	appName := "TestApp"
	binName := "testapp"
	execPath := "/opt/testapp.AppImage"
	opts := core.InstallOptions{}

	resultPath, err := backend.createDesktopFile(squashfsRoot, appName, binName, execPath, metadata, opts)
	_ = resultPath
	_ = err
}

func TestAppImageBackend_extractAppImage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create a fake AppImage file (not a real one, just to test the flow)
	fakeAppImage := filepath.Join(tmpDir, "test.AppImage")
	require.NoError(t, os.WriteFile(fakeAppImage, []byte("fake appimage"), 0755))

	ctx := context.Background()
	outputDir := filepath.Join(tmpDir, "output")

	// This will fail because it's not a real AppImage, but we can test the function gets called
	err := backend.extractAppImage(ctx, fakeAppImage, outputDir)
	_ = err
}

func TestAppImageBackend_installIcons(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create test icons in the squashfs root
	squashfsRoot := tmpDir
	iconDir := filepath.Join(squashfsRoot, "usr", "share", "icons", "hicolor", "256x256", "apps")
	require.NoError(t, os.MkdirAll(iconDir, 0755))

	iconPath := filepath.Join(iconDir, "testapp.png")
	require.NoError(t, os.WriteFile(iconPath, []byte("fake icon"), 0644))

	normalizedName := "testapp"
	metadata := &appImageMetadata{}
	icons, err := backend.installIcons(squashfsRoot, normalizedName, metadata)
	_ = icons
	_ = err
}

func TestAppImageBackend_Detect(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	ctx := context.Background()

	t.Run("non-existent file", func(t *testing.T) {
		isAppImage, err := backend.Detect(ctx, "/nonexistent/file.AppImage")
		assert.NoError(t, err)
		assert.False(t, isAppImage)
	})

	t.Run("file without AppImage extension", func(t *testing.T) {
		otherFile := filepath.Join(tmpDir, "test.txt")
		require.NoError(t, os.WriteFile(otherFile, []byte("not an appimage"), 0644))

		isAppImage, err := backend.Detect(ctx, otherFile)
		assert.NoError(t, err)
		assert.False(t, isAppImage)
	})

	t.Run("file with AppImage extension but invalid content", func(t *testing.T) {
		appImageFile := filepath.Join(tmpDir, "test.AppImage")
		require.NoError(t, os.WriteFile(appImageFile, []byte("not a real appimage"), 0755))

		isAppImage, err := backend.Detect(ctx, appImageFile)
		assert.NoError(t, err)
		assert.False(t, isAppImage)
	})
}

func TestAppImageBackend_Uninstall(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	record := &core.InstallRecord{
		InstallID:    "test-123",
		Name:         "testapp",
		PackageType:  "appimage",
		InstallPath:  "",
		Metadata: core.Metadata{
			IconFiles: []string{},
		},
		DesktopFile: "",
	}

	ctx := context.Background()
	err := backend.Uninstall(ctx, record)
	_ = err
}

func TestAppImageBackend_parseAppImageMetadata(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}
	logger := zerolog.New(io.Discard)
	backend := New(cfg, &logger)

	// Create squashfs root with .desktop file
	squashfsRoot := tmpDir
	applicationsDir := filepath.Join(squashfsRoot, "usr", "share", "applications")
	require.NoError(t, os.MkdirAll(applicationsDir, 0755))

	desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
Icon=testapp
Categories=Utility;`
	desktopPath := filepath.Join(applicationsDir, "testapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

	metadata, err := backend.parseAppImageMetadata(squashfsRoot)
	assert.NoError(t, err)
	assert.NotNil(t, metadata)
	_ = metadata
}
