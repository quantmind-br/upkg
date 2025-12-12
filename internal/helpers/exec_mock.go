package helpers

import (
	"context"
	"io"
	"os/exec"
)

// MockCommandRunner is a mock implementation of CommandRunner for testing
type MockCommandRunner struct {
	CommandExistsFunc          func(name string) bool
	RequireCommandFunc         func(name string) error
	RunCommandFunc             func(ctx context.Context, name string, args ...string) (string, error)
	RunCommandInDirFunc        func(ctx context.Context, dir, name string, args ...string) (string, error)
	RunCommandWithOutputFunc   func(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
	GetExitCodeFunc            func(err error) int
	RunCommandStreamingFunc    func(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error
	RunCommandInDirStreamingFunc func(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error
	PrepareCommandFunc         func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// CommandExists implements CommandRunner.CommandExists
func (m *MockCommandRunner) CommandExists(name string) bool {
	if m.CommandExistsFunc != nil {
		return m.CommandExistsFunc(name)
	}
	return false
}

// RequireCommand implements CommandRunner.RequireCommand
func (m *MockCommandRunner) RequireCommand(name string) error {
	if m.RequireCommandFunc != nil {
		return m.RequireCommandFunc(name)
	}
	return nil
}

// RunCommand implements CommandRunner.RunCommand
func (m *MockCommandRunner) RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	if m.RunCommandFunc != nil {
		return m.RunCommandFunc(ctx, name, args...)
	}
	return "", nil
}

// RunCommandInDir implements CommandRunner.RunCommandInDir
func (m *MockCommandRunner) RunCommandInDir(ctx context.Context, dir, name string, args ...string) (string, error) {
	if m.RunCommandInDirFunc != nil {
		return m.RunCommandInDirFunc(ctx, dir, name, args...)
	}
	return "", nil
}

// RunCommandWithOutput implements CommandRunner.RunCommandWithOutput
func (m *MockCommandRunner) RunCommandWithOutput(ctx context.Context, name string, args ...string) (stdout, stderr string, err error) {
	if m.RunCommandWithOutputFunc != nil {
		return m.RunCommandWithOutputFunc(ctx, name, args...)
	}
	return "", "", nil
}

// GetExitCode implements CommandRunner.GetExitCode
func (m *MockCommandRunner) GetExitCode(err error) int {
	if m.GetExitCodeFunc != nil {
		return m.GetExitCodeFunc(err)
	}
	return 0
}

// RunCommandStreaming implements CommandRunner.RunCommandStreaming
func (m *MockCommandRunner) RunCommandStreaming(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	if m.RunCommandStreamingFunc != nil {
		return m.RunCommandStreamingFunc(ctx, stdout, stderr, name, args...)
	}
	return nil
}

// RunCommandInDirStreaming implements CommandRunner.RunCommandInDirStreaming
func (m *MockCommandRunner) RunCommandInDirStreaming(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error {
	if m.RunCommandInDirStreamingFunc != nil {
		return m.RunCommandInDirStreamingFunc(ctx, dir, stdout, stderr, name, args...)
	}
	return nil
}

// PrepareCommand implements CommandRunner.PrepareCommand
func (m *MockCommandRunner) PrepareCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	if m.PrepareCommandFunc != nil {
		return m.PrepareCommandFunc(ctx, name, args...)
	}
	return nil
}
