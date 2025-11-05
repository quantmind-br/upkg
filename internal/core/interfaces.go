package core

import (
	"context"
	"io"
)

// Installer defines the interface for package installers
type Installer interface {
	// Detect checks if the installer can handle the given package
	Detect(ctx context.Context, packagePath string) (bool, error)

	// Install installs the package
	Install(ctx context.Context, packagePath string, opts InstallOptions) (*InstallRecord, error)

	// Uninstall removes the installed package
	Uninstall(ctx context.Context, installID string) error
}

// InstallOptions contains options for package installation
type InstallOptions struct {
	Force           bool     // Force installation even if already installed
	SkipDesktop     bool     // Skip desktop integration
	CustomName      string   // Custom application name
	WaylandEnvVars  []string // Additional Wayland environment variables
	SkipWaylandEnv  bool     // Skip Wayland environment variable injection
}

// DatabaseStore defines the interface for database operations
type DatabaseStore interface {
	// Create creates a new install record
	Create(ctx context.Context, record *InstallRecord) error

	// Get retrieves an install record by ID
	Get(ctx context.Context, installID string) (*InstallRecord, error)

	// List retrieves all install records
	List(ctx context.Context) ([]InstallRecord, error)

	// Update updates an existing install record
	Update(ctx context.Context, record *InstallRecord) error

	// Delete removes an install record
	Delete(ctx context.Context, installID string) error

	// FindByName finds install records by package name
	FindByName(ctx context.Context, name string) ([]InstallRecord, error)

	// Close closes the database connection
	Close() error
}

// Logger defines the interface for logging operations
type Logger interface {
	Debug() LogEvent
	Info() LogEvent
	Warn() LogEvent
	Error() LogEvent
}

// LogEvent defines the interface for building log events
type LogEvent interface {
	Str(key, val string) LogEvent
	Int(key string, val int) LogEvent
	Bool(key string, val bool) LogEvent
	Err(error) LogEvent
	Msg(string)
}

// FileSystem defines the interface for filesystem operations
type FileSystem interface {
	// Create creates or truncates a file
	Create(name string) (io.WriteCloser, error)

	// Open opens a file for reading
	Open(name string) (io.ReadCloser, error)

	// Stat returns file info
	Stat(name string) (FileInfo, error)

	// MkdirAll creates a directory and all parents
	MkdirAll(path string, perm uint32) error

	// Remove removes a file or directory
	Remove(name string) error

	// RemoveAll removes a path and any children
	RemoveAll(path string) error

	// Exists checks if a path exists
	Exists(path string) bool

	// IsDir checks if a path is a directory
	IsDir(path string) bool
}

// FileInfo represents file metadata
type FileInfo interface {
	Name() string
	Size() int64
	IsDir() bool
}
