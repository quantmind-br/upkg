package cache

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/quantmind-br/upkg/internal/helpers"
	"github.com/rs/zerolog"
)

// CacheManager handles cache updates
type CacheManager struct {
	runner helpers.CommandRunner
}

// NewCacheManager creates a new CacheManager with the default command runner
func NewCacheManager() *CacheManager {
	return &CacheManager{
		runner: helpers.NewOSCommandRunner(),
	}
}

// NewCacheManagerWithRunner creates a new CacheManager with a custom command runner
func NewCacheManagerWithRunner(runner helpers.CommandRunner) *CacheManager {
	return &CacheManager{
		runner: runner,
	}
}

// UpdateIconCache updates the icon cache using gtk-update-icon-cache
func (c *CacheManager) UpdateIconCache(iconDir string, log *zerolog.Logger) error {
	cmdName := c.detectIconCacheCommand()
	if cmdName == "" {
		log.Warn().Msg("gtk-update-icon-cache not found, skipping icon cache update")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	execName := cmdName
	cmdArgs := []string{"-f", "-t", iconDir}
	if c.needsSudo(iconDir) {
		execName = "sudo"
		cmdArgs = append([]string{cmdName}, cmdArgs...)
	}

	if _, err := c.runner.RunCommand(ctx, execName, cmdArgs...); err != nil {
		log.Warn().Err(err).Msg("icon cache update failed (non-fatal)")
		return nil // Non-fatal
	}

	log.Debug().Str("icon_dir", iconDir).Msg("icon cache updated")
	return nil
}

// UpdateDesktopDatabase updates the desktop database using update-desktop-database
func (c *CacheManager) UpdateDesktopDatabase(appsDir string, log *zerolog.Logger) error {
	if !c.runner.CommandExists("update-desktop-database") {
		log.Warn().Msg("update-desktop-database not found, skipping desktop database update")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	execName := "update-desktop-database"
	cmdArgs := []string{appsDir}
	if c.needsSudo(appsDir) {
		execName = "sudo"
		cmdArgs = append([]string{"update-desktop-database"}, cmdArgs...)
	}

	if _, err := c.runner.RunCommand(ctx, execName, cmdArgs...); err != nil {
		log.Warn().Err(err).Msg("desktop database update failed (non-fatal)")
		return nil // Non-fatal
	}

	log.Debug().Str("apps_dir", appsDir).Msg("desktop database updated")
	return nil
}

func (c *CacheManager) detectIconCacheCommand() string {
	if c.runner.CommandExists("gtk4-update-icon-cache") {
		return "gtk4-update-icon-cache"
	}
	if c.runner.CommandExists("gtk-update-icon-cache") {
		return "gtk-update-icon-cache"
	}
	return ""
}

func (c *CacheManager) needsSudo(path string) bool {
	cleaned := filepath.Clean(path)
	systemPrefixes := []string{"/usr", "/opt", "/var", "/etc"}
	for _, prefix := range systemPrefixes {
		if cleaned == prefix || strings.HasPrefix(cleaned, prefix+"/") {
			return true
		}
	}
	return false
}
