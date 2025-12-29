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

func TestDBEdgeCases(t *testing.T) {
	ctx := context.Background()

	t.Run("Get non-existent install", func(t *testing.T) {
		tmpfile := t.TempDir() + "/test_edge.db"
		db, err := New(ctx, tmpfile)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		_, err = db.Get(ctx, "non-existent")
		if err == nil {
			t.Error("Expected error when getting non-existent install, got nil")
		}
	})

	t.Run("Delete non-existent install", func(t *testing.T) {
		tmpfile := t.TempDir() + "/test_edge2.db"
		db, err := New(ctx, tmpfile)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		// Deleting a non-existent install returns error
		err = db.Delete(ctx, "non-existent")
		if err == nil {
			t.Error("Expected error when deleting non-existent install, got nil")
		}
	})

	t.Run("Update non-existent install", func(t *testing.T) {
		tmpfile := t.TempDir() + "/test_edge3.db"
		db, err := New(ctx, tmpfile)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		install := &Install{
			InstallID:   "non-existent",
			PackageType: "appimage",
			Name:        "Test",
		}

		err = db.Update(ctx, install)
		if err == nil {
			t.Error("Expected error when updating non-existent install, got nil")
		}
	})

	t.Run("Create with duplicate install ID", func(t *testing.T) {
		tmpfile := t.TempDir() + "/test_edge4.db"
		db, err := New(ctx, tmpfile)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		install := &Install{
			InstallID:    "duplicate",
			PackageType:  "appimage",
			Name:         "Test",
			OriginalFile: "/tmp/test",
			InstallPath:  "/opt/test",
		}

		// First create should succeed
		err = db.Create(ctx, install)
		if err != nil {
			t.Fatalf("Failed to create install: %v", err)
		}

		// Second create with same ID should fail
		err = db.Create(ctx, install)
		if err == nil {
			t.Error("Expected error when creating duplicate install ID, got nil")
		}
	})

	t.Run("List with empty database", func(t *testing.T) {
		tmpfile := t.TempDir() + "/test_edge5.db"
		db, err := New(ctx, tmpfile)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		installs, err := db.List(ctx)
		if err != nil {
			t.Fatalf("Failed to list installs: %v", err)
		}

		if len(installs) != 0 {
			t.Errorf("List() length = %d, want 0", len(installs))
		}
	})

	t.Run("Create with nil metadata", func(t *testing.T) {
		tmpfile := t.TempDir() + "/test_edge6.db"
		db, err := New(ctx, tmpfile)
		if err != nil {
			t.Fatalf("Failed to create database: %v", err)
		}
		defer db.Close()

		install := &Install{
			InstallID:    "nil-meta",
			PackageType:  "appimage",
			Name:         "Test",
			OriginalFile: "/tmp/test",
			InstallPath:  "/opt/test",
			Metadata:     nil,
		}

		err = db.Create(ctx, install)
		if err != nil {
			t.Fatalf("Failed to create install with nil metadata: %v", err)
		}

		got, err := db.Get(ctx, "nil-meta")
		if err != nil {
			t.Fatalf("Failed to get install: %v", err)
		}

		// Nil metadata is stored as null and comes back as empty map
		if got.Metadata == nil {
			// This is acceptable behavior
			t.Log("Metadata is nil after retrieval (acceptable)")
		}
	})
}

func TestDBCloseIdempotent(t *testing.T) {
	ctx := context.Background()
	tmpfile := t.TempDir() + "/test_close.db"
	db, err := New(ctx, tmpfile)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// First close
	err = db.Close()
	if err != nil {
		t.Errorf("First Close() failed: %v", err)
	}

	// Second close should be safe
	err = db.Close()
	if err != nil {
		t.Errorf("Second Close() failed: %v", err)
	}
}

func TestDBNewInvalidPath(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid directory path", func(t *testing.T) {
		// Use a path that cannot be created
		db, err := New(ctx, "/root/nonexistent/invalid.db")
		if err == nil {
			db.Close()
			t.Error("Expected error when creating DB in invalid path, got nil")
		}
	})
}
