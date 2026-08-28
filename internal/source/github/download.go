package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/quantmind-br/upkg/internal/helpers"
)

// DownloadResult is a verified on-disk artifact.
type DownloadResult struct {
	Path   string
	Digest string // "sha256:<hex>"
	Size   int64
	ETag   string
}

// DownloadAsset downloads assetURL into cacheDir with mode 0700/0600 O_EXCL,
// computes sha256, and optionally verifies against expectedDigest ("sha256:hex").
// The file is never marked executable here.
func (c *Client) DownloadAsset(ctx context.Context, assetURL, fileName, cacheDir, expectedDigest string) (*DownloadResult, error) {
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	// Ensure directory mode even if it already existed with looser perms.
	if chmodErr := os.Chmod(cacheDir, 0o700); chmodErr != nil {
		return nil, fmt.Errorf("chmod cache dir: %w", chmodErr)
	}

	safe := sanitizeFileName(fileName)
	dest := filepath.Join(cacheDir, safe)

	// Write to a private temp file and publish atomically after verification
	// so concurrent downloads cannot race on dest or defeat O_EXCL.
	f, err := os.CreateTemp(cacheDir, safe+".part-*")
	if err != nil {
		return nil, fmt.Errorf("create download temp file: %w", err)
	}
	tmpPath := f.Name()
	cleanup := true
	defer func() {
		_ = f.Close()
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if chmodErr := os.Chmod(tmpPath, 0o600); chmodErr != nil {
		return nil, fmt.Errorf("chmod download temp: %w", chmodErr)
	}

	h := sha256.New()
	w := io.MultiWriter(f, h)
	n, etag, err := c.DownloadGET(ctx, assetURL, w)
	if err != nil {
		return nil, err
	}
	if err := f.Sync(); err != nil {
		return nil, fmt.Errorf("sync download: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close download: %w", err)
	}

	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))
	if expectedDigest != "" {
		if !digestsEqual(expectedDigest, digest) {
			return nil, fmt.Errorf("digest mismatch: expected %s, got %s", expectedDigest, digest)
		}
	}

	// Replace any previous dest atomically after the digest check.
	_ = os.Remove(dest)
	if err := os.Rename(tmpPath, dest); err != nil {
		return nil, fmt.Errorf("publish download: %w", err)
	}
	cleanup = false
	return &DownloadResult{Path: dest, Digest: digest, Size: n, ETag: etag}, nil
}

// VerifyLocalFileType aborts when the name-hinted format disagrees with content magic.
// expectedFormat is an assetmatch format hint (deb, rpm, appimage, tarball, zip, binary, …).
func VerifyLocalFileType(path, expectedFormat string) error {
	ft, err := helpers.DetectFileType(path)
	if err != nil {
		return fmt.Errorf("detect file type: %w", err)
	}
	if expectedFormat == "" || expectedFormat == "unknown" {
		return nil
	}
	if !formatAgrees(expectedFormat, string(ft)) {
		return fmt.Errorf("content type %q does not match asset name format %q (refusing to reclassify)", ft, expectedFormat)
	}
	return nil
}

//nolint:gocyclo // format matrix
func formatAgrees(nameFormat, detected string) bool {
	nameFormat = strings.ToLower(nameFormat)
	detected = strings.ToLower(detected)
	// Content magic is inconclusive (gzip-wrapped single binary, .tar.zst, …).
	// There is no evidence of reclassification, so do not block.
	if detected == "" || detected == "unknown" {
		return true
	}
	switch nameFormat {
	case "deb":
		return detected == "deb"
	case "rpm":
		return detected == "rpm"
	case "appimage":
		// AppImages are ELF; detection may report appimage or elf/binary depending on helper.
		return detected == "appimage" || detected == "elf" || detected == "binary"
	case "tarball":
		return detected == "tar" || detected == "tar.gz" || detected == "tar.xz" ||
			detected == "tar.bz2" || detected == "tar.zst" || detected == "tarball" ||
			strings.HasPrefix(detected, "tar")
	case "zip":
		return detected == "zip"
	case "binary":
		return detected == "elf" || detected == "binary" || detected == "appimage" ||
			detected == "script"
	default:
		return true
	}
}

func sanitizeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." {
		return "asset.bin"
	}
	// Strip path separators smuggled in.
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	if strings.HasPrefix(name, "-") {
		name = "_" + name
	}
	return name
}

func digestsEqual(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	a = strings.TrimPrefix(a, "sha256:")
	b = strings.TrimPrefix(b, "sha256:")
	return a == b && a != ""
}
