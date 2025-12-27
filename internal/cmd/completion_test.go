package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestNewCompletionCmd(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	cmd := NewCompletionCmd(cfg, &logger)

	assert.NotNil(t, cmd)
	assert.Contains(t, cmd.Use, "completion")
}

func TestCompletionCmd_Bash(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	cfg := &config.Config{}

	cmd := NewCompletionCmd(cfg, &logger)
	cmd.SetArgs([]string{"bash"})

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should generate completion script
	if err == nil {
		assert.NotEmpty(t, output)
	}
}
