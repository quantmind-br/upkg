package main

import (
	"context"
	"fmt"
	"os"

	"github.com/quantmind-br/upkg/internal/cmd"
	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/logging"
)

var version = "dev"

func main() {
	ctx := context.Background()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logging.NewLogger(logging.Config{
		Level:   cfg.Logging.Level,
		LogFile: cfg.Paths.LogFile,
		NoColor: cfg.Logging.Color == "never",
	})

	// Execute root command
	rootCmd := cmd.NewRootCmd(cfg, log, version)
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		log.Error().Err(err).Msg("command failed")
		os.Exit(1)
	}
}
