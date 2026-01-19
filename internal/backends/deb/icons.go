package deb

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/quantmind-br/upkg/internal/desktop"
	"github.com/quantmind-br/upkg/internal/icons"
	"github.com/quantmind-br/upkg/internal/security"
)

var (
	iconSizePattern = regexp.MustCompile(`(?i)(\d+)x(\d+)`)
	standardSizes   = map[string]struct{}{
		"16x16":    {},
		"22x22":    {},
		"24x24":    {},
		"32x32":    {},
		"48x48":    {},
		"64x64":    {},
		"128x128":  {},
		"256x256":  {},
		"512x512":  {},
		"scalable": {},
	}
)

func iconNameMatches(iconPath, iconName string) bool {
	if iconName == "" {
		return false
	}
	base := strings.TrimSuffix(filepath.Base(iconPath), filepath.Ext(iconPath))
	return strings.EqualFold(base, iconName)
}

func iconSizeFromPath(path string) (string, bool) {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "scalable") {
		return "scalable", true
	}
	matches := iconSizePattern.FindStringSubmatch(lower)
	if len(matches) >= 3 {
		return matches[1] + "x" + matches[2], true
	}
	return "", false
}

func hasStandardIcon(iconFiles []string, iconName string) bool {
	for _, iconFile := range iconFiles {
		if !iconNameMatches(iconFile, iconName) {
			continue
		}
		size, ok := iconSizeFromPath(iconFile)
		if !ok {
			continue
		}
		if _, exists := standardSizes[size]; exists {
			return true
		}
	}
	return false
}

func selectBestIconSource(iconFiles []string, iconName string) string {
	var matches []string
	for _, iconFile := range iconFiles {
		if iconNameMatches(iconFile, iconName) {
			matches = append(matches, iconFile)
		}
	}

	if len(matches) == 0 {
		return ""
	}

	for _, iconFile := range matches {
		if strings.EqualFold(filepath.Ext(iconFile), ".svg") {
			return iconFile
		}
	}

	best := matches[0]
	bestScore := iconPathSizeScore(best)
	for _, iconFile := range matches[1:] {
		if score := iconPathSizeScore(iconFile); score > bestScore {
			best = iconFile
			bestScore = score
		}
	}

	return best
}

func iconPathSizeScore(path string) int {
	size, ok := iconSizeFromPath(path)
	if !ok {
		return 0
	}
	if size == "scalable" {
		return 100000
	}
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	if height > width {
		return height
	}
	return width
}

func (d *DebBackend) installUserIconFallback(iconFiles []string, desktopFile string) ([]string, error) {
	if len(iconFiles) == 0 || desktopFile == "" {
		return nil, nil
	}

	iconName, err := d.iconNameFromDesktopFile(desktopFile)
	if err != nil || iconName == "" {
		return nil, err
	}

	if hasStandardIcon(iconFiles, iconName) {
		return nil, nil
	}

	source := selectBestIconSource(iconFiles, iconName)
	if source == "" {
		return nil, nil
	}
	if pathErr := security.ValidatePath(source); pathErr != nil {
		return nil, fmt.Errorf("invalid icon source path: %w", pathErr)
	}

	homeDir := d.Paths.HomeDir()
	if homeDir == "" {
		return nil, nil
	}
	if homeErr := security.ValidatePath(homeDir); homeErr != nil {
		return nil, fmt.Errorf("invalid home directory: %w", homeErr)
	}

	iconSize := icons.DetectIconSize(source)
	iconDir := filepath.Join(homeDir, ".local", "share", "icons")
	manager := icons.NewManager(d.Fs, iconDir)

	installedPath, err := manager.InstallIcon(source, iconName, iconSize)
	if err != nil {
		return nil, err
	}

	d.Log.Debug().
		Str("source", source).
		Str("target", installedPath).
		Msg("installed fallback icon")

	return []string{installedPath}, nil
}

func (d *DebBackend) iconNameFromDesktopFile(desktopPath string) (string, error) {
	if desktopPath == "" {
		return "", nil
	}
	if err := security.ValidatePath(desktopPath); err != nil {
		return "", fmt.Errorf("invalid desktop file path: %w", err)
	}

	file, err := d.Fs.Open(desktopPath)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			d.Log.Debug().Err(closeErr).Str("desktop_file", desktopPath).Msg("failed to close desktop file")
		}
	}()

	entry, err := desktop.Parse(file)
	if err != nil {
		return "", err
	}

	iconName := strings.TrimSpace(entry.Icon)
	if iconName == "" || filepath.IsAbs(iconName) || strings.ContainsRune(iconName, filepath.Separator) {
		return "", nil
	}
	if err := security.ValidatePath(iconName); err != nil {
		return "", fmt.Errorf("invalid icon name: %w", err)
	}

	iconName = filepath.Base(iconName)
	iconName = strings.TrimSuffix(iconName, filepath.Ext(iconName))
	return iconName, nil
}

func (d *DebBackend) removeUserIcons(iconPaths []string) bool {
	homeDir := d.Paths.HomeDir()
	if homeDir == "" {
		return false
	}
	if err := security.ValidatePath(homeDir); err != nil {
		d.Log.Debug().Err(err).Str("home_dir", homeDir).Msg("invalid home directory")
		return false
	}

	homeDir = filepath.Clean(homeDir)
	removedAny := false

	for _, iconPath := range iconPaths {
		if err := security.ValidatePath(iconPath); err != nil {
			d.Log.Debug().Err(err).Str("path", iconPath).Msg("invalid icon path")
			continue
		}
		cleanPath := filepath.Clean(iconPath)
		if !strings.HasPrefix(cleanPath, homeDir+string(filepath.Separator)) && cleanPath != homeDir {
			continue
		}
		if err := d.Fs.Remove(cleanPath); err != nil {
			d.Log.Warn().Err(err).Str("path", cleanPath).Msg("failed to remove icon")
			continue
		}
		removedAny = true
	}

	return removedAny
}
