package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewVersionCmd(t *testing.T) {
	t.Run("creates version command", func(t *testing.T) {
		cmd := NewVersionCmd("1.2.3")
		assert.NotNil(t, cmd)
		assert.Equal(t, "version", cmd.Use)
		assert.Equal(t, "Show version information", cmd.Short)
	})

	t.Run("command has run function", func(t *testing.T) {
		cmd := NewVersionCmd("1.2.3")
		assert.NotNil(t, cmd.Run)
	})
}
