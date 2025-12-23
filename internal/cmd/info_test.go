package cmd

import (
	"io"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestNewInfoCmd(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}
	
	cmd := NewInfoCmd(cfg, &logger)
	
	assert.NotNil(t, cmd)
	assert.Contains(t, cmd.Use, "info")
}
