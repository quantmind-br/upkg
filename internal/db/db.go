package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// DB represents the database with separate read/write pools
type DB struct {
	write *sql.DB
	read  *sql.DB
	path  string
}

// New creates a new database instance with separate read/write pools
func New(ctx context.Context, dbPath string) (*DB, error) {
	// Connection string with pragmas
	connStr := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dbPath)

	// Write pool: MUST be 1 connection only
	write, err := sql.Open("sqlite", connStr)
	if err != nil {
		return nil, fmt.Errorf("open write connection: %w", err)
	}
	write.SetMaxOpenConns(1)
	write.SetMaxIdleConns(1)
	write.SetConnMaxIdleTime(time.Minute)
	write.SetConnMaxLifetime(time.Hour)

	// Read pool: Can have multiple connections
	read, err := sql.Open("sqlite", connStr)
	if err != nil {
		write.Close()
		return nil, fmt.Errorf("open read connection: %w", err)
	}
	read.SetMaxOpenConns(10)
	read.SetMaxIdleConns(5)
	read.SetConnMaxIdleTime(time.Minute)
	read.SetConnMaxLifetime(time.Hour)

	db := &DB{
		write: write,
		read:  read,
		path:  dbPath,
	}

	// Initialize schema
	if err := db.initSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return db, nil
}

// Close closes both database connections
func (db *DB) Close() error {
	writeErr := db.write.Close()
	readErr := db.read.Close()
	if writeErr != nil {
		return writeErr
	}
	return readErr
}

// initSchema creates the schema if it doesn't exist
func (db *DB) initSchema(ctx context.Context) error {
	schema := `
CREATE TABLE IF NOT EXISTS installs (
    install_id TEXT PRIMARY KEY,
    package_type TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT,
    install_date DATETIME DEFAULT CURRENT_TIMESTAMP,
    original_file TEXT NOT NULL,
    install_path TEXT NOT NULL,
    desktop_file TEXT,
    metadata TEXT
);

CREATE INDEX IF NOT EXISTS idx_installs_name ON installs(name);
CREATE INDEX IF NOT EXISTS idx_installs_type ON installs(package_type);

CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    description TEXT
);
	`

	_, err := db.write.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	return nil
}

// Install holds install record methods
type Install struct {
	InstallID    string
	PackageType  string
	Name         string
	Version      string
	InstallDate  time.Time
	OriginalFile string
	InstallPath  string
	DesktopFile  string
	Metadata     map[string]interface{}
}

// Create creates a new install record
func (db *DB) Create(ctx context.Context, install *Install) error {
	metadataJSON, err := json.Marshal(install.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := `
INSERT INTO installs (install_id, package_type, name, version, install_date, original_file, install_path, desktop_file, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = db.write.ExecContext(ctx, query,
		install.InstallID,
		install.PackageType,
		install.Name,
		install.Version,
		install.InstallDate,
		install.OriginalFile,
		install.InstallPath,
		install.DesktopFile,
		string(metadataJSON),
	)

	if err != nil {
		return fmt.Errorf("insert install: %w", err)
	}

	return nil
}

// Get retrieves an install record by ID
func (db *DB) Get(ctx context.Context, installID string) (*Install, error) {
	query := `
SELECT install_id, package_type, name, version, install_date, original_file, install_path, desktop_file, metadata
FROM installs WHERE install_id = ?
	`

	var install Install
	var metadataJSON string

	err := db.read.QueryRowContext(ctx, query, installID).Scan(
		&install.InstallID,
		&install.PackageType,
		&install.Name,
		&install.Version,
		&install.InstallDate,
		&install.OriginalFile,
		&install.InstallPath,
		&install.DesktopFile,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("install not found: %s", installID)
	}
	if err != nil {
		return nil, fmt.Errorf("query install: %w", err)
	}

	if err := json.Unmarshal([]byte(metadataJSON), &install.Metadata); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}

	return &install, nil
}

// List retrieves all install records
func (db *DB) List(ctx context.Context) ([]Install, error) {
	query := `
SELECT install_id, package_type, name, version, install_date, original_file, install_path, desktop_file, metadata
FROM installs ORDER BY install_date DESC
	`

	rows, err := db.read.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query installs: %w", err)
	}
	defer rows.Close()

	var installs []Install
	for rows.Next() {
		var install Install
		var metadataJSON string

		err := rows.Scan(
			&install.InstallID,
			&install.PackageType,
			&install.Name,
			&install.Version,
			&install.InstallDate,
			&install.OriginalFile,
			&install.InstallPath,
			&install.DesktopFile,
			&metadataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("scan install: %w", err)
		}

		if err := json.Unmarshal([]byte(metadataJSON), &install.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}

		installs = append(installs, install)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return installs, nil
}

// Delete removes an install record
func (db *DB) Delete(ctx context.Context, installID string) error {
	query := "DELETE FROM installs WHERE install_id = ?"

	result, err := db.write.ExecContext(ctx, query, installID)
	if err != nil {
		return fmt.Errorf("delete install: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("install not found: %s", installID)
	}

	return nil
}
