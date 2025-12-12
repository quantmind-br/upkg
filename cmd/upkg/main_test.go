package main

import (
	"context"
	"testing"

	"github.com/quantmind-br/upkg/internal/cmd"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const colorNever = "never"

func TestMain(t *testing.T) {
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

	// Test command execution
	rootCmd := cmd.NewRootCmd(cfg, log, version)
	ctx := context.Background()
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

func TestCommandExecution(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err, "Configuration should load without error")

	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == colorNever,
	})

	rootCmd := cmd.NewRootCmd(cfg, log, version)
	ctx := context.Background()
	err = rootCmd.ExecuteContext(ctx)
	assert.NoError(t, err, "Command execution should not return an error")
}
