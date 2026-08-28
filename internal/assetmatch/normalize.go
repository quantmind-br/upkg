package assetmatch

import (
	"runtime"
	"strings"
)

// HostTarget builds a Target from the running process and optional distro/libc hints.
func HostTarget(distroID, libc string) Target {
	return Target{
		OS:       normalizeOS(runtime.GOOS),
		Arch:     normalizeArch(runtime.GOARCH),
		Libc:     normalizeLibc(libc),
		DistroID: strings.ToLower(strings.TrimSpace(distroID)),
	}
}

func normalizeOS(os string) string {
	switch strings.ToLower(strings.TrimSpace(os)) {
	case "linux":
		return "linux"
	case "darwin", "macos", "osx", "mac":
		return "darwin"
	case "windows", "win", "win32", "win64":
		return "windows"
	case "freebsd":
		return "freebsd"
	default:
		return strings.ToLower(strings.TrimSpace(os))
	}
}

func normalizeArch(arch string) string {
	a := strings.ToLower(strings.TrimSpace(arch))
	switch a {
	case "amd64", "x86_64", "x64", "x86-64":
		return "amd64"
	case "arm64", "aarch64", "arm64v8":
		return "arm64"
	case "386", "i386", "i686", "x86", "ia32":
		return "386"
	case "arm", "armv7", "armhf", "armv6":
		return "arm"
	case "riscv64":
		return "riscv64"
	case "ppc64le":
		return "ppc64le"
	case "s390x":
		return "s390x"
	default:
		return a
	}
}

func normalizeLibc(libc string) string {
	switch strings.ToLower(strings.TrimSpace(libc)) {
	case "glibc", "gnu", "libc":
		return "glibc"
	case "musl", "musl-libc":
		return "musl"
	default:
		return strings.ToLower(strings.TrimSpace(libc))
	}
}

// lowerName returns a lowercase asset name for matching.
func lowerName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// archAliases are tokens in asset names that map to a normalized arch.
var archAliases = map[string]string{
	"amd64":       "amd64",
	"x86_64":      "amd64",
	"x86-64":      "amd64",
	"x64":         "amd64",
	"arm64":       "arm64",
	"aarch64":     "arm64",
	"arm64v8":     "arm64",
	"armv8l":      "arm64",
	"i386":        "386",
	"i686":        "386",
	"x86":         "386",
	"ia32":        "386",
	"386":         "386",
	"armv7":       "arm",
	"armv7l":      "arm",
	"armhf":       "arm",
	"armv6":       "arm",
	"armv6l":      "arm",
	"riscv64":     "riscv64",
	"ppc64le":     "ppc64le",
	"s390x":       "s390x",
	"universal2":  archUniversal,
	archUniversal: archUniversal,
	"any":         "universal",
	"noarch":      "universal",
	"all":         "universal",
}

// osAliases map name tokens to OS.
var osAliases = map[string]string{
	"linux":   "linux",
	"darwin":  "darwin",
	"macos":   "darwin",
	"osx":     "darwin",
	"mac":     "darwin",
	"windows": "windows",
	"win32":   "windows",
	"win64":   "windows",
	"win":     "windows",
	"freebsd": "freebsd",
}

// detectArchFromName returns the first arch token found in name, or "".
// Longer tokens are checked first to avoid "64" false positives inside versions.
func detectArchFromName(name string) string {
	n := lowerName(name)
	// Prefer longer tokens first.
	// Longer / more specific tokens first so armv7l is not lost to bare "arm".
	order := []string{
		"universal2", "x86_64", "x86-64", "aarch64", "arm64v8", "ppc64le", "riscv64",
		"amd64", "arm64", "armv8l", "armv7l", "armv6l", "i386", "i686", "armv7", "armhf", "armv6",
		"s390x", "x64", "x86", "386", "universal", "noarch",
	}
	for _, tok := range order {
		if tokenPresent(n, tok) {
			if arch, ok := archAliases[tok]; ok {
				return arch
			}
		}
	}
	// bare "arm" last (short, easy false positive)
	if tokenPresent(n, "arm") && !tokenPresent(n, "arm64") && !strings.Contains(n, "darwin") {
		return "arm"
	}
	return ""
}

// detectOSFromName returns OS from name tokens. Empty if unspecified.
// macOS app bundles (.app, .dmg) force darwin even without an OS token.
//
//nolint:gocyclo // OS token table with strong-signal suffixes
func detectOSFromName(name string) string {
	n := lowerName(name)
	// Strong darwin signals first (Buzz ships x64.app.tar.gz for macOS).
	// Use suffix checks so Arch Linux .pkg.tar.zst is not classified as darwin.
	if strings.HasSuffix(n, ".dmg") || strings.HasSuffix(n, ".pkg") {
		return "darwin"
	}
	if strings.Contains(n, ".app.") || strings.HasSuffix(n, ".app") || strings.Contains(n, ".app.tar") {
		return "darwin"
	}
	if strings.HasSuffix(n, ".exe") || strings.HasSuffix(n, ".msi") || strings.Contains(n, ".exe.") {
		return "windows"
	}
	order := []string{"linux", "darwin", "macos", "osx", "windows", "win64", "win32", "freebsd"}
	for _, tok := range order {
		if tokenPresent(n, tok) {
			if os, ok := osAliases[tok]; ok {
				return os
			}
			if tok == "win64" || tok == "win32" {
				return "windows"
			}
		}
	}
	// bare "mac" / "win" as path segments
	if tokenPresent(n, "mac") {
		return "darwin"
	}
	if tokenPresent(n, "win") {
		return "windows"
	}
	return ""
}

// detectLibcFromName returns glibc/musl/"" from name.
func detectLibcFromName(name string) string {
	n := lowerName(name)
	if strings.Contains(n, "musl") {
		return "musl"
	}
	// gnu in target triple form (unknown-linux-gnu / gnueabihf), not product names.
	if strings.Contains(n, "linux-gnu") || tokenPresent(n, "gnu") || tokenPresent(n, "gnueabihf") {
		return "glibc"
	}
	if strings.Contains(n, "glibc") {
		return "glibc"
	}
	// linuxstatic often musl-ish static; treat as musl penalty signal via name later
	return ""
}

// DetectFormat returns a coarse format hint from an asset file name.
func DetectFormat(name string) string {
	return detectFormatFromName(name)
}

// detectFormatFromName returns a coarse format hint.
func detectFormatFromName(name string) string {
	n := lowerName(name)
	switch {
	case strings.HasSuffix(n, ".appimage") || strings.Contains(n, ".appimage."):
		return formatAppImage
	case strings.HasSuffix(n, ".deb"):
		return "deb"
	case strings.HasSuffix(n, ".rpm"):
		return "rpm"
	case strings.HasSuffix(n, ".dmg") || strings.HasSuffix(n, ".pkg"):
		return "darwin-native"
	case strings.HasSuffix(n, ".exe") || strings.HasSuffix(n, ".msi"):
		return "windows-native"
	case strings.HasSuffix(n, ".zip"):
		return "zip"
	case strings.HasSuffix(n, ".tar.gz"), strings.HasSuffix(n, ".tgz"),
		strings.HasSuffix(n, ".tar.xz"), strings.HasSuffix(n, ".txz"),
		strings.HasSuffix(n, ".tar.bz2"), strings.HasSuffix(n, ".tbz2"),
		strings.HasSuffix(n, ".tar.zst"), strings.HasSuffix(n, ".tar"):
		return "tarball"
	case strings.HasSuffix(n, ".gz") && !strings.Contains(n, ".tar."):
		// single-file gzip (helper binaries)
		return formatBinary
	default:
		return "unknown"
	}
}

// tokenPresent checks tok as a path/name token (not a bare substring inside a version).
// Allows separators: start/end, [-_.] around the token.
func tokenPresent(name, tok string) bool {
	if tok == "" || name == "" {
		return false
	}
	idx := 0
	for {
		i := strings.Index(name[idx:], tok)
		if i < 0 {
			return false
		}
		i += idx
		beforeOK := i == 0 || isSep(name[i-1])
		after := i + len(tok)
		afterOK := after >= len(name) || isSep(name[after])
		if beforeOK && afterOK {
			return true
		}
		idx = i + 1
	}
}

func isSep(b byte) bool {
	switch b {
	case '-', '_', '.', '/', '+':
		return true
	default:
		return false
	}
}
