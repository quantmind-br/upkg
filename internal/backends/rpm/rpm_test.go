package rpm

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/rs/zerolog"
)

func TestName(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)
	if backend.Name() != "rpm" {
		t.Errorf("Name() = %q, want %q", backend.Name(), "rpm")
	}
}

func TestDetect(t *testing.T) {
	t.Parallel()
	logger := zerolog.New(io.Discard)
	backend := New(&config.Config{}, &logger)

	tmpDir := t.TempDir()

	// Text file (Not RPM)
	txtFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(txtFile, []byte("not an rpm"), 0644); err != nil {
		t.Fatal(err)
	}
	ok, err := backend.Detect(context.Background(), txtFile)
	if err != nil {
		t.Errorf("Detect failed for text file: %v", err)
	}
	if ok {
		t.Error("Detect returned true for text file")
	}
}
