// Package version provides layered comparison of release tags and version strings.
//
// Layers (first decisive wins):
//  1. semver — after stripping common monorepo/prefix noise
//  2. date  — YYYY.MM.DD / YYYY-MM-DD / YYYYMMDD
//  3. natural — numeric segments compared as numbers
//  4. published_at — fallback for opaque tags
package version

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Compare mode identifiers persisted in install metadata.
const (
	ModeSemver    = "semver"
	ModeDate      = "date"
	ModeNatural   = "natural"
	ModePublished = "published"
)

// Result of a comparison of two version strings (or their published times).
type Result int

const (
	// Less means a is older / smaller than b.
	Less Result = -1
	// Equal means a and b are the same under the chosen mode.
	Equal Result = 0
	// Greater means a is newer / larger than b.
	Greater Result = 1
)

// String returns a stable label for Result.
func (r Result) String() string {
	switch r {
	case Less:
		return "less"
	case Equal:
		return "equal"
	case Greater:
		return "greater"
	default:
		return fmt.Sprintf("Result(%d)", int(r))
	}
}

// dateDotRE matches 2024.01.15 or 2024.1.5
var dateDotRE = regexp.MustCompile(`^(\d{4})\.(\d{1,2})\.(\d{1,2})$`)

// dateDashRE matches 2024-01-15
var dateDashRE = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})$`)

// dateCompactRE matches 20240115
var dateCompactRE = regexp.MustCompile(`^(\d{4})(\d{2})(\d{2})$`)

// semverCoreRE is major.minor.patch with optional pre-release and build.
var semverCoreRE = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$`)

// componentStripRE strips monorepo component prefixes before a version digit.
var componentStripRE = regexp.MustCompile(`(?i)^[a-z][a-z0-9]*(?:[-_][a-z][a-z0-9]*)*[-_]v?(\d)`)

// componentPrefixRE extracts a monorepo component prefix from a tag.
var componentPrefixRE = regexp.MustCompile(`(?i)^([a-z][a-z0-9]*(?:[-_][a-z][a-z0-9]*)*)[-_](?:v?\d|latest)`)

// Compare compares version tags a and b using layered detection.
// When both sides parse under the same high-confidence layer (date, semver,
// natural), that layer decides and mode is returned. Otherwise comparison
// falls through to publishedAt (ModePublished). Equal tags always return
// Equal with the best common mode (date first, then semver, then natural;
// opaque tags with no digits report ModePublished).
//
// publishedA/publishedB are only consulted for ModePublished decisions.
// A zero time is treated as older than any non-zero time.
func Compare(a, b string, publishedA, publishedB time.Time) (Result, string) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)

	if a == b && a != "" {
		n := normalizeTag(a)
		if _, ok := parseDate(n); ok {
			return Equal, ModeDate
		}
		if _, ok := parseSemver(n); ok {
			return Equal, ModeSemver
		}
		if hasDigit(n) {
			return Equal, ModeNatural
		}
		return Equal, ModePublished
	}

	na, nb := normalizeTag(a), normalizeTag(b)

	// Date before semver when both sides are calendar versions: YYYY.MM.DD
	// also matches major.minor.patch numerically (2024.01.15) but must report
	// ModeDate so callers persist the right compare_mode.
	da, dateA := parseDate(na)
	db, dateB := parseDate(nb)
	if dateA && dateB {
		return compareTime(da, db), ModeDate
	}

	// Semver
	sa, okA := parseSemver(na)
	sb, okB := parseSemver(nb)
	if okA && okB {
		return compareSemver(sa, sb), ModeSemver
	}

	// 3. Natural — only when neither side is empty after normalize
	if na != "" && nb != "" {
		// Prefer natural over published when both look version-like
		// (contain a digit) so opaque-but-numeric tags still order.
		if hasDigit(na) && hasDigit(nb) {
			return compareNatural(na, nb), ModeNatural
		}
	}

	// 4. published_at
	return compareTime(publishedA, publishedB), ModePublished
}

// IsNewer reports whether candidate is strictly newer than current under
// Compare. Same tag is never newer (caller handles digest-mismatch separately).
func IsNewer(current, candidate string, currentPub, candidatePub time.Time) (bool, string) {
	res, mode := Compare(candidate, current, candidatePub, currentPub)
	return res == Greater, mode
}

// NormalizeForDisplay strips common prefixes for user-facing version fields
// while leaving the original tag intact for pin/ETag purposes.
func NormalizeForDisplay(tag string) string {
	n := normalizeTag(strings.TrimSpace(tag))
	if n == "" {
		return strings.TrimSpace(tag)
	}
	return n
}

// ComponentPrefix extracts a monorepo component prefix from a tag when present.
// "desktop-v0.5.5" → "desktop"; "v0.5.2" → ""; "sprig-latest" → "sprig".
func ComponentPrefix(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	m := componentPrefixRE.FindStringSubmatch(tag)
	if len(m) == 2 {
		return strings.ToLower(m[1])
	}
	return ""
}

func normalizeTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	// Strip repeated leading "v" / "release-" / "<component>-v" noise.
	// Apply iteratively for release-v1.0.0 style.
	for {
		lower := strings.ToLower(tag)
		switch {
		case strings.HasPrefix(lower, "release-"):
			tag = tag[len("release-"):]
			continue
		case strings.HasPrefix(lower, "release_"):
			tag = tag[len("release_"):]
			continue
		}
		break
	}

	// component-vX.Y.Z or component-X.Y.Z
	if loc := componentStripRE.FindStringSubmatchIndex(tag); loc != nil {
		// Keep from the captured digit onward (group 1 start).
		tag = tag[loc[2]:]
	} else if len(tag) > 1 {
		lower := strings.ToLower(tag)
		if lower[0] == 'v' && unicode.IsDigit(rune(tag[1])) {
			tag = tag[1:]
		}
	}
	return tag
}

type semver struct {
	major, minor, patch int
	pre                 []string // empty = release
}

func parseSemver(s string) (semver, bool) {
	if m := semverCoreRE.FindStringSubmatch(s); m != nil {
		maj, ok1 := atoi(m[1])
		minor, ok2 := atoi(m[2])
		pat, ok3 := atoi(m[3])
		if !ok1 || !ok2 || !ok3 {
			return semver{}, false
		}
		sv := semver{major: maj, minor: minor, patch: pat}
		if m[4] != "" {
			sv.pre = strings.Split(m[4], ".")
		}
		return sv, true
	}
	return parseLooseSemver(s)
}

// parseLooseSemver accepts major.minor(.patch) with optional pre-release.
func parseLooseSemver(s string) (semver, bool) {
	parts := strings.SplitN(s, "-", 2)
	core := parts[0]
	core = strings.SplitN(core, "+", 2)[0]
	segs := strings.Split(core, ".")
	if len(segs) != 2 && len(segs) != 3 {
		return semver{}, false
	}
	for _, seg := range segs {
		if seg == "" || !isAllDigits(seg) {
			return semver{}, false
		}
	}
	maj, ok := atoi(segs[0])
	if !ok {
		return semver{}, false
	}
	minor, ok := atoi(segs[1])
	if !ok {
		return semver{}, false
	}
	pat := 0
	if len(segs) == 3 {
		pat, ok = atoi(segs[2])
		if !ok {
			return semver{}, false
		}
	}
	if maj > 9999 {
		return semver{}, false
	}
	sv := semver{major: maj, minor: minor, patch: pat}
	if len(parts) == 2 && parts[1] != "" {
		pre := strings.SplitN(parts[1], "+", 2)[0]
		if pre != "" {
			sv.pre = strings.Split(pre, ".")
		}
	}
	return sv, true
}

func compareSemver(a, b semver) Result {
	if a.major != b.major {
		return cmpInt(a.major, b.major)
	}
	if a.minor != b.minor {
		return cmpInt(a.minor, b.minor)
	}
	if a.patch != b.patch {
		return cmpInt(a.patch, b.patch)
	}
	// Pre-release: absence > presence (1.0.0 > 1.0.0-rc1)
	if len(a.pre) == 0 && len(b.pre) == 0 {
		return Equal
	}
	if len(a.pre) == 0 {
		return Greater
	}
	if len(b.pre) == 0 {
		return Less
	}
	return comparePre(a.pre, b.pre)
}

func comparePre(a, b []string) Result {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		ai, aNum := parsePreIdent(a[i])
		bi, bNum := parsePreIdent(b[i])
		if aNum && bNum {
			if ai != bi {
				return cmpInt(ai, bi)
			}
			continue
		}
		if aNum && !bNum {
			return Less // numeric < alphanumeric per semver
		}
		if !aNum && bNum {
			return Greater
		}
		if a[i] != b[i] {
			if a[i] < b[i] {
				return Less
			}
			return Greater
		}
	}
	return cmpInt(len(a), len(b))
}

func parsePreIdent(s string) (int, bool) {
	if s == "" || !isAllDigits(s) {
		return 0, false
	}
	// Leading zeros make it non-numeric per strict semver; treat as alnum.
	if len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

func matchDateParts(s string) []string {
	if m := dateDotRE.FindStringSubmatch(s); m != nil {
		return m
	}
	if m := dateDashRE.FindStringSubmatch(s); m != nil {
		return m
	}
	return dateCompactRE.FindStringSubmatch(s)
}

func parseDate(s string) (time.Time, bool) {
	m := matchDateParts(s)
	if m == nil {
		return time.Time{}, false
	}
	y, ok1 := atoi(m[1])
	mo, ok2 := atoi(m[2])
	d, ok3 := atoi(m[3])
	if !ok1 || !ok2 || !ok3 {
		return time.Time{}, false
	}
	if mo < 1 || mo > 12 || d < 1 || d > 31 || y < 1970 || y > 9999 {
		return time.Time{}, false
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	// Reject overflow (e.g. Feb 31)
	if t.Year() != y || int(t.Month()) != mo || t.Day() != d {
		return time.Time{}, false
	}
	return t, true
}

func compareTime(a, b time.Time) Result {
	aZero, bZero := a.IsZero(), b.IsZero()
	if aZero && bZero {
		return Equal
	}
	if aZero {
		return Less
	}
	if bZero {
		return Greater
	}
	switch {
	case a.Before(b):
		return Less
	case a.After(b):
		return Greater
	default:
		return Equal
	}
}

//nolint:gocyclo // natural version tokenization is inherently branched
func compareNatural(a, b string) Result {
	ia, ib := 0, 0
	for ia < len(a) && ib < len(b) {
		ca, cb := a[ia], b[ib]
		aDigit, bDigit := isDigit(ca), isDigit(cb)
		if aDigit && bDigit {
			// Consume full numeric runs
			ja, jb := ia, ib
			for ja < len(a) && isDigit(a[ja]) {
				ja++
			}
			for jb < len(b) && isDigit(b[jb]) {
				jb++
			}
			// Skip leading zeros for value compare but preserve length for tie
			na := strings.TrimLeft(a[ia:ja], "0")
			nb := strings.TrimLeft(b[ib:jb], "0")
			if na == "" {
				na = "0"
			}
			if nb == "" {
				nb = "0"
			}
			if len(na) != len(nb) {
				return cmpInt(len(na), len(nb))
			}
			if na != nb {
				if na < nb {
					return Less
				}
				return Greater
			}
			ia, ib = ja, jb
			continue
		}
		if ca != cb {
			// digit vs non-digit: digit sorts first for natural feel? use byte order
			if ca < cb {
				return Less
			}
			return Greater
		}
		ia++
		ib++
	}
	// Pre-release / build suffixes: leftover starting with '-' or '+' is lower
	// precedence (same idea as semver), so stable > pre-release for non-semver.
	if ia < len(a) && (a[ia] == '-' || a[ia] == '+') && ib >= len(b) {
		return Less
	}
	if ib < len(b) && (b[ib] == '-' || b[ib] == '+') && ia >= len(a) {
		return Greater
	}
	return cmpInt(len(a)-ia, len(b)-ib)
}

func cmpInt(a, b int) Result {
	switch {
	case a < b:
		return Less
	case a > b:
		return Greater
	default:
		return Equal
	}
}

func atoi(s string) (int, bool) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

func hasDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if isDigit(s[i]) {
			return true
		}
	}
	return false
}
