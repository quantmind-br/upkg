package assetmatch

import (
	"regexp"
	"strings"
)

// Hard-exclude patterns applied to the lowercased asset name before scoring.
// Order does not matter; first match wins for the reason string.
var hardExcludePatterns = []struct {
	re     *regexp.Regexp
	reason string
}{
	{regexp.MustCompile(`\.(sha256|sha512|sha1|md5)(\.txt)?$`), "checksum file"},
	{regexp.MustCompile(`\.(sig|asc|minisig)$`), "signature file"},
	{regexp.MustCompile(`(^|[-_.])(checksums?|shasums?|sha256sums?)([-_.]|$)`), "checksum manifest"},
	{regexp.MustCompile(`(^|[-_.])(debug|dbg|symbols)([-_.]|$)`), "debug symbols"},
	{regexp.MustCompile(`\.pdb$`), "debug symbols"},
	// Only genuine source tarballs/zips — not tools named "src" (e.g. src_linux_amd64).
	{regexp.MustCompile(`(^|[-_.])(sources?|src|source[-_]?code)\.(tar(\.(gz|xz|bz2|zst))?|zip)$`), "source archive"},
	{regexp.MustCompile(`\.src\.tar`), "source archive"},
	{regexp.MustCompile(`(^|[-_.])vendor([-_.]|$)`), "vendor bundle"},
	{regexp.MustCompile(`\.(zsync|torrent)$`), "delta/torrent metadata"},
	{regexp.MustCompile(`(^|[-_.])(sbom|provenance)([-_.]|$)`), "sbom/provenance"},
	// electron-builder / update manifests (latest-linux.yml, latest-mac.yml, …)
	{regexp.MustCompile(`(^|[-_/])latest[-_].*\.(yml|yaml)$`), "update manifest"},
	{regexp.MustCompile(`\.(yml|yaml)$`), "yaml metadata"},
	// Narrow JSON exclude to known metadata names (not all .json artifacts).
	{regexp.MustCompile(`(^|[-_.])(latest|metadata|manifest|blockmap|attestation)[-_.a-z0-9]*\.json$`), "json metadata"},
}

// excludeReason returns a non-empty reason if the asset must be dropped.
func excludeReason(a Asset, target Target) string {
	n := lowerName(a.Name)
	if n == "" {
		return "empty name"
	}
	for _, p := range hardExcludePatterns {
		if p.re.MatchString(n) {
			return p.reason
		}
	}

	os := detectOSFromName(a.Name)
	if os != "" && target.OS != "" && os != target.OS {
		return "os mismatch: " + os
	}

	arch := detectArchFromName(a.Name)
	if arch != "" && arch != archUniversal && target.Arch != "" && arch != target.Arch {
		return "arch mismatch: " + arch
	}

	return ""
}

// formatMatches reports whether the asset format hint satisfies a --format filter.
func formatMatches(format, filter string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}
	format = strings.ToLower(format)
	if format == filter {
		return true
	}
	// Aliases
	switch filter {
	case "tar", "tar.gz", "tgz", "tar.xz":
		return format == formatTarball
	case "archive":
		return format == formatTarball || format == formatZip
	}
	return false
}
