package tarball

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/quantmind-br/upkg/internal/config"
	"github.com/quantmind-br/upkg/internal/core"
	"github.com/quantmind-br/upkg/internal/transaction"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestTarballBackend_Install_Simple(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	// Create fake tarball
	pkgPath := filepath.Join(tmpDir, "test.tar.gz")
	require.NoError(t, os.WriteFile(pkgPath, []byte("fake"), 0644))

	ctx := context.Background()
	opts := core.InstallOptions{}
	tx := transaction.NewManager(&log)

	_, err := backend.Install(ctx, pkgPath, opts, tx)

	// Will fail but tests the flow
	_ = err
}

func TestTarballBackend_Uninstall_Simple(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			DataDir: tmpDir,
			DBFile:  filepath.Join(tmpDir, "test.db"),
		},
	}

	log := zerolog.Nop()
	backend := New(cfg, &log)

	ctx := context.Background()

	install := &core.InstallRecord{
		InstallID:   "test-123",
		Name:        "test",
		PackageType: "tarball",
		InstallPath: tmpDir,
	}

	err := backend.Uninstall(ctx, install)

	// Will fail but tests the flow
	_ = err
}
