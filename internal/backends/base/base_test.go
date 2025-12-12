package base

import (
	"io"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)

	backend := New(cfg, &logger)

	require.NotNil(t, backend)
	require.Equal(t, cfg, backend.Cfg)
	require.Equal(t, &logger, backend.Log)
	require.NotNil(t, backend.Fs)
	require.NotNil(t, backend.Runner)
	require.NotNil(t, backend.Paths)
}

func TestNewWithDeps(t *testing.T) {
	cfg := &config.Config{}
	logger := zerolog.New(io.Discard)
	fs := afero.NewMemMapFs()
	runner := &helpers.MockCommandRunner{}

	backend := NewWithDeps(cfg, &logger, fs, runner)

	require.NotNil(t, backend)
	require.Equal(t, cfg, backend.Cfg)
	require.Equal(t, &logger, backend.Log)
	require.Equal(t, fs, backend.Fs)
	require.Equal(t, runner, backend.Runner)
	require.NotNil(t, backend.Paths)
}

func TestNewWithNilConfig(t *testing.T) {
	logger := zerolog.New(io.Discard)

	backend := New(nil, &logger)

	require.NotNil(t, backend)
	require.Nil(t, backend.Cfg)
	require.Equal(t, &logger, backend.Log)
	require.NotNil(t, backend.Fs)
	require.NotNil(t, backend.Runner)
	require.NotNil(t, backend.Paths)
}

func TestNewWithNilLogger(t *testing.T) {
	cfg := &config.Config{}

	backend := New(cfg, nil)

	require.NotNil(t, backend)
	require.Equal(t, cfg, backend.Cfg)
	require.Nil(t, backend.Log)
	require.NotNil(t, backend.Fs)
	require.NotNil(t, backend.Runner)
	require.NotNil(t, backend.Paths)
}
