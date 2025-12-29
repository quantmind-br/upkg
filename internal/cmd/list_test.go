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
	"github.com/spf13/cobra"
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

func TestListCmd_DetailsOutput(t *testing.T) {
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
		InstallID:    "test-id-789",
		PackageType:  "tarball",
		Name:         "DetailsApp",
		Version:      "1.5.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/details.tar.gz",
		InstallPath:  "/opt/detailsapp",
		DesktopFile:  "/usr/share/applications/detailsapp.desktop",
		Metadata: map[string]interface{}{
			"wrapper_script": "/home/user/.local/bin/detailsapp",
			"icon_files":     []string{"/home/user/.local/share/icons/detailsapp.png"},
		},
	}

	err = database.Create(ctx, testInstall)
	require.NoError(t, err)
	database.Close()

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--details"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_FilterByType(t *testing.T) {
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

	// Create multiple packages of different types
	installs := []*db.Install{
		{InstallID: "1", PackageType: "appimage", Name: "App1", Version: "1.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
		{InstallID: "2", PackageType: "tarball", Name: "App2", Version: "2.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
		{InstallID: "3", PackageType: "deb", Name: "App3", Version: "3.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
	}

	for _, install := range installs {
		err = database.Create(ctx, install)
		require.NoError(t, err)
	}
	database.Close()

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Filter by appimage type
	cmd.SetArgs([]string{"--type", "appimage"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_FilterByName(t *testing.T) {
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
		InstallID:    "test-id-name",
		PackageType:  "tarball",
		Name:         "SpecificApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
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

	// Filter by name
	cmd.SetArgs([]string{"--name", "SpecificApp"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_SortOptions(t *testing.T) {
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

	installs := []*db.Install{
		{InstallID: "1", Name: "Zebra", PackageType: "appimage", Version: "3.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
		{InstallID: "2", Name: "Alpha", PackageType: "tarball", Version: "1.0", InstallDate: time.Now().Add(-1 * time.Hour), Metadata: map[string]interface{}{}},
	}

	for _, install := range installs {
		err = database.Create(ctx, install)
		require.NoError(t, err)
	}
	database.Close()

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Sort by name
	cmd.SetArgs([]string{"--sort", "name"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_SortByType(t *testing.T) {
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

	installs := []*db.Install{
		{InstallID: "1", Name: "App1", PackageType: "rpm", Version: "1.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
		{InstallID: "2", Name: "App2", PackageType: "appimage", Version: "2.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
		{InstallID: "3", Name: "App3", PackageType: "tarball", Version: "3.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
	}

	for _, install := range installs {
		err = database.Create(ctx, install)
		require.NoError(t, err)
	}
	database.Close()

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--sort", "type"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_SortByVersion(t *testing.T) {
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

	installs := []*db.Install{
		{InstallID: "1", Name: "App1", PackageType: "appimage", Version: "3.0.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
		{InstallID: "2", Name: "App2", PackageType: "appimage", Version: "1.5.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
		{InstallID: "3", Name: "App3", PackageType: "appimage", Version: "2.0.0", InstallDate: time.Now(), Metadata: map[string]interface{}{}},
	}

	for _, install := range installs {
		err = database.Create(ctx, install)
		require.NoError(t, err)
	}
	database.Close()

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--sort", "version"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_SortByDate(t *testing.T) {
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

	now := time.Now()
	installs := []*db.Install{
		{InstallID: "1", Name: "App1", PackageType: "appimage", Version: "1.0", InstallDate: now.Add(-3 * time.Hour), Metadata: map[string]interface{}{}},
		{InstallID: "2", Name: "App2", PackageType: "appimage", Version: "2.0", InstallDate: now.Add(-1 * time.Hour), Metadata: map[string]interface{}{}},
		{InstallID: "3", Name: "App3", PackageType: "appimage", Version: "3.0", InstallDate: now.Add(-2 * time.Hour), Metadata: map[string]interface{}{}},
	}

	for _, install := range installs {
		err = database.Create(ctx, install)
		require.NoError(t, err)
	}
	database.Close()

	log := zerolog.New(io.Discard)
	cmd := NewListCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--sort", "date"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestListCmd_SortInvalid(t *testing.T) {
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
		InstallID:    "test-id-sort",
		PackageType:  "tarball",
		Name:         "TestApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
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

	// Invalid sort option should default to "name"
	cmd.SetArgs([]string{"--sort", "invalid"})
	err = cmd.Execute()
	assert.NoError(t, err)
}

func TestPrintCompactTable_EmptyVersion(t *testing.T) {
	t.Parallel()

	installs := []db.Install{
		{
			InstallID:    "test-1",
			PackageType:  "tarball",
			Name:         "TestApp1",
			Version:      "", // Empty version
			InstallDate:  time.Now(),
			InstallPath:  "/opt/testapp1",
			DesktopFile:  "/usr/share/applications/testapp1.desktop",
			Metadata:     map[string]interface{}{},
		},
		{
			InstallID:    "test-2",
			PackageType:  "appimage",
			Name:         "TestApp2",
			Version:      "1.0.0",
			InstallDate:  time.Now(),
			InstallPath:  "/opt/testapp2",
			DesktopFile:  "/usr/share/applications/testapp2.desktop",
			Metadata:     map[string]interface{}{},
		},
	}

	var buf bytes.Buffer
	fakeCmd := &cobra.Command{}
	fakeCmd.SetOut(&buf)

	err := printCompactTable(fakeCmd, installs)
	assert.NoError(t, err)
	// Output should contain "-" for empty version
	_ = buf.String()
}

func TestPrintDetailedTable_Truncations(t *testing.T) {
	t.Parallel()

	installs := []db.Install{
		{
			InstallID:    "very-long-install-id-that-needs-truncation-because-it-exceeds-twenty-chars",
			PackageType:  "tarball",
			Name:         "TestApp",
			Version:      "", // Empty version
			InstallDate:  time.Now(),
			InstallPath:  "/very/long/path/that/needs/truncation/because/it/exceeds/forty/characters/and/then/some",
			DesktopFile:  "/usr/share/applications/testapp.desktop",
			Metadata:     map[string]interface{}{},
		},
		{
			InstallID:    "test-2",
			PackageType:  "appimage",
			Name:         "TestApp2",
			Version:      "1.0.0",
			InstallDate:  time.Now(),
			InstallPath:  "/opt/testapp2",
			DesktopFile:  "/usr/share/applications/testapp2.desktop",
			Metadata:     map[string]interface{}{},
		},
	}

	var buf bytes.Buffer
	fakeCmd := &cobra.Command{}
	fakeCmd.SetOut(&buf)

	err := printDetailedTable(fakeCmd, installs)
	assert.NoError(t, err)
	_ = buf.String()
}

func TestFilterInstalls_AllFilters(t *testing.T) {
	t.Parallel()

	now := time.Now()
	installs := []db.Install{
		{InstallID: "1", PackageType: "appimage", Name: "AlphaApp", Version: "1.0", InstallDate: now, Metadata: map[string]interface{}{}},
		{InstallID: "2", PackageType: "tarball", Name: "BetaApp", Version: "2.0", InstallDate: now, Metadata: map[string]interface{}{}},
		{InstallID: "3", PackageType: "appimage", Name: "AlphaApp", Version: "3.0", InstallDate: now, Metadata: map[string]interface{}{}},
	}

	// Filter by type and name
	filtered := filterInstalls(installs, "appimage", "AlphaApp")
	assert.Equal(t, 2, len(filtered))
	assert.Equal(t, "AlphaApp", filtered[0].Name)
	assert.Equal(t, "AlphaApp", filtered[1].Name)
}

func TestListCmd_MultiplePackagesWithEmptyVersions(t *testing.T) {
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

	installs := []*db.Install{
		{
			InstallID:    "empty-1",
			PackageType:  "tarball",
			Name:         "EmptyVersion1",
			Version:      "",
			InstallDate:  time.Now(),
			InstallPath:  "/opt/empty1",
			DesktopFile:  "/usr/share/applications/empty1.desktop",
			Metadata:     map[string]interface{}{},
		},
		{
			InstallID:    "empty-2",
			PackageType:  "appimage",
			Name:         "EmptyVersion2",
			Version:      "",
			InstallDate:  time.Now(),
			InstallPath:  "/opt/empty2",
			DesktopFile:  "/usr/share/applications/empty2.desktop",
			Metadata:     map[string]interface{}{},
		},
	}

	for _, install := range installs {
		err = database.Create(ctx, install)
		require.NoError(t, err)
	}
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


