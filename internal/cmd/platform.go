package cmd

import (
	"bufio"
	"os"
	"runtime"
	"strings"

	"github.com/quantmind-br/upkg/internal/assetmatch"
)

// detectDistroID reads ID= from /etc/os-release (empty on failure).
func detectDistroID() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "ID=") {
			v := strings.TrimPrefix(line, "ID=")
			v = strings.Trim(v, `"'`)
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	if err := sc.Err(); err != nil {
		return ""
	}
	return ""
}

// detectLibc returns "musl" when a musl dynamic linker is found, "glibc" when a
// GNU loader is found, and "" when detection is inconclusive (assetmatch treats
// an empty Libc as unknown and applies no libc scoring).
func detectLibc() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	libDirs := []string{"/lib", "/lib64", "/usr/lib", "/usr/lib64"}
	foundGlibc := false
	for _, dir := range libDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, "ld-musl") {
				return "musl"
			}
			if strings.HasPrefix(name, "ld-linux") || name == "libc.so.6" {
				foundGlibc = true
			}
		}
	}
	if foundGlibc {
		return "glibc"
	}
	return ""
}

// hostAssetTarget builds the assetmatch.Target for this machine.
func hostAssetTarget() assetmatch.Target {
	return assetmatch.HostTarget(detectDistroID(), detectLibc())
}
