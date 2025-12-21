package icons

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/core"
	"github.com/spf13/afero"
)

const testIconsDir = "/test/icons"

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

	if len(icons) != 4 {
		t.Errorf("DiscoverIcons should find 4 icons, got %d", len(icons))
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
	if !foundTypes["ico"] {
		t.Error("DiscoverIcons should find ICO icons")
	}
	if !foundTypes["xpm"] {
		t.Error("DiscoverIcons should find XPM icons")
	}
}

func TestInstallIcon(t *testing.T) {
	fs := afero.NewMemMapFs()
	iconDir := testIconsDir
	manager := NewManager(fs, iconDir)

	// Create source icon
	srcPath := "/test/source/app.png"
	afero.WriteFile(fs, srcPath, []byte("png content"), 0644)

	normalizedName := "test-app"
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
	srcPath := "/test/source/app.png"
	afero.WriteFile(fs, srcPath, []byte("png content"), 0644)

	normalizedName := "test-app"
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
	if len(icons) != 4 {
		t.Errorf("DiscoverIcons should find 4 icons, got %d", len(icons))
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
	_, err = InstallIcon(iconFile, "test-app", tmpHome)
	if err != nil {
		t.Errorf("InstallIcon should not return error: %v", err)
	}
}
