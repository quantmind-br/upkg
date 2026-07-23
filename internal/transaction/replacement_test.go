package transaction

import (
	"testing"

	"github.com/rs/zerolog"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renameErrorFs wraps an afero.Fs and forces Rename to fail.
type renameErrorFs struct {
	afero.Fs
}

func (fs *renameErrorFs) Rename(_, _ string) error {
	return assert.AnError
}

func TestBackupForReplace_MissingTargetIsNoop(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	m := NewManager(&logger)
	fs := afero.NewMemMapFs()

	err := m.BackupForReplace(fs, "/bin/absent")
	require.NoError(t, err)
	assert.Equal(t, 0, len(m.rollbacks))
	assert.Equal(t, 0, len(m.commits))
}

func TestBackupForReplace_RollbackRestoresOriginal(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	m := NewManager(&logger)
	fs := afero.NewMemMapFs()

	target := "/bin/app"
	require.NoError(t, afero.WriteFile(fs, target, []byte("old"), 0755))

	require.NoError(t, m.BackupForReplace(fs, target))

	// The original is renamed away to a backup; simulate writing a new file.
	require.NoError(t, afero.WriteFile(fs, target, []byte("new"), 0755))

	// Rollback must remove the new file and restore the old content.
	require.NoError(t, m.Rollback())

	data, err := afero.ReadFile(fs, target)
	require.NoError(t, err)
	assert.Equal(t, "old", string(data))

	// No stray backup files remain.
	assertNoBackups(t, fs, "/bin")
}

func TestBackupForReplace_CommitDropsBackup(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	m := NewManager(&logger)
	fs := afero.NewMemMapFs()

	target := "/bin/app"
	require.NoError(t, afero.WriteFile(fs, target, []byte("old"), 0755))

	require.NoError(t, m.BackupForReplace(fs, target))
	require.NoError(t, afero.WriteFile(fs, target, []byte("new"), 0755))

	// Commit runs the finalizer that drops the backup.
	require.NoError(t, m.Commit())

	data, err := afero.ReadFile(fs, target)
	require.NoError(t, err)
	assert.Equal(t, "new", string(data))
	assertNoBackups(t, fs, "/bin")
}

func TestBackupForReplace_RenameFailureLeavesSafeHooks(t *testing.T) {
	t.Parallel()
	logger := zerolog.Nop()
	m := NewManager(&logger)
	fs := &renameErrorFs{Fs: afero.NewMemMapFs()}

	target := "/bin/app"
	require.NoError(t, afero.WriteFile(fs, target, []byte("old"), 0755))

	err := m.BackupForReplace(fs, target)
	require.Error(t, err)

	// The original must be untouched (rename failed before moving it).
	data, rerr := afero.ReadFile(fs, target)
	require.NoError(t, rerr)
	assert.Equal(t, "old", string(data))

	// The registered hooks (added before the rename) must be safe no-ops: no
	// backup exists, so rollback neither errors nor clobbers the original.
	require.NoError(t, m.Rollback())
	data, rerr = afero.ReadFile(fs, target)
	require.NoError(t, rerr)
	assert.Equal(t, "old", string(data))
}

func assertNoBackups(t *testing.T, fs afero.Fs, dir string) {
	t.Helper()
	entries, err := afero.ReadDir(fs, dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), "upkg-backup-", "unexpected residual backup: %s", e.Name())
	}
}
