package db

import (
	"context"
	"testing"
	"time"
)

func TestDBOperations(t *testing.T) {
	// Create temporary database file for testing
	ctx := context.Background()
	tmpfile := t.TempDir() + "/test.db"
	db, err := New(ctx, tmpfile)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test Create
	install := &Install{
		InstallID:    "test-123",
		PackageType:  "appimage",
		Name:         "TestApp",
		Version:      "1.0.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/test.appimage",
		InstallPath:  "/opt/test",
		DesktopFile:  "/usr/share/applications/test.desktop",
		Metadata: map[string]interface{}{
			"key": "value",
		},
	}

	err = db.Create(ctx, install)
	if err != nil {
		t.Fatalf("Failed to create install: %v", err)
	}

	// Test Get
	gotInstall, err := db.Get(ctx, "test-123")
	if err != nil {
		t.Fatalf("Failed to get install: %v", err)
	}

	if gotInstall.InstallID != install.InstallID {
		t.Errorf("Get() InstallID = %v, want %v", gotInstall.InstallID, install.InstallID)
	}
	if gotInstall.Name != install.Name {
		t.Errorf("Get() Name = %v, want %v", gotInstall.Name, install.Name)
	}

	// Test List
	installs, err := db.List(ctx)
	if err != nil {
		t.Fatalf("Failed to list installs: %v", err)
	}

	if len(installs) != 1 {
		t.Errorf("List() length = %d, want 1", len(installs))
	}

	// Test Update
	install.Name = "UpdatedApp"
	err = db.Update(ctx, install)
	if err != nil {
		t.Fatalf("Failed to update install: %v", err)
	}

	updatedInstall, err := db.Get(ctx, "test-123")
	if err != nil {
		t.Fatalf("Failed to get updated install: %v", err)
	}

	if updatedInstall.Name != "UpdatedApp" {
		t.Errorf("Update() Name = %v, want UpdatedApp", updatedInstall.Name)
	}

	// Test Delete
	err = db.Delete(ctx, "test-123")
	if err != nil {
		t.Fatalf("Failed to delete install: %v", err)
	}

	// Verify deletion
	_, err = db.Get(ctx, "test-123")
	if err == nil {
		t.Error("Expected error after deletion, got nil")
	}
}

func TestApplyMigrations(t *testing.T) {
	ctx := context.Background()
	tmpfile := t.TempDir() + "/test_migrations.db"
	db, err := New(ctx, tmpfile)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Migrations should be applied automatically in New()
	// We can verify by checking if the schema_migrations table exists
	var count int
	err = db.read.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query schema_migrations: %v", err)
	}

	if count != 1 {
		t.Errorf("schema_migrations count = %d, want 1", count)
	}
}
