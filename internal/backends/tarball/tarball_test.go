package tarball

import (
	"io"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/rs/zerolog"
)

func TestNewTarballBackend(t *testing.T) {
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	backend := New(cfg, &logger)

	if backend == nil {
		t.Fatal("expected backend to be created")
	}

	if backend.Name() != "tarball" {
		t.Errorf("expected name 'tarball', got %s", backend.Name())
	}
}
