package backends

import (
	"io"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNewRegistry_OrderIsPreserved(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	registry := NewRegistry(&config.Config{}, &logger)

	require.Equal(t, []string{"deb", "rpm", "appimage", "binary", "tarball"}, registry.ListBackends())
}
