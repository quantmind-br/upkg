package core

import (
	"encoding/json"
	"time"
)

// PackageType represents the type of package being installed
type PackageType string

const (
	PackageTypeDeb      PackageType = "deb"
	PackageTypeRpm      PackageType = "rpm"
	PackageTypeAppImage PackageType = "appimage"
	PackageTypeTarball  PackageType = "tarball"
	PackageTypeZip      PackageType = "zip"
	PackageTypeBinary   PackageType = "binary"
	PackageTypeFlatpak  PackageType = "flatpak"
)

// InstallRecord represents a package installation in the database
type InstallRecord struct {
	InstallID    string      `json:"install_id"`
	PackageType  PackageType `json:"package_type"`
	Name         string      `json:"name"`
	Version      string      `json:"version,omitempty"`
	InstallDate  time.Time   `json:"install_date"`
	OriginalFile string      `json:"original_file"`
	InstallPath  string      `json:"install_path"`
	DesktopFile  string      `json:"desktop_file,omitempty"`
	Metadata     Metadata    `json:"metadata"`
}

// GetDesktopFiles returns all desktop files for this install.
// Falls back to the singular DesktopFile for backward compatibility.
func (r *InstallRecord) GetDesktopFiles() []string {
	if len(r.Metadata.DesktopFiles) > 0 {
		return r.Metadata.DesktopFiles
	}
	if r.DesktopFile != "" {
		return []string{r.DesktopFile}
	}
	return nil
}

// Metadata contains additional package-specific metadata
type Metadata struct {
	IconFiles           []string          `json:"icon_files,omitempty"`
	WrapperScript       string            `json:"wrapper_script,omitempty"`
	WaylandSupport      string            `json:"wayland_support,omitempty"`
	InstallMethod       string            `json:"install_method,omitempty"`
	ExtractedMeta       ExtractedMetadata `json:"extracted_metadata,omitempty"`
	OriginalDesktopFile string            `json:"original_desktop_file,omitempty"` // Original .desktop path before rename for dock compatibility
	DesktopFiles        []string          `json:"desktop_files,omitempty"`
	// Source records remote origin (e.g. GitHub) for upkg update. Optional;
	// legacy rows without this field continue to load.
	Source *SourceRef `json:"source,omitempty"`
}

// SourceRef pins a remote package origin inside Metadata JSON (no schema migration).
type SourceRef struct {
	Kind        string    `json:"kind"` // "github"
	Owner       string    `json:"owner"`
	Repo        string    `json:"repo"`
	RepoID      int64     `json:"repo_id"` // immutable anti-takeover pin
	Tag         string    `json:"tag"`     // raw tag e.g. desktop-v0.5.5
	Version     string    `json:"version"` // normalized display version
	AssetName   string    `json:"asset_name"`
	AssetDigest string    `json:"asset_digest,omitempty"` // sha256:…
	DownloadURL string    `json:"download_url"`
	PublishedAt time.Time `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	ETag        string    `json:"etag,omitempty"`
	CompareMode string    `json:"compare_mode"` // semver|date|natural|published
	Hold        bool      `json:"hold,omitempty"`
	// AssetPattern is an optional regex the user chose (--asset) reused on update.
	AssetPattern string `json:"asset_pattern,omitempty"`
	// ComponentPrefix pins monorepo tag prefixes (e.g. "desktop" from desktop-v0.5.5).
	ComponentPrefix string `json:"component_prefix,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling to handle legacy formats
func (m *Metadata) UnmarshalJSON(data []byte) error {
	type Alias Metadata
	aux := &struct {
		IconFiles    interface{} `json:"icon_files,omitempty"`
		DesktopFiles interface{} `json:"desktop_files,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(m),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	m.IconFiles = convertToStringSlice(aux.IconFiles)
	m.DesktopFiles = convertToStringSlice(aux.DesktopFiles)

	return nil
}

func convertToStringSlice(val interface{}) []string {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case []string:
		return v
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	default:
		return nil
	}
}

// Install method constants
const (
	InstallMethodLocal  = "local"
	InstallMethodPacman = "pacman"
)

// ExtractedMetadata contains metadata extracted from the package
type ExtractedMetadata struct {
	Categories     []string `json:"categories,omitempty"`
	Comment        string   `json:"comment,omitempty"`
	StartupWMClass string   `json:"startup_wm_class,omitempty"`
}

// DesktopEntry represents a .desktop file
type DesktopEntry struct {
	Type           string   `ini:"Type"`
	Version        string   `ini:"Version,omitempty"`
	Name           string   `ini:"Name"`
	GenericName    string   `ini:"GenericName,omitempty"`
	Comment        string   `ini:"Comment,omitempty"`
	Icon           string   `ini:"Icon,omitempty"`
	Exec           string   `ini:"Exec"`
	Path           string   `ini:"Path,omitempty"`
	Terminal       bool     `ini:"Terminal,omitempty"`
	Categories     []string `ini:"Categories,omitempty"`
	MimeType       []string `ini:"MimeType,omitempty"`
	StartupWMClass string   `ini:"StartupWMClass,omitempty"`
	NoDisplay      bool     `ini:"NoDisplay,omitempty"`
	Keywords       []string `ini:"Keywords,omitempty"`
	StartupNotify  bool     `ini:"StartupNotify,omitempty"`
}

// IconFile represents an icon discovered during installation
type IconFile struct {
	Path string // Absolute path to icon file
	Size string // "256x256", "scalable", etc.
	Ext  string // "png", "svg", "ico", "xpm"
}

// Exit codes (align with CLI.md section 12)
const (
	ExitSuccess         = 0
	ExitGeneral         = 1
	ExitInvalidArgs     = 2
	ExitInstallFailed   = 3
	ExitUninstallFailed = 4
	ExitDatabase        = 5
	ExitPermission      = 6
	ExitNetwork         = 7
	ExitCommandNotFound = 8
	ExitInterrupted     = 130
)

// WaylandSupport indicates Wayland compatibility level
type WaylandSupport string

const (
	WaylandUnknown  WaylandSupport = "unknown"
	WaylandNative   WaylandSupport = "native"
	WaylandXWayland WaylandSupport = "xwayland"
	WaylandHybrid   WaylandSupport = "hybrid"
)
