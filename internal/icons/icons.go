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

	"github.com/quantmind-br/upkg/internal/core"
	"github.com/spf13/afero"
)

// standardSizes contains the XDG-compliant hicolor icon sizes that desktop
// environments actually search. Icons installed to non-standard sizes
// (like 4096x4096) will not be found by the theme engine.
var standardSizes = []int{16, 22, 24, 32, 48, 64, 128, 256, 512}

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
		// Note: .ico files are skipped because Windows ICO format is not supported
		// by Linux desktop environments in the hicolor icon theme
		if ext == ".png" || ext == ".svg" || ext == ".xpm" {
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

	// Get the larger dimension for non-square images
	// (hicolor theme expects square sizes like 48x48, 256x256, etc.)
	width := config.Width
	height := config.Height
	dimension := width
	if height > width {
		dimension = height
	}

	// Normalize to standard XDG hicolor size
	normalized := normalizeToStandardSize(dimension)
	return fmt.Sprintf("%dx%d", normalized, normalized)
}

// normalizeToStandardSize maps an arbitrary dimension to the nearest standard
// XDG hicolor icon size. Uses "round up to nearest standard" strategy:
// - Dimensions smaller than or equal to a standard size map to that size
// - Dimensions larger than 512 map to 512 (the largest standard size)
// This ensures icons are placed in directories that desktop environments search.
func normalizeToStandardSize(dimension int) int {
	// Find the smallest standard size >= dimension
	for _, size := range standardSizes {
		if dimension <= size {
			return size
		}
	}
	// If larger than all standard sizes, use the largest (512)
	return standardSizes[len(standardSizes)-1]
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
	icons, err := m.DiscoverIcons(sourceDir)
	if err != nil {
		return nil
	}
	return icons
}

// InstallIcon installs an icon file to the hicolor theme (convenience function)
func InstallIcon(iconFile core.IconFile, normalizedName, homeDir string) (string, error) {
	iconDir := filepath.Join(homeDir, ".local", "share", "icons")
	m := NewManager(afero.NewOsFs(), iconDir)

	return m.InstallIcon(iconFile.Path, normalizedName, iconFile.Size)
}
