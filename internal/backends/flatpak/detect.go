package flatpak

import (
	"bufio"
	"context"
	"regexp"
	"strings"

	"github.com/spf13/afero"
)

// App ID regex: at least 3 segments, each starting with letter, containing only letters/numbers/underscores
var appIDRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*){2,}$`)

// Detect checks if the input is a flatpak package, flatpakref, or App ID
func Detect(ctx context.Context, fs afero.Fs, input string) (bool, error) {
	// Check if input looks like a file path (contains / or starts with .)
	isFilePath := strings.Contains(input, "/") || strings.HasPrefix(input, ".")

	if isFilePath {
		// Try to detect as file
		return detectFile(fs, input)
	}

	// Not a file path - check if it's an App ID
	return appIDRegex.MatchString(input), nil
}

// detectFile checks if the file is a .flatpak or .flatpakref
func detectFile(fs afero.Fs, path string) (bool, error) {
	// Check if file exists
	if _, err := fs.Stat(path); err != nil {
		return false, nil
	}

	// Check extension
	if strings.HasSuffix(path, ".flatpak") {
		return detectFlatpakBundle(fs, path)
	}

	if strings.HasSuffix(path, ".flatpakref") {
		return detectFlatpakRef(fs, path)
	}

	return false, nil
}

// detectFlatpakBundle checks for flatpak bundle formats:
// - OSTree/GVariant: starts with "flatpak\x00" (8 bytes)
// - OCI bundle: ZIP magic (PK\x03\x04)
func detectFlatpakBundle(fs afero.Fs, path string) (bool, error) {
	file, err := fs.Open(path)
	if err != nil {
		return false, nil
	}
	defer file.Close()

	magic := make([]byte, 8)
	n, err := file.Read(magic)
	if err != nil || n < 4 {
		return false, nil
	}

	// Check for OSTree/GVariant flatpak bundle: "flatpak\x00"
	if n >= 8 && string(magic[:7]) == "flatpak" && magic[7] == 0x00 {
		return true, nil
	}

	// Check for OCI bundle (ZIP magic): 0x50 0x4B 0x03 0x04
	if magic[0] == 0x50 && magic[1] == 0x4B && magic[2] == 0x03 && magic[3] == 0x04 {
		return true, nil
	}

	return false, nil
}

// detectFlatpakRef checks for [Flatpak Ref] header
func detectFlatpakRef(fs afero.Fs, path string) (bool, error) {
	file, err := fs.Open(path)
	if err != nil {
		return false, nil
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return false, nil
	}

	firstLine := strings.TrimSpace(scanner.Text())
	return firstLine == "[Flatpak Ref]", nil
}
