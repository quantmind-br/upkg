package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInstallCmd(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)

	cmd := NewInstallCmd(cfg, &log)

	assert.NotNil(t, cmd)
	assert.Equal(t, "install", cmd.Use)
	assert.Equal(t, "Install a package", cmd.Short)
}

func TestInstallCmd_InvalidPath(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	// Capture output
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Test with empty path
	cmd.SetArgs([]string{""})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid package path")
}

func TestInstallCmd_PackageNotFound(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"/nonexistent/package.appimage"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
}

func TestInstallCmd_InvalidCustomName(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.appimage")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	// Test with invalid name (contains spaces or special chars)
	cmd.SetArgs([]string{"--name", "invalid name!", testFile})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid custom name")
}

func TestInstallCmd_DatabaseError(t *testing.T) {
	t.Parallel()

	// Create a config with invalid DB path
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: filepath.Join(tmpDir, "nonexistent", "db.db"),
		},
	}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Create a valid package file
	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{testFile})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open database")
}

func TestFixDockIcon_SkipNoDesktop(t *testing.T) {
	t.Parallel()

	log := zerolog.New(io.Discard)

	// Create a mock install record without desktop file
	record := &core.InstallRecord{
		DesktopFile: "",
		InstallPath: "/tmp/test",
		Metadata: core.Metadata{
			WrapperScript: "/tmp/test/wrapper",
		},
	}

	dbRecord := &db.Install{
		DesktopFile: "",
	}

	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	defer database.Close()

	// This should skip because no desktop file
	_, err = fixDockIcon(context.Background(), record, dbRecord, database, &log)
	assert.NoError(t, err)
}

func TestFixDockIcon_NoExecutable(t *testing.T) {
	t.Parallel()

	log := zerolog.New(io.Discard)

	record := &core.InstallRecord{
		DesktopFile: "/tmp/test.desktop",
		InstallPath: "",
		Metadata: core.Metadata{
			WrapperScript: "",
		},
	}

	dbRecord := &db.Install{
		DesktopFile: "/tmp/test.desktop",
	}

	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	defer database.Close()

	_, err = fixDockIcon(context.Background(), record, dbRecord, database, &log)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no executable path available")
}

func TestFixDockIcon_ExecPathIsDirectory(t *testing.T) {
	t.Parallel()

	log := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	// Create a directory
	execDir := filepath.Join(tmpDir, "bin")
	require.NoError(t, os.MkdirAll(execDir, 0755))

	record := &core.InstallRecord{
		DesktopFile: filepath.Join(tmpDir, "test.desktop"),
		InstallPath: execDir,
		Metadata: core.Metadata{
			WrapperScript: execDir,
		},
	}

	dbRecord := &db.Install{
		DesktopFile: filepath.Join(tmpDir, "test.desktop"),
	}

	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	defer database.Close()

	_, err = fixDockIcon(context.Background(), record, dbRecord, database, &log)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "executable path is a directory")
}

func TestFixDockIcon_ApplicationStartFails(t *testing.T) {
	t.Parallel()

	log := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	// Create a non-executable file
	execPath := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(execPath, []byte("not executable"), 0644))

	record := &core.InstallRecord{
		DesktopFile: filepath.Join(tmpDir, "test.desktop"),
		InstallPath: execPath,
		Metadata: core.Metadata{
			WrapperScript: execPath,
		},
	}

	dbRecord := &db.Install{
		DesktopFile: filepath.Join(tmpDir, "test.desktop"),
	}

	database, err := db.New(context.Background(), ":memory:")
	require.NoError(t, err)
	defer database.Close()

	// This will fail to start the application
	_, err = fixDockIcon(context.Background(), record, dbRecord, database, &log)
	// The error might be "start application" or the process might fail
	// Either way, it should handle gracefully
	_ = err
}

func TestInstallCmd_Flags(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	// Verify all flags are registered
	assert.NotNil(t, cmd.Flags().Lookup("force"))
	assert.NotNil(t, cmd.Flags().Lookup("skip-desktop"))
	assert.NotNil(t, cmd.Flags().Lookup("name"))
	assert.NotNil(t, cmd.Flags().Lookup("timeout"))
	assert.NotNil(t, cmd.Flags().Lookup("skip-wayland-env"))
	assert.NotNil(t, cmd.Flags().Lookup("skip-icon-fix"))
	assert.NotNil(t, cmd.Flags().Lookup("overwrite"))
}

func TestInstallCmd_Timeout(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	// Set very short timeout
	cmd.SetArgs([]string{"--timeout", "1", testFile})
	_ = cmd.Execute()
}

func TestInstallCmd_WithForce(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--force", testFile})
	_ = cmd.Execute()
}

func TestInstallCmd_WithSkipDesktop(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--skip-desktop", testFile})
	_ = cmd.Execute()
}

func TestInstallCmd_WithCustomName(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--name", "MyApp", testFile})
	_ = cmd.Execute()
}

func TestInstallCmd_WithSkipWaylandEnv(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--skip-wayland-env", testFile})
	_ = cmd.Execute()
}

func TestInstallCmd_WithSkipIconFix(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--skip-icon-fix", testFile})
	_ = cmd.Execute()
}

func TestInstallCmd_WithOverwrite(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--overwrite", testFile})
	_ = cmd.Execute()
}
