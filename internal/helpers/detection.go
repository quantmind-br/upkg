package helpers

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"debug/elf"
	"fmt"
	"io"
	"os"

	"github.com/ulikunitz/xz"
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

// DetectFileType identifies the type of a file purely from its content
// signature. The file extension is never used to classify content. For the
// compressed archive formats (gzip/xz/bzip2) the compressor signature alone is
// insufficient, so the stream is decompressed far enough to require a "ustar"
// TAR magic before the type is accepted.
//
//nolint:gocyclo // detection relies on multiple signature checks.
func DetectFileType(filePath string) (FileType, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return FileTypeUnknown, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	header := make([]byte, 512)
	n, err := f.Read(header)
	if err != nil && err != io.EOF {
		return FileTypeUnknown, fmt.Errorf("failed to read file header: %w", err)
	}
	header = header[:n]

	// ELF magic: 0x7F 'E' 'L' 'F'
	if len(header) >= 4 && bytes.Equal(header[:4], []byte{0x7F, 'E', 'L', 'F'}) {
		// Check if it's an AppImage (ELF with embedded squashfs)
		if hasSquashFS(f) {
			return FileTypeAppImage, nil
		}
		return FileTypeELF, nil
	}

	// Shell script magic: #!
	if len(header) >= 2 && bytes.Equal(header[:2], []byte{'#', '!'}) {
		return FileTypeScript, nil
	}

	// DEB: an ar archive ("!<arch>\n") whose first member is "debian-binary".
	if len(header) >= 24 &&
		bytes.Equal(header[:8], []byte("!<arch>\n")) &&
		bytes.HasPrefix(header[8:24], []byte("debian-binary")) {
		return FileTypeDEB, nil
	}

	// RPM magic: 0xED 0xAB 0xEE 0xDB
	if len(header) >= 4 && bytes.Equal(header[:4], []byte{0xED, 0xAB, 0xEE, 0xDB}) {
		return FileTypeRPM, nil
	}

	// Uncompressed tar magic: "ustar" at offset 257
	if len(header) >= 262 && bytes.Equal(header[257:262], []byte("ustar")) {
		return FileTypeTar, nil
	}

	// ZIP magic: "PK"
	if len(header) >= 2 && bytes.Equal(header[:2], []byte{'P', 'K'}) {
		return FileTypeZip, nil
	}

	// Compressed streams: the compressor signature is not enough. Decompress the
	// first TAR block and require the "ustar" magic before accepting the type.
	// Gzip magic: 0x1F 0x8B
	if len(header) >= 2 && bytes.Equal(header[:2], []byte{0x1F, 0x8B}) {
		if isCompressedTar(filePath, codecGzip) {
			return FileTypeTarGz, nil
		}
		return FileTypeUnknown, nil
	}

	// XZ magic: 0xFD '7' 'z' 'X' 'Z' 0x00
	if len(header) >= 6 && bytes.Equal(header[:6], []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}) {
		if isCompressedTar(filePath, codecXz) {
			return FileTypeTarXz, nil
		}
		return FileTypeUnknown, nil
	}

	// BZ2 magic: 'B' 'Z' 'h'
	if len(header) >= 3 && bytes.Equal(header[:3], []byte{'B', 'Z', 'h'}) {
		if isCompressedTar(filePath, codecBzip2) {
			return FileTypeTarBz2, nil
		}
		return FileTypeUnknown, nil
	}

	return FileTypeUnknown, nil
}

type compressionCodec int

const (
	codecGzip compressionCodec = iota
	codecXz
	codecBzip2
)

// isCompressedTar decompresses the start of filePath with the given codec and
// reports whether the first decompressed block carries the "ustar" TAR magic at
// offset 257. Any open/decompress/read error, premature EOF, or missing magic
// yields false.
func isCompressedTar(filePath string, codec compressionCodec) bool {
	f, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()

	var reader io.Reader
	switch codec {
	case codecGzip:
		gz, gerr := gzip.NewReader(f)
		if gerr != nil {
			return false
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	case codecXz:
		xzr, xerr := xz.NewReader(f)
		if xerr != nil {
			return false
		}
		reader = xzr
	case codecBzip2:
		reader = bzip2.NewReader(f)
	default:
		return false
	}

	block := make([]byte, 512)
	if _, err := io.ReadFull(reader, block); err != nil {
		return false
	}
	return bytes.Equal(block[257:262], []byte("ustar"))
}

// IsELF checks if a file is a valid ELF executable
// Uses fast-path magic number check before expensive elf.Open parsing
func IsELF(filePath string) (bool, error) {
	// OPTIMIZATION: Fast-path check - read first 4 bytes for ELF magic
	// This avoids expensive elf.Open() parsing for non-ELF files
	file, err := os.Open(filePath)
	if err != nil {
		return false, nil // Can't open, not an ELF
	}
	defer file.Close()

	// Read ELF magic: 0x7F 'E' 'L' 'F'
	magic := make([]byte, 4)
	n, err := file.Read(magic)
	if err != nil || n < 4 {
		return false, nil // Can't read magic, not an ELF
	}

	// Quick reject if not ELF magic
	if !bytes.Equal(magic, []byte{0x7F, 'E', 'L', 'F'}) {
		return false, nil
	}

	// Magic matches, now do full ELF validation with elf.Open
	// This parses headers, sections, and symbols to ensure it's valid
	f, err := elf.Open(filePath)
	if err != nil {
		return false, nil // Invalid ELF structure
	}
	defer func() { _ = f.Close() }()

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
	defer func() { _ = f.Close() }()

	return hasSquashFS(f), nil
}

func hasSquashFS(f *os.File) bool {
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
				return true
			}
		}
	}

	return false
}
