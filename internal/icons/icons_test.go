package icons

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quantmind-br/upkg/internal/core"
	"github.com/spf13/afero"
)

const (
	testIconsDir       = "/test/icons"
	testSourceAppPng   = "/test/source/app.png"
	testNormalizedName = "test-app"
)

func TestNewManager(t *testing.T) {
	fs := afero.NewMemMapFs()
	manager := NewManager(fs, testIconsDir)

	if manager == nil {
		t.Fatal("NewManager should not return nil")
	}
	if manager.fs != fs {
		t.Error("NewManager should use provided filesystem")
	}
	if manager.iconDir != testIconsDir {
		t.Errorf("NewManager iconDir = %q, want %q", manager.iconDir, testIconsDir)
	}
}

func TestDetectIconSize(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"Path with size", "icons/48x48/app.png", "48x48"},
		{"Path with scalable", "icons/scalable/app.svg", "scalable"},
		{"SVG file", "icons/app.svg", "scalable"},
		{"Unknown size", "icons/app.png", "48x48"},
		{"Path with size in middle", "48x48-icons/app.png", "48x48"},
		{"Large size", "icons/4096x4096/app.png", "512x512"},
		{"Very large size", "icons/1024x1024/app.png", "512x512"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectIconSize(tt.path)
			if result != tt.expected {
				t.Errorf("DetectIconSize(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGetImageDimensions(t *testing.T) {
	// Test with unsupported format
	tests := []struct {
		name     string
		file     string
		content  []byte
		expected string
	}{
		{
			name:     "Unsupported format",
			file:     "test.txt",
			content:  []byte("not an image"),
			expected: "",
		},
		{
			name:     "Empty file",
			file:     "test.empty",
			content:  []byte(""),
			expected: "",
		},
		{
			name:     "File with .png extension but invalid content",
			file:     "test.png",
			content:  []byte("invalid png content"),
			expected: "",
		},
		{
			name:     "File with .jpg extension but invalid content",
			file:     "test.jpg",
			content:  []byte("invalid jpg content"),
			expected: "",
		},
		{
			name:     "File with .jpeg extension but invalid content",
			file:     "test.jpeg",
			content:  []byte("invalid jpeg content"),
			expected: "",
		},
		{
			name:     "File with .gif extension but invalid content",
			file:     "test.gif",
			content:  []byte("invalid gif content"),
			expected: "",
		},
		{
			name:     "File with .PNG extension (uppercase)",
			file:     "test.PNG",
			content:  []byte("invalid png content"),
			expected: "",
		},
		{
			name:     "File with .JPG extension (uppercase)",
			file:     "test.JPG",
			content:  []byte("invalid jpg content"),
			expected: "",
		},
		{
			name:     "File with .GIF extension (uppercase)",
			file:     "test.GIF",
			content:  []byte("invalid gif content"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpFile, err := os.CreateTemp("", tt.file)
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())

			// Write content
			if _, err := tmpFile.Write(tt.content); err != nil {
				t.Fatalf("Failed to write to temp file: %v", err)
			}
			tmpFile.Close()

			// Test getImageDimensions
			result := getImageDimensions(tmpFile.Name())
			if result != tt.expected {
				t.Errorf("getImageDimensions(%q) = %q, want %q", tt.file, result, tt.expected)
			}
		})
	}
}

func TestNormalizeIconName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"Simple name", "app.png", "app"},
		{"Path with name", "icons/app.png", "app"},
		{"Complex name", "My App 123.png", "my-app-123"},
		{"Special chars", "app@name#123.png", "app-name-123"},
		{"Multiple dots", "app.icon.png", "app.icon"},
		{"Uppercase", "APP.PNG", "app"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeIconName(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeIconName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDiscoverIcons(t *testing.T) {
	fs := afero.NewMemMapFs()

	// Create test directory structure
	afero.WriteFile(fs, filepath.Join(testIconsDir, "app.png"), []byte("png content"), 0644)
	afero.WriteFile(fs, filepath.Join(testIconsDir, "app.svg"), []byte("svg content"), 0644)
	afero.WriteFile(fs, filepath.Join(testIconsDir, "app.ico"), []byte("ico content"), 0644)
	afero.WriteFile(fs, filepath.Join(testIconsDir, "app.xpm"), []byte("xpm content"), 0644)
	afero.WriteFile(fs, filepath.Join(testIconsDir, "readme.txt"), []byte("text content"), 0644)

	manager := NewManager(fs, testIconsDir)
	icons, err := manager.DiscoverIcons(testIconsDir)

	if err != nil {
		t.Errorf("DiscoverIcons should not return error: %v", err)
	}

	if len(icons) != 3 {
		t.Errorf("DiscoverIcons should find 3 icons (ICO skipped), got %d", len(icons))
	}

	// Check that we found the expected icon types
	foundTypes := make(map[string]bool)
	for _, icon := range icons {
		foundTypes[icon.Ext] = true
	}

	if !foundTypes["png"] {
		t.Error("DiscoverIcons should find PNG icons")
	}
	if !foundTypes["svg"] {
		t.Error("DiscoverIcons should find SVG icons")
	}
	// Note: .ico files are intentionally skipped (Windows ICO format not supported in Linux)
	if !foundTypes["xpm"] {
		t.Error("DiscoverIcons should find XPM icons")
	}
}

func TestInstallIcon(t *testing.T) {
	fs := afero.NewMemMapFs()
	iconDir := testIconsDir
	manager := NewManager(fs, iconDir)

	// Create source icon
	srcPath := testSourceAppPng
	afero.WriteFile(fs, srcPath, []byte("png content"), 0644)

	normalizedName := testNormalizedName
	size := "48x48"

	dstPath, err := manager.InstallIcon(srcPath, normalizedName, size)

	if err != nil {
		t.Errorf("InstallIcon should not return error: %v", err)
	}

	expectedPath := filepath.Join(iconDir, "hicolor", size, "apps", normalizedName+".png")
	if dstPath != expectedPath {
		t.Errorf("InstallIcon dstPath = %q, want %q", dstPath, expectedPath)
	}

	// Check that file was created
	if exists, _ := afero.Exists(fs, dstPath); !exists {
		t.Errorf("InstallIcon should create file at %q", dstPath)
	}

	// Check file content
	content, err := afero.ReadFile(fs, dstPath)
	if err != nil {
		t.Errorf("InstallIcon should create readable file: %v", err)
	}
	if string(content) != "png content" {
		t.Errorf("InstallIcon should copy content correctly")
	}
}

func TestInstallIconWithSubdirs(t *testing.T) {
	fs := afero.NewMemMapFs()
	iconDir := testIconsDir
	manager := NewManager(fs, iconDir)

	// Create source icon
	srcPath := testSourceAppPng
	afero.WriteFile(fs, srcPath, []byte("png content"), 0644)

	normalizedName := testNormalizedName
	size := "256x256"

	dstPath, err := manager.InstallIcon(srcPath, normalizedName, size)

	if err != nil {
		t.Errorf("InstallIcon should not return error: %v", err)
	}

	expectedPath := filepath.Join(iconDir, "hicolor", size, "apps", normalizedName+".png")
	if dstPath != expectedPath {
		t.Errorf("InstallIcon dstPath = %q, want %q", dstPath, expectedPath)
	}

	// Check that directory structure was created
	hicolorDir := filepath.Join(iconDir, "hicolor")
	sizeDir := filepath.Join(hicolorDir, size)
	appsDir := filepath.Join(sizeDir, "apps")

	if exists, _ := afero.Exists(fs, hicolorDir); !exists {
		t.Errorf("InstallIcon should create hicolor directory")
	}
	if exists, _ := afero.Exists(fs, sizeDir); !exists {
		t.Errorf("InstallIcon should create size directory")
	}
	if exists, _ := afero.Exists(fs, appsDir); !exists {
		t.Errorf("InstallIcon should create apps directory")
	}
}

func TestInstallIconCreatesIndexTheme(t *testing.T) {
	fs := afero.NewMemMapFs()
	iconDir := testIconsDir
	manager := NewManager(fs, iconDir)

	srcPath := testSourceAppPng
	afero.WriteFile(fs, srcPath, []byte("png content"), 0644)

	_, err := manager.InstallIcon(srcPath, testNormalizedName, "48x48")
	if err != nil {
		t.Fatalf("InstallIcon should not return error: %v", err)
	}

	indexPath := filepath.Join(iconDir, "hicolor", "index.theme")
	content, err := afero.ReadFile(fs, indexPath)
	if err != nil {
		t.Fatalf("Expected index.theme to be created: %v", err)
	}

	if !strings.Contains(string(content), "Directories=48x48/apps") {
		t.Errorf("index.theme should include 48x48/apps in Directories")
	}
	if !strings.Contains(string(content), "[48x48/apps]") {
		t.Errorf("index.theme should include section for 48x48/apps")
	}
}

func TestInstallIconUpdatesIndexThemeDirectories(t *testing.T) {
	fs := afero.NewMemMapFs()
	iconDir := testIconsDir
	manager := NewManager(fs, iconDir)

	hicolorDir := filepath.Join(iconDir, "hicolor")
	if err := fs.MkdirAll(hicolorDir, 0755); err != nil {
		t.Fatalf("Failed to create hicolor dir: %v", err)
	}
	initialTheme := `[Icon Theme]
Name=Hicolor
Comment=Fallback icon theme
Hidden=true
Directories=128x128/apps

[128x128/apps]
Size=128
Context=Applications
Type=Threshold
`
	if err := afero.WriteFile(fs, filepath.Join(hicolorDir, "index.theme"), []byte(initialTheme), 0644); err != nil {
		t.Fatalf("Failed to write initial index.theme: %v", err)
	}

	srcPath := testSourceAppPng
	afero.WriteFile(fs, srcPath, []byte("png content"), 0644)

	_, err := manager.InstallIcon(srcPath, testNormalizedName, "512x512")
	if err != nil {
		t.Fatalf("InstallIcon should not return error: %v", err)
	}

	updated, err := afero.ReadFile(fs, filepath.Join(hicolorDir, "index.theme"))
	if err != nil {
		t.Fatalf("Failed to read updated index.theme: %v", err)
	}

	lines := strings.Split(strings.TrimRight(string(updated), "\n"), "\n")
	start, end := findSection(lines, "Icon Theme")
	if start == -1 {
		t.Fatalf("index.theme should include [Icon Theme] section")
	}

	directoriesLine := ""
	for i := start + 1; i < end; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "Directories=") {
			directoriesLine = lines[i]
			break
		}
	}
	if directoriesLine == "" {
		t.Fatalf("index.theme should include Directories line")
	}

	dirs := parseDirectories(directoriesLine)
	if !containsString(dirs, "128x128/apps") || !containsString(dirs, "512x512/apps") {
		t.Errorf("Directories should include both 128x128/apps and 512x512/apps, got %v", dirs)
	}
	if !sectionExists(lines, "512x512/apps") {
		t.Errorf("index.theme should include section for 512x512/apps")
	}
}

func TestPackageLevelFunctions(t *testing.T) {
	// Test DiscoverIcons convenience function
	// Create a temporary directory with test icons
	tmpDir, err := os.MkdirTemp("", "icons-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test icons
	iconFiles := []string{"app.png", "app.svg", "app.ico", "app.xpm"}
	for _, file := range iconFiles {
		content := []byte("test content for " + file)
		writeErr := os.WriteFile(filepath.Join(tmpDir, file), content, 0644)
		if writeErr != nil {
			t.Fatalf("Failed to create test icon: %v", writeErr)
		}
	}

	icons := DiscoverIcons(tmpDir)
	if len(icons) != 3 {
		t.Errorf("DiscoverIcons should find 3 icons (ICO skipped), got %d", len(icons))
	}

	// Test InstallIcon convenience function
	// This would require more complex setup with actual filesystem
	// so we'll just test that it doesn't crash
	iconFile := core.IconFile{
		Path: filepath.Join(tmpDir, "app.png"),
		Size: "48x48",
		Ext:  "png",
	}

	tmpHome, err := os.MkdirTemp("", "icons-home-test")
	if err != nil {
		t.Fatalf("Failed to create temp home dir: %v", err)
	}
	defer os.RemoveAll(tmpHome)

	// Note: This will actually install the icon, but we'll clean it up
	_, err = InstallIcon(iconFile, testNormalizedName, tmpHome)
	if err != nil {
		t.Errorf("InstallIcon should not return error: %v", err)
	}
}

func TestInstallIconWithResizing(t *testing.T) {
	fs := afero.NewMemMapFs()
	iconDir := testIconsDir
	manager := NewManager(fs, iconDir)

	// Create source icon (100x100)
	srcPath := "/test/source/large_app.png"
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	// Fill with some color
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}

	f, _ := fs.Create(srcPath)
	png.Encode(f, img)
	f.Close()

	normalizedName := testNormalizedName
	size := "50x50" // Target smaller size

	dstPath, err := manager.InstallIcon(srcPath, normalizedName, size)

	if err != nil {
		t.Errorf("InstallIcon should not return error: %v", err)
	}

	expectedPath := filepath.Join(iconDir, "hicolor", size, "apps", normalizedName+".png")
	if dstPath != expectedPath {
		t.Errorf("InstallIcon dstPath = %q, want %q", dstPath, expectedPath)
	}

	// Verify destination exists
	fDst, err := fs.Open(dstPath)
	if err != nil {
		t.Fatalf("Failed to open destination icon: %v", err)
	}
	defer fDst.Close()

	// Verify dimensions of destination
	cfg, _, err := image.DecodeConfig(fDst)
	if err != nil {
		t.Fatalf("Failed to decode destination icon config: %v", err)
	}

	if cfg.Width != 50 || cfg.Height != 50 {
		t.Errorf("Destination icon size = %dx%d, want 50x50", cfg.Width, cfg.Height)
	}
}

func TestIsValidRatio(t *testing.T) {
	tests := []struct {
		name     string
		width    float64
		height   float64
		expected bool
	}{
		{"Perfect square", 48, 48, true},
		{"Slight rectangle", 50, 48, true},
		{"Maximum acceptable ratio", 48, 37, true},
		{"Just over limit", 48, 36, false},
		{"Very wide", 100, 50, false},
		{"Very tall", 50, 100, false},
		{"Almost square", 64, 64, true},
		{"Square large", 512, 512, true},
		{"Rectangle within limit", 100, 80, true},
		{"Rectangle at limit", 130, 100, true},
		{"Rectangle over limit", 131, 100, false},
		{"Zero width (invalid)", 0, 48, false},
		{"Zero height (invalid)", 48, 0, false},
		{"Both zero", 0, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidRatio(tt.width, tt.height)
			if result != tt.expected {
				t.Errorf("isValidRatio(%v, %v) = %v, want %v", tt.width, tt.height, result, tt.expected)
			}
		})
	}
}

func TestExtractSVGDimensions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		hasWidth bool
		hasHeight bool
	}{
		{
			name: "Valid SVG with width and height",
			content: `<svg width="48" height="48" xmlns="http://www.w3.org/2000/svg">
				<rect width="48" height="48"/>
			</svg>`,
			hasWidth: true,
			hasHeight: true,
		},
		{
			name: "SVG without dimensions but with rect (extracts from first rect)",
			content: `<svg xmlns="http://www.w3.org/2000/svg">
				<rect width="48" height="48"/>
			</svg>`,
			hasWidth: true,
			hasHeight: true,
		},
		{
			name: "SVG with viewBox only (extracts from viewBox)",
			content: `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
				<rect width="24" height="24"/>
			</svg>`,
			hasWidth: true,
			hasHeight: true,
		},
		{
			name: "Invalid SVG",
			content: `not an svg`,
			hasWidth: false,
			hasHeight: false,
		},
		{
			name: "SVG with fractional dimensions",
			content: `<svg width="48.5" height="48.5" xmlns="http://www.w3.org/2000/svg">
				<rect width="48.5" height="48.5"/>
			</svg>`,
			hasWidth: true,
			hasHeight: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			width, height, _ := extractSVGDimensions(tt.content)
			hasWidth := width > 0
			hasHeight := height > 0

			if hasWidth != tt.hasWidth {
				t.Errorf("extractSVGDimensions() width = %v, hasWidth %v, want %v", width, hasWidth, tt.hasWidth)
			}
			if hasHeight != tt.hasHeight {
				t.Errorf("extractSVGDimensions() height = %v, hasHeight %v, want %v", height, hasHeight, tt.hasHeight)
			}
		})
	}
}

func TestExtractSVGViewBox(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		hasViewBox bool
	}{
		{
			name: "SVG with viewBox",
			content: `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
				<rect width="24" height="24"/>
			</svg>`,
			hasViewBox: true,
		},
		{
			name: "SVG without viewBox",
			content: `<svg xmlns="http://www.w3.org/2000/svg">
				<rect width="24" height="24"/>
			</svg>`,
			hasViewBox: false,
		},
		{
			name:       "Invalid SVG",
			content:    `not an svg`,
			hasViewBox: false,
		},
		{
			name: "SVG with malformed viewBox",
			content: `<svg viewBox="invalid" xmlns="http://www.w3.org/2000/svg">
				<rect width="24" height="24"/>
			</svg>`,
			hasViewBox: false, // Function returns false for invalid viewBox
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, hasViewBox := extractSVGViewBox(tt.content)
			if hasViewBox != tt.hasViewBox {
				t.Errorf("extractSVGViewBox() hasViewBox = %v, want %v", hasViewBox, tt.hasViewBox)
			}
		})
	}
}

func TestIsValidIconAspectRatio(t *testing.T) {
	fs := afero.NewMemMapFs()
	manager := NewManager(fs, testIconsDir)

	t.Run("non-existent file returns true (assumes valid)", func(t *testing.T) {
		result := manager.isValidIconAspectRatio("/nonexistent/icon.png")
		if !result {
			t.Error("isValidIconAspectRatio() should return true for non-existent file")
		}
	})

	t.Run("valid square icon", func(t *testing.T) {
		// Create a test PNG file
		iconPath := filepath.Join(t.TempDir(), "test.png")
		createTestPNG(t, iconPath, 48, 48)

		manager := NewManager(afero.NewOsFs(), testIconsDir)
		result := manager.isValidIconAspectRatio(iconPath)
		if !result {
			t.Error("isValidIconAspectRatio() should return true for square icon")
		}
	})

	t.Run("invalid rectangular icon", func(t *testing.T) {
		// Create a very wide PNG
		iconPath := filepath.Join(t.TempDir(), "wide.png")
		createTestPNG(t, iconPath, 200, 48)

		manager := NewManager(afero.NewOsFs(), testIconsDir)
		result := manager.isValidIconAspectRatio(iconPath)
		if result {
			t.Error("isValidIconAspectRatio() should return false for very wide icon")
		}
	})

	t.Run("invalid PNG file", func(t *testing.T) {
		// Create invalid PNG file
		iconPath := filepath.Join(t.TempDir(), "invalid.png")
		err := os.WriteFile(iconPath, []byte("not a png"), 0644)
		if err != nil {
			t.Fatal(err)
		}

		manager := NewManager(afero.NewOsFs(), testIconsDir)
		result := manager.isValidIconAspectRatio(iconPath)
		if !result {
			t.Error("isValidIconAspectRatio() should return true for undecodable file (assumes valid)")
		}
	})
}

func TestIsValidSVGAspectRatio(t *testing.T) {
	fs := afero.NewMemMapFs()
	manager := NewManager(fs, testIconsDir)

	t.Run("non-existent SVG returns true", func(t *testing.T) {
		result := manager.isValidSVGAspectRatio("/nonexistent/icon.svg")
		if !result {
			t.Error("isValidSVGAspectRatio() should return true for non-existent file")
		}
	})

	t.Run("valid square SVG", func(t *testing.T) {
		svgPath := filepath.Join(t.TempDir(), "test.svg")
		content := `<svg width="48" height="48" xmlns="http://www.w3.org/2000/svg">
			<rect width="48" height="48"/>
		</svg>`
		afero.WriteFile(fs, svgPath, []byte(content), 0644)

		result := manager.isValidSVGAspectRatio(svgPath)
		if !result {
			t.Error("isValidSVGAspectRatio() should return true for square SVG")
		}
	})

	t.Run("invalid rectangular SVG", func(t *testing.T) {
		svgPath := filepath.Join(t.TempDir(), "wide.svg")
		content := `<svg width="200" height="48" xmlns="http://www.w3.org/2000/svg">
			<rect width="200" height="48"/>
		</svg>`
		afero.WriteFile(fs, svgPath, []byte(content), 0644)

		result := manager.isValidSVGAspectRatio(svgPath)
		if result {
			t.Error("isValidSVGAspectRatio() should return false for very wide SVG")
		}
	})

	t.Run("SVG with viewBox", func(t *testing.T) {
		svgPath := filepath.Join(t.TempDir(), "viewbox.svg")
		content := `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg">
			<rect width="24" height="24"/>
		</svg>`
		afero.WriteFile(fs, svgPath, []byte(content), 0644)

		result := manager.isValidSVGAspectRatio(svgPath)
		if !result {
			t.Error("isValidSVGAspectRatio() should return true for square viewBox SVG")
		}
	})
}

// createTestPNG creates a simple PNG file with specified dimensions
func createTestPNG(t *testing.T, path string, width, height int) {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	err = png.Encode(file, img)
	if err != nil {
		t.Fatal(err)
	}
}
