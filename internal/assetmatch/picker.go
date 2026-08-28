package assetmatch

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Default auto-pick thresholds from design §5.3.
// MinLead is 20 so a distro-preferred format (e.g. deb +90 vs AppImage +70)
// still auto-picks under --yes, while near-ties still require a menu/--asset.
const (
	DefaultMinScore = 60
	DefaultMinLead  = 20
)

// Score weights (design §5.2).
const (
	scoreOSExact       = 400
	scoreArchExact     = 300
	scoreArchUniversal = 100
	scoreLibcExact     = 60
	scoreDistroDeb     = 90
	scoreDistroRpm     = 80
	scoreAppImage      = 70
	scoreTarball       = 30
	scoreZip           = 20
	scoreAppName       = 15
	penaltyMuslOnGlibc = -40
	penaltyUnsigned    = -10
	penaltyTiny        = -5
)

// Pick filters, scores, and optionally auto-selects a release asset.
//
//nolint:gocyclo // filter/score/threshold branches
func Pick(assets []Asset, target Target, opts PickOptions) (PickResult, error) {
	minScore := opts.MinScore
	if minScore <= 0 {
		minScore = DefaultMinScore
	}
	minLead := opts.MinLead
	if minLead <= 0 {
		minLead = DefaultMinLead
	}

	var assetRE *regexp.Regexp
	if opts.AssetRegex != "" {
		re, err := regexp.Compile(opts.AssetRegex)
		if err != nil {
			return PickResult{}, fmt.Errorf("invalid --asset regex: %w", err)
		}
		assetRE = re
	}

	var excluded []Candidate
	var compatible []Candidate

	for _, a := range assets {
		format := detectFormatFromName(a.Name)

		if reason := excludeReason(a, target); reason != "" {
			excluded = append(excluded, Candidate{Asset: a, Format: format, ExcludeReason: reason})
			continue
		}
		if assetRE != nil && !assetRE.MatchString(a.Name) {
			excluded = append(excluded, Candidate{Asset: a, Format: format, ExcludeReason: "asset regex mismatch"})
			continue
		}
		if !formatMatches(format, opts.Format) {
			excluded = append(excluded, Candidate{Asset: a, Format: format, ExcludeReason: "format filter: " + opts.Format})
			continue
		}

		score := scoreAsset(a, format, target, opts.AppName)
		compatible = append(compatible, Candidate{Asset: a, Score: score, Format: format})
	}

	sort.SliceStable(compatible, func(i, j int) bool {
		if compatible[i].Score != compatible[j].Score {
			return compatible[i].Score > compatible[j].Score
		}
		// Deterministic tie-break: lexicographic name.
		return compatible[i].Asset.Name < compatible[j].Asset.Name
	})

	res := PickResult{
		Compatible: compatible,
		Excluded:   excluded,
	}

	if len(compatible) == 0 {
		res.NeedsMenu = false
		return res, nil
	}

	if len(compatible) == 1 {
		w := compatible[0]
		res.Winner = &w
		res.NeedsMenu = false
		return res, nil
	}

	lead := compatible[0].Score - compatible[1].Score
	clearWinner := compatible[0].Score >= minScore && lead >= minLead

	// --yes auto-picks only when the top candidate is a clear winner
	// (score≥MinScore and lead≥MinLead). Otherwise the caller must prompt
	// or pass --asset / --format. Scripts that need a specific asset should
	// pass --asset explicitly.
	if opts.Yes && clearWinner {
		w := compatible[0]
		res.Winner = &w
		res.NeedsMenu = false
		return res, nil
	}

	res.NeedsMenu = true
	return res, nil
}

//nolint:gocyclo // weighted multi-signal scorer
func scoreAsset(a Asset, format string, target Target, appName string) int {
	n := lowerName(a.Name)
	score := 0

	os := detectOSFromName(a.Name)
	if os != "" && os == target.OS {
		score += scoreOSExact
	} else if os == "" && target.OS == "linux" {
		// Unspecified OS on a linux-looking package format: mild OS credit.
		switch format {
		case formatAppImage, formatDeb, formatRPM:
			score += scoreOSExact // these formats are effectively linux-only
		case "tarball", "zip", "binary":
			// weak: many tarballs omit "linux" in the name
			score += scoreOSExact / 2
		}
	}

	arch := detectArchFromName(a.Name)
	switch {
	case arch != "" && arch == target.Arch:
		score += scoreArchExact
	case arch == archUniversal:
		score += scoreArchUniversal
	case arch == "" && (format == "deb" || format == "rpm" || format == "appimage"):
		// No arch token: small benefit only for single-arch typical packages is risky;
		// give nothing — multi-arch repos usually label arch.
	}

	libc := detectLibcFromName(a.Name)
	if libc != "" && target.Libc != "" && libc == target.Libc {
		score += scoreLibcExact
	}
	if libc == "musl" && target.Libc == "glibc" {
		score += penaltyMuslOnGlibc
	}

	// Distro-preferred package format
	switch target.DistroID {
	case "arch", "manjaro", "endeavouros", "garuda":
		// Arch users still prefer appimage/user-space over deb; deb gets less than appimage.
		// Design table: preferred format +90/+80 — for Arch, no native deb/rpm preference.
	case "ubuntu", "debian", "linuxmint", "pop":
		if format == "deb" {
			score += scoreDistroDeb
		}
	case "fedora", "rhel", "centos", "rocky", "almalinux", "opensuse", "suse":
		if format == "rpm" {
			score += scoreDistroRpm
		}
	default:
		// Unknown distro: mild deb preference on debian-family names in asset only
		if format == formatDeb && (strings.Contains(n, "debian") || strings.Contains(n, "ubuntu")) {
			score += scoreDistroDeb / 2
		}
	}

	switch format {
	case formatAppImage:
		score += scoreAppImage
	case formatTarball:
		score += scoreTarball
	case formatZip:
		score += scoreZip
	case formatDeb:
		switch target.DistroID {
		case "ubuntu", "debian", "linuxmint", "pop":
			// distro boost already applied above
		case "", "arch", "manjaro", "endeavouros", "garuda":
			score += 50
		}
	case formatRPM:
		if target.DistroID == "" {
			score += 40
		}
	}

	// PreferredFormats override/boost
	for _, pf := range target.PreferredFormats {
		if strings.EqualFold(pf, format) {
			score += 25
		}
	}

	if appName != "" && strings.Contains(n, strings.ToLower(appName)) {
		score += scoreAppName
	}

	if strings.Contains(n, "unsigned") || strings.Contains(n, "alpha") {
		score += penaltyUnsigned
	}
	if a.Size > 0 && a.Size < 1<<20 { // < 1 MiB
		score += penaltyTiny
	}

	return score
}

// ExplainExcluded returns a multi-line diagnostic for "nothing matched".
func ExplainExcluded(excluded []Candidate) string {
	if len(excluded) == 0 {
		return "no assets in release"
	}
	var b strings.Builder
	b.WriteString("no compatible assets; excluded:\n")
	for _, c := range excluded {
		fmt.Fprintf(&b, "  %s (%s)\n", c.Asset.Name, c.ExcludeReason)
	}
	b.WriteString("hint: pass --asset 'regex' to force a name match")
	return b.String()
}
