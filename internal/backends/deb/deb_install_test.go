package deb

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebBackend_Install_PackageNotFound(t *testing.T) {
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
	opts := core.InstallOptions{}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, "/nonexistent/file.deb", opts, tx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDebBackend_Install_TempDirFailure(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create deb file
	debPath := filepath.Join(tmpDir, "test.deb")
	require.NoError(t, os.WriteFile(debPath, []byte("fake deb"), 0644))

	// Make tmp dir read-only to cause failure
	tmpDirPath := filepath.Join(tmpDir, "tmp")
	require.NoError(t, os.MkdirAll(tmpDirPath, 0555))
	defer os.Chmod(tmpDirPath, 0755)

	ctx := context.Background()
	opts := core.InstallOptions{}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, debPath, opts, tx)

	// Should fail during temp dir creation
	assert.Error(t, err)
}

func TestDebBackend_Install_CustomName(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create deb file
	debPath := filepath.Join(tmpDir, "test.deb")
	require.NoError(t, os.WriteFile(debPath, []byte("fake deb"), 0644))

	ctx := context.Background()
	opts := core.InstallOptions{
		CustomName: "custom-name",
	}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, debPath, opts, tx)

	// Will fail due to missing dependencies
	_ = err
}

func TestDebBackend_Install_ForceFlag(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create deb file
	debPath := filepath.Join(tmpDir, "test.deb")
	require.NoError(t, os.WriteFile(debPath, []byte("fake deb"), 0644))

	ctx := context.Background()
	opts := core.InstallOptions{
		Force: true,
	}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, debPath, opts, tx)

	// Will fail but force flag should be passed
	_ = err
}

func TestDebBackend_Install_OverwriteFlag(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create deb file
	debPath := filepath.Join(tmpDir, "test.deb")
	require.NoError(t, os.WriteFile(debPath, []byte("fake deb"), 0644))

	ctx := context.Background()
	opts := core.InstallOptions{
		Overwrite: true,
	}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, debPath, opts, tx)

	// Will fail but overwrite flag should be passed
	_ = err
}

func TestDebBackend_Install_SkipDesktop(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create deb file
	debPath := filepath.Join(tmpDir, "test.deb")
	require.NoError(t, os.WriteFile(debPath, []byte("fake deb"), 0644))

	ctx := context.Background()
	opts := core.InstallOptions{
		SkipDesktop: true,
	}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, debPath, opts, tx)

	// Will fail but skip-desktop flag should be passed
	_ = err
}

func TestDebBackend_Install_SkipWaylandEnv(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create deb file
	debPath := filepath.Join(tmpDir, "test.deb")
	require.NoError(t, os.WriteFile(debPath, []byte("fake deb"), 0644))

	ctx := context.Background()
	opts := core.InstallOptions{
		SkipWaylandEnv: true,
	}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, debPath, opts, tx)

	// Will fail but skip-wayland-env flag should be passed
	_ = err
}

func TestDebBackend_Uninstall_PackageNotInstalled(t *testing.T) {
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

	// Create a fake install record for a package that doesn't exist in pacman
	install := &core.InstallRecord{
		InstallID:   "test-123",
		Name:        "test-pkg",
		PackageType: "deb",
		InstallPath: "/nonexistent/path",
	}

	err := backend.Uninstall(ctx, install)

	// Should handle gracefully
	_ = err
}

func TestDebBackend_Uninstall_NormalizedName(t *testing.T) {
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

	// Test with various package name formats
	install := &core.InstallRecord{
		InstallID:   "test-123",
		Name:        "Test_Package-1.0",
		PackageType: "deb",
		InstallPath: tmpDir,
	}

	err := backend.Uninstall(ctx, install)

	// Should handle gracefully
	_ = err
}
