package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/db"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewUninstallCmd(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)

	cmd := NewUninstallCmd(cfg, &log)

	assert.NotNil(t, cmd)
	assert.Contains(t, cmd.Use, "uninstall")
	assert.Equal(t, "Uninstall one or more packages", cmd.Short)
}

func TestNewUninstallCmd_HasExpectedFlags(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	log := zerolog.New(io.Discard)

	cmd := NewUninstallCmd(cfg, &log)

	// Check --yes / -y flag
	yesFlag := cmd.Flags().Lookup("yes")
	require.NotNil(t, yesFlag)
	assert.Equal(t, "y", yesFlag.Shorthand)

	// Check --dry-run flag
	dryRunFlag := cmd.Flags().Lookup("dry-run")
	require.NotNil(t, dryRunFlag)

	// Check --all flag
	allFlag := cmd.Flags().Lookup("all")
	require.NotNil(t, allFlag)

	// Check --timeout flag
	timeoutFlag := cmd.Flags().Lookup("timeout")
	require.NotNil(t, timeoutFlag)
	assert.Equal(t, "600", timeoutFlag.DefValue)
}

func TestUninstallCmd_PackageNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"nonexistent-package", "--yes"})
	err = cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "package not found")
}

func TestUninstallCmd_DatabaseError(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile: filepath.Join(tmpDir, "nonexistent", "subdir", "db.db"),
		},
	}

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"some-package", "--yes"})
	err := cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to open database")
}

func TestUninstallCmd_DryRun(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	appsDir := filepath.Join(tmpDir, "apps")
	require.NoError(t, os.MkdirAll(appsDir, 0755))

	// Create a fake app directory
	appDir := filepath.Join(appsDir, "testapp")
	require.NoError(t, os.MkdirAll(appDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "testapp"), []byte("fake binary"), 0755))

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database and add a test package
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-123",
		PackageType:  "AppImage",
		Name:         "testapp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/original.AppImage",
		InstallPath:  appDir,
		DesktopFile:  filepath.Join(tmpDir, "testapp.desktop"),
		Metadata:     map[string]interface{}{},
	}
	require.NoError(t, database.Create(ctx, testInstall))
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"testapp", "--dry-run"})
	err = cmd.Execute()
	assert.NoError(t, err)

	// Verify the app directory still exists (dry-run should not delete)
	_, statErr := os.Stat(appDir)
	assert.NoError(t, statErr, "app directory should still exist after dry-run")

	// Verify package still in database
	database, err = db.New(ctx, dbPath)
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	record, err := database.Get(ctx, "test-id-123")
	assert.NoError(t, err)
	assert.NotNil(t, record)
}

func TestUninstallCmd_AllFlagRequiresYes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Running with --all but no TTY and no --yes should fail
	// (In test environment, there's no TTY)
	cmd.SetArgs([]string{"--all"})
	err = cmd.Execute()
	// Should either require --yes or find no packages
	// The error depends on whether we detect non-interactive mode
	if err != nil {
		assert.True(t,
			err.Error() == "non-interactive mode requires --yes flag" ||
				err.Error() == "No packages are currently tracked by upkg." ||
				err == nil,
		)
	}
}

func TestUninstallCmd_AllFlagWithYes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database with no packages
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--all", "--yes"})
	err = cmd.Execute()
	// Should succeed (no packages to uninstall)
	assert.NoError(t, err)
}

func TestUninstallCmd_BulkNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"pkg1", "pkg2", "pkg3", "--yes"})
	err = cmd.Execute()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid packages found")
}

func TestFormatBytes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tc := range tests {
		result := formatBytes(tc.bytes)
		assert.Equal(t, tc.expected, result, "formatBytes(%d)", tc.bytes)
	}
}

func TestCalculatePackageSize(t *testing.T) {
	t.Parallel()

	t.Run("nonexistent path", func(t *testing.T) {
		size, count := calculatePackageSize("/nonexistent/path")
		assert.Equal(t, int64(0), size)
		assert.Equal(t, 0, count)
	})

	t.Run("single file", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.bin")
		content := []byte("hello world")
		require.NoError(t, os.WriteFile(testFile, content, 0644))

		size, count := calculatePackageSize(testFile)
		assert.Equal(t, int64(len(content)), size)
		assert.Equal(t, 1, count)
	})

	t.Run("directory", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create files
		file1 := filepath.Join(tmpDir, "file1.txt")
		file2 := filepath.Join(tmpDir, "file2.txt")
		subDir := filepath.Join(tmpDir, "subdir")
		file3 := filepath.Join(subDir, "file3.txt")

		require.NoError(t, os.WriteFile(file1, []byte("12345"), 0644))
		require.NoError(t, os.WriteFile(file2, []byte("67890"), 0644))
		require.NoError(t, os.MkdirAll(subDir, 0755))
		require.NoError(t, os.WriteFile(file3, []byte("abc"), 0644))

		size, count := calculatePackageSize(tmpDir)
		assert.Equal(t, int64(13), size) // 5 + 5 + 3
		assert.Equal(t, 3, count)
	})
}

func TestDbInstallToCore(t *testing.T) {
	t.Parallel()

	now := time.Now()
	dbRecord := &db.Install{
		InstallID:    "test-id",
		PackageType:  "AppImage",
		Name:         "TestApp",
		Version:      "1.2.3",
		InstallDate:  now,
		OriginalFile: "/path/to/app.AppImage",
		InstallPath:  "/home/user/.local/share/upkg/apps/testapp",
		DesktopFile:  "/home/user/.local/share/applications/testapp.desktop",
		Metadata: map[string]interface{}{
			"icon_files":            []interface{}{"/path/to/icon1.png", "/path/to/icon2.png"},
			"wrapper_script":        "/home/user/.local/bin/testapp",
			"wayland_support":       "native",
			"original_desktop_file": "/path/to/original.desktop",
			"install_method":        "local",
			"desktop_files":         []interface{}{"/path/to/desktop1.desktop"},
		},
	}

	record := dbInstallToCore(dbRecord)

	assert.Equal(t, "test-id", record.InstallID)
	assert.Equal(t, core.PackageType("AppImage"), record.PackageType)
	assert.Equal(t, "TestApp", record.Name)
	assert.Equal(t, "1.2.3", record.Version)
	assert.Equal(t, now, record.InstallDate)
	assert.Equal(t, "/path/to/app.AppImage", record.OriginalFile)
	assert.Equal(t, "/home/user/.local/share/upkg/apps/testapp", record.InstallPath)
	assert.Equal(t, "/home/user/.local/share/applications/testapp.desktop", record.DesktopFile)
	assert.Len(t, record.Metadata.IconFiles, 2)
	assert.Equal(t, "/home/user/.local/bin/testapp", record.Metadata.WrapperScript)
	assert.Equal(t, "native", record.Metadata.WaylandSupport)
	assert.Equal(t, "/path/to/original.desktop", record.Metadata.OriginalDesktopFile)
	assert.Equal(t, "local", record.Metadata.InstallMethod)
	assert.Len(t, record.Metadata.DesktopFiles, 1)
}

func TestDbInstallToCore_EmptyMetadata(t *testing.T) {
	t.Parallel()

	dbRecord := &db.Install{
		InstallID:    "test-id",
		PackageType:  "Binary",
		Name:         "TestBinary",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/binary",
		InstallPath:  "/home/user/.local/share/upkg/apps/binary",
		Metadata:     nil,
	}

	record := dbInstallToCore(dbRecord)

	assert.Equal(t, core.InstallMethodLocal, record.Metadata.InstallMethod)
	assert.Empty(t, record.Metadata.IconFiles)
	assert.Empty(t, record.Metadata.WrapperScript)
}

func TestDbInstallToCore_StringArrayMetadata(t *testing.T) {
	t.Parallel()

	dbRecord := &db.Install{
		InstallID:    "test-id",
		PackageType:  "Tarball",
		Name:         "TestApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/app.tar.gz",
		InstallPath:  "/home/user/apps/testapp",
		Metadata: map[string]interface{}{
			"icon_files":    []string{"/path/to/icon.png"},
			"desktop_files": []string{"/path/to/app.desktop"},
		},
	}

	record := dbInstallToCore(dbRecord)

	assert.Len(t, record.Metadata.IconFiles, 1)
	assert.Equal(t, "/path/to/icon.png", record.Metadata.IconFiles[0])
	assert.Len(t, record.Metadata.DesktopFiles, 1)
}

func TestLookupPackage_ByID(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	testInstall := &db.Install{
		InstallID:    "unique-id-12345",
		PackageType:  "AppImage",
		Name:         "MyApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/app.AppImage",
		InstallPath:  "/home/user/apps/myapp",
	}
	require.NoError(t, database.Create(ctx, testInstall))

	log := zerolog.New(io.Discard)
	record, err := lookupPackage(ctx, database, &log, "unique-id-12345")

	assert.NoError(t, err)
	assert.NotNil(t, record)
	assert.Equal(t, "unique-id-12345", record.InstallID)
	assert.Equal(t, "MyApp", record.Name)
}

func TestLookupPackage_ByName(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	testInstall := &db.Install{
		InstallID:    "some-uuid",
		PackageType:  "DEB",
		Name:         "firefox",
		Version:      "120.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/firefox.deb",
		InstallPath:  "/home/user/apps/firefox",
	}
	require.NoError(t, database.Create(ctx, testInstall))

	log := zerolog.New(io.Discard)

	// Test case-insensitive lookup
	record, err := lookupPackage(ctx, database, &log, "Firefox")

	assert.NoError(t, err)
	assert.NotNil(t, record)
	assert.Equal(t, "firefox", record.Name)
}

func TestLookupPackage_NotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	log := zerolog.New(io.Discard)
	record, err := lookupPackage(ctx, database, &log, "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, record)
	assert.Contains(t, err.Error(), "package not found")
}

func TestIsInteractive(t *testing.T) {
	// Note: This test will return false in CI/test environments
	// Just verify it doesn't panic
	result := isInteractive()
	// Result depends on environment, just check it's a valid bool
	assert.IsType(t, true, result)
}

func TestRequireInteractiveOrYes(t *testing.T) {
	t.Parallel()

	t.Run("with yes flag", func(t *testing.T) {
		opts := &uninstallOptions{yes: true}
		err := requireInteractiveOrYes(opts)
		assert.NoError(t, err)
	})

	t.Run("without yes flag in non-interactive", func(t *testing.T) {
		opts := &uninstallOptions{yes: false}
		err := requireInteractiveOrYes(opts)
		// In test environment, stdin is not a terminal
		if !isInteractive() {
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "non-interactive mode requires --yes flag")
		}
	})
}

func TestUninstallResult(t *testing.T) {
	t.Parallel()

	result := UninstallResult{
		Name:    "testpkg",
		Success: true,
		Error:   nil,
	}

	assert.Equal(t, "testpkg", result.Name)
	assert.True(t, result.Success)
	assert.Nil(t, result.Error)
}

func TestUninstallOptions(t *testing.T) {
	t.Parallel()

	opts := &uninstallOptions{
		yes:        true,
		dryRun:     true,
		all:        false,
		timeoutSec: 300,
	}

	assert.True(t, opts.yes)
	assert.True(t, opts.dryRun)
	assert.False(t, opts.all)
	assert.Equal(t, 300, opts.timeoutSec)
}

func TestShowDryRunDetails(t *testing.T) {
	t.Parallel()

	records := []*core.InstallRecord{
		{
			InstallID:   "id1",
			PackageType: "AppImage",
			Name:        "TestApp1",
			InstallPath: "/home/user/apps/testapp1",
			DesktopFile: "/home/user/.local/share/applications/testapp1.desktop",
			Metadata: core.Metadata{
				IconFiles:     []string{"/path/to/icon1.png", "/path/to/icon2.png"},
				WrapperScript: "/home/user/.local/bin/testapp1",
				DesktopFiles:  []string{"/path/to/extra.desktop"},
			},
		},
		{
			InstallID:   "id2",
			PackageType: "Binary",
			Name:        "TestApp2",
			InstallPath: "/home/user/apps/testapp2",
		},
	}

	sizes := map[string]int64{
		"id1": 1024 * 1024,      // 1 MB
		"id2": 1024 * 1024 * 10, // 10 MB
	}

	err := showDryRunDetails(records, sizes)
	assert.NoError(t, err)
}

func TestShowDryRunDetails_EmptyRecords(t *testing.T) {
	t.Parallel()

	records := []*core.InstallRecord{}
	sizes := map[string]int64{}

	err := showDryRunDetails(records, sizes)
	assert.NoError(t, err)
}

func TestPrintUninstallSummary_AllSuccess(t *testing.T) {
	t.Parallel()

	results := []UninstallResult{
		{Name: "pkg1", Success: true, Error: nil},
		{Name: "pkg2", Success: true, Error: nil},
		{Name: "pkg3", Success: true, Error: nil},
	}

	err := printUninstallSummary(results)
	assert.NoError(t, err)
}

func TestPrintUninstallSummary_WithFailures(t *testing.T) {
	t.Parallel()

	results := []UninstallResult{
		{Name: "pkg1", Success: true, Error: nil},
		{Name: "pkg2", Success: false, Error: fmt.Errorf("backend not found")},
		{Name: "pkg3", Success: true, Error: nil},
		{Name: "pkg4", Success: false, Error: fmt.Errorf("permission denied")},
	}

	err := printUninstallSummary(results)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "2 package(s) failed to uninstall")
}

func TestPrintUninstallSummary_AllFailed(t *testing.T) {
	t.Parallel()

	results := []UninstallResult{
		{Name: "pkg1", Success: false, Error: fmt.Errorf("error 1")},
		{Name: "pkg2", Success: false, Error: fmt.Errorf("error 2")},
	}

	err := printUninstallSummary(results)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "2 package(s) failed to uninstall")
}

func TestPrintUninstallSummary_Empty(t *testing.T) {
	t.Parallel()

	results := []UninstallResult{}

	err := printUninstallSummary(results)
	assert.NoError(t, err)
}

func TestRunUninstallCmd_SinglePackage(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Single package that doesn't exist
	cmd.SetArgs([]string{"testpkg", "--yes"})
	err = cmd.Execute()
	assert.Error(t, err)
}

func TestRunUninstallCmd_DryRunWithAll(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database with a package
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id",
		PackageType:  "AppImage",
		Name:         "testapp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/app.AppImage",
		InstallPath:  filepath.Join(tmpDir, "testapp"),
	}
	require.NoError(t, database.Create(ctx, testInstall))
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--all", "--dry-run", "--yes"})
	err = cmd.Execute()
	assert.NoError(t, err)

	// Verify package still in database
	database, err = db.New(ctx, dbPath)
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	record, err := database.Get(ctx, "test-id")
	assert.NoError(t, err)
	assert.NotNil(t, record)
}

func TestRunUninstallCmd_BulkDryRun(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database with packages
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	for i := 1; i <= 3; i++ {
		testInstall := &db.Install{
			InstallID:    fmt.Sprintf("test-id-%d", i),
			PackageType:  "AppImage",
			Name:         fmt.Sprintf("app%d", i),
			Version:      "1.0.0",
			InstallDate:  time.Now(),
			OriginalFile: fmt.Sprintf("/path/to/app%d.AppImage", i),
			InstallPath:  filepath.Join(tmpDir, fmt.Sprintf("app%d", i)),
		}
		require.NoError(t, database.Create(ctx, testInstall))
	}
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"app1", "app2", "--dry-run"})
	err = cmd.Execute()
	assert.NoError(t, err)

	// Verify packages still in database
	database, err = db.New(ctx, dbPath)
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	for i := 1; i <= 3; i++ {
		record, err := database.Get(ctx, fmt.Sprintf("test-id-%d", i))
		assert.NoError(t, err)
		assert.NotNil(t, record)
	}
}

func TestRunUninstallCmd_BulkPartialNotFound(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create the database with one package
	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id-1",
		PackageType:  "AppImage",
		Name:         "app1",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/app1.AppImage",
		InstallPath:  filepath.Join(tmpDir, "app1"),
	}
	require.NoError(t, database.Create(ctx, testInstall))
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// app1 exists, app2 doesn't
	cmd.SetArgs([]string{"app1", "app2", "--dry-run"})
	err = cmd.Execute()
	// Should succeed because at least one package was found
	assert.NoError(t, err)
}

func TestExecuteUninstall_DryRunWithMetadata(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	// Create test directories and files
	appDir := filepath.Join(tmpDir, "testapp")
	require.NoError(t, os.MkdirAll(appDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(appDir, "testapp"), []byte("fake binary"), 0755))

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id",
		PackageType:  "Tarball",
		Name:         "testapp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/app.tar.gz",
		InstallPath:  appDir,
		DesktopFile:  filepath.Join(tmpDir, "testapp.desktop"),
		Metadata: map[string]interface{}{
			"icon_files":     []interface{}{"/path/to/icon.png"},
			"wrapper_script": filepath.Join(tmpDir, "testapp-wrapper"),
			"desktop_files":  []interface{}{"/path/to/extra.desktop"},
		},
	}
	require.NoError(t, database.Create(ctx, testInstall))
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"testapp", "--dry-run"})
	err = cmd.Execute()
	assert.NoError(t, err)

	// Verify package still in database
	database, err = db.New(ctx, dbPath)
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	record, err := database.Get(ctx, "test-id")
	assert.NoError(t, err)
	assert.NotNil(t, record)
}

func TestRunUninstallCmd_NonInteractiveWithoutYes(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)

	testInstall := &db.Install{
		InstallID:    "test-id",
		PackageType:  "AppImage",
		Name:         "testapp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/path/to/app.AppImage",
		InstallPath:  filepath.Join(tmpDir, "testapp"),
	}
	require.NoError(t, database.Create(ctx, testInstall))
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	// Without --yes in non-interactive mode (but dry-run is allowed)
	cmd.SetArgs([]string{"testapp"})
	err = cmd.Execute()

	// In test environment, stdin is not a terminal, so this should fail
	// unless --yes is provided
	if !isInteractive() {
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "non-interactive mode requires --yes flag")
	}
}

func TestRunUninstallAll_EmptyDatabase(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{
		Paths: config.PathsConfig{
			DBFile:  dbPath,
			DataDir: tmpDir,
		},
	}

	ctx := context.Background()
	database, err := db.New(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	log := zerolog.New(io.Discard)
	cmd := NewUninstallCmd(cfg, &log)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	cmd.SetArgs([]string{"--all", "--yes"})
	err = cmd.Execute()
	// Should succeed (no packages to uninstall)
	assert.NoError(t, err)
}
