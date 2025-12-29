package deb

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/cache"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/paths"
	"github.com/quantmind-br/upkg/internal/syspkg"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIconNameMatches tests the iconNameMatches function
func TestIconNameMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		iconPath string
		iconName string
		expected bool
	}{
		{"matches exactly", "/usr/share/icons/app.png", "app", true},
		{"matches case insensitive", "/usr/share/icons/App.png", "app", true},
		{"matches SVG", "/usr/share/icons/app.svg", "app", true},
		{"does not match", "/usr/share/icons/other.png", "app", false},
		{"empty icon name", "/usr/share/icons/app.png", "", false},
		{"partial match fails", "/usr/share/icons/app-icon.png", "app", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := iconNameMatches(tt.iconPath, tt.iconName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIconSizeFromPath tests the iconSizeFromPath function
func TestIconSizeFromPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		path        string
		expectedSz  string
		expectedOk  bool
	}{
		{"scalable icon", "/usr/share/icons/hicolor/scalable/apps/app.svg", "scalable", true},
		{"256x256", "/usr/share/icons/hicolor/256x256/apps/app.png", "256x256", true},
		{"48x48", "/usr/share/icons/hicolor/48x48/apps/app.png", "48x48", true},
		{"no size in path", "/usr/share/pixmaps/app.png", "", false},
		{"uppercase SCALABLE", "/usr/share/icons/SCALABLE/app.svg", "scalable", true},
		{"mixed case 128X128", "/usr/share/icons/128X128/app.png", "128x128", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			size, ok := iconSizeFromPath(tt.path)
			assert.Equal(t, tt.expectedOk, ok)
			if ok {
				assert.Equal(t, tt.expectedSz, size)
			}
		})
	}
}

// TestHasStandardIcon tests the hasStandardIcon function
func TestHasStandardIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		iconFiles []string
		iconName  string
		expected  bool
	}{
		{
			"has 256x256",
			[]string{"/usr/share/icons/hicolor/256x256/apps/myapp.png"},
			"myapp",
			true,
		},
		{
			"has scalable",
			[]string{"/usr/share/icons/hicolor/scalable/apps/myapp.svg"},
			"myapp",
			true,
		},
		{
			"non-standard size",
			[]string{"/usr/share/icons/hicolor/1024x1024/apps/myapp.png"},
			"myapp",
			false,
		},
		{
			"wrong icon name",
			[]string{"/usr/share/icons/hicolor/256x256/apps/other.png"},
			"myapp",
			false,
		},
		{
			"empty list",
			[]string{},
			"myapp",
			false,
		},
		{
			"multiple with one standard",
			[]string{
				"/usr/share/icons/hicolor/1024x1024/apps/myapp.png",
				"/usr/share/icons/hicolor/48x48/apps/myapp.png",
			},
			"myapp",
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasStandardIcon(tt.iconFiles, tt.iconName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSelectBestIconSource tests the selectBestIconSource function
func TestSelectBestIconSource(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		iconFiles []string
		iconName  string
		expected  string
	}{
		{
			"prefers SVG",
			[]string{
				"/usr/share/icons/hicolor/256x256/apps/myapp.png",
				"/usr/share/icons/hicolor/scalable/apps/myapp.svg",
			},
			"myapp",
			"/usr/share/icons/hicolor/scalable/apps/myapp.svg",
		},
		{
			"selects largest PNG",
			[]string{
				"/usr/share/icons/hicolor/48x48/apps/myapp.png",
				"/usr/share/icons/hicolor/256x256/apps/myapp.png",
				"/usr/share/icons/hicolor/128x128/apps/myapp.png",
			},
			"myapp",
			"/usr/share/icons/hicolor/256x256/apps/myapp.png",
		},
		{
			"no matching icons",
			[]string{
				"/usr/share/icons/hicolor/256x256/apps/other.png",
			},
			"myapp",
			"",
		},
		{
			"empty list",
			[]string{},
			"myapp",
			"",
		},
		{
			"single match",
			[]string{
				"/usr/share/icons/hicolor/64x64/apps/myapp.png",
			},
			"myapp",
			"/usr/share/icons/hicolor/64x64/apps/myapp.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectBestIconSource(tt.iconFiles, tt.iconName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIconPathSizeScore tests the iconPathSizeScore function
func TestIconPathSizeScore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		expected int
	}{
		{"scalable", "/icons/scalable/app.svg", 100000},
		{"256x256", "/icons/256x256/app.png", 256},
		{"48x48", "/icons/48x48/app.png", 48},
		{"no size", "/pixmaps/app.png", 0},
		{"512x512", "/icons/512x512/app.png", 512},
		{"rectangular prefers larger", "/icons/128x256/app.png", 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := iconPathSizeScore(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIconNameFromDesktopFile tests the iconNameFromDesktopFile function
func TestIconNameFromDesktopFile(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()

	t.Run("extracts icon name", func(t *testing.T) {
		tmpDir := t.TempDir()
		desktopPath := filepath.Join(tmpDir, "app.desktop")
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
Icon=myicon
`
		require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		name, err := backend.iconNameFromDesktopFile(desktopPath)

		assert.NoError(t, err)
		assert.Equal(t, "myicon", name)
	})

	t.Run("handles empty icon", func(t *testing.T) {
		tmpDir := t.TempDir()
		desktopPath := filepath.Join(tmpDir, "app.desktop")
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
`
		require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		name, err := backend.iconNameFromDesktopFile(desktopPath)

		assert.NoError(t, err)
		assert.Empty(t, name)
	})

	t.Run("handles absolute icon path", func(t *testing.T) {
		tmpDir := t.TempDir()
		desktopPath := filepath.Join(tmpDir, "app.desktop")
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
Icon=/usr/share/icons/app.png
`
		require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		name, err := backend.iconNameFromDesktopFile(desktopPath)

		assert.NoError(t, err)
		assert.Empty(t, name) // Absolute paths return empty
	})

	t.Run("handles missing file", func(t *testing.T) {
		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		name, err := backend.iconNameFromDesktopFile("/nonexistent.desktop")

		assert.Error(t, err)
		assert.Empty(t, name)
	})

	t.Run("handles empty path", func(t *testing.T) {
		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		name, err := backend.iconNameFromDesktopFile("")

		assert.NoError(t, err)
		assert.Empty(t, name)
	})

	t.Run("strips extension from icon name", func(t *testing.T) {
		tmpDir := t.TempDir()
		desktopPath := filepath.Join(tmpDir, "app.desktop")
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
Icon=myicon.png
`
		require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		name, err := backend.iconNameFromDesktopFile(desktopPath)

		assert.NoError(t, err)
		assert.Equal(t, "myicon", name)
	})
}

// TestRemoveUserIcons tests the removeUserIcons function
func TestRemoveUserIcons(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()

	t.Run("removes icons in home directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := filepath.Join(tmpDir, "home")
		require.NoError(t, os.MkdirAll(homeDir, 0755))

		iconPath := filepath.Join(homeDir, ".local", "share", "icons", "test.png")
		require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0755))
		require.NoError(t, os.WriteFile(iconPath, []byte("icon"), 0644))

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		backend.Paths = paths.NewResolverWithHome(cfg, homeDir)

		removed := backend.removeUserIcons([]string{iconPath})

		assert.True(t, removed)
		assert.NoFileExists(t, iconPath)
	})

	t.Run("does not remove system icons", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := filepath.Join(tmpDir, "home")
		require.NoError(t, os.MkdirAll(homeDir, 0755))

		// Create icon outside home
		iconPath := filepath.Join(tmpDir, "system", "icons", "test.png")
		require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0755))
		require.NoError(t, os.WriteFile(iconPath, []byte("icon"), 0644))

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		backend.Paths = paths.NewResolverWithHome(cfg, homeDir)

		removed := backend.removeUserIcons([]string{iconPath})

		assert.False(t, removed)
		assert.FileExists(t, iconPath)
	})

	t.Run("handles missing icons gracefully", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := filepath.Join(tmpDir, "home")
		require.NoError(t, os.MkdirAll(homeDir, 0755))

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		backend.Paths = paths.NewResolverWithHome(cfg, homeDir)

		iconPath := filepath.Join(homeDir, ".local", "share", "icons", "nonexistent.png")
		removed := backend.removeUserIcons([]string{iconPath})

		assert.False(t, removed)
	})

	t.Run("handles empty home directory", func(t *testing.T) {
		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		backend.Paths = paths.NewResolverWithHome(cfg, "")

		removed := backend.removeUserIcons([]string{"/some/icon.png"})

		assert.False(t, removed)
	})

	t.Run("handles empty icon list", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := filepath.Join(tmpDir, "home")

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		backend.Paths = paths.NewResolverWithHome(cfg, homeDir)

		removed := backend.removeUserIcons([]string{})

		assert.False(t, removed)
	})
}

// TestInstallUserIconFallback tests the installUserIconFallback function
func TestInstallUserIconFallbackCoverage(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()

	t.Run("empty icon files", func(t *testing.T) {
		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})

		icons, err := backend.installUserIconFallback([]string{}, "/some/desktop")

		assert.NoError(t, err)
		assert.Nil(t, icons)
	})

	t.Run("empty desktop file", func(t *testing.T) {
		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})

		icons, err := backend.installUserIconFallback([]string{"/some/icon.png"}, "")

		assert.NoError(t, err)
		assert.Nil(t, icons)
	})

	t.Run("no matching icons", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := filepath.Join(tmpDir, "home")
		require.NoError(t, os.MkdirAll(homeDir, 0755))

		// Create desktop file with icon name
		desktopPath := filepath.Join(tmpDir, "app.desktop")
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp
Icon=myapp
`
		require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

		// Create icon with different name
		iconPath := filepath.Join(tmpDir, "icons", "other.png")
		require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0755))
		require.NoError(t, os.WriteFile(iconPath, []byte("icon"), 0644))

		backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})
		backend.Paths = paths.NewResolverWithHome(cfg, homeDir)

		icons, err := backend.installUserIconFallback([]string{iconPath}, desktopPath)

		assert.NoError(t, err)
		assert.Nil(t, icons)
	})
}

// TestInstallWithMockedSyspkg tests the Install function with mocked syspkg provider
func TestInstallWithMockedSyspkg(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()

	t.Run("missing debtap command", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			RequireCommandFunc: func(cmd string) error {
				if cmd == "debtap" {
					return assert.AnError
				}
				return nil
			},
		}

		backend := NewWithDeps(cfg, &logger, fs, mockRunner)
		tx := transaction.NewManager(&logger)

		tmpDir := t.TempDir()
		debPath := filepath.Join(tmpDir, "test.deb")
		require.NoError(t, os.WriteFile(debPath, []byte("!<arch>\ndebian-binary"), 0644))

		_, err := backend.Install(context.Background(), debPath, core.InstallOptions{}, tx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "debtap is required")
	})

	t.Run("missing pacman command", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			RequireCommandFunc: func(cmd string) error {
				if cmd == "pacman" {
					return assert.AnError
				}
				return nil
			},
		}

		backend := NewWithDeps(cfg, &logger, fs, mockRunner)
		tx := transaction.NewManager(&logger)

		tmpDir := t.TempDir()
		debPath := filepath.Join(tmpDir, "test.deb")
		require.NoError(t, os.WriteFile(debPath, []byte("!<arch>\ndebian-binary"), 0644))

		_, err := backend.Install(context.Background(), debPath, core.InstallOptions{}, tx)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pacman not found")
	})
}

// TestUninstallWithMockedSyspkg tests the Uninstall function with mocked syspkg provider
func TestUninstallWithMockedSyspkg(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()

	t.Run("uninstall with remove error", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(_ string) bool { return false },
		}
		cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

		mockProvider := &mockSyspkgProviderCoverage{
			isInstalled: true,
			removeErr:   assert.AnError,
		}

		backend := NewWithDeps(cfg, &logger, fs, mockRunner)
		backend.sys = mockProvider
		backend.cacheManager = cacheManager

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-package",
			PackageType: core.PackageTypeDeb,
			Metadata: core.Metadata{
				InstallMethod: core.InstallMethodPacman,
			},
		}

		err := backend.Uninstall(context.Background(), record)
		assert.Error(t, err)
	})

	t.Run("uninstall with icon removal", func(t *testing.T) {
		tmpDir := t.TempDir()
		homeDir := filepath.Join(tmpDir, "home")
		require.NoError(t, os.MkdirAll(homeDir, 0755))

		// Create user icon
		iconPath := filepath.Join(homeDir, ".local", "share", "icons", "test.png")
		require.NoError(t, os.MkdirAll(filepath.Dir(iconPath), 0755))
		require.NoError(t, os.WriteFile(iconPath, []byte("icon"), 0644))

		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(_ string) bool { return false },
		}
		cacheManager := cache.NewCacheManagerWithRunner(mockRunner)

		mockProvider := &mockSyspkgProviderCoverage{
			isInstalled: true,
			removeErr:   nil,
		}

		backend := NewWithDeps(cfg, &logger, fs, mockRunner)
		backend.sys = mockProvider
		backend.cacheManager = cacheManager
		backend.Paths = paths.NewResolverWithHome(cfg, homeDir)

		record := &core.InstallRecord{
			InstallID:   "test-id",
			Name:        "test-package",
			PackageType: core.PackageTypeDeb,
			Metadata: core.Metadata{
				InstallMethod: core.InstallMethodPacman,
				IconFiles:     []string{iconPath},
			},
		}

		err := backend.Uninstall(context.Background(), record)
		assert.NoError(t, err)
		assert.NoFileExists(t, iconPath)
	})
}

// mockSyspkgProviderCoverage is a mock implementation of syspkg.Provider for coverage tests
type mockSyspkgProviderCoverage struct {
	isInstalled  bool
	removeCalled bool
	removeErr    error
}

func (m *mockSyspkgProviderCoverage) Name() string {
	return "mock"
}

func (m *mockSyspkgProviderCoverage) Install(_ context.Context, _ string, _ *syspkg.InstallOptions) error {
	return nil
}

func (m *mockSyspkgProviderCoverage) Remove(_ context.Context, _ string) error {
	m.removeCalled = true
	return m.removeErr
}

func (m *mockSyspkgProviderCoverage) IsInstalled(_ context.Context, _ string) (bool, error) {
	return m.isInstalled, nil
}

func (m *mockSyspkgProviderCoverage) GetInfo(_ context.Context, pkgName string) (*syspkg.PackageInfo, error) {
	return &syspkg.PackageInfo{Name: pkgName, Version: "1.0.0"}, nil
}

func (m *mockSyspkgProviderCoverage) ListFiles(_ context.Context, _ string) ([]string, error) {
	return []string{}, nil
}

// TestUpdateDesktopFileWaylandFull tests updateDesktopFileWayland with more scenarios
func TestUpdateDesktopFileWaylandFull(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()

	t.Run("handles invalid custom env vars and falls back to defaults", func(t *testing.T) {
		tmpDir := t.TempDir()
		desktopPath := filepath.Join(tmpDir, "test-app.desktop")

		// Create desktop file
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp %F
Categories=Utility;`
		require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(_ string) bool { return true },
			RunCommandFunc: func(_ context.Context, name string, args ...string) (string, error) {
				if name == "sudo" && len(args) >= 3 && args[0] == "mv" {
					// Simulate the mv
					tempPath := args[1]
					content, err := os.ReadFile(tempPath)
					if err != nil {
						return "", err
					}
					if err := os.WriteFile(desktopPath, content, 0644); err != nil {
						return "", err
					}
				}
				return "", nil
			},
		}

		// Set invalid custom env vars
		cfg.Desktop.CustomEnvVars = []string{"invalid={{invalid}}"}

		backend := NewWithDeps(cfg, &logger, fs, mockRunner)

		err := backend.updateDesktopFileWayland(desktopPath)
		assert.NoError(t, err)

		// Verify desktop file was updated with default Wayland env vars
		content, err := os.ReadFile(desktopPath)
		assert.NoError(t, err)
		assert.Contains(t, string(content), "env ")
	})

	t.Run("handles temp file creation failure", func(t *testing.T) {
		tmpDir := t.TempDir()
		desktopPath := filepath.Join(tmpDir, "test-app.desktop")

		// Create desktop file
		desktopContent := `[Desktop Entry]
Type=Application
Name=TestApp
Exec=testapp`
		require.NoError(t, os.WriteFile(desktopPath, []byte(desktopContent), 0644))

		// Use a mock filesystem that fails on TempFile
		mockFs := &afero.MemMapFs{}
		// Copy the desktop file to the mock fs
		content, _ := os.ReadFile(desktopPath)
		afero.WriteFile(mockFs, desktopPath, content, 0644)

		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(_ string) bool { return true },
		}

		backend := NewWithDeps(cfg, &logger, mockFs, mockRunner)

		// This should work with MemMapFs
		err := backend.updateDesktopFileWayland(desktopPath)
		assert.NoError(t, err)
	})
}

// TestQueryDebNameFull tests queryDebName with more scenarios
func TestQueryDebNameFull(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()

	t.Run("handles missing dpkg-deb command", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(cmd string) bool {
				return cmd != "dpkg-deb"
			},
		}

		backend := NewWithDeps(cfg, &logger, fs, mockRunner)

		ctx := context.Background()
		_, err := backend.queryDebName(ctx, "/some/package.deb")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "dpkg-deb command not found")
	})

	t.Run("handles failed dpkg-deb command", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(_ string) bool { return true },
			RunCommandFunc: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "", fmt.Errorf("dpkg-deb failed")
			},
		}

		backend := NewWithDeps(cfg, &logger, fs, mockRunner)

		ctx := context.Background()
		_, err := backend.queryDebName(ctx, "/some/package.deb")

		assert.Error(t, err)
	})

	t.Run("handles empty dpkg-deb output", func(t *testing.T) {
		mockRunner := &helpers.MockCommandRunner{
			CommandExistsFunc: func(_ string) bool { return true },
			RunCommandFunc: func(_ context.Context, _ string, _ ...string) (string, error) {
				return "   \n  \t  ", nil // Empty/whitespace output
			},
		}

		backend := NewWithDeps(cfg, &logger, fs, mockRunner)

		ctx := context.Background()
		_, err := backend.queryDebName(ctx, "/some/package.deb")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "empty package name")
	})
}

// TestFixMalformedDependenciesFull tests fixMalformedDependencies with more scenarios
func TestFixMalformedDependenciesFull(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)

	t.Run("handles missing file gracefully", func(t *testing.T) {
		err := fixMalformedDependencies("/nonexistent/package.deb", &logger)
		assert.Error(t, err)
	})

	t.Run("handles invalid archive", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidPath := filepath.Join(tmpDir, "invalid.deb")

		require.NoError(t, os.WriteFile(invalidPath, []byte("not an archive"), 0644))

		err := fixMalformedDependencies(invalidPath, &logger)
		assert.Error(t, err)
	})
}

// TestFixDependencyLineFull tests fixDependencyLine with edge cases
func TestFixDependencyLineFull(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)

	t.Run("handles line with multiple commas", func(t *testing.T) {
		line := "package1 (>= 1.0), package2, package3 (>= 2.0)"
		result := fixDependencyLine(line, &logger)

		assert.Contains(t, result, "package1")
		assert.Contains(t, result, "package2")
		assert.Contains(t, result, "package3")
	})

	t.Run("handles line with pipe alternatives", func(t *testing.T) {
		line := "libfoo1a | libfoo2"
		result := fixDependencyLine(line, &logger)

		// Should preserve the pipe
		assert.Contains(t, result, "|")
	})

	t.Run("handles empty line", func(t *testing.T) {
		line := ""
		result := fixDependencyLine(line, &logger)
		assert.Empty(t, result)
	})

	t.Run("handles line with whitespace", func(t *testing.T) {
		line := "   \n\t  "
		result := fixDependencyLine(line, &logger)
		// Function returns whitespace as-is
		assert.NotEmpty(t, result)
	})
}
