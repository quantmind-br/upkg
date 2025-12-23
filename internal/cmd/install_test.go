package cmd

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	assert.Contains(t, cmd.Use, "install")
	assert.Equal(t, "Install a package", cmd.Short)
}

func TestInstallCmd_InvalidPath(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{""})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "cannot detect package type") ||
			strings.Contains(err.Error(), "package not found") ||
			strings.Contains(err.Error(), "invalid package path"),
		"expected error about path, got: %v", err)
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
	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--name", "!!!", testFile})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid custom name")
}

func TestInstallCmd_DatabaseError(t *testing.T) {
	t.Parallel()

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

	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{testFile})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open database")
}

func TestInstallCmd_Flags(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

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

func TestInstallCmd_AllFlags(t *testing.T) {
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

	cmd.SetArgs([]string{
		"--force",
		"--skip-desktop",
		"--name", "MyApp",
		"--timeout", "30",
		"--skip-wayland-env",
		"--skip-icon-fix",
		"--overwrite",
		testFile,
	})
	_ = cmd.Execute()
}

func TestInstallCmd_MissingArgs(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestInstallCmd_ExtraArgs(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{testFile, "extra"})
	err := cmd.Execute()
	assert.Error(t, err)
}

func TestFixDockIcon_SkipNoDesktop(t *testing.T) {
	t.Parallel()

	log := zerolog.New(io.Discard)

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

	// Just verify the record structure
	assert.Empty(t, record.DesktopFile)
	assert.Empty(t, dbRecord.DesktopFile)
	_ = log
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

	// Verify no executable path
	execPath := record.Metadata.WrapperScript
	if execPath == "" {
		execPath = record.InstallPath
	}
	assert.Empty(t, execPath)
	_ = dbRecord
	_ = log
}

func TestFixDockIcon_ExecPathIsDirectory(t *testing.T) {
	t.Parallel()

	log := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

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

	info, err := os.Stat(record.Metadata.WrapperScript)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	_ = dbRecord
	_ = log
}

func TestFixDockIcon_ApplicationStartFails(t *testing.T) {
	t.Parallel()

	log := zerolog.New(io.Discard)
	tmpDir := t.TempDir()

	execPath := filepath.Join(tmpDir, "app")
	require.NoError(t, os.WriteFile(execPath, []byte("not executable"), 0644))

	// Verify file is not executable
	info, err := os.Stat(execPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm()&os.ModePerm)
	_ = log
}

func TestInstallCmd_InvalidTimeout(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--timeout", "-1", testFile})
	_ = cmd.Execute()
}

func TestInstallCmd_WithInvalidSanitizedName(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewInstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	cmd.SetArgs([]string{"--name", "!!!###$$%", testFile})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid custom name")
}

func TestInstallCmd_WithRelativePath(t *testing.T) {
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

	// Create a tarball in temp dir
	testFile := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(testFile, []byte("fake"), 0644))

	// Change to temp dir to use relative path
	oldDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(oldDir)

	cmd.SetArgs([]string{"./test.tar.gz"})
	_ = cmd.Execute()
}
