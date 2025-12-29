package cmd

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInfoCmd(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	cmd := NewInfoCmd(cfg, &logger)

	assert.NotNil(t, cmd)
	assert.Contains(t, cmd.Use, "info")
}

func TestInfoCmd_PrintPackageInfo(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-123",
		PackageType:  "tarball",
		Name:         "TestApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/test.tar.gz",
		InstallPath:  "/opt/testapp",
		DesktopFile:  "/usr/share/applications/testapp.desktop",
		Metadata:     map[string]interface{}{},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Test with valid install ID
	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_PrintPackageInfo_NotFound(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	// Create empty database
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Test with non-existent install ID
	cmd.SetArgs([]string{"nonexistent-id"})
	err = cmd.Execute()
	// Should error because package not found
	assert.Error(t, err)
}

func TestInfoCmd_PrintPackageInfo_WithMetadata(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-meta",
		PackageType:  "appimage",
		Name:         "MetaApp",
		Version:      "2.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/meta.AppImage",
		InstallPath:  "/opt/metaapp",
		DesktopFile:  "/usr/share/applications/metaapp.desktop",
		Metadata: map[string]interface{}{
			"wrapper_script": "/home/user/.local/bin/metaapp",
			"icon_files":     []string{"/home/user/.local/share/icons/metaapp.png"},
		},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_NoArgs(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Test with no arguments
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	// Should require an argument
	assert.Error(t, err)
}

func TestInfoCmd_WithWaylandSupport(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-wayland",
		PackageType:  "tarball",
		Name:         "WaylandApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/wayland.tar.gz",
		InstallPath:  "/opt/waylandapp",
		DesktopFile:  "/usr/share/applications/waylandapp.desktop",
		Metadata: map[string]interface{}{
			"wayland_support": "native",
			"icon_files":      []string{"/path/to/icon.png"},
		},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_WithOtherMetadata(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-other",
		PackageType:  "tarball",
		Name:         "OtherMetaApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/other.tar.gz",
		InstallPath:  "/opt/otherapp",
		DesktopFile:  "/usr/share/applications/otherapp.desktop",
		Metadata: map[string]interface{}{
			"custom_field":     "custom_value",
			"install_method":   "user",
			"wrapper_script":   "/home/user/.local/bin/otherapp",
			"icon_files":       []string{"/path/to/icon.png"},
			"wayland_support":  "electron",
		},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_WithComplexMetadata(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-complex",
		PackageType:  "appimage",
		Name:         "ComplexMetaApp",
		Version:      "2.5.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/complex.AppImage",
		InstallPath:  "/opt/complexapp",
		DesktopFile:  "/usr/share/applications/complexapp.desktop",
		Metadata: map[string]interface{}{
			"wrapper_script":       "/home/user/.local/bin/complexapp",
			"icon_files":           []string{"/path/to/icon1.png", "/path/to/icon2.svg"},
			"wayland_support":      "native",
			"architecture":         "x86_64",
			"license":              "MIT",
			"homepage":             "https://example.com",
			"repository":           "https://github.com/example/app",
			"original_desktop_file": "/tmp/original.desktop",
			"desktop_files":        []string{"/path/to/extra.desktop"},
		},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_AllMetadataTypes(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-alltypes",
		PackageType:  "binary",
		Name:         "AllTypesApp",
		Version:      "3.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/usr/local/bin/alltypes",
		InstallPath:  "/opt/alltypes",
		Metadata: map[string]interface{}{
			"string_field":  "string_value",
			"number_field":   42,
			"bool_field":     true,
			"slice_field":    []string{"a", "b", "c"},
			"wrapper_script": "/home/user/.local/bin/alltypes",
		},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_EmptyVersion(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-noversion",
		PackageType:  "tarball",
		Name:         "NoVersionApp",
		Version:      "", // Empty version
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/noversion.tar.gz",
		InstallPath:  "/opt/noversionapp",
		DesktopFile:  "/usr/share/applications/noversionapp.desktop",
		Metadata:     map[string]interface{}{},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_EmptyDesktopFile(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-nodesktop",
		PackageType:  "binary",
		Name:         "NoDesktopApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/usr/local/bin/nodesktop",
		InstallPath:  "/opt/nodesktop",
		DesktopFile:  "", // Empty desktop file
		Metadata:     map[string]interface{}{},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_IconFilesAsInterfaceSlice(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-interfaceicons",
		PackageType:  "appimage",
		Name:         "InterfaceIconsApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/interfaceicons.AppImage",
		InstallPath:  "/opt/interfaceicons",
		DesktopFile:  "/usr/share/applications/interfaceicons.desktop",
		Metadata: map[string]interface{}{
			// icon_files as []interface{} instead of []string
			"icon_files":     []interface{}{"/path/to/icon1.png", "/path/to/icon2.svg"},
			"wrapper_script": "/home/user/.local/bin/interfaceicons",
		},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{testInstall.InstallID})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_SearchByName(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-search",
		PackageType:  "tarball",
		Name:         "SearchByNameApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/search.tar.gz",
		InstallPath:  "/opt/searchapp",
		DesktopFile:  "/usr/share/applications/searchapp.desktop",
		Metadata:     map[string]interface{}{},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Search by name (not ID)
	cmd.SetArgs([]string{"SearchByNameApp"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestInfoCmd_SearchByNameCaseInsensitive(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: dbPath,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-case",
		PackageType:  "tarball",
		Name:         "CaseSensitiveApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/case.tar.gz",
		InstallPath:  "/opt/caseapp",
		DesktopFile:  "/usr/share/applications/caseapp.desktop",
		Metadata:     map[string]interface{}{},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	cmd := NewInfoCmd(cfg, &logger)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Search with different case
	cmd.SetArgs([]string{"casesensitiveapp"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

