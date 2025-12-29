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

func TestNewListCmd(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)

	cmd := NewListCmd(cfg, &log)

	assert.NotNil(t, cmd)
	assert.Contains(t, cmd.Use, "list")
	assert.Equal(t, "List installed packages", cmd.Short)
}

func TestListCmd_EmptyDatabase(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{})
	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_WithPackages(t *testing.T) {
	t.Parallel()

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

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_JSONOutput(t *testing.T) {
	t.Parallel()

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
		InstallID:    "test-id-456",
		PackageType:  "appimage",
		Name:         "MyApp",
		Version:      "2.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/myapp.AppImage",
		InstallPath:  "/opt/myapp",
		Metadata:     map[string]interface{}{},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--json"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_Flags(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	assert.NotNil(t, cmd.Flags().Lookup("json"))
	assert.NotNil(t, cmd.Flags().Lookup("type"))
	assert.NotNil(t, cmd.Flags().Lookup("name"))
	assert.NotNil(t, cmd.Flags().Lookup("sort"))
	assert.NotNil(t, cmd.Flags().Lookup("details"))
}

func TestSortInstalls(t *testing.T) {
	t.Parallel()

	now := time.Now()

	installs := []db.Install{
		{InstallID: "1", Name: "Zebra", PackageType: "appimage", Version: "3.0", InstallDate: now},
		{InstallID: "2", Name: "Apple", PackageType: "tarball", Version: "1.0", InstallDate: now.Add(-1 * time.Hour)},
		{InstallID: "3", Name: "Beta", PackageType: "deb", Version: "2.0", InstallDate: now.Add(-2 * time.Hour)},
	}

	// Test sorting by name
	sortedByType := make([]db.Install, len(installs))
	copy(sortedByType, installs)
	sortInstalls(sortedByType, "name")
	assert.Equal(t, "Apple", sortedByType[0].Name)

	// Test sorting by type
	sortedByType = make([]db.Install, len(installs))
	copy(sortedByType, installs)
	sortInstalls(sortedByType, "type")
	assert.Equal(t, "appimage", sortedByType[0].PackageType)

	// Test sorting by date
	sortedByType = make([]db.Install, len(installs))
	copy(sortedByType, installs)
	sortInstalls(sortedByType, "date")
	assert.Equal(t, "Zebra", sortedByType[0].Name) // Most recent

	// Test sorting by version
	sortedByType = make([]db.Install, len(installs))
	copy(sortedByType, installs)
	sortInstalls(sortedByType, "version")
	assert.Equal(t, "1.0", sortedByType[0].Version)

	// Test invalid sort field (defaults to name)
	sortedByType = make([]db.Install, len(installs))
	copy(sortedByType, installs)
	sortInstalls(sortedByType, "invalid")
	assert.Equal(t, "Apple", sortedByType[0].Name)
}
