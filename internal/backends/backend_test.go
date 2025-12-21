package backends

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/backends/base"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry_OrderIsPreserved(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	registry := NewRegistry(&config.Config{}, &logger)

	require.Equal(t, []string{"deb", "rpm", "appimage", "binary", "tarball"}, registry.ListBackends())
}

func TestBaseBackend_New(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)

	backend := base.New(cfg, &logger)

	require.NotNil(t, backend)
	require.Equal(t, cfg, backend.Cfg)
	require.Equal(t, &logger, backend.Log)
	require.NotNil(t, backend.Fs)
	require.NotNil(t, backend.Runner)
	require.NotNil(t, backend.Paths)
}

func TestBaseBackend_NewWithDeps(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)
	fs := afero.NewMemMapFs()
	runner := &helpers.MockCommandRunner{}

	backend := base.NewWithDeps(cfg, &logger, fs, runner)

	require.NotNil(t, backend)
	require.Equal(t, cfg, backend.Cfg)
	require.Equal(t, &logger, backend.Log)
	require.Equal(t, fs, backend.Fs)
	require.Equal(t, runner, backend.Runner)
	require.NotNil(t, backend.Paths)
}

func TestBaseBackend_NewWithNilConfig(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)

	backend := base.New(nil, &logger)

	require.NotNil(t, backend)
	require.Nil(t, backend.Cfg)
	require.Equal(t, &logger, backend.Log)
	require.NotNil(t, backend.Fs)
	require.NotNil(t, backend.Runner)
	require.NotNil(t, backend.Paths)
}

func TestBaseBackend_NewWithNilLogger(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}

	backend := base.New(cfg, nil)

	require.NotNil(t, backend)
	require.Equal(t, cfg, backend.Cfg)
	require.Nil(t, backend.Log)
	require.NotNil(t, backend.Fs)
	require.NotNil(t, backend.Runner)
	require.NotNil(t, backend.Paths)
}

func TestDetectBackend(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)

	t.Run("detects backend for DEB package", func(t *testing.T) {
		registry := NewRegistry(cfg, &logger)
		tmpDir := t.TempDir()
		debPath := filepath.Join(tmpDir, "test.deb")
		require.NoError(t, os.WriteFile(debPath, []byte("!<arch>\ndebian-binary"), 0644))

		backend, err := registry.DetectBackend(context.Background(), debPath)
		require.NoError(t, err)
		require.NotNil(t, backend)
		require.Equal(t, "deb", backend.Name())
	})

	t.Run("detects backend for AppImage package", func(t *testing.T) {
		registry := NewRegistry(cfg, &logger)
		tmpDir := t.TempDir()
		appImagePath := filepath.Join(tmpDir, "test.AppImage")
		require.NoError(t, os.WriteFile(appImagePath, []byte{0x7F, 'E', 'L', 'F', 0x00, 0x00, 0x00, 0x00}, 0755))

		backend, err := registry.DetectBackend(context.Background(), appImagePath)
		require.NoError(t, err)
		require.NotNil(t, backend)
		require.Equal(t, "appimage", backend.Name())
	})

	t.Run("returns error for unsupported package", func(t *testing.T) {
		registry := NewRegistry(cfg, &logger)
		tmpDir := t.TempDir()
		unknownPath := filepath.Join(tmpDir, "test.unknown")
		require.NoError(t, os.WriteFile(unknownPath, []byte("unknown file type"), 0644))

		backend, err := registry.DetectBackend(context.Background(), unknownPath)
		require.Error(t, err)
		require.Nil(t, backend)
	})

	t.Run("handles non-existent file", func(t *testing.T) {
		registry := NewRegistry(cfg, &logger)

		backend, err := registry.DetectBackend(context.Background(), "/nonexistent/file.deb")
		require.Error(t, err)
		require.Nil(t, backend)
	})
}

func TestCreateDetectionError(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)
	registry := NewRegistry(cfg, &logger)

	err := registry.createDetectionError("test-file.deb")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot detect package type")
	require.Contains(t, err.Error(), "test-file.deb")
	require.Contains(t, err.Error(), "Supported package types")
}

func TestDetectFileType(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)
	registry := NewRegistry(cfg, &logger)

	t.Run("detects DEB file type", func(t *testing.T) {
		tmpDir := t.TempDir()
		debPath := filepath.Join(tmpDir, "test.deb")
		require.NoError(t, os.WriteFile(debPath, []byte("!<arch>\ndebian-binary"), 0644))

		fileType, err := registry.detectFileType(debPath)
		require.NoError(t, err)
		require.Equal(t, "deb", fileType)
	})

	t.Run("detects RPM file type", func(t *testing.T) {
		tmpDir := t.TempDir()
		rpmPath := filepath.Join(tmpDir, "test.rpm")
		require.NoError(t, os.WriteFile(rpmPath, []byte{0xED, 0xAB, 0xEE, 0xDB}, 0644))

		fileType, err := registry.detectFileType(rpmPath)
		require.NoError(t, err)
		require.Equal(t, "rpm", fileType)
	})

	t.Run("detects tarball file type", func(t *testing.T) {
		tmpDir := t.TempDir()
		tarPath := filepath.Join(tmpDir, "test.tar.gz")
		require.NoError(t, os.WriteFile(tarPath, []byte{0x1F, 0x8B}, 0644))

		fileType, err := registry.detectFileType(tarPath)
		require.NoError(t, err)
		require.Equal(t, "tarball", fileType)
	})

	t.Run("handles non-existent file", func(t *testing.T) {
		fileType, err := registry.detectFileType("/nonexistent/file")
		require.Error(t, err)
		require.Empty(t, fileType)
	})

	t.Run("handles unknown file type", func(t *testing.T) {
		tmpDir := t.TempDir()
		unknownPath := filepath.Join(tmpDir, "test.unknown")
		require.NoError(t, os.WriteFile(unknownPath, []byte("unknown content"), 0644))

		fileType, err := registry.detectFileType(unknownPath)
		require.Error(t, err)
		require.Empty(t, fileType)
	})
}

func TestGetBackend(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)
	registry := NewRegistry(cfg, &logger)

	t.Run("returns existing backend", func(t *testing.T) {
		backend, err := registry.GetBackend("deb")
		require.NoError(t, err)
		require.NotNil(t, backend)
		require.Equal(t, "deb", backend.Name())
	})

	t.Run("returns error for non-existent backend", func(t *testing.T) {
		backend, err := registry.GetBackend("nonexistent")
		require.Error(t, err)
		require.Nil(t, backend)
	})
}

func TestListBackends(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)
	registry := NewRegistry(cfg, &logger)

	backends := registry.ListBackends()
	require.NotEmpty(t, backends)
	require.Contains(t, backends, "deb")
	require.Contains(t, backends, "rpm")
	require.Contains(t, backends, "appimage")
	require.Contains(t, backends, "binary")
	require.Contains(t, backends, "tarball")
}
