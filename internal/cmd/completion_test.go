package cmd

import (
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestNewCompletionCmd(t *testing.T) {
	cfg := &config.Config{}
	log := zerolog.Nop()

	t.Run("creates completion command", func(t *testing.T) {
		cmd := NewCompletionCmd(cfg, &log)
		assert.NotNil(t, cmd)
		assert.Equal(t, "completion [bash|zsh|fish|powershell]", cmd.Use)
		assert.Equal(t, "Generate shell completion scripts", cmd.Short)
	})

	t.Run("command has run function", func(t *testing.T) {
		cmd := NewCompletionCmd(cfg, &log)
		assert.NotNil(t, cmd.RunE)
	})
}
