package helpers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasSquashFS(t *testing.T) {
	tests := []struct {
		name           string
		offset         int64
		magic          string
		expectedResult bool
	}{
		{
			name:           "magic at start",
			offset:         0,
			magic:          "hsqs",
			expectedResult: true,
		},
		{
			name:           "magic at common offset 0x8000",
			offset:         0x8000,
			magic:          "hsqs",
			expectedResult: true,
		},
		{
			name:           "magic at MasterPDF offset 0x2f4c0",
			offset:         0x2f4c0,
			magic:          "hsqs",
			expectedResult: true,
		},
		{
			name:           "magic at 1MB offset",
			offset:         1024 * 1024,
			magic:          "hsqs",
			expectedResult: true,
		},
		{
			name:           "magic at 2MB offset (limit)",
			offset:         2 * 1024 * 1024,
			magic:          "hsqs",
			expectedResult: false, // Beyond 2MB scan limit
		},
		{
			name:           "big-endian magic",
			offset:         0x10000,
			magic:          "sqsh",
			expectedResult: true,
		},
		{
			name:           "no magic",
			offset:         0x10000,
			magic:          "test",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpfile, err := os.CreateTemp("", "test_squashfs_*.bin")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpfile.Name())
			defer tmpfile.Close()

			// Write padding up to offset
			if tt.offset > 0 {
				padding := make([]byte, tt.offset)
				if _, err := tmpfile.Write(padding); err != nil {
					t.Fatalf("Failed to write padding: %v", err)
				}
			}

			// Write magic bytes
			if _, err := tmpfile.Write([]byte(tt.magic)); err != nil {
				t.Fatalf("Failed to write magic bytes: %v", err)
			}

			// Seek to start and test
			if _, err := tmpfile.Seek(0, 0); err != nil {
				t.Fatalf("Failed to seek to start: %v", err)
			}

			result, err := hasSquashFS(tmpfile)
			if err != nil {
				t.Fatalf("hasSquashFS returned error: %v", err)
			}

			if result != tt.expectedResult {
				t.Errorf("hasSquashFS() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}

func TestDetectFileType(t *testing.T) {
	// Test with real AppImage that has squashfs at 0x2f4c0
	appImagePath := filepath.Join("/home/diogo/dev/upkg/pkg-test", "MasterPDFEditor-4.3.89-x86_64-1_JB.AppImage")

	if _, err := os.Stat(appImagePath); err == nil {
		t.Run("real AppImage detection", func(t *testing.T) {
			fileType, err := DetectFileType(appImagePath)
			if err != nil {
				t.Fatalf("DetectFileType failed: %v", err)
			}

			if fileType != FileTypeAppImage {
				t.Errorf("DetectFileType() = %v, want %v", fileType, FileTypeAppImage)
			}
		})
	}

	// Test with regular ELF binary (non-AppImage)
	elfPath := "/bin/ls"
	if _, err := os.Stat(elfPath); err == nil {
		t.Run("regular ELF detection", func(t *testing.T) {
			fileType, err := DetectFileType(elfPath)
			if err != nil {
				t.Fatalf("DetectFileType failed: %v", err)
			}

			if fileType != FileTypeELF {
				t.Errorf("DetectFileType() = %v, want %v", fileType, FileTypeELF)
			}
		})
	}
}

func TestIsAppImage(t *testing.T) {
	// Test with real AppImage
	appImagePath := filepath.Join("/home/diogo/dev/upkg/pkg-test", "MasterPDFEditor-4.3.89-x86_64-1_JB.AppImage")

	if _, err := os.Stat(appImagePath); err == nil {
		t.Run("real AppImage check", func(t *testing.T) {
			isAppImage, err := IsAppImage(appImagePath)
			if err != nil {
				t.Fatalf("IsAppImage failed: %v", err)
			}

			if !isAppImage {
				t.Errorf("IsAppImage() = %v, want true", isAppImage)
			}
		})
	}

	// Test with regular ELF binary
	elfPath := "/bin/ls"
	if _, err := os.Stat(elfPath); err == nil {
		t.Run("regular ELF check", func(t *testing.T) {
			isAppImage, err := IsAppImage(elfPath)
			if err != nil {
				t.Fatalf("IsAppImage failed: %v", err)
			}

			if isAppImage {
				t.Errorf("IsAppImage() = %v, want false", isAppImage)
			}
		})
	}
}
