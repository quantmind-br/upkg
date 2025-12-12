package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	Paths   PathsConfig   `mapstructure:"paths"`
	Desktop DesktopConfig `mapstructure:"desktop"`
	Logging LoggingConfig `mapstructure:"logging"`
}

// PathsConfig contains path-related configuration
type PathsConfig struct {
	DataDir string `mapstructure:"data_dir"`
	DBFile  string `mapstructure:"db_file"`
	LogFile string `mapstructure:"log_file"`
}

// DesktopConfig contains desktop integration configuration
type DesktopConfig struct {
	WaylandEnvVars         bool     `mapstructure:"wayland_env_vars"`
	CustomEnvVars          []string `mapstructure:"custom_env_vars"`
	ElectronDisableSandbox bool     `mapstructure:"electron_disable_sandbox"`
}

// LoggingConfig contains logging configuration
type LoggingConfig struct {
	Level string `mapstructure:"level"`
	Color string `mapstructure:"color"`
}

// Load loads configuration from file and environment
func Load() (*Config, error) {
	// Set config name and paths
	viper.SetConfigName("config")
	viper.SetConfigType("toml")

	// Add config paths
	homeDir, err := os.UserHomeDir()
	if err == nil {
		viper.AddConfigPath(filepath.Join(homeDir, ".config", "upkg"))
	}
	viper.AddConfigPath(".")

	// Set defaults
	setDefaults()

	// Environment variable overrides
	viper.SetEnvPrefix("UPKG")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// Read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
		// Config file not found - use defaults
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Expand paths
	cfg.Paths.DataDir = expandPath(cfg.Paths.DataDir)
	cfg.Paths.DBFile = expandPath(cfg.Paths.DBFile)
	cfg.Paths.LogFile = expandPath(cfg.Paths.LogFile)

	return &cfg, nil
}

// setDefaults sets default configuration values
func setDefaults() {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		homeDir = os.Getenv("HOME")
	}
	if homeDir == "" {
		homeDir = "."
	}

	viper.SetDefault("paths.data_dir", filepath.Join(homeDir, ".local", "share", "upkg"))
	viper.SetDefault("paths.db_file", filepath.Join(homeDir, ".local", "share", "upkg", "installed.db"))
	viper.SetDefault("paths.log_file", filepath.Join(homeDir, ".local", "share", "upkg", "upkg.log"))

	viper.SetDefault("desktop.wayland_env_vars", true)
	viper.SetDefault("desktop.custom_env_vars", []string{})
	viper.SetDefault("desktop.electron_disable_sandbox", false) // Sandbox enabled by default for security

	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.color", "auto")
}

// expandPath expands ~ and environment variables in paths
func expandPath(path string) string {
	if path == "" {
		return path
	}

	// Expand ~
	if len(path) > 0 && path[0] == '~' {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(homeDir, path[1:])
		}
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	return path
}
