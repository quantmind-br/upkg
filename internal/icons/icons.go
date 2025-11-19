package icons

import (
	"fmt"
	"image"
	_ "image/gif"  // Register GIF format
	_ "image/jpeg" // Register JPEG format
	_ "image/png"  // Register PNG format
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diogo/upkg/internal/core"
	"github.com/spf13/afero"
)

// Manager handles icon operations
type Manager struct {
	fs      afero.Fs
	iconDir string
}

// NewManager creates a new icon manager
func NewManager(fs afero.Fs, iconDir string) *Manager {
	return &Manager{
		fs:      fs,
		iconDir: iconDir,
	}
}

// DiscoverIcons finds icons in a directory
func (m *Manager) DiscoverIcons(sourceDir string) ([]core.IconFile, error) {
	var icons []core.IconFile

	// Walk directory to find icon files
	err := afero.Walk(m.fs, sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".png" || ext == ".svg" || ext == ".ico" || ext == ".xpm" {
			size := DetectIconSize(path)
			icons = append(icons, core.IconFile{
				Path: path,
				Size: size,
				Ext:  ext[1:], // remove dot
			})
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return icons, nil
}

// DetectIconSize detects icon size from path or filename
func DetectIconSize(iconPath string) string {
	// Try to detect from path (e.g., "48x48", "256x256")
	re := regexp.MustCompile(`(\d+)x(\d+)`)
	matches := re.FindStringSubmatch(iconPath)
	if len(matches) >= 2 {
		return matches[0]
	}

	// Check for "scalable"
	if strings.Contains(strings.ToLower(iconPath), "scalable") || strings.HasSuffix(strings.ToLower(iconPath), ".svg") {
		return "scalable"
	}

	// Try to read actual image dimensions for PNG/JPEG/GIF
	if size := getImageDimensions(iconPath); size != "" {
		return size
	}

	// Default to 48x48 if unknown
	return "48x48"
}

// getImageDimensions reads actual dimensions from image file
func getImageDimensions(imagePath string) string {
	ext := strings.ToLower(filepath.Ext(imagePath))

	// Only try to read dimensions for supported formats
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" && ext != ".gif" {
		return ""
	}

	file, err := os.Open(imagePath)
	if err != nil {
		return ""
	}
	defer file.Close()

	// Decode only the config (dimensions) without loading full image
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return ""
	}

	// Return dimensions as "WxH" string
	width := config.Width
	height := config.Height

	// For non-square images, use the larger dimension for both
	// (hicolor theme expects square sizes like 48x48, 256x256, etc.)
	if width != height {
		if width > height {
			return fmt.Sprintf("%dx%d", width, width)
		}
		return fmt.Sprintf("%dx%d", height, height)
	}

	return fmt.Sprintf("%dx%d", width, height)
}

// NormalizeIconName normalizes an icon name
func NormalizeIconName(rawName string) string {
	// Strip path and extension
	base := filepath.Base(rawName)
	base = strings.TrimSuffix(base, filepath.Ext(base))

	// Lowercase
	base = strings.ToLower(base)

	// Replace non-alphanumeric (except ._-) with -
	reg := regexp.MustCompile(`[^a-z0-9._-]`)
	return reg.ReplaceAllString(base, "-")
}

// InstallIcon installs an icon to the hicolor theme
func (m *Manager) InstallIcon(srcPath, normalizedName, size string) (string, error) {
	// Determine destination path
	ext := filepath.Ext(srcPath)
	dstPath := filepath.Join(m.iconDir, "hicolor", size, "apps", normalizedName+ext)

	// Ensure directory exists
	dstDir := filepath.Dir(dstPath)
	if err := m.fs.MkdirAll(dstDir, 0755); err != nil {
		return "", fmt.Errorf("create icon directory: %w", err)
	}

	// Copy icon
	content, err := afero.ReadFile(m.fs, srcPath)
	if err != nil {
		return "", fmt.Errorf("read source icon: %w", err)
	}

	if err := afero.WriteFile(m.fs, dstPath, content, 0644); err != nil {
		return "", fmt.Errorf("write destination icon: %w", err)
	}

	return dstPath, nil
}

// Package-level convenience functions

// DiscoverIcons finds icons in a directory (convenience function)
func DiscoverIcons(sourceDir string) []core.IconFile {
	m := NewManager(afero.NewOsFs(), "")
	icons, _ := m.DiscoverIcons(sourceDir)
	return icons
}

// InstallIcon installs an icon file to the hicolor theme (convenience function)
func InstallIcon(iconFile core.IconFile, normalizedName, homeDir string) (string, error) {
	iconDir := filepath.Join(homeDir, ".local", "share", "icons")
	m := NewManager(afero.NewOsFs(), iconDir)

	return m.InstallIcon(iconFile.Path, normalizedName, iconFile.Size)
}
