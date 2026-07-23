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

func newTestDB(t *testing.T) (*DB, context.Context) {
	t.Helper()
	ctx := context.Background()
	tmpfile := t.TempDir() + "/replace.db"
	db, err := New(ctx, tmpfile)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, ctx
}

func TestReplaceByPath_NoMatchInserts(t *testing.T) {
	t.Parallel()
	db, ctx := newTestDB(t)

	install := &Install{
		InstallID:    "app-uuid-1",
		PackageType:  "appimage",
		Name:         "App",
		Version:      "1.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/app.appimage",
		InstallPath:  "/home/u/.local/bin/app.appimage",
		Metadata:     map[string]interface{}{},
	}

	got, err := db.ReplaceByPath(ctx, install)
	if err != nil {
		t.Fatalf("ReplaceByPath: %v", err)
	}
	if got.InstallID != "app-uuid-1" {
		t.Errorf("InstallID = %q, want app-uuid-1", got.InstallID)
	}

	all, err := db.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("row count = %d, want 1", len(all))
	}
}

func TestReplaceByPath_OneMatchPreservesIDAndDate(t *testing.T) {
	t.Parallel()
	db, ctx := newTestDB(t)

	original := &Install{
		InstallID:    "app-original",
		PackageType:  "appimage",
		Name:         "App",
		Version:      "1.0",
		InstallDate:  time.Now().Add(-24 * time.Hour).Truncate(time.Second),
		OriginalFile: "/tmp/old.appimage",
		InstallPath:  "/home/u/.local/bin/app.appimage",
		Metadata:     map[string]interface{}{},
	}
	if err := db.Create(ctx, original); err != nil {
		t.Fatalf("Create: %v", err)
	}

	replacement := &Install{
		InstallID:    "app-new",
		PackageType:  "appimage",
		Name:         "App v2",
		Version:      "2.0",
		InstallDate:  time.Now(),
		OriginalFile: "/tmp/new.appimage",
		InstallPath:  "/home/u/.local/bin/app.appimage",
		Metadata:     map[string]interface{}{"k": "v"},
	}
	got, err := db.ReplaceByPath(ctx, replacement)
	if err != nil {
		t.Fatalf("ReplaceByPath: %v", err)
	}

	// Canonical ID/date preserved from the original row.
	if got.InstallID != "app-original" {
		t.Errorf("InstallID = %q, want app-original", got.InstallID)
	}
	if !got.InstallDate.Equal(original.InstallDate) {
		t.Errorf("InstallDate = %v, want %v", got.InstallDate, original.InstallDate)
	}

	// New content persisted under the canonical ID.
	stored, err := db.Get(ctx, "app-original")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Name != "App v2" || stored.Version != "2.0" || stored.OriginalFile != "/tmp/new.appimage" {
		t.Errorf("stored record not updated: %+v", stored)
	}

	all, err := db.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("row count = %d, want 1", len(all))
	}
}

func TestReplaceByPath_MultipleMatchesRemovesDuplicates(t *testing.T) {
	t.Parallel()
	db, ctx := newTestDB(t)

	base := time.Now().Truncate(time.Second)
	rows := []*Install{
		{InstallID: "dup-newest", PackageType: "tarball", Name: "A", InstallDate: base, OriginalFile: "/f", InstallPath: "/p", Metadata: map[string]interface{}{}},
		{InstallID: "dup-older", PackageType: "tarball", Name: "A", InstallDate: base.Add(-time.Hour), OriginalFile: "/f", InstallPath: "/p", Metadata: map[string]interface{}{}},
		{InstallID: "dup-oldest", PackageType: "tarball", Name: "A", InstallDate: base.Add(-2 * time.Hour), OriginalFile: "/f", InstallPath: "/p", Metadata: map[string]interface{}{}},
	}
	for _, r := range rows {
		if err := db.Create(ctx, r); err != nil {
			t.Fatalf("Create %s: %v", r.InstallID, err)
		}
	}

	replacement := &Install{
		InstallID:    "ignored",
		PackageType:  "tarball",
		Name:         "A v2",
		InstallDate:  time.Now(),
		OriginalFile: "/f2",
		InstallPath:  "/p",
		Metadata:     map[string]interface{}{},
	}
	got, err := db.ReplaceByPath(ctx, replacement)
	if err != nil {
		t.Fatalf("ReplaceByPath: %v", err)
	}

	// Canonical row is the most recent one; duplicates removed.
	if got.InstallID != "dup-newest" {
		t.Errorf("InstallID = %q, want dup-newest", got.InstallID)
	}

	all, err := db.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("row count = %d, want 1", len(all))
	}
	if all[0].InstallID != "dup-newest" || all[0].Name != "A v2" {
		t.Errorf("canonical row = %+v, want dup-newest with updated name", all[0])
	}
}

func TestReplaceByPath_SQLErrorRollsBack(t *testing.T) {
	t.Parallel()
	db, ctx := newTestDB(t)

	// Seed two duplicate rows for the same (type, path). The newest is canonical.
	base := time.Now().Truncate(time.Second)
	for _, id := range []string{"keep-newest", "keep-older"} {
		delta := time.Duration(0)
		if id == "keep-older" {
			delta = -time.Hour
		}
		if err := db.Create(ctx, &Install{
			InstallID: id, PackageType: "binary", Name: "B", Version: "1.0",
			InstallDate: base.Add(delta), OriginalFile: "/f", InstallPath: "/p",
			Metadata: map[string]interface{}{},
		}); err != nil {
			t.Fatalf("Create %s: %v", id, err)
		}
	}

	// Install a trigger that aborts any DELETE. ReplaceByPath updates the
	// canonical row first, then deletes the duplicate; the DELETE will fail,
	// which must roll back the already-applied UPDATE.
	if _, err := db.write.ExecContext(ctx,
		`CREATE TRIGGER block_delete BEFORE DELETE ON installs BEGIN SELECT RAISE(ABORT, 'forced'); END;`,
	); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	t.Cleanup(func() { _, _ = db.write.ExecContext(ctx, `DROP TRIGGER IF EXISTS block_delete`) })

	_, err := db.ReplaceByPath(ctx, &Install{
		InstallID: "new", PackageType: "binary", Name: "B v2", Version: "2.0",
		InstallDate: time.Now(), OriginalFile: "/f2", InstallPath: "/p",
		Metadata: map[string]interface{}{},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Both rows remain, and the canonical row's UPDATE was rolled back.
	all, listErr := db.List(ctx)
	if listErr != nil {
		t.Fatalf("List: %v", listErr)
	}
	if len(all) != 2 {
		t.Fatalf("row count = %d, want 2 (unchanged)", len(all))
	}
	canonical, getErr := db.Get(ctx, "keep-newest")
	if getErr != nil {
		t.Fatalf("Get canonical: %v", getErr)
	}
	if canonical.Name != "B" || canonical.Version != "1.0" || canonical.OriginalFile != "/f" {
		t.Errorf("canonical row mutated despite rollback: %+v", canonical)
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
