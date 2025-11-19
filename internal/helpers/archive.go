package helpers

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/quantmind-br/upkg/internal/security"
	"github.com/ulikunitz/xz"
)

// Extraction limits to prevent archive bombs
const (
	MaxExtractedSize      = 10 * 1024 * 1024 * 1024 // 10GB
	MaxFileCount          = 100000                  // 100k files
	MaxCompressionRatio   = 1000                    // 1000:1 ratio
	MaxIndividualFileSize = 5 * 1024 * 1024 * 1024  // 5GB per file
)

// extractionLimiter tracks extraction metrics to prevent bombs
type extractionLimiter struct {
	totalBytes   int64
	fileCount    int
	originalSize int64
}

func newExtractionLimiter(originalSize int64) *extractionLimiter {
	return &extractionLimiter{
		originalSize: originalSize,
	}
}

func (e *extractionLimiter) checkLimits(fileSize int64) error {
	e.totalBytes += fileSize
	e.fileCount++

	if e.totalBytes > MaxExtractedSize {
		return fmt.Errorf("extraction size limit exceeded: %d bytes (max %d)", e.totalBytes, MaxExtractedSize)
	}

	if e.fileCount > MaxFileCount {
		return fmt.Errorf("file count limit exceeded: %d files (max %d)", e.fileCount, MaxFileCount)
	}

	if fileSize > MaxIndividualFileSize {
		return fmt.Errorf("individual file too large: %d bytes (max %d)", fileSize, MaxIndividualFileSize)
	}

	if e.originalSize > 0 && e.totalBytes > e.originalSize*MaxCompressionRatio {
		return fmt.Errorf("compression ratio too high: %d:1 (possible archive bomb, max %d:1)",
			e.totalBytes/e.originalSize, MaxCompressionRatio)
	}

	return nil
}

// ExtractTarGz extracts a .tar.gz archive with security checks
func ExtractTarGz(archivePath, destDir string) error {
	// Get original file size for compression ratio check
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	limiter := newExtractionLimiter(info.Size())
	return extractTar(gzr, destDir, limiter)
}

// ExtractTar extracts a .tar archive with security checks
func ExtractTar(archivePath, destDir string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	limiter := newExtractionLimiter(info.Size())
	return extractTar(file, destDir, limiter)
}

// ExtractTarXz extracts a .tar.xz archive with security checks
func ExtractTarXz(archivePath, destDir string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	// Use xz decompressor
	xzr, err := xz.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create xz reader: %w", err)
	}

	limiter := newExtractionLimiter(info.Size())
	return extractTar(xzr, destDir, limiter)
}

// ExtractTarBz2 extracts a .tar.bz2 archive with security checks
func ExtractTarBz2(archivePath, destDir string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	// Use bzip2 decompressor
	bzr := bzip2.NewReader(file)

	limiter := newExtractionLimiter(info.Size())
	return extractTar(bzr, destDir, limiter)
}

func extractTar(r io.Reader, destDir string, limiter *extractionLimiter) error {
	tr := tar.NewReader(r)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		// Security: Validate path to prevent directory traversal
		if err := security.ValidateExtractPath(destDir, header.Name); err != nil {
			return fmt.Errorf("invalid path in archive: %w", err)
		}

		target := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}

		case tar.TypeReg:
			// Check extraction limits before extracting file
			if err := limiter.checkLimits(header.Size); err != nil {
				return fmt.Errorf("archive bomb protection triggered: %w", err)
			}

			if err := extractFile(tr, target, header.Mode); err != nil {
				return fmt.Errorf("failed to extract file %s: %w", header.Name, err)
			}

		case tar.TypeSymlink:
			// Security: Validate symlink target
			if err := security.ValidateSymlink(destDir, target, header.Linkname); err != nil {
				return fmt.Errorf("invalid symlink: %w", err)
			}

			if err := os.Symlink(header.Linkname, target); err != nil {
				return fmt.Errorf("failed to create symlink: %w", err)
			}

		case tar.TypeLink:
			// Hard link - validate and create
			linkTarget := filepath.Join(destDir, header.Linkname)
			if err := security.ValidateExtractPath(destDir, header.Linkname); err != nil {
				return fmt.Errorf("invalid hard link target: %w", err)
			}

			if err := os.Link(linkTarget, target); err != nil {
				return fmt.Errorf("failed to create hard link: %w", err)
			}

		default:
			// Skip unsupported types (TypeBlock, TypeChar, TypeFifo, etc.)
			continue
		}
	}

	return nil
}

func extractFile(r io.Reader, target string, mode int64) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create file with proper permissions
	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(mode))
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// ExtractZip extracts a .zip archive with security checks
func ExtractZip(archivePath, destDir string) error {
	info, err := os.Stat(archivePath)
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}

	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	limiter := newExtractionLimiter(info.Size())

	for _, f := range r.File {
		// Security: Validate path
		if err := security.ValidateExtractPath(destDir, f.Name); err != nil {
			return fmt.Errorf("invalid path in zip: %w", err)
		}

		target := filepath.Join(destDir, f.Name)

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
			continue
		}

		// Check extraction limits before extracting file
		if err := limiter.checkLimits(int64(f.UncompressedSize64)); err != nil {
			return fmt.Errorf("archive bomb protection triggered: %w", err)
		}

		if err := extractZipFile(f, target); err != nil {
			return fmt.Errorf("failed to extract %s: %w", f.Name, err)
		}
	}

	return nil
}

func extractZipFile(f *zip.File, target string) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("failed to open zip file entry: %w", err)
	}
	defer rc.Close()

	outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	if _, err := io.Copy(outFile, rc); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
