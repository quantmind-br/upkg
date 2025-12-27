package cmd

import (
	"io"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestNewRootCmd(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	cmd := NewRootCmd(cfg, &logger, "1.0.0")

	assert.NotNil(t, cmd)
	assert.Equal(t, "upkg", cmd.Use)
}
