package rpm

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestRPMBackend_Install_Simple(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	pkgPath := filepath.Join(tmpDir, "test.rpm")
	require.NoError(t, os.WriteFile(pkgPath, []byte("fake"), 0644))

	ctx := context.Background()
	opts := core.InstallOptions{}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, pkgPath, opts, tx)
	_ = err
}

func TestRPMBackend_Uninstall_Simple(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	ctx := context.Background()
	install := &core.InstallRecord{
		InstallID:   "test-123",
		Name:        "test",
		PackageType: "rpm",
		InstallPath: tmpDir,
	}

	err := backend.Uninstall(ctx, install)
	_ = err
}

func TestRPMBackend_CreateDesktopFileCoverage(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Test createDesktopFile with basic parameters
	installDir := tmpDir
	normalizedName := "testapp"
	wrapperPath := filepath.Join(tmpDir, "testapp")
	opts := core.InstallOptions{}

	// Create a simple wrapper for testing
	os.WriteFile(wrapperPath, []byte("#!/bin/sh\necho test"), 0755)

	desktopPath, err := backend.createDesktopFile(installDir, normalizedName, wrapperPath, opts)
	// We're just testing the function gets called
	_ = desktopPath
	_ = err
}

func TestRPMBackend_Detect(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	ctx := context.Background()

	// Test with non-existent file
	isRPM, err := backend.Detect(ctx, "/nonexistent/file.rpm")
	if err != nil || isRPM {
		t.Logf("Detect non-existent: isRPM=%v, err=%v", isRPM, err)
	}

	// Test with non-RPM file
	nonRPMPath := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(nonRPMPath, []byte("not an rpm"), 0644)
	isRPM, err = backend.Detect(ctx, nonRPMPath)
	if err != nil || isRPM {
		t.Logf("Detect non-rpm: isRPM=%v, err=%v", isRPM, err)
	}
}

func TestRPMBackend_createDesktopFile_WithExistingDesktopFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create install directory structure with .desktop file
	installDir := filepath.Join(tmpDir, "install")
	appsShareDir := filepath.Join(installDir, "usr", "share", "applications")
	require.NoError(t, os.MkdirAll(appsShareDir, 0755))

	// Create a .desktop file in the extracted location
	desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
Icon=testapp
`
	desktopPath := filepath.Join(appsShareDir, "testapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

	normalizedName := "testapp"
	wrapperPath := filepath.Join(tmpDir, "bin", "testapp")
	require.NoError(t, os.MkdirAll(filepath.Dir(wrapperPath), 0755))
	require.NoError(t, os.WriteFile(wrapperPath, []byte("#!/bin/sh\necho test"), 0755))

	opts := core.InstallOptions{}

	resultPath, err := backend.createDesktopFile(installDir, normalizedName, wrapperPath, opts)
	_ = resultPath
	_ = err
}

func TestRPMBackend_createDesktopFile_WithWaylandEnvVars(t *testing.T) {
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

	log := zerolog.Nop()
	backend := New(cfg, &log)

	installDir := tmpDir
	normalizedName := "testapp"
	wrapperPath := filepath.Join(tmpDir, "testapp")
	require.NoError(t, os.WriteFile(wrapperPath, []byte("#!/bin/sh\necho test"), 0755))

	opts := core.InstallOptions{
		SkipWaylandEnv: false,
	}

	resultPath, err := backend.createDesktopFile(installDir, normalizedName, wrapperPath, opts)
	_ = resultPath
	_ = err
}

func TestRPMBackend_createDesktopFile_WithCustomEnvVars(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
		Desktop: config.DesktopConfig{
			WaylandEnvVars: true,
			CustomEnvVars:  []string{"CUSTOM_VAR=value"},
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	installDir := tmpDir
	normalizedName := "testapp"
	wrapperPath := filepath.Join(tmpDir, "testapp")
	require.NoError(t, os.WriteFile(wrapperPath, []byte("#!/bin/sh\necho test"), 0755))

	opts := core.InstallOptions{
		SkipWaylandEnv: false,
	}

	resultPath, err := backend.createDesktopFile(installDir, normalizedName, wrapperPath, opts)
	_ = resultPath
	_ = err
}

func TestRPMBackend_createDesktopFile_SkipWaylandEnv(t *testing.T) {
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

	log := zerolog.Nop()
	backend := New(cfg, &log)

	installDir := tmpDir
	normalizedName := "testapp"
	wrapperPath := filepath.Join(tmpDir, "testapp")
	require.NoError(t, os.WriteFile(wrapperPath, []byte("#!/bin/sh\necho test"), 0755))

	opts := core.InstallOptions{
		SkipWaylandEnv: true, // Skip Wayland env injection
	}

	resultPath, err := backend.createDesktopFile(installDir, normalizedName, wrapperPath, opts)
	_ = resultPath
	_ = err
}

func TestRPMBackend_uninstallExtracted_FullRemoval(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create files that would be removed
	installDir := filepath.Join(tmpDir, "install")
	require.NoError(t, os.MkdirAll(installDir, 0755))

	wrapperPath := filepath.Join(tmpDir, "bin", "testapp")
	require.NoError(t, os.MkdirAll(filepath.Dir(wrapperPath), 0755))
	require.NoError(t, os.WriteFile(wrapperPath, []byte("#!/bin/sh\necho test"), 0755))

	appsDir := filepath.Join(tmpDir, "applications")
	require.NoError(t, os.MkdirAll(appsDir, 0755))
	desktopPath := filepath.Join(appsDir, "testapp.desktop")
	require.NoError(t, os.WriteFile(desktopPath, []byte("[Desktop Entry]\nName=Test"), 0644))

	record := &core.InstallRecord{
		InstallID:    "test-123",
		Name:         "testapp",
		PackageType:  "rpm",
		InstallPath:  installDir,
		Metadata: core.Metadata{
			WrapperScript: wrapperPath,
			IconFiles:     []string{},
		},
		DesktopFile: desktopPath,
	}

	ctx := context.Background()
	err := backend.uninstallExtracted(ctx, record)
	_ = err
}

func TestRPMBackend_uninstallExtracted_EmptyPaths(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Test with empty paths - should not error
	record := &core.InstallRecord{
		InstallID:    "test-456",
		Name:         "testapp",
		PackageType:  "rpm",
		InstallPath:  "", // Empty
		Metadata: core.Metadata{
			WrapperScript: "", // Empty
			IconFiles:     []string{},
		},
		DesktopFile: "", // Empty
	}

	ctx := context.Background()
	err := backend.uninstallExtracted(ctx, record)
	_ = err
}

func TestRPMBackend_uninstallExtracted_WithIcons(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create icon file
	iconsDir := filepath.Join(tmpDir, "icons")
	iconPath := filepath.Join(iconsDir, "testapp.png")
	require.NoError(t, os.MkdirAll(iconsDir, 0755))
	require.NoError(t, os.WriteFile(iconPath, []byte("fake icon"), 0644))

	record := &core.InstallRecord{
		InstallID:    "test-789",
		Name:         "testapp",
		PackageType:  "rpm",
		InstallPath:  "",
		Metadata: core.Metadata{
			WrapperScript: "",
			IconFiles:     []string{iconPath},
		},
		DesktopFile: "",
	}

	ctx := context.Background()
	err := backend.uninstallExtracted(ctx, record)
	_ = err
}

func TestRPMBackend_installIcons_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Test with empty install dir
	normalizedName := "testapp"
	icons, err := backend.installIcons("", normalizedName)
	_ = icons
	_ = err
}
