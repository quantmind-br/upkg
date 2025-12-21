package helpers

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOSCommandRunner(t *testing.T) {
	runner := NewOSCommandRunner()

	t.Run("CommandExists", func(t *testing.T) {
		assert.True(t, runner.CommandExists("echo"))
		assert.False(t, runner.CommandExists("nonexistentcommand123"))
	})

	t.Run("RequireCommand", func(t *testing.T) {
		err := runner.RequireCommand("echo")
		assert.NoError(t, err)

		err = runner.RequireCommand("nonexistentcommand123")
		assert.Error(t, err)
	})

	t.Run("RunCommand", func(t *testing.T) {
		ctx := context.Background()
		output, err := runner.RunCommand(ctx, "echo", "test")
		assert.NoError(t, err)
		assert.Contains(t, output, "test")
	})

	t.Run("RunCommandWithOutput", func(t *testing.T) {
		ctx := context.Background()
		stdout, stderr, err := runner.RunCommandWithOutput(ctx, "echo", "hello")
		assert.NoError(t, err)
		assert.Contains(t, stdout, "hello")
		assert.Empty(t, stderr)
	})

	t.Run("RunCommandInDir", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := context.Background()
		output, err := runner.RunCommandInDir(ctx, tmpDir, "pwd")
		assert.NoError(t, err)
		assert.Contains(t, output, tmpDir)
	})

	t.Run("RunCommand with timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		_, err := runner.RunCommand(ctx, "sleep", "0.1")
		assert.NoError(t, err)
	})

	t.Run("RunCommand timeout exceeded", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, err := runner.RunCommand(ctx, "sleep", "5")
		assert.Error(t, err)
	})

	t.Run("GetExitCode", func(t *testing.T) {
		ctx := context.Background()
		_, err := runner.RunCommand(ctx, "false")
		assert.Error(t, err)
		code := runner.GetExitCode(err)
		// Exit code for false is typically 1, but may vary
		assert.NotEqual(t, 0, code)
	})

	t.Run("PrepareCommand", func(t *testing.T) {
		ctx := context.Background()
		cmd := runner.PrepareCommand(ctx, "echo", "test")
		assert.NotNil(t, cmd)
	})

	t.Run("RunCommandStreaming", func(t *testing.T) {
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		err := runner.RunCommandStreaming(ctx, &stdout, &stderr, "echo", "test")
		assert.NoError(t, err)
		assert.Contains(t, stdout.String(), "test")
	})

	t.Run("RunCommandInDirStreaming", func(t *testing.T) {
		tmpDir := t.TempDir()
		ctx := context.Background()
		var stdout, stderr bytes.Buffer
		err := runner.RunCommandInDirStreaming(ctx, tmpDir, &stdout, &stderr, "pwd")
		assert.NoError(t, err)
		assert.Contains(t, stdout.String(), tmpDir)
	})
}

func TestCommandRunnerInterface(t *testing.T) {
	var _ CommandRunner = &OSCommandRunner{}
}
