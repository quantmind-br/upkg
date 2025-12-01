package core

import "time"

// PackageType represents the type of package being installed
type PackageType string

const (
	PackageTypeDeb      PackageType = "deb"
	PackageTypeRpm      PackageType = "rpm"
	PackageTypeAppImage PackageType = "appimage"
	PackageTypeTarball  PackageType = "tarball"
	PackageTypeZip      PackageType = "zip"
	PackageTypeBinary   PackageType = "binary"
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

// Metadata contains additional package-specific metadata
type Metadata struct {
	IconFiles           []string          `json:"icon_files,omitempty"`
	WrapperScript       string            `json:"wrapper_script,omitempty"`
	WaylandSupport      string            `json:"wayland_support,omitempty"`
	ExtractedMeta       ExtractedMetadata `json:"extracted_metadata,omitempty"`
	OriginalDesktopFile string            `json:"original_desktop_file,omitempty"` // Original .desktop path before rename for dock compatibility
}

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
