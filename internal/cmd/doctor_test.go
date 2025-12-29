package cmd

import (
	"bytes"
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

func TestCheckDependency(t *testing.T) {
	t.Run("existing command", func(t *testing.T) {
		result := checkDependency("ls", "ls", "list files", true)
		assert.True(t, result)
	})

	t.Run("non-existing command", func(t *testing.T) {
		result := checkDependency("nonexistentcommand123", "test", "test purpose", true)
		assert.False(t, result)
	})
}

func TestCheckDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("directory exists and is writable", func(t *testing.T) {
		testDir := filepath.Join(tmpDir, "exists")
		require.NoError(t, os.MkdirAll(testDir, 0755))
		result := checkDirectory(testDir, "test", false)
		assert.True(t, result)
	})

	t.Run("directory doesn't exist without fix", func(t *testing.T) {
		testDir := filepath.Join(tmpDir, "nonexistent")
		result := checkDirectory(testDir, "test", false)
		assert.False(t, result)
	})

	t.Run("directory doesn't exist with fix", func(t *testing.T) {
		testDir := filepath.Join(tmpDir, "create_me")
		result := checkDirectory(testDir, "test", true)
		assert.True(t, result)
		_, err := os.Stat(testDir)
		assert.NoError(t, err)
	})

	t.Run("path is a file", func(t *testing.T) {
		testFile := filepath.Join(tmpDir, "file.txt")
		require.NoError(t, os.WriteFile(testFile, []byte("test"), 0644))
		result := checkDirectory(testFile, "test", false)
		assert.False(t, result)
	})

	t.Run("directory not writable", func(t *testing.T) {
		testDir := filepath.Join(tmpDir, "readonly")
		require.NoError(t, os.MkdirAll(testDir, 0555))
		defer os.Chmod(testDir, 0755)
		result := checkDirectory(testDir, "test", false)
		// May fail or succeed depending on permissions
		// Just ensure it doesn't panic
		_ = result
	})
}

func TestGetDesktopFilesFromDB(t *testing.T) {
	t.Run("with desktop_files array", func(t *testing.T) {
		install := db.Install{
			Metadata: map[string]interface{}{
				"desktop_files": []string{"/path/to/file1.desktop", "/path/to/file2.desktop"},
			},
		}
		files := getDesktopFilesFromDB(install)
		assert.Equal(t, []string{"/path/to/file1.desktop", "/path/to/file2.desktop"}, files)
	})

	t.Run("with desktop_files interface array", func(t *testing.T) {
		install := db.Install{
			Metadata: map[string]interface{}{
				"desktop_files": []interface{}{"/path/to/file1.desktop", "/path/to/file2.desktop"},
			},
		}
		files := getDesktopFilesFromDB(install)
		assert.Equal(t, []string{"/path/to/file1.desktop", "/path/to/file2.desktop"}, files)
	})

	t.Run("with desktop_file field", func(t *testing.T) {
		install := db.Install{
			DesktopFile: "/path/to/single.desktop",
		}
		files := getDesktopFilesFromDB(install)
		assert.Equal(t, []string{"/path/to/single.desktop"}, files)
	})

	t.Run("empty metadata", func(t *testing.T) {
		install := db.Install{}
		files := getDesktopFilesFromDB(install)
		assert.Empty(t, files)
	})
}

func TestIsSystemManagedInstall(t *testing.T) {
	t.Run("pacman method in metadata", func(t *testing.T) {
		install := db.Install{
			Metadata: map[string]interface{}{
				"install_method": core.InstallMethodPacman,
			},
		}
		assert.True(t, isSystemManagedInstall(install))
	})

	t.Run("other method", func(t *testing.T) {
		install := db.Install{
			Metadata: map[string]interface{}{
				"install_method": "manual",
			},
		}
		assert.False(t, isSystemManagedInstall(install))
	})

	t.Run("backward compatibility - pacman path", func(t *testing.T) {
		install := db.Install{
			InstallPath: "/usr/share/pacman/package",
		}
		assert.True(t, isSystemManagedInstall(install))
	})

	t.Run("normal path", func(t *testing.T) {
		install := db.Install{
			InstallPath: "/opt/upkg/package",
		}
		assert.False(t, isSystemManagedInstall(install))
	})
}

func TestCheckPackageIntegrity(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("system managed install", func(t *testing.T) {
		installs := []db.Install{
			{
				Name:        "pacman-pkg",
				InstallPath: "/usr/bin/pacman-pkg",
				Metadata:    map[string]interface{}{"install_method": core.InstallMethodPacman},
			},
		}
		broken := checkPackageIntegrity(installs)
		assert.Empty(t, broken)
	})

	t.Run("missing install path", func(t *testing.T) {
		installs := []db.Install{
			{
				Name:        "missing-pkg",
				InstallID:   "missing-123",
				InstallPath: filepath.Join(tmpDir, "nonexistent"),
			},
		}
		broken := checkPackageIntegrity(installs)
		assert.Len(t, broken, 1)
		assert.Equal(t, "missing-pkg", broken[0].install.Name)
		assert.Len(t, broken[0].missing, 1)
	})

	t.Run("missing desktop file", func(t *testing.T) {
		installs := []db.Install{
			{
				Name:        "desktop-pkg",
				InstallID:   "desktop-123",
				InstallPath: tmpDir,
				DesktopFile: filepath.Join(tmpDir, "missing.desktop"),
			},
		}
		broken := checkPackageIntegrity(installs)
		assert.Len(t, broken, 1)
		assert.Len(t, broken[0].missing, 1)
	})

	t.Run("missing wrapper script", func(t *testing.T) {
		installs := []db.Install{
			{
				Name:        "wrapper-pkg",
				InstallID:   "wrapper-123",
				InstallPath: tmpDir,
				Metadata: map[string]interface{}{
					"wrapper_script": filepath.Join(tmpDir, "missing.sh"),
				},
			},
		}
		broken := checkPackageIntegrity(installs)
		assert.Len(t, broken, 1)
	})

	t.Run("missing icon files", func(t *testing.T) {
		installs := []db.Install{
			{
				Name:        "icon-pkg",
				InstallID:   "icon-123",
				InstallPath: tmpDir,
				Metadata: map[string]interface{}{
					"icon_files": []string{filepath.Join(tmpDir, "icon1.png"), filepath.Join(tmpDir, "icon2.png")},
				},
			},
		}
		broken := checkPackageIntegrity(installs)
		assert.Len(t, broken, 1)
		assert.Len(t, broken[0].missing, 2)
	})

	t.Run("intact package", func(t *testing.T) {
		// Create actual files
		installPath := filepath.Join(tmpDir, "intact")
		require.NoError(t, os.MkdirAll(installPath, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(installPath, "binary"), []byte("test"), 0755))

		desktopFile := filepath.Join(tmpDir, "intact.desktop")
		require.NoError(t, os.WriteFile(desktopFile, []byte("[Desktop Entry]"), 0644))

		wrapperScript := filepath.Join(tmpDir, "wrapper.sh")
		require.NoError(t, os.WriteFile(wrapperScript, []byte("#!/bin/sh"), 0755))

		iconFile := filepath.Join(tmpDir, "icon.png")
		require.NoError(t, os.WriteFile(iconFile, []byte("icon"), 0644))

		installs := []db.Install{
			{
				Name:        "intact-pkg",
				InstallID:   "intact-123",
				InstallPath: installPath,
				DesktopFile: desktopFile,
				Metadata: map[string]interface{}{
					"wrapper_script": wrapperScript,
					"icon_files":     []string{iconFile},
				},
			},
		}
		broken := checkPackageIntegrity(installs)
		assert.Empty(t, broken)
	})
}

func TestNewDoctorCmd(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
			LogFile: filepath.Join(tmpDir, "test.log"),
		},
	}
	log := zerolog.Nop()

	t.Run("creates doctor command", func(t *testing.T) {
		cmd := NewDoctorCmd(cfg, &log)
		assert.NotNil(t, cmd)
		assert.Equal(t, "doctor", cmd.Use)
		assert.Equal(t, "Check system dependencies and integrity", cmd.Short)
	})

	t.Run("executes without flags", func(_ *testing.T) {
		cmd := NewDoctorCmd(cfg, &log)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		err := cmd.Execute()
		// May fail due to missing dependencies, but should not panic
		_ = err
	})

	t.Run("executes with verbose flag", func(_ *testing.T) {
		cmd := NewDoctorCmd(cfg, &log)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--verbose"})
		err := cmd.Execute()
		_ = err
	})

	t.Run("executes with fix flag", func(t *testing.T) {
		// Create a config with non-existent directory
		fixCfg := &config.Config{
			Paths: config.PathsConfig{
				DataDir: filepath.Join(tmpDir, "fix_test"),
				DBFile:  filepath.Join(tmpDir, "fix_test", "test.db"),
				LogFile: filepath.Join(tmpDir, "fix_test", "test.log"),
			},
		}
		cmd := NewDoctorCmd(fixCfg, &log)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"--fix"})
		err := cmd.Execute()
		// Should create directories
		_, statErr := os.Stat(fixCfg.Paths.DataDir)
		assert.NoError(t, statErr)
		_ = err
	})
}

func TestCheckEnvironment(t *testing.T) {
	t.Parallel()

	// Save and restore original env vars
	origVars := map[string]string{
		"XDG_DATA_HOME":              os.Getenv("XDG_DATA_HOME"),
		"XDG_CONFIG_HOME":             os.Getenv("XDG_CONFIG_HOME"),
		"XDG_CACHE_HOME":              os.Getenv("XDG_CACHE_HOME"),
		"WAYLAND_DISPLAY":             os.Getenv("WAYLAND_DISPLAY"),
		"HYPRLAND_INSTANCE_SIGNATURE": os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"),
	}
	defer func() {
		for k, v := range origVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Test with no env vars set
	t.Run("with no env vars", func(t *testing.T) {
		os.Unsetenv("XDG_DATA_HOME")
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("XDG_CACHE_HOME")

		checkEnvironment()
		// Function doesn't error, just prints
	})

	// Test with env vars set
	t.Run("with env vars set", func(t *testing.T) {
		os.Setenv("XDG_DATA_HOME", "/test/data")
		os.Setenv("WAYLAND_DISPLAY", "wayland-0")

		checkEnvironment()
		// Function doesn't error, just prints
	})
}
