package helpers

import (
	"bytes"
	"context"
	"os"
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

func TestCommandRunnerInterface(_ *testing.T) {
	var _ CommandRunner = &OSCommandRunner{}
}

func TestValidateDesktopFile(t *testing.T) {
	t.Run("non-existent desktop file", func(t *testing.T) {
		output, valid, err := ValidateDesktopFile("/nonexistent/file.desktop")
		assert.NoError(t, err)
		// desktop-file-validate returns output but still reports as invalid
		// The function returns valid=false for validation failures
		assert.False(t, valid, "Non-existent file should be invalid")
		// Output may contain error message from desktop-file-validate
		assert.NotEmpty(t, output, "Should have validation output")
	})

	t.Run("tool not available", func(t *testing.T) {
		// Create a mock runner where desktop-file-validate is not available
		originalRunner := NewOSCommandRunner()
		if originalRunner.CommandExists("desktop-file-validate") {
			t.Skip("desktop-file-validate is available, cannot test absence")
		}

		tmpDir := t.TempDir()
		desktopPath := tmpDir + "/test.desktop"
		err := os.WriteFile(desktopPath, []byte("[Desktop Entry]\nType=Application\nName=Test"), 0644)
		if err != nil {
			t.Fatal(err)
		}

		output, valid, err := ValidateDesktopFile(desktopPath)
		assert.NoError(t, err)
		assert.True(t, valid, "Should be valid when tool is not available")
		assert.Empty(t, output)
	})

	t.Run("valid desktop file with tool available", func(t *testing.T) {
		runner := NewOSCommandRunner()
		if !runner.CommandExists("desktop-file-validate") {
			t.Skip("desktop-file-validate not available")
		}

		tmpDir := t.TempDir()
		desktopPath := tmpDir + "/test.desktop"
		content := `[Desktop Entry]
Type=Application
Name=Test Application
Exec=test
Icon=test
Categories=Utility;`
		err := os.WriteFile(desktopPath, []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}

		_, valid, err := ValidateDesktopFile(desktopPath)
		assert.NoError(t, err)
		assert.True(t, valid, "Valid desktop file should pass validation")
	})

	t.Run("invalid desktop file", func(t *testing.T) {
		runner := NewOSCommandRunner()
		if !runner.CommandExists("desktop-file-validate") {
			t.Skip("desktop-file-validate not available")
		}

		tmpDir := t.TempDir()
		desktopPath := tmpDir + "/invalid.desktop"
		// Missing required keys
		err := os.WriteFile(desktopPath, []byte("Not a desktop file"), 0644)
		if err != nil {
			t.Fatal(err)
		}

		output, valid, err := ValidateDesktopFile(desktopPath)
		assert.NoError(t, err)
		assert.False(t, valid, "Invalid desktop file should fail validation")
		assert.NotEmpty(t, output, "Should have validation output for invalid file")
	})
}

func TestGetExitCode(t *testing.T) {
	runner := NewOSCommandRunner()

	t.Run("exit code 0", func(t *testing.T) {
		ctx := context.Background()
		_, err := runner.RunCommand(ctx, "true")
		assert.NoError(t, err)
		code := runner.GetExitCode(err)
		assert.Equal(t, 0, code)
	})

	t.Run("exit code 1", func(t *testing.T) {
		ctx := context.Background()
		_, err := runner.RunCommand(ctx, "false")
		assert.Error(t, err)
		code := runner.GetExitCode(err)
		assert.Equal(t, 1, code)
	})

	t.Run("exit code from command", func(t *testing.T) {
		ctx := context.Background()
		_, err := runner.RunCommand(ctx, "sh", "-c", "exit 42")
		assert.Error(t, err)
		code := runner.GetExitCode(err)
		assert.Equal(t, 42, code)
	})

	t.Run("nil error returns 0", func(t *testing.T) {
		code := runner.GetExitCode(nil)
		assert.Equal(t, 0, code)
	})
}
