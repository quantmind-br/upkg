package fsops

import (
	"fmt"
	"os"

	"github.com/spf13/afero"
)

// CreateTempDir creates a temporary directory with the given prefix
func CreateTempDir(fs afero.Fs, prefix string) (string, error) {
	tmpDir := os.TempDir()
	dir, err := afero.TempDir(fs, tmpDir, prefix)
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	return dir, nil
}

// CheckWritable checks if a path is writable
func CheckWritable(fs afero.Fs, path string) error {
	testFile := path + "/.write_test"
	f, err := fs.Create(testFile)
	if err != nil {
		return fmt.Errorf("path not writable: %w", err)
	}
	f.Close()
	fs.Remove(testFile)
	return nil
}

// EnsureDir ensures a directory exists with the given permissions
func EnsureDir(fs afero.Fs, path string, perm os.FileMode) error {
	if err := fs.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("ensure directory: %w", err)
	}
	return nil
}

// Exists checks if a path exists
func Exists(fs afero.Fs, path string) bool {
	_, err := fs.Stat(path)
	return err == nil
}

// IsDir checks if a path is a directory
func IsDir(fs afero.Fs, path string) bool {
	info, err := fs.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CopyFile copies a file from src to dst
func CopyFile(fs afero.Fs, src, dst string) error {
	srcFile, err := fs.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := fs.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer dstFile.Close()

	if _, err := afero.ReadFile(fs, src); err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	content, err := afero.ReadFile(fs, src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}

	if _, err := dstFile.Write(content); err != nil {
		return fmt.Errorf("write destination: %w", err)
	}

	return nil
}
