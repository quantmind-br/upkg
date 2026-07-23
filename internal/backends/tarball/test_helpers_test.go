package tarball

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/ulikunitz/xz"
)

// makeZip builds a real zip archive containing the given file.
func makeZip(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// makeTarGz builds a real gzip-compressed tar archive containing the given
// executable file (mode 0755) so detection and extraction exercise a genuine
// archive rather than a magic-byte stub.
func makeTarGz(t *testing.T, execName string, execContent []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTarEntry(t, tw, execName, execContent)
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// makeTar builds a real uncompressed tar archive.
func makeTar(t *testing.T, execName string, execContent []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	writeTarEntry(t, tw, execName, execContent)
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return buf.Bytes()
}

// makeTarXz builds a real xz-compressed tar archive.
func makeTarXz(t *testing.T, execName string, execContent []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	xw, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz writer: %v", err)
	}
	tw := tar.NewWriter(xw)
	writeTarEntry(t, tw, execName, execContent)
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("close xz: %v", err)
	}
	return buf.Bytes()
}

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

// makeTarGzWithELF builds a real gzip tar archive whose single entry is the
// test binary itself (a genuine ELF) at mode 0755, so FindExecutables (which
// only accepts ELF files) treats it as an executable.
func makeTarGzWithELF(t *testing.T, name string) []byte {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	body, err := os.ReadFile(self)
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	writeTarEntry(t, tw, name, body)
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// binDirFailFs delegates to a real OS filesystem but fails MkdirAll for any
// path under failPrefix, exercising the "failed to create bin directory"
// branch without disturbing extraction (which uses the real filesystem).
type binDirFailFs struct {
	afero.Fs
	failPrefix string
}

func (f *binDirFailFs) MkdirAll(path string, perm os.FileMode) error {
	if strings.HasPrefix(path, f.failPrefix) {
		return fmt.Errorf("mkdir %s: simulated failure", path)
	}
	return f.Fs.MkdirAll(path, perm)
}

// zerologNop returns a discarding logger pointer for transaction managers.
func zerologNop() *zerolog.Logger {
	l := zerolog.Nop()
	return &l
}
