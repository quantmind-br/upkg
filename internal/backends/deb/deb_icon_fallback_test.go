package deb

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/quantmind-br/upkg/internal/paths"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebBackend_InstallUserIconFallback(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()
	backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	backend.Paths = paths.NewResolverWithHome(cfg, homeDir)

	iconPath := filepath.Join(tmpDir, "usr", "share", "icons", "hicolor", "1024x1024", "apps", "easycli.png")
	desktopPath := filepath.Join(tmpDir, "usr", "share", "applications", "EasyCLI.desktop")

	require.NoError(t, fs.MkdirAll(filepath.Dir(iconPath), 0755))
	require.NoError(t, afero.WriteFile(fs, iconPath, []byte("fake icon"), 0644))

	require.NoError(t, fs.MkdirAll(filepath.Dir(desktopPath), 0755))
	desktopContent := `[Desktop Entry]
Type=Application
Name=EasyCLI
Exec=easycli
Icon=easycli
`
	require.NoError(t, afero.WriteFile(fs, desktopPath, []byte(desktopContent), 0644))

	installed, err := backend.installUserIconFallback([]string{iconPath}, desktopPath)
	require.NoError(t, err)
	require.Len(t, installed, 1)

	expected := filepath.Join(homeDir, ".local", "share", "icons", "hicolor", "512x512", "apps", "easycli.png")
	assert.FileExists(t, expected)
	assert.Equal(t, expected, installed[0])
}

func TestDebBackend_InstallUserIconFallback_SkipsStandardSize(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	fs := afero.NewOsFs()
	backend := NewWithDeps(cfg, &logger, fs, &helpers.MockCommandRunner{})

	tmpDir := t.TempDir()
	homeDir := filepath.Join(tmpDir, "home")
	backend.Paths = paths.NewResolverWithHome(cfg, homeDir)

	iconPath := filepath.Join(tmpDir, "usr", "share", "icons", "hicolor", "256x256", "apps", "easycli.png")
	desktopPath := filepath.Join(tmpDir, "usr", "share", "applications", "EasyCLI.desktop")

	require.NoError(t, fs.MkdirAll(filepath.Dir(iconPath), 0755))
	require.NoError(t, afero.WriteFile(fs, iconPath, []byte("fake icon"), 0644))

	require.NoError(t, fs.MkdirAll(filepath.Dir(desktopPath), 0755))
	desktopContent := `[Desktop Entry]
Type=Application
Name=EasyCLI
Exec=easycli
Icon=easycli
`
	require.NoError(t, afero.WriteFile(fs, desktopPath, []byte(desktopContent), 0644))

	installed, err := backend.installUserIconFallback([]string{iconPath}, desktopPath)
	require.NoError(t, err)
	assert.Empty(t, installed)
}
