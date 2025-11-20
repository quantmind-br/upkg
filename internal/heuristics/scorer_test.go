package heuristics

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestChooseBestExecutablePrefersCoreBinary(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	scorer := NewScorer(&logger)

	installDir := filepath.Join(t.TempDir(), "mod-desktop-0.0.12-linux-x86-64")
	appDir := filepath.Join(installDir, "mod-desktop-0.0.12-linux-x86_64", "mod-desktop")

	targetBinary := filepath.Join(appDir, "mod-desktop")
	helperBinary := filepath.Join(appDir, "mod-pedalboard")
	jackBinary := filepath.Join(appDir, "jackd")

	writeExecutable(t, targetBinary, 250*1024) // Small launcher
	writeExecutable(t, helperBinary, 3*1024*1024)
	writeExecutable(t, jackBinary, 2*1024*1024)

	executables := []string{helperBinary, jackBinary, targetBinary}
	best := scorer.ChooseBest(executables, "mod-desktop-0.0.12-linux-x86-64", installDir)

	if best != targetBinary {
		t.Fatalf("expected %s to be selected, got %s", targetBinary, best)
	}
}

func TestScoreExecutablePenalizesSharedLibraries(t *testing.T) {
	t.Parallel()

	logger := zerolog.New(io.Discard)
	scorer := NewScorer(&logger)

	installDir := filepath.Join(t.TempDir(), "mod-desktop-0.0.12-linux-x86-64")
	appDir := filepath.Join(installDir, "mod-desktop")

	libJack := filepath.Join(appDir, "libjack.so.0")
	mainBinary := filepath.Join(appDir, "mod-desktop")

	writeExecutable(t, libJack, 2*1024*1024)
	writeExecutable(t, mainBinary, 200*1024)

	libScore := scorer.ScoreExecutable(libJack, "mod-desktop-0.0.12-linux-x86-64", installDir)
	binScore := scorer.ScoreExecutable(mainBinary, "mod-desktop-0.0.12-linux-x86-64", installDir)

	if libScore >= binScore {
		t.Fatalf("expected library score (%d) to be lower than binary score (%d)", libScore, binScore)
	}
}

func writeExecutable(t *testing.T, path string, size int) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}

	if err := os.WriteFile(path, bytes.Repeat([]byte{0}, size), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
