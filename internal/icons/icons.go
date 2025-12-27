package icons

import (
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"  // Register GIF format
	_ "image/jpeg" // Register JPEG format
	"image/png"

	// Explicitly import for encoding
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/quantmind-br/upkg/internal/core"
	"github.com/spf13/afero"
	xdraw "golang.org/x/image/draw"
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

		// Skip symlinks to avoid duplicates when icons are referenced both as
		// symlinks and as actual files (common in AppImages)
		if info.Mode()&os.ModeSymlink != 0 {
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
	if len(matches) >= 3 {
		// Parse dimensions - regex guarantees these are valid integers
		w, _ := strconv.Atoi(matches[1]) //nolint:errcheck // regex ensures valid int
		h, _ := strconv.Atoi(matches[2]) //nolint:errcheck // regex ensures valid int

		// Use largest dimension
		top := w
		if h > w {
			top = h
		}

		// Normalize to standard size
		normalized := normalizeToStandardSize(top)
		return fmt.Sprintf("%dx%d", normalized, normalized)
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
	if err := m.ensureHicolorIndex(size); err != nil {
		return "", err
	}

	// Determine destination path
	ext := filepath.Ext(srcPath)
	dstPath := filepath.Join(m.iconDir, "hicolor", size, "apps", normalizedName+ext)

	// Ensure directory exists
	dstDir := filepath.Dir(dstPath)
	if err := m.fs.MkdirAll(dstDir, 0755); err != nil {
		return "", fmt.Errorf("create icon directory: %w", err)
	}

	// Parse target size
	var targetSize int
	if _, err := fmt.Sscanf(size, "%dx%d", &targetSize, &targetSize); err != nil {
		// If not a standard resolution (e.g. "scalable"), just copy
		return m.copyIcon(srcPath, dstPath)
	}

	// Read source file
	srcFile, err := m.fs.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("open source icon: %w", err)
	}
	defer srcFile.Close()

	// Decode source config to check dimensions
	config, _, err := image.DecodeConfig(srcFile)
	if err != nil {
		// If not an image we can decode, just copy
		return m.copyIcon(srcPath, dstPath)
	}

	// If source is larger than target, resize
	// We only resize if it's significantly larger to avoid quality loss on small diffs
	// Also only if target provided (standardSizes check implicitly done by DetectIconSize logic)
	if config.Width > targetSize || config.Height > targetSize {
		// Reset file pointer
		if _, err := srcFile.Seek(0, 0); err != nil {
			return m.copyIcon(srcPath, dstPath)
		}
		srcImg, _, err := image.Decode(srcFile)
		if err != nil {
			return m.copyIcon(srcPath, dstPath)
		}

		// Create target image
		dstImg := image.NewRGBA(image.Rect(0, 0, targetSize, targetSize))

		// Resize using Catmull-Rom resampling for high quality
		xdraw.CatmullRom.Scale(dstImg, dstImg.Bounds(), srcImg, srcImg.Bounds(), draw.Over, nil)

		// Create destination file
		// Note: We force PNG extension for resized images as we always encode to PNG
		dstPath = filepath.Join(m.iconDir, "hicolor", size, "apps", normalizedName+".png")
		dstFile, err := m.fs.Create(dstPath)
		if err != nil {
			return "", fmt.Errorf("create destination icon: %w", err)
		}
		defer dstFile.Close()

		// Encode as PNG
		if err := png.Encode(dstFile, dstImg); err != nil {
			return "", fmt.Errorf("encode resized icon: %w", err)
		}

		return dstPath, nil
	}

	return m.copyIcon(srcPath, dstPath)
}

func (m *Manager) ensureHicolorIndex(size string) error {
	if size == "" {
		return nil
	}

	hicolorDir := filepath.Join(m.iconDir, "hicolor")
	if err := m.fs.MkdirAll(hicolorDir, 0755); err != nil {
		return fmt.Errorf("create hicolor dir: %w", err)
	}

	indexPath := filepath.Join(hicolorDir, "index.theme")
	lines, err := m.readIndexTheme(indexPath)
	if err != nil {
		return err
	}

	dirName := size + "/apps"
	lines, modified := m.ensureIconThemeSection(lines, dirName)
	lines, sectionAdded := m.ensureDirectorySection(lines, dirName, size)

	if !modified && !sectionAdded {
		return nil
	}

	output := strings.Join(lines, "\n") + "\n"
	if err := afero.WriteFile(m.fs, indexPath, []byte(output), 0644); err != nil {
		return fmt.Errorf("write index.theme: %w", err)
	}

	return nil
}

// readIndexTheme reads and parses the index.theme file
func (m *Manager) readIndexTheme(indexPath string) ([]string, error) {
	content, err := afero.ReadFile(m.fs, indexPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read index.theme: %w", err)
	}

	if err != nil {
		return []string{}, nil
	}

	trimmed := strings.TrimRight(string(content), "\n")
	if trimmed == "" {
		return []string{}, nil
	}
	return strings.Split(trimmed, "\n"), nil
}

// ensureIconThemeSection ensures the [Icon Theme] section exists with the directory
func (m *Manager) ensureIconThemeSection(lines []string, dirName string) ([]string, bool) {
	iconThemeStart, iconThemeEnd := findSection(lines, "Icon Theme")
	if iconThemeStart == -1 {
		return m.createIconThemeSection(lines, dirName), true
	}
	return m.updateDirectoriesLine(lines, iconThemeStart, iconThemeEnd, dirName)
}

// createIconThemeSection creates a new [Icon Theme] section
func (m *Manager) createIconThemeSection(lines []string, dirName string) []string {
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	return append(lines,
		"[Icon Theme]",
		"Name=Hicolor",
		"Comment=Fallback icon theme",
		"Hidden=true",
		"Directories="+dirName,
	)
}

// updateDirectoriesLine updates the Directories= line in an existing section
func (m *Manager) updateDirectoriesLine(lines []string, start, end int, dirName string) ([]string, bool) {
	dirIdx := -1
	for i := start + 1; i < end; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "Directories=") {
			dirIdx = i
			break
		}
	}

	if dirIdx == -1 {
		insertAt := end
		lines = append(lines[:insertAt], append([]string{"Directories=" + dirName}, lines[insertAt:]...)...)
		return lines, true
	}

	dirs := parseDirectories(lines[dirIdx])
	if containsString(dirs, dirName) {
		return lines, false
	}

	dirs = append(dirs, dirName)
	lines[dirIdx] = "Directories=" + strings.Join(dirs, ",")
	return lines, true
}

// ensureDirectorySection ensures the directory section exists
func (m *Manager) ensureDirectorySection(lines []string, dirName, size string) ([]string, bool) {
	if sectionExists(lines, dirName) {
		return lines, false
	}

	section := buildDirectorySection(dirName, size)
	if len(section) == 0 {
		return lines, false
	}

	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	return append(lines, section...), true
}

func findSection(lines []string, name string) (int, int) {
	header := "[" + name + "]"
	for i, line := range lines {
		if strings.TrimSpace(line) == header {
			end := len(lines)
			for j := i + 1; j < len(lines); j++ {
				trimmed := strings.TrimSpace(lines[j])
				if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
					end = j
					break
				}
			}
			return i, end
		}
	}
	return -1, -1
}

func sectionExists(lines []string, name string) bool {
	header := "[" + name + "]"
	for _, line := range lines {
		if strings.TrimSpace(line) == header {
			return true
		}
	}
	return false
}

func parseDirectories(line string) []string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return nil
	}
	raw := parts[1]
	rawParts := strings.Split(raw, ",")
	dirs := make([]string, 0, len(rawParts))
	for _, part := range rawParts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			dirs = append(dirs, trimmed)
		}
	}
	return dirs
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func buildDirectorySection(dirName, size string) []string {
	header := "[" + dirName + "]"
	if size == "scalable" {
		return []string{
			header,
			"MinSize=1",
			"Size=128",
			"MaxSize=256",
			"Context=Applications",
			"Type=Scalable",
		}
	}

	dimension := parseSquareSize(size)
	if dimension == 0 {
		return nil
	}

	return []string{
		header,
		fmt.Sprintf("Size=%d", dimension),
		"Context=Applications",
		"Type=Threshold",
	}
}

func parseSquareSize(size string) int {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0
	}
	w, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	h, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0
	}
	if h > w {
		w = h
	}
	return w
}

// copyIcon performs a simple file copy
func (m *Manager) copyIcon(srcPath, dstPath string) (string, error) {
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
