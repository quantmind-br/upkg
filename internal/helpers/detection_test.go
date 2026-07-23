package helpers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/ulikunitz/xz"
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

func writeTarBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	body := []byte("hello upkg")
	hdr := &tar.Header{Name: "hello.txt", Mode: 0644, Size: int64(len(body)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buf.Bytes()
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func xzBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}
	if _, err := xw.Write(data); err != nil {
		t.Fatalf("xz write: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("xz close: %v", err)
	}
	return buf.Bytes()
}

// Precomputed bzip2 fixtures (compress/bzip2 has no encoder). tarBz2Fixture is a
// bzip2-compressed tar archive; nonTarBz2Fixture is a bzip2 stream of plain
// (non-TAR) text.
var tarBz2Fixture = []byte{
	66, 90, 104, 57, 49, 65, 89, 38, 83, 89, 180, 240, 125, 2, 0, 0, 122, 123, 132, 194, 0, 0, 192, 64, 1, 127, 128, 0, 128, 102, 236, 222, 64, 0, 1, 0, 8, 32, 0, 116, 26, 18, 97, 13, 13, 25, 13, 6, 153, 4, 148, 106, 104, 104, 0, 0, 3, 111, 187, 139, 50, 16, 105, 0, 18, 42, 129, 101, 108, 210, 21, 57, 36, 15, 112, 254, 159, 17, 174, 32, 44, 86, 72, 12, 196, 118, 197, 90, 45, 234, 35, 114, 61, 142, 138, 144, 50, 230, 163, 76, 32, 129, 235, 49, 220, 5, 183, 232, 83, 193, 65, 48, 98, 130, 191, 29, 173, 90, 90, 82, 34, 32, 63, 23, 114, 69, 56, 80, 144, 180, 240, 125, 2,
}

var nonTarBz2Fixture = []byte{
	66, 90, 104, 57, 49, 65, 89, 38, 83, 89, 157, 220, 110, 105, 0, 0, 22, 209, 128, 0, 16, 64, 0, 62, 103, 223, 0, 32, 0, 33, 170, 104, 241, 79, 106, 76, 143, 209, 66, 153, 49, 50, 12, 140, 40, 124, 74, 13, 50, 57, 199, 17, 136, 184, 103, 242, 52, 50, 83, 58, 78, 42, 134, 189, 47, 121, 171, 159, 197, 220, 145, 78, 20, 36, 39, 119, 27, 154, 64,
}

// writeTempFile writes content to a temp file named after name and returns the
// path, registering cleanup.
func writeTempFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	tmpfile, err := os.CreateTemp("", "test_*_"+name)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(tmpfile.Name()) })
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatalf("close temp file: %v", err)
	}
	return tmpfile.Name()
}

func TestDetectFileType_SignatureBased(t *testing.T) {
	tarBytes := writeTarBytes(t)

	tests := []struct {
		name     string
		content  []byte
		wantType FileType
	}{
		{"ELF", []byte{0x7F, 'E', 'L', 'F', 0, 0, 0, 0}, FileTypeELF},
		{"script", []byte("#!/bin/bash\necho hi\n"), FileTypeScript},
		{"DEB", []byte("!<arch>\ndebian-binary   2.0\n"), FileTypeDEB},
		{"RPM", []byte{0xED, 0xAB, 0xEE, 0xDB, 0, 0}, FileTypeRPM},
		{"uncompressed tar", tarBytes, FileTypeTar},
		{"zip", []byte{'P', 'K', 0x03, 0x04}, FileTypeZip},
		{"real tar.gz", gzipBytes(t, tarBytes), FileTypeTarGz},
		{"real tar.xz", xzBytes(t, tarBytes), FileTypeTarXz},
		{"real tar.bz2", tarBz2Fixture, FileTypeTarBz2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, tt.name, tt.content)
			got, err := DetectFileType(path)
			if err != nil {
				t.Fatalf("DetectFileType error: %v", err)
			}
			if got != tt.wantType {
				t.Errorf("DetectFileType() = %v, want %v", got, tt.wantType)
			}
		})
	}
}

func TestDetectFileType_RenamedExtensionsAreUnknown(t *testing.T) {
	// Text content with archive extensions must never be classified by name.
	text := []byte("this is just plain text, definitely not a package\n")
	names := []string{"fake.deb", "fake.rpm", "fake.zip", "fake.tar", "fake.tar.gz", "fake.tar.xz", "fake.tar.bz2"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			path := writeTempFile(t, name, text)
			got, err := DetectFileType(path)
			if err != nil {
				t.Fatalf("DetectFileType error: %v", err)
			}
			if got != FileTypeUnknown {
				t.Errorf("DetectFileType(%s) = %v, want Unknown", name, got)
			}
		})
	}
}

func TestDetectFileType_CompressedNonTarIsUnknown(t *testing.T) {
	// Valid compressor streams whose payload is not a TAR must be Unknown.
	nonTar := []byte("this is compressed but not a tar archive at all\n")
	tests := []struct {
		name    string
		content []byte
	}{
		{"gzip non-tar", gzipBytes(t, nonTar)},
		{"xz non-tar", xzBytes(t, nonTar)},
		{"bzip2 non-tar", nonTarBz2Fixture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTempFile(t, tt.name, tt.content)
			got, err := DetectFileType(path)
			if err != nil {
				t.Fatalf("DetectFileType error: %v", err)
			}
			if got != FileTypeUnknown {
				t.Errorf("DetectFileType() = %v, want Unknown", got)
			}
		})
	}
}

func TestDetectFileType_MissingFileErrors(t *testing.T) {
	_, err := DetectFileType(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
