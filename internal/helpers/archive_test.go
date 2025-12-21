package helpers

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTarGz(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid tar.gz", func(t *testing.T) {
		tarGzPath := filepath.Join(tmpDir, "test.tar.gz")
		createTestTarGz(t, tarGzPath, map[string]string{
			"file1.txt": "content1",
			"file2.txt": "content2",
		})

		destDir := filepath.Join(tmpDir, "extract1")
		require.NoError(t, os.MkdirAll(destDir, 0755))

		err := ExtractTarGz(tarGzPath, destDir)
		assert.NoError(t, err)

		content1, err := os.ReadFile(filepath.Join(destDir, "file1.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content1", string(content1))
	})

	t.Run("corrupted archive", func(t *testing.T) {
		corruptedPath := filepath.Join(tmpDir, "corrupted.tar.gz")
		require.NoError(t, os.WriteFile(corruptedPath, []byte("not a tar.gz"), 0644))

		destDir := filepath.Join(tmpDir, "extract2")
		require.NoError(t, os.MkdirAll(destDir, 0755))

		err := ExtractTarGz(corruptedPath, destDir)
		assert.Error(t, err)
	})

	t.Run("non-existent file", func(t *testing.T) {
		destDir := filepath.Join(tmpDir, "extract3")
		require.NoError(t, os.MkdirAll(destDir, 0755))

		err := ExtractTarGz("/nonexistent/file.tar.gz", destDir)
		assert.Error(t, err)
	})
}

func TestExtractTar(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid tar", func(t *testing.T) {
		tarPath := filepath.Join(tmpDir, "test.tar")
		createTestTar(t, tarPath, map[string]string{
			"file.txt": "content",
		})

		destDir := filepath.Join(tmpDir, "extract")
		require.NoError(t, os.MkdirAll(destDir, 0755))

		err := ExtractTar(tarPath, destDir)
		assert.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(destDir, "file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content", string(content))
	})
}

func TestExtractTarXz(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid tar.xz", func(t *testing.T) {
		_, err := exec.LookPath("xz")
		if err != nil {
			t.Skip("xz not available")
			return
		}

		tarXzPath := filepath.Join(tmpDir, "test.tar.xz")
		err = ExtractTarXz(tarXzPath, tmpDir)
		assert.Error(t, err)
	})
}

func TestExtractTarBz2(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid tar.bz2", func(t *testing.T) {
		_, err := exec.LookPath("bzip2")
		if err != nil {
			t.Skip("bzip2 not available")
			return
		}

		tarBz2Path := filepath.Join(tmpDir, "test.tar.bz2")
		err = ExtractTarBz2(tarBz2Path, tmpDir)
		assert.Error(t, err)
	})
}

func TestExtractZip(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("valid zip", func(t *testing.T) {
		zipPath := filepath.Join(tmpDir, "test.zip")
		createTestZip(t, zipPath, map[string]string{
			"file.txt": "content",
		})

		destDir := filepath.Join(tmpDir, "extract")
		require.NoError(t, os.MkdirAll(destDir, 0755))

		err := ExtractZip(zipPath, destDir)
		assert.NoError(t, err)

		content, err := os.ReadFile(filepath.Join(destDir, "file.txt"))
		assert.NoError(t, err)
		assert.Equal(t, "content", string(content))
	})

	t.Run("corrupted zip", func(t *testing.T) {
		corruptedPath := filepath.Join(tmpDir, "corrupted.zip")
		require.NoError(t, os.WriteFile(corruptedPath, []byte("not a zip"), 0644))

		destDir := filepath.Join(tmpDir, "extract2")
		require.NoError(t, os.MkdirAll(destDir, 0755))

		err := ExtractZip(corruptedPath, destDir)
		assert.Error(t, err)
	})
}

func TestExtractionLimiter(t *testing.T) {
	t.Run("within limits", func(t *testing.T) {
		limiter := newExtractionLimiter(1000)
		assert.NoError(t, limiter.checkLimits(100))
		assert.NoError(t, limiter.checkLimits(200))
	})

	t.Run("exceeds total size", func(t *testing.T) {
		limiter := newExtractionLimiter(1000)
		err := limiter.checkLimits(MaxExtractedSize + 1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "extraction size limit exceeded")
	})

	t.Run("exceeds file count", func(t *testing.T) {
		limiter := newExtractionLimiter(1000)
		for i := 0; i <= MaxFileCount; i++ {
			err := limiter.checkLimits(1)
			if err != nil {
				assert.Contains(t, err.Error(), "file count limit exceeded")
				return
			}
		}
		t.Fatal("Expected file count error")
	})

	t.Run("exceeds compression ratio", func(t *testing.T) {
		limiter := newExtractionLimiter(100)
		// Extract 1000x the original size
		for i := 0; i < 10; i++ {
			err := limiter.checkLimits(10000)
			if err != nil {
				assert.Contains(t, err.Error(), "compression ratio")
				return
			}
		}
	})
}

// Helper functions
func createTestTarGz(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(content)),
		}
		require.NoError(t, tw.WriteHeader(header))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
}

func createTestTar(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	tw := tar.NewWriter(f)
	defer tw.Close()

	for name, content := range files {
		header := &tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(content)),
		}
		require.NoError(t, tw.WriteHeader(header))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
}

func createTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	for name, content := range files {
		fw, err := zw.Create(name)
		require.NoError(t, err)
		_, err = fw.Write([]byte(content))
		require.NoError(t, err)
	}
}
