package backends

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/ulikunitz/xz"
)

// writeTarEntry writes a single regular-file entry into tw.
func writeTarEntry(t *testing.T, tw *tar.Writer, name string, content []byte) {
	t.Helper()
	hdr := &tar.Header{
		Name:     name,
		Mode:     0755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
}

// makeTarGz builds a real gzip-compressed tar archive so signature detection
// (which decompresses and requires a ustar block) accepts it.
func makeTarGz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTarEntry(t, tw, "app", []byte("#!/bin/sh\necho hi\n"))
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// makeTarXz builds a real xz-compressed tar archive.
func makeTarXz(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}
	tw := tar.NewWriter(xw)
	writeTarEntry(t, tw, "app", []byte("#!/bin/sh\necho hi\n"))
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("close xz: %v", err)
	}
	return buf.Bytes()
}

// tarBz2Fixture is a precomputed bzip2-compressed tar archive (compress/bzip2
// has no encoder). It decompresses to a valid ustar block, so signature
// detection classifies it as tar.bz2.
var tarBz2Fixture = []byte{
	66, 90, 104, 57, 49, 65, 89, 38, 83, 89, 180, 240, 125, 2, 0, 0, 122, 123, 132, 194, 0, 0, 192, 64, 1, 127, 128, 0, 128, 102, 236, 222, 64, 0, 1, 0, 8, 32, 0, 116, 26, 18, 97, 13, 13, 25, 13, 6, 153, 4, 148, 106, 104, 104, 0, 0, 3, 111, 187, 139, 50, 16, 105, 0, 18, 42, 129, 101, 108, 210, 21, 57, 36, 15, 112, 254, 159, 17, 174, 32, 44, 86, 72, 12, 196, 118, 197, 90, 45, 234, 35, 114, 61, 142, 138, 144, 50, 230, 163, 76, 32, 129, 235, 49, 220, 5, 183, 232, 83, 193, 65, 48, 98, 130, 191, 29, 173, 90, 90, 82, 34, 32, 63, 23, 114, 69, 56, 80, 144, 180, 240, 125, 2,
}
