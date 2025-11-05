package cache

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/diogo/pkgctl/internal/helpers"
	"github.com/rs/zerolog"
)

// UpdateIconCache updates the icon cache using gtk-update-icon-cache
func UpdateIconCache(iconDir string, log *zerolog.Logger) error {
	cmdName := detectIconCacheCommand()
	if cmdName == "" {
		log.Warn().Msg("gtk-update-icon-cache not found, skipping icon cache update")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	execName := cmdName
	cmdArgs := []string{"-f", "-t", iconDir}
	if needsSudo(iconDir) {
		execName = "sudo"
		cmdArgs = append([]string{cmdName}, cmdArgs...)
	}

	if _, err := helpers.RunCommand(ctx, execName, cmdArgs...); err != nil {
		log.Warn().Err(err).Msg("icon cache update failed (non-fatal)")
		return nil // Non-fatal
	}

	log.Debug().Str("icon_dir", iconDir).Msg("icon cache updated")
	return nil
}

// UpdateDesktopDatabase updates the desktop database using update-desktop-database
func UpdateDesktopDatabase(appsDir string, log *zerolog.Logger) error {
	if !helpers.CommandExists("update-desktop-database") {
		log.Warn().Msg("update-desktop-database not found, skipping desktop database update")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	execName := "update-desktop-database"
	cmdArgs := []string{appsDir}
	if needsSudo(appsDir) {
		execName = "sudo"
		cmdArgs = append([]string{"update-desktop-database"}, cmdArgs...)
	}

	if _, err := helpers.RunCommand(ctx, execName, cmdArgs...); err != nil {
		log.Warn().Err(err).Msg("desktop database update failed (non-fatal)")
		return nil // Non-fatal
	}

	log.Debug().Str("apps_dir", appsDir).Msg("desktop database updated")
	return nil
}

func detectIconCacheCommand() string {
	if helpers.CommandExists("gtk4-update-icon-cache") {
		return "gtk4-update-icon-cache"
	}
	if helpers.CommandExists("gtk-update-icon-cache") {
		return "gtk-update-icon-cache"
	}
	return ""
}

func needsSudo(path string) bool {
	cleaned := filepath.Clean(path)
	systemPrefixes := []string{"/usr", "/opt", "/var", "/etc"}
	for _, prefix := range systemPrefixes {
		if cleaned == prefix || strings.HasPrefix(cleaned, prefix+"/") {
			return true
		}
	}
	return false
}
