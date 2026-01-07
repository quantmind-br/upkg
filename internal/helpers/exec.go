package helpers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// CommandRunner defines an interface for executing system commands
// This allows for mocking in tests and dependency injection
type CommandRunner interface {
	// CommandExists checks if a command is available in PATH
	CommandExists(name string) bool

	// RequireCommand ensures a command exists or returns error
	RequireCommand(name string) error

	// RunCommand executes a command with timeout and returns stdout
	RunCommand(ctx context.Context, name string, args ...string) (string, error)

	// RunCommandInDir executes a command in a specific working directory
	RunCommandInDir(ctx context.Context, dir, name string, args ...string) (string, error)

	// RunCommandWithOutput runs a command and returns both stdout and stderr
	RunCommandWithOutput(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)

	// GetExitCode extracts the exit code from a command error
	GetExitCode(err error) int

	// RunCommandStreaming executes a command and streams output to provided writers
	RunCommandStreaming(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error

	// RunCommandInDirStreaming executes a command in a specific directory with streaming output
	RunCommandInDirStreaming(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error

	// PrepareCommand prepares a command but does not execute it
	PrepareCommand(ctx context.Context, name string, args ...string) *exec.Cmd
}

// OSCommandRunner is the default implementation using os/exec
type OSCommandRunner struct {
	commandCache sync.Map // map[string]bool
}

// NewOSCommandRunner creates a new OSCommandRunner instance
func NewOSCommandRunner() *OSCommandRunner {
	return &OSCommandRunner{}
}

// CommandExists checks if a command is available in PATH
func (r *OSCommandRunner) CommandExists(name string) bool {
	if cached, ok := r.commandCache.Load(name); ok {
		if exists, ok := cached.(bool); ok {
			return exists
		}
		r.commandCache.Delete(name)
	}

	_, err := exec.LookPath(name)
	exists := err == nil
	r.commandCache.Store(name, exists)
	return exists
}

// RequireCommand ensures a command exists or returns error
func (r *OSCommandRunner) RequireCommand(name string) error {
	if !r.CommandExists(name) {
		return fmt.Errorf("required command %q not found in PATH", name)
	}
	return nil
}

// RunCommand executes a command with timeout and returns stdout
// SECURITY: Uses exec.CommandContext with separate arguments to prevent command injection
func (r *OSCommandRunner) RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command %q failed: %w\nstderr: %s", name, err, stderr.String())
	}

	return stdout.String(), nil
}

// RunCommandInDir executes a command in a specific working directory
// SECURITY: Uses exec.CommandContext with separate arguments to prevent command injection
func (r *OSCommandRunner) RunCommandInDir(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command %q failed: %w\nstderr: %s", name, err, stderr.String())
	}

	return stdout.String(), nil
}

// RunCommandWithOutput runs a command and returns both stdout and stderr
func (r *OSCommandRunner) RunCommandWithOutput(ctx context.Context, name string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err = cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	if err != nil {
		err = fmt.Errorf("command %q failed: %w", name, err)
	}

	return stdout, stderr, err
}

// GetExitCode extracts the exit code from a command error
func (r *OSCommandRunner) GetExitCode(err error) int {
	if err == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}

	return -1
}

// RunCommandStreaming executes a command and streams output to provided writers
// This avoids buffering large outputs in memory, reducing memory pressure
// Pass nil for stdout/stderr to discard output (equivalent to > /dev/null)
// SECURITY: Uses exec.CommandContext with separate arguments to prevent command injection
func (r *OSCommandRunner) RunCommandStreaming(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)

	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed: %w", name, err)
	}

	return nil
}

// RunCommandInDirStreaming executes a command in a specific directory with streaming output
// SECURITY: Uses exec.CommandContext with separate arguments to prevent command injection
func (r *OSCommandRunner) RunCommandInDirStreaming(ctx context.Context, dir string, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	if stdout != nil {
		cmd.Stdout = stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command %q failed in dir %q: %w", name, dir, err)
	}

	return nil
}

// PrepareCommand prepares a command but does not execute it
// Callers can configure Stdout/Stderr/Stdin and other settings before calling Run() or Start()
// This provides maximum flexibility for custom command execution
// SECURITY: Uses exec.CommandContext with separate arguments to prevent command injection
func (r *OSCommandRunner) PrepareCommand(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// ValidateDesktopFile validates a .desktop file and returns warnings/errors
// Returns (validationOutput, isValid, error)
func ValidateDesktopFile(desktopFilePath string) (string, bool, error) {
	runner := NewOSCommandRunner()
	if !runner.CommandExists("desktop-file-validate") {
		return "", true, nil // Tool not available, skip validation
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdout, stderr, err := runner.RunCommandWithOutput(ctx, "desktop-file-validate", desktopFilePath)

	// Combine stdout and stderr for validation output
	output := stdout
	if stderr != "" {
		if output != "" {
			output += "\n"
		}
		output += stderr
	}

	// desktop-file-validate returns non-zero for errors/warnings
	if err != nil {
		return output, false, nil // Invalid but not a command execution error
	}

	return output, true, nil
}
