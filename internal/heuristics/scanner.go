package heuristics

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/quantmind-br/upkg/internal/helpers"
)

// FindExecutables finds all executable files in a directory recursively
func FindExecutables(dir string) ([]string, error) {
	var executables []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Check if file is executable
		if info.Mode()&0111 != 0 {
			// Exclude shared libraries (.so files)
			// .so, .so.X, .so.X.Y, .so.X.Y.Z patterns
			baseName := filepath.Base(path)
			if strings.HasSuffix(baseName, ".so") || strings.Contains(baseName, ".so.") {
				return nil
			}

			// Check if it's an ELF binary using helper
			isElf, elfErr := helpers.IsELF(path)
			if elfErr != nil {
				return nil // Skip unreadable or invalid files
			}
			if isElf {
				executables = append(executables, path)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return executables, nil
}
