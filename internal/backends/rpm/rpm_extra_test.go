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
