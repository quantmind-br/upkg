package helpers

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

// CommandExists checks if a command is available in PATH
func CommandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// RequireCommand ensures a command exists or returns error
func RequireCommand(name string) error {
	if !CommandExists(name) {
		return fmt.Errorf("required command %q not found in PATH", name)
	}
	return nil
}

// RunCommand executes a command with timeout and returns stdout
// SECURITY: Uses exec.CommandContext with separate arguments to prevent command injection
func RunCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("command %q failed: %w\nstderr: %s", name, err, stderr.String())
	}

	return stdout.String(), nil
}

// RunCommandWithOutput runs a command and returns both stdout and stderr
func RunCommandWithOutput(ctx context.Context, name string, args ...string) (stdout, stderr string, err error) {
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

// RunCommandWithTimeout executes a command with a specific timeout
func RunCommandWithTimeout(name string, timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return RunCommand(ctx, name, args...)
}

// GetExitCode extracts the exit code from a command error
func GetExitCode(err error) int {
	if err == nil {
		return 0
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}

	return -1
}

// ValidateDesktopFile validates a .desktop file and returns warnings/errors
// Returns (validationOutput, isValid, error)
func ValidateDesktopFile(desktopFilePath string) (string, bool, error) {
	if !CommandExists("desktop-file-validate") {
		return "", true, nil // Tool not available, skip validation
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stdout, stderr, err := RunCommandWithOutput(ctx, "desktop-file-validate", desktopFilePath)

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
