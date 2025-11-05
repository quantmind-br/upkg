package helpers

import (
	"bytes"
	"debug/elf"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FileType represents the detected type of a package file
type FileType string

const (
	FileTypeELF      FileType = "elf"
	FileTypeAppImage FileType = "appimage"
	FileTypeScript   FileType = "script"
	FileTypeDEB      FileType = "deb"
	FileTypeRPM      FileType = "rpm"
	FileTypeTarGz    FileType = "tar.gz"
	FileTypeTarXz    FileType = "tar.xz"
	FileTypeTarBz2   FileType = "tar.bz2"
	FileTypeTar      FileType = "tar"
	FileTypeZip      FileType = "zip"
	FileTypeUnknown  FileType = "unknown"
)

// DetectFileType identifies the type of a file based on extension and magic numbers
func DetectFileType(filePath string) (FileType, error) {
	// Check extension first for quick detection
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".deb":
		return FileTypeDEB, nil
	case ".rpm":
		return FileTypeRPM, nil
	case ".zip":
		return FileTypeZip, nil
	case ".appimage":
		// Verify it's actually an AppImage
		if isAppImage, err := IsAppImage(filePath); err == nil && isAppImage {
			return FileTypeAppImage, nil
		}
	}

	// Check for compound extensions
	if strings.HasSuffix(strings.ToLower(filePath), ".tar.gz") ||
		strings.HasSuffix(strings.ToLower(filePath), ".tgz") {
		return FileTypeTarGz, nil
	}

	if strings.HasSuffix(strings.ToLower(filePath), ".tar.xz") ||
		strings.HasSuffix(strings.ToLower(filePath), ".txz") {
		return FileTypeTarXz, nil
	}

	if strings.HasSuffix(strings.ToLower(filePath), ".tar.bz2") ||
		strings.HasSuffix(strings.ToLower(filePath), ".tbz2") {
		return FileTypeTarBz2, nil
	}

	if ext == ".tar" {
		return FileTypeTar, nil
	}

	// Read magic numbers for more precise detection
	f, err := os.Open(filePath)
	if err != nil {
		return FileTypeUnknown, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return FileTypeUnknown, fmt.Errorf("failed to read file header: %w", err)
	}
	header = header[:n]

	// ELF magic: 0x7F 'E' 'L' 'F'
	if len(header) >= 4 && bytes.Equal(header[:4], []byte{0x7F, 'E', 'L', 'F'}) {
		// Check if it's an AppImage (ELF with embedded squashfs)
		if isAppImage, _ := hasSquashFS(f); isAppImage {
			return FileTypeAppImage, nil
		}
		return FileTypeELF, nil
	}

	// Shell script magic: #!
	if len(header) >= 2 && bytes.Equal(header[:2], []byte{'#', '!'}) {
		return FileTypeScript, nil
	}

	// DEB magic: "!<arch>\ndebian"
	if len(header) >= 16 && bytes.Contains(header[:60], []byte("debian")) {
		return FileTypeDEB, nil
	}

	// RPM magic: 0xED 0xAB 0xEE 0xDB
	if len(header) >= 4 && bytes.Equal(header[:4], []byte{0xED, 0xAB, 0xEE, 0xDB}) {
		return FileTypeRPM, nil
	}

	// Tar magic: "ustar" at offset 257
	if len(header) >= 262 && bytes.Equal(header[257:262], []byte("ustar")) {
		return FileTypeTar, nil
	}

	// Gzip magic: 0x1F 0x8B
	if len(header) >= 2 && bytes.Equal(header[:2], []byte{0x1F, 0x8B}) {
		// Could be tar.gz, but we can't tell without extracting
		return FileTypeTarGz, nil
	}

	// XZ magic: 0xFD '7' 'z' 'X' 'Z' 0x00
	if len(header) >= 6 && bytes.Equal(header[:6], []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}) {
		return FileTypeTarXz, nil
	}

	// BZ2 magic: 'B' 'Z' 'h'
	if len(header) >= 3 && bytes.Equal(header[:3], []byte{'B', 'Z', 'h'}) {
		return FileTypeTarBz2, nil
	}

	// ZIP magic: "PK"
	if len(header) >= 2 && bytes.Equal(header[:2], []byte{'P', 'K'}) {
		return FileTypeZip, nil
	}

	return FileTypeUnknown, nil
}

// IsELF checks if a file is a valid ELF executable
func IsELF(filePath string) (bool, error) {
	f, err := elf.Open(filePath)
	if err != nil {
		return false, nil // Not an ELF file, not an error
	}
	defer f.Close()

	// Check if it's an executable or dynamic library
	return f.Type == elf.ET_EXEC || f.Type == elf.ET_DYN, nil
}

// IsAppImage checks if a file is an AppImage
func IsAppImage(filePath string) (bool, error) {
	// First check if it's an ELF
	isElf, err := IsELF(filePath)
	if err != nil || !isElf {
		return false, err
	}

	// Check for embedded squashfs
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	return hasSquashFS(f)
}

func hasSquashFS(f *os.File) (bool, error) {
	// squashfs magic: "hsqs" (little-endian) or "sqsh" (big-endian)
	// AppImages embed squashfs at various offsets, scan incrementally to find it

	// Scan incrementally from start to 2MB (covers most AppImage layouts)
	// Read chunks and search within each chunk to avoid missing misaligned signatures
	const maxScan = 2 * 1024 * 1024 // 2MB
	const chunkSize = 8192          // 8KB chunks for better coverage

	buf := make([]byte, chunkSize)
	magicLE := []byte{'h', 's', 'q', 's'} // Little-endian
	magicBE := []byte{'s', 'q', 's', 'h'} // Big-endian

	for offset := int64(0); offset < maxScan; offset += int64(chunkSize) {
		// Seek to current chunk offset
		if _, err := f.Seek(offset, 0); err != nil {
			break // Can't seek anymore, reached end
		}

		// Read chunk
		n, err := f.Read(buf)
		if err != nil {
			if err == io.EOF {
				break // Reached end of file
			}
			continue // Try next chunk on read error
		}
		if n < 4 {
			break // Not enough bytes to contain magic
		}

		// Search for magic signature within this chunk
		// Check every byte position in the chunk
		for i := 0; i <= n-4; i++ {
			if bytes.Equal(buf[i:i+4], magicLE) || bytes.Equal(buf[i:i+4], magicBE) {
				// Found squashfs signature
				// Actual offset is: offset + i
				return true, nil
			}
		}
	}

	return false, nil
}

// GetArchiveType returns the archive type based on file extension
func GetArchiveType(filePath string) string {
	lower := strings.ToLower(filePath)

	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return "tar.gz"
	}
	if strings.HasSuffix(lower, ".tar.bz2") || strings.HasSuffix(lower, ".tbz2") {
		return "tar.bz2"
	}
	if strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".txz") {
		return "tar.xz"
	}
	if strings.HasSuffix(lower, ".tar") {
		return "tar"
	}
	if strings.HasSuffix(lower, ".zip") {
		return "zip"
	}

	return ""
}

// IsExecutable checks if a file has execute permissions
func IsExecutable(filePath string) (bool, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return false, err
	}

	mode := info.Mode()
	return mode&0111 != 0, nil
}

// MakeExecutable sets the executable bit on a file
func MakeExecutable(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	// Add execute permissions for owner, group, and others
	newMode := info.Mode() | 0111
	return os.Chmod(filePath, newMode)
}

// ExtractVersionFromFilename attempts to extract a version string from a filename
// Supports common version patterns like:
// - myapp-1.2.3.tar.gz -> "1.2.3"
// - myapp_v2.0.1.AppImage -> "2.0.1"
// - MyApp-3.14.159-x86_64.tar.gz -> "3.14.159"
func ExtractVersionFromFilename(filename string) string {
	// Remove file extensions first
	name := strings.ToLower(filepath.Base(filename))
	name = strings.TrimSuffix(name, ".appimage")
	name = strings.TrimSuffix(name, ".tar.gz")
	name = strings.TrimSuffix(name, ".tar.xz")
	name = strings.TrimSuffix(name, ".tar.bz2")
	name = strings.TrimSuffix(name, ".tgz")
	name = strings.TrimSuffix(name, ".tar")
	name = strings.TrimSuffix(name, ".zip")
	name = strings.TrimSuffix(name, ".deb")
	name = strings.TrimSuffix(name, ".rpm")

	// Common version patterns (ordered by specificity)
	patterns := []string{
		// Semantic versioning with optional v prefix and suffixes
		`v?(\d+\.\d+\.\d+(?:\.\d+)?(?:-(?:alpha|beta|rc|dev)\d*)?(?:\+[a-z0-9]+)?)`,
		// Two-part version
		`v?(\d+\.\d+)(?:[^0-9]|$)`,
		// Single number version
		`v?(\d+)(?:[^0-9]|$)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(name)
		if len(matches) >= 2 && matches[1] != "" {
			return matches[1]
		}
	}

	return ""
}
