package helpers

import (
	"strings"
	"unicode"
)

var (
	archSuffixTokens = map[string]struct{}{
		"x86": {}, "x64": {}, "x86_64": {}, "x86-64": {}, "amd64": {},
		"arm": {}, "arm64": {}, "aarch64": {}, "armhf": {}, "armv7": {},
		"armv7l": {}, "armv6": {}, "armel": {}, "riscv64": {}, "ppc64le": {},
		"s390x": {}, "i386": {}, "i686": {}, "ia32": {}, "sparc": {},
	}
	platformSuffixTokens = map[string]struct{}{
		"linux": {}, "win": {}, "windows": {}, "mac": {}, "macos": {}, "osx": {},
		"darwin": {}, "unix": {}, "gnu": {}, "glibc": {}, "musl": {}, "appimage": {},
		"portable": {}, "release": {}, "cli": {}, "gtk": {}, "qt": {}, "flatpak": {},
		"tarball": {}, "tar": {},
	}
	releaseSuffixPrefixes = []string{"rc", "beta", "alpha", "nightly", "snapshot", "preview"}
)

// CleanAppName removes version numbers, architecture, and platform suffixes
func CleanAppName(baseName string) string {
	// Handle underscores as separators too for cleaning
	// But we want to preserve the original separator style if possible
	// For simplicity, we assume '-' is the primary separator for versions

	tokens := strings.Split(baseName, "-")

	// Walk backwards removing suffix tokens
	for len(tokens) > 1 {
		last := tokens[len(tokens)-1]
		// Check lowercased version against suffix rules
		// We trim spaces/dots just in case
		cleanLast := strings.Trim(strings.ToLower(last), " ._")

		if isSuffixToken(cleanLast) {
			tokens = tokens[:len(tokens)-1]
		} else {
			break
		}
	}

	return strings.Join(tokens, "-")
}

// GenerateNameVariants produces different normalized variants for matching executable names
func GenerateNameVariants(baseName string) []string {
	normalized := strings.Trim(strings.ToLower(baseName), "-_.")
	if normalized == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var variants []string

	addVariant := func(v string) {
		v = strings.Trim(v, "-_.")
		if v == "" {
			return
		}
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			variants = append(variants, v)
		}
	}

	addVariant(normalized)

	// Iteratively trim suffix tokens like version numbers, platforms, arches
	tokens := strings.Split(normalized, "-")
	for len(tokens) > 1 {
		last := strings.Trim(tokens[len(tokens)-1], "-_.")
		if !isSuffixToken(last) {
			break
		}
		tokens = tokens[:len(tokens)-1]
		addVariant(strings.Join(tokens, "-"))
	}

	// Add compact variants without separators for binaries named without dashes
	originalVariants := append([]string(nil), variants...)
	for _, v := range originalVariants {
		compact := strings.ReplaceAll(v, "-", "")
		addVariant(compact)
	}

	return variants
}

// FormatDisplayName converts a normalized package name to a human-readable display name
// Examples:
//   - "git-butler-nightly" -> "Git Butler Nightly"
//   - "cursor" -> "Cursor"
//   - "firefox-esr" -> "Firefox ESR"
func FormatDisplayName(normalizedName string) string {
	// Replace hyphens and underscores with spaces
	displayName := strings.ReplaceAll(normalizedName, "-", " ")
	displayName = strings.ReplaceAll(displayName, "_", " ")

	// Title case each word
	words := strings.Fields(displayName)
	for i, word := range words {
		if len(word) > 0 {
			// Handle common acronyms that should stay uppercase
			upperWord := strings.ToUpper(word)
			if isCommonAcronym(upperWord) {
				words[i] = upperWord
			} else {
				// Title case: First letter uppercase, rest lowercase
				words[i] = strings.ToUpper(string(word[0])) + strings.ToLower(word[1:])
			}
		}
	}

	return strings.Join(words, " ")
}

func isCommonAcronym(word string) bool {
	acronyms := map[string]bool{
		"API": true, "SDK": true, "IDE": true, "CLI": true,
		"GUI": true, "UI": true, "UX": true, "HTML": true,
		"CSS": true, "JS": true, "JSON": true, "XML": true,
		"SQL": true, "HTTP": true, "HTTPS": true, "FTP": true,
		"SSH": true, "VPN": true, "DNS": true, "URL": true,
		"ESR": true, "LTS": true, "RC": true, "DVD": true,
		"CD": true, "USB": true, "RAM": true, "CPU": true,
		"GPU": true, "AI": true, "ML": true, "AR": true,
		"VR": true, "OS": true, "DB": true, "VM": true,
	}
	return acronyms[word]
}

func isSuffixToken(token string) bool {
	if token == "" {
		return false
	}
	token = strings.Trim(token, "-_.")
	if token == "" {
		return false
	}
	return isVersionToken(token) || isArchToken(token) || isPlatformToken(token) || isReleaseToken(token)
}

func isVersionToken(token string) bool {
	if token == "" {
		return false
	}
	if token[0] == 'v' && len(token) > 1 && looksNumeric(token[1:]) {
		return true
	}
	return looksNumeric(token)
}

func looksNumeric(token string) bool {
	hasDigit := false
	for _, r := range token {
		if unicode.IsDigit(r) {
			hasDigit = true
			continue
		}
		if r == '.' {
			continue
		}
		return false
	}
	return hasDigit
}

func isArchToken(token string) bool {
	if _, ok := archSuffixTokens[token]; ok {
		return true
	}

	// Tokens like "armhf.tar" should already be trimmed, but keep a fallback
	token = strings.ReplaceAll(token, "_", "-")
	if _, ok := archSuffixTokens[token]; ok {
		return true
	}

	return false
}

func isPlatformToken(token string) bool {
	_, ok := platformSuffixTokens[token]
	return ok
}

func isReleaseToken(token string) bool {
	for _, prefix := range releaseSuffixPrefixes {
		if token == prefix || strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}
