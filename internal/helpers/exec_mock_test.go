package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMockCommandRunner_CommandExists(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		mock := &MockCommandRunner{
			CommandExistsFunc: func(name string) bool {
				return name == "go"
			},
		}

		assert.True(t, mock.CommandExists("go"))
		assert.False(t, mock.CommandExists("unknown"))
	})

	t.Run("without custom function", func(t *testing.T) {
		mock := &MockCommandRunner{}
		assert.False(t, mock.CommandExists("go"))
	})
}

func TestMockCommandRunner_RequireCommand(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		expectedErr := errors.New("command not found")
		mock := &MockCommandRunner{
			RequireCommandFunc: func(name string) error {
				if name == "missing" {
					return expectedErr
				}
				return nil
			},
		}

		assert.NoError(t, mock.RequireCommand("go"))
		assert.Equal(t, expectedErr, mock.RequireCommand("missing"))
	})

	t.Run("without custom function", func(t *testing.T) {
		mock := &MockCommandRunner{}
		assert.NoError(t, mock.RequireCommand("anything"))
	})
}

func TestMockCommandRunner_RunCommand(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		mock := &MockCommandRunner{
			RunCommandFunc: func(_ context.Context, name string, args ...string) (string, error) {
				return "output from " + name, nil
			},
		}

		output, err := mock.RunCommand(context.Background(), "echo", "hello")
		assert.NoError(t, err)
		assert.Equal(t, "output from echo", output)
	})

	t.Run("without custom function", func(t *testing.T) {
		mock := &MockCommandRunner{}
		output, err := mock.RunCommand(context.Background(), "echo", "hello")
		assert.NoError(t, err)
		assert.Empty(t, output)
	})
}

func TestMockCommandRunner_RunCommandInDir(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		mock := &MockCommandRunner{
			RunCommandInDirFunc: func(_ context.Context, dir, name string, _ ...string) (string, error) {
				return "ran " + name + " in " + dir, nil
			},
		}

		output, err := mock.RunCommandInDir(context.Background(), "/tmp", "ls", "-la")
		assert.NoError(t, err)
		assert.Equal(t, "ran ls in /tmp", output)
	})

	t.Run("without custom function", func(t *testing.T) {
		mock := &MockCommandRunner{}
		output, err := mock.RunCommandInDir(context.Background(), "/tmp", "ls", "-la")
		assert.NoError(t, err)
		assert.Empty(t, output)
	})
}

func TestMockCommandRunner_RunCommandWithOutput(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		mock := &MockCommandRunner{
			RunCommandWithOutputFunc: func(_ context.Context, _ string, _ ...string) (stdout, stderr string, err error) {
				return "stdout", "stderr", nil
			},
		}

		stdout, stderr, err := mock.RunCommandWithOutput(context.Background(), "cmd")
		assert.NoError(t, err)
		assert.Equal(t, "stdout", stdout)
		assert.Equal(t, "stderr", stderr)
	})

	t.Run("without custom function", func(t *testing.T) {
		mock := &MockCommandRunner{}
		stdout, stderr, err := mock.RunCommandWithOutput(context.Background(), "cmd")
		assert.NoError(t, err)
		assert.Empty(t, stdout)
		assert.Empty(t, stderr)
	})
}

func TestMockCommandRunner_GetExitCode(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		mock := &MockCommandRunner{
			GetExitCodeFunc: func(_ error) int {
				return 42
			},
		}

		code := mock.GetExitCode(errors.New("some error"))
		assert.Equal(t, 42, code)
	})

	t.Run("without custom function", func(t *testing.T) {
		mock := &MockCommandRunner{}
		code := mock.GetExitCode(errors.New("some error"))
		assert.Equal(t, 0, code)
	})
}

func TestMockCommandRunner_RunCommandStreaming(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		var stdoutBuf, stderrBuf bytes.Buffer
		mock := &MockCommandRunner{
			RunCommandStreamingFunc: func(_ context.Context, stdout, stderr io.Writer, _ string, _ ...string) error {
				// Write to the buffers
				if w, ok := stdout.(*bytes.Buffer); ok {
					w.WriteString("streaming output")
				}
				return nil
			},
		}

		err := mock.RunCommandStreaming(context.Background(), &stdoutBuf, &stderrBuf, "cmd")
		assert.NoError(t, err)
		assert.Equal(t, "streaming output", stdoutBuf.String())
	})

	t.Run("without custom function", func(t *testing.T) {
		var stdoutBuf, stderrBuf bytes.Buffer
		mock := &MockCommandRunner{}
		err := mock.RunCommandStreaming(context.Background(), &stdoutBuf, &stderrBuf, "cmd")
		assert.NoError(t, err)
	})
}

func TestMockCommandRunner_RunCommandInDirStreaming(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		var stdoutBuf, stderrBuf bytes.Buffer
		mock := &MockCommandRunner{
			RunCommandInDirStreamingFunc: func(_ context.Context, dir string, stdout, _ io.Writer, _ string, _ ...string) error {
				if w, ok := stdout.(*bytes.Buffer); ok {
					w.WriteString("output in " + dir)
				}
				return nil
			},
		}

		err := mock.RunCommandInDirStreaming(context.Background(), "/tmp", &stdoutBuf, &stderrBuf, "cmd")
		assert.NoError(t, err)
		assert.Equal(t, "output in /tmp", stdoutBuf.String())
	})

	t.Run("without custom function", func(t *testing.T) {
		var stdoutBuf, stderrBuf bytes.Buffer
		mock := &MockCommandRunner{}
		err := mock.RunCommandInDirStreaming(context.Background(), "/tmp", &stdoutBuf, &stderrBuf, "cmd")
		assert.NoError(t, err)
	})
}

func TestMockCommandRunner_PrepareCommand(t *testing.T) {
	t.Parallel()

	t.Run("with custom function", func(t *testing.T) {
		expectedCmd := exec.Command("echo", "test")
		mock := &MockCommandRunner{
			PrepareCommandFunc: func(_ context.Context, _ string, _ ...string) *exec.Cmd {
				return expectedCmd
			},
		}

		cmd := mock.PrepareCommand(context.Background(), "echo", "test")
		assert.Equal(t, expectedCmd, cmd)
	})

	t.Run("without custom function", func(t *testing.T) {
		mock := &MockCommandRunner{}
		cmd := mock.PrepareCommand(context.Background(), "echo", "test")
		assert.Nil(t, cmd)
	})
}
