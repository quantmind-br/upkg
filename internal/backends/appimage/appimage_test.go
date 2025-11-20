package appimage

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/rs/zerolog"
)

func TestDetect(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)

	// Create a dummy file that is NOT an AppImage
	tmpDir := t.TempDir()
	dummyFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(dummyFile, []byte("not an appimage"), 0644); err != nil {
		t.Fatalf("failed to create dummy file: %v", err)
	}

	// Test detection on non-AppImage
	ok, err := backend.Detect(context.Background(), dummyFile)
	if err != nil {
		t.Errorf("Detect returned error for non-AppImage: %v", err)
	}
	if ok {
		t.Errorf("Detect returned true for non-AppImage")
	}

	// Create a dummy file that LOOKS like an AppImage (ELF + magic bytes)
	// This is harder without a real AppImage, but helpers.IsAppImage checks for ELF header + specific magic.
	// We can test the negative case effectively.
}

func TestName(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)
	if backend.Name() != "appimage" {
		t.Errorf("Name() = %q, want %q", backend.Name(), "appimage")
	}
}
