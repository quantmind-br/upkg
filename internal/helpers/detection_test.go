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

			result := hasSquashFS(tmpfile)

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

func TestIsELF(t *testing.T) {
	tests := []struct {
		name       string
		filePath   string
		wantResult bool
		wantErr    bool
	}{
		{
			name:       "regular ELF binary",
			filePath:   "/bin/ls",
			wantResult: true,
			wantErr:    false,
		},
		{
			name:       "non-existent file",
			filePath:   "/nonexistent/file",
			wantResult: false,
			wantErr:    false,
		},
		{
			name:       "text file",
			filePath:   "/etc/hosts",
			wantResult: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := os.Stat(tt.filePath); os.IsNotExist(err) && tt.name != "non-existent file" {
				t.Skipf("File %s does not exist, skipping test", tt.filePath)
			}

			result, err := IsELF(tt.filePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("IsELF() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.wantResult {
				t.Errorf("IsELF() = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestGetArchiveType(t *testing.T) {
	tests := []struct {
		name       string
		filePath   string
		wantResult string
	}{
		{
			name:       "tar.gz file",
			filePath:   "test.tar.gz",
			wantResult: "tar.gz",
		},
		{
			name:       "tgz file",
			filePath:   "test.tgz",
			wantResult: "tar.gz",
		},
		{
			name:       "tar.bz2 file",
			filePath:   "test.tar.bz2",
			wantResult: "tar.bz2",
		},
		{
			name:       "tbz2 file",
			filePath:   "test.tbz2",
			wantResult: "tar.bz2",
		},
		{
			name:       "tar.xz file",
			filePath:   "test.tar.xz",
			wantResult: "tar.xz",
		},
		{
			name:       "txz file",
			filePath:   "test.txz",
			wantResult: "tar.xz",
		},
		{
			name:       "tar file",
			filePath:   "test.tar",
			wantResult: "tar",
		},
		{
			name:       "zip file",
			filePath:   "test.zip",
			wantResult: "zip",
		},
		{
			name:       "unknown file",
			filePath:   "test.txt",
			wantResult: "",
		},
		{
			name:       "case insensitive",
			filePath:   "test.TAR.GZ",
			wantResult: "tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetArchiveType(tt.filePath)
			if result != tt.wantResult {
				t.Errorf("GetArchiveType() = %v, want %v", result, tt.wantResult)
			}
		})
	}
}

func TestDetectFileTypeWithMockFiles(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		content  []byte
		wantType FileType
		wantErr  bool
	}{
		{
			name:     "ELF file",
			filePath: "test.elf",
			content:  []byte{0x7F, 'E', 'L', 'F'}, // ELF magic
			wantType: FileTypeELF,
			wantErr:  false,
		},
		{
			name:     "shell script",
			filePath: "test.sh",
			content:  []byte{'#', '!', '/', 'b', 'i', 'n', '/', 'b', 'a', 's', 'h'},
			wantType: FileTypeScript,
			wantErr:  false,
		},
		{
			name:     "DEB file",
			filePath: "test.deb",
			content:  []byte("!<arch>\ndebian-binary   "),
			wantType: FileTypeDEB,
			wantErr:  false,
		},
		{
			name:     "RPM file",
			filePath: "test.rpm",
			content:  []byte{0xED, 0xAB, 0xEE, 0xDB},
			wantType: FileTypeRPM,
			wantErr:  false,
		},
		{
			name:     "TAR file",
			filePath: "test.tar",
			content:  make([]byte, 262),
			wantType: FileTypeTar,
			wantErr:  false,
		},
		{
			name:     "GZIP file",
			filePath: "test.gz",
			content:  []byte{0x1F, 0x8B},
			wantType: FileTypeTarGz,
			wantErr:  false,
		},
		{
			name:     "XZ file",
			filePath: "test.xz",
			content:  []byte{0xFD, '7', 'z', 'X', 'Z', 0x00},
			wantType: FileTypeTarXz,
			wantErr:  false,
		},
		{
			name:     "BZ2 file",
			filePath: "test.bz2",
			content:  []byte{'B', 'Z', 'h'},
			wantType: FileTypeTarBz2,
			wantErr:  false,
		},
		{
			name:     "ZIP file",
			filePath: "test.zip",
			content:  []byte{'P', 'K'},
			wantType: FileTypeZip,
			wantErr:  false,
		},
		{
			name:     "unknown file",
			filePath: "test.unknown",
			content:  []byte{'X', 'Y', 'Z'},
			wantType: FileTypeUnknown,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpfile, err := os.CreateTemp("", "test_*"+filepath.Ext(tt.filePath))
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			defer os.Remove(tmpfile.Name())
			defer tmpfile.Close()

			// Write content
			if _, err := tmpfile.Write(tt.content); err != nil {
				t.Fatalf("Failed to write content: %v", err)
			}

			// Test detection
			fileType, err := DetectFileType(tmpfile.Name())
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectFileType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if fileType != tt.wantType {
				t.Errorf("DetectFileType() = %v, want %v", fileType, tt.wantType)
			}
		})
	}
}
