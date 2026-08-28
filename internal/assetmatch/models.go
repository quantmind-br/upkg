// Package assetmatch ranks GitHub release assets for the host platform.
package assetmatch

// Asset is a release artifact candidate (name + size + optional digest/URL).
type Asset struct {
	Name        string
	Size        int64
	Digest      string // "sha256:hex" when API provides it
	DownloadURL string
	ContentType string
}

// Target describes the machine we are selecting for.
type Target struct {
	OS   string // "linux", "darwin", "windows"
	Arch string // "amd64", "arm64", "386", ...
	Libc string // "glibc", "musl", or empty if unknown
	// DistroID from /etc/os-release ID= (arch, ubuntu, debian, ...). Optional.
	DistroID string
	// PreferredFormats, if non-empty, boosts matching format names
	// (e.g. "appimage", "deb", "rpm", "tarball").
	PreferredFormats []string
}

// Candidate is a scored asset after exclusion filtering.
type Candidate struct {
	Asset  Asset
	Score  int
	Format string // detected format hint from name: appimage|deb|rpm|tarball|zip|binary|darwin-native|windows-native|unknown
	// ExcludeReason is set only when the asset was hard-excluded (for diagnostics).
	ExcludeReason string
}

// PickOptions controls auto-selection vs menu.
type PickOptions struct {
	// Yes enables auto-install when a clear winner exists (score≥MinScore and lead≥MinLead).
	Yes bool
	// Format filters to a single format hint (appimage, deb, ...). Empty = any.
	Format string
	// AssetRegex, if non-empty, is a regexp matched against the asset name;
	// only matching assets remain after exclusion.
	AssetRegex string
	// AppName, if non-empty, boosts assets whose name contains it.
	AppName string
	// MinScore / MinLead override defaults (60 / 40) for auto-pick with Yes.
	MinScore int
	MinLead  int
}

// PickResult is the outcome of Pick.
type PickResult struct {
	// Winner is set when auto-selection applies (single candidate or Yes+clear lead).
	Winner *Candidate
	// Compatible are non-excluded candidates sorted by score desc, name asc.
	Compatible []Candidate
	// Excluded lists hard-excluded assets with reasons (debug / error messages).
	Excluded []Candidate
	// NeedsMenu is true when the caller must prompt (ambiguous or Yes not set).
	NeedsMenu bool
}
