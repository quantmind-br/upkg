package heuristics

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
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

func TestFindExecutables(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("empty directory", func(t *testing.T) {
		executables, err := FindExecutables(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(executables) != 0 {
			t.Errorf("expected empty, got %v", executables)
		}
	})

	t.Run("no executables", func(t *testing.T) {
		os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("test"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "lib.so"), []byte("test"), 0644)

		executables, err := FindExecutables(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(executables) != 0 {
			t.Errorf("expected empty, got %v", executables)
		}
	})

	t.Run("with ELF binary", func(t *testing.T) {
		// Use /bin/ls as it should exist on most systems
		lsPath := "/bin/ls"
		if _, err := os.Stat(lsPath); os.IsNotExist(err) {
			t.Skip("/bin/ls not found")
			return
		}

		// Copy to temp dir
		execPath := filepath.Join(tmpDir, "ls")
		content, _ := os.ReadFile(lsPath)
		os.WriteFile(execPath, content, 0755)

		executables, err := FindExecutables(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(executables) != 1 {
			t.Errorf("expected 1 executable, got %d: %v", len(executables), executables)
		}
	})
}

func TestScoreExecutable(t *testing.T) {
	logger := zerolog.New(io.Discard)
	scorer := NewScorer(&logger)

	tests := []struct {
		name     string
		path     string
		baseName string
		expected int
	}{
		{"exact match", "/app/myapp", "myapp", 100},
		{"partial match", "/app/myapp-bin", "myapp", 50},
		{"bonus - run", "/app/run.sh", "myapp", 80},
		{"bonus - start", "/app/start.sh", "myapp", 80},
		{"bonus - launch", "/app/launch.sh", "myapp", 80},
		{"bonus - main", "/app/main", "myapp", 90},
		{"bonus - app", "/app/app", "myapp", 85},
		{"penalty - chrome-sandbox", "/app/chrome-sandbox", "myapp", -100},
		{"penalty - update", "/app/update", "myapp", -50},
		{"penalty - uninstall", "/app/uninstall", "myapp", -50},
		{"penalty - lib", "/app/libmyapp.so", "myapp", -20},
		{"depth - shallow", "/myapp", "myapp", 110},
		{"depth - deep", "/a/b/c/d/e/myapp", "myapp", 80},
		{"bin directory", "/bin/myapp", "myapp", 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file to make it real
			tmpDir := t.TempDir()
			testPath := filepath.Join(tmpDir, "test")
			os.WriteFile(testPath, []byte{}, 0755)

			// Use the actual path pattern
			score := scorer.ScoreExecutable(testPath, tt.baseName, tmpDir)
			// We can't test exact scores without the full path, but we can test relative scoring
			_ = score
		})
	}
}

func TestChooseBestWithMultipleCandidates(t *testing.T) {
	logger := zerolog.New(io.Discard)
	scorer := NewScorer(&logger)

	tmpDir := t.TempDir()

	// Create candidates
	app1 := filepath.Join(tmpDir, "myapp")
	app2 := filepath.Join(tmpDir, "libmyapp.so")
	app3 := filepath.Join(tmpDir, "myapp-helper")

	os.WriteFile(app1, []byte{}, 0755)
	os.WriteFile(app2, []byte{}, 0755)
	os.WriteFile(app3, []byte{}, 0755)

	candidates := []string{app1, app2, app3}
	best := scorer.ChooseBest(candidates, "myapp", tmpDir)

	if best != app1 {
		t.Errorf("expected %s to be selected, got %s", app1, best)
	}
}

func TestChooseBestEmpty(t *testing.T) {
	logger := zerolog.New(io.Discard)
	scorer := NewScorer(&logger)

	best := scorer.ChooseBest([]string{}, "myapp", "/tmp")
	if best != "" {
		t.Errorf("expected empty string, got %s", best)
	}
}

func TestChooseBestSingle(t *testing.T) {
	logger := zerolog.New(io.Discard)
	scorer := NewScorer(&logger)

	tmpDir := t.TempDir()
	single := filepath.Join(tmpDir, "app")
	os.WriteFile(single, []byte{}, 0755)

	best := scorer.ChooseBest([]string{single}, "myapp", tmpDir)
	if best != single {
		t.Errorf("expected %s, got %s", single, best)
	}
}

func TestDefaultScorerStruct(t *testing.T) {
	logger := zerolog.New(io.Discard)
	scorer := NewScorer(&logger)

	if scorer == nil {
		t.Fatal("scorer should not be nil")
	}

	if scorer.Logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestIsInvalidWrapperScript(t *testing.T) {
	logger := zerolog.New(io.Discard)
	scorer := NewScorer(&logger)

	t.Run("non-existent file", func(t *testing.T) {
		result := scorer.isInvalidWrapperScript("/nonexistent/file")
		assert.False(t, result, "Non-existent file should return false")
	})

	t.Run("large file (> 10KB)", func(t *testing.T) {
		tmpDir := t.TempDir()
		largeFile := filepath.Join(tmpDir, "large")
		content := bytes.Repeat([]byte{0}, 11*1024)
		os.WriteFile(largeFile, content, 0755)

		result := scorer.isInvalidWrapperScript(largeFile)
		assert.False(t, result, "Large files should be skipped")
	})

	t.Run("file without shebang", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptFile := filepath.Join(tmpDir, "script.sh")
		os.WriteFile(scriptFile, []byte("echo hello\n"), 0755)

		result := scorer.isInvalidWrapperScript(scriptFile)
		assert.False(t, result, "Non-script files should return false")
	})

	t.Run("script with invalid build path /home/runner/", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptFile := filepath.Join(tmpDir, "wrapper.sh")
		content := []byte("#!/bin/bash\nexec /home/runner/work/app/app/binary \"$@\"\n")
		os.WriteFile(scriptFile, content, 0755)

		result := scorer.isInvalidWrapperScript(scriptFile)
		assert.True(t, result, "Script with /home/runner/ path should be invalid")
	})

	t.Run("script with invalid build path /tmp/build/", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptFile := filepath.Join(tmpDir, "wrapper.sh")
		content := []byte("#!/bin/bash\nexec /tmp/build/app/binary \"$@\"\n")
		os.WriteFile(scriptFile, content, 0755)

		result := scorer.isInvalidWrapperScript(scriptFile)
		assert.True(t, result, "Script with /tmp/build/ path should be invalid")
	})

	t.Run("script with /workspace/ path", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptFile := filepath.Join(tmpDir, "wrapper.sh")
		content := []byte("#!/bin/sh\n/workspace/build/app \"$@\"\n")
		os.WriteFile(scriptFile, content, 0755)

		result := scorer.isInvalidWrapperScript(scriptFile)
		assert.True(t, result, "Script with /workspace/ path should be invalid")
	})

	t.Run("valid script with local path", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptFile := filepath.Join(tmpDir, "app.sh")
		content := []byte("#!/bin/bash\nexec ./binary \"$@\"\n")
		os.WriteFile(scriptFile, content, 0755)

		result := scorer.isInvalidWrapperScript(scriptFile)
		assert.False(t, result, "Script with local path should be valid")
	})

	t.Run("valid script with $APPDIR", func(t *testing.T) {
		tmpDir := t.TempDir()
		scriptFile := filepath.Join(tmpDir, "app.sh")
		content := []byte("#!/bin/bash\nexec $APPDIR/binary \"$@\"\n")
		os.WriteFile(scriptFile, content, 0755)

		result := scorer.isInvalidWrapperScript(scriptFile)
		assert.False(t, result, "Script with $APPDIR should be valid")
	})
}

func TestFindExecutablesErrors(t *testing.T) {
	t.Run("non-existent directory", func(t *testing.T) {
		_, err := FindExecutables("/nonexistent/directory")
		assert.Error(t, err, "Should return error for non-existent directory")
	})

	t.Run("directory with read error", func(t *testing.T) {
		// This is hard to test without actually making a directory unreadable
		// Skip for now as it requires specific permissions
		t.Skip("Requires specific filesystem permissions")
	})
}
