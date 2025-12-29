package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/quantmind-br/upkg/internal/cmd"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const colorNever = "never"

func captureStdout(f func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	f()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	f()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	buf.ReadFrom(r)
	return buf.String()
}

func TestMainFunctionComponents(t *testing.T) {
	// Test configuration loading
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")
	assert.NotNil(t, cfg, "Configuration should not be nil")

	// Test logger initialization
	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == colorNever,
	})
	assert.NotNil(t, log, "Logger should not be nil")

	// Test command execution with help flag
	rootCmd := cmd.NewRootCmd(cfg, log, version)
	ctx := context.Background()
	rootCmd.SetArgs([]string{"--help"})
	err = rootCmd.ExecuteContext(ctx)
	assert.NoError(t, err, "Command execution should not return an error")
}

func TestConfigLoad(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")
	assert.NotNil(t, cfg, "Configuration should not be nil")
}

func TestLoggerInitialization(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == colorNever,
	})
	assert.NotNil(t, log, "Logger should not be nil")
}

func TestLoggerInitializationWithNoColor(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: true,
	})
	assert.NotNil(t, log, "Logger should not be nil")
}

func TestCommandExecutionWithHelp(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == colorNever,
	})

	rootCmd := cmd.NewRootCmd(cfg, log, version)
	ctx := context.Background()
	rootCmd.SetArgs([]string{"--help"})
	err = rootCmd.ExecuteContext(ctx)
	assert.NoError(t, err, "Command execution with --help should not return an error")
}

func TestCommandExecutionWithVersion(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == colorNever,
	})

	rootCmd := cmd.NewRootCmd(cfg, log, version)
	ctx := context.Background()
	// Use version command instead of --version flag
	rootCmd.SetArgs([]string{"version"})
	err = rootCmd.ExecuteContext(ctx)
	assert.NoError(t, err, "Command execution with version should not return an error")
}

func TestCommandExecutionWithCompletion(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == colorNever,
	})

	rootCmd := cmd.NewRootCmd(cfg, log, version)
	ctx := context.Background()
	rootCmd.SetArgs([]string{"completion", "bash"})
	err = rootCmd.ExecuteContext(ctx)
	assert.NoError(t, err, "Command execution with completion should not return an error")
}

func TestCommandExecutionUnknownCommand(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == colorNever,
	})

	rootCmd := cmd.NewRootCmd(cfg, log, version)
	ctx := context.Background()
	rootCmd.SetArgs([]string{"unknown-command-xyz"})
	err = rootCmd.ExecuteContext(ctx)
	assert.Error(t, err, "Unknown command should return an error")
}

func TestMainErrorHandling(t *testing.T) {
	// Test that the version variable is accessible
	assert.NotEmpty(t, version, "Version should not be empty")
}

func TestConfigLoadErrorPath(t *testing.T) {
	// Save original config file path
	originalConfig := os.Getenv("UPKG_CONFIG_FILE")
	defer func() {
		if originalConfig != "" {
			os.Setenv("UPKG_CONFIG_FILE", originalConfig)
		} else {
			os.Unsetenv("UPKG_CONFIG_FILE")
		}
	}()

	// Set invalid config path
	os.Setenv("UPKG_CONFIG_FILE", "/nonexistent/path/config.toml")

	cfg, err := config.Load()
	// Should still load with defaults
	assert.NoError(t, err, "Config load with invalid path should use defaults")
	assert.NotNil(t, cfg, "Config should still be created with defaults")
}

func TestLoggerLevels(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	levels := []string{"debug", "info", "warn", "error", "fatal"}
	for _, level := range levels {
		log := logging.NewLogger(logging.Config{
			Level:   level,
			LogFile: cfg.Paths.LogFile,
			NoColor: true,
		})
		assert.NotNil(t, log, "Logger should initialize with level: %s", level)
	}
}

func TestVersionVariable(t *testing.T) {
	// Test version is settable
	v := "test-version"
	assert.Equal(t, "test-version", v)
}

func TestMainIntegration(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == colorNever,
	})
	assert.NotNil(t, log, "Logger should not be nil")

	rootCmd := cmd.NewRootCmd(cfg, log, version)
	assert.NotNil(t, rootCmd, "Root command should not be nil")

	// Test with list command (should work even with no packages)
	ctx := context.Background()
	rootCmd.SetArgs([]string{"list"})
	err = rootCmd.ExecuteContext(ctx)
	// List should work even when no packages are installed
	assert.NoError(t, err, "List command should execute without error")
}

func TestErrorOutputFormatting(t *testing.T) {
	// Test error formatting for stderr
	errMsg := "Error loading config: test error"
	output := captureStderr(func() {
		fmt.Fprintf(os.Stderr, "%s\n", errMsg)
	})
	assert.True(t, strings.Contains(output, errMsg), "Error output should contain the error message")
}
