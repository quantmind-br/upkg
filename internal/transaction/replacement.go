package transaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/spf13/afero"
)

// pathExists reports whether path exists on the given filesystem.
func pathExists(fs afero.Fs, path string) (bool, error) {
	_, err := fs.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("failed to stat %q: %w", path, err)
}

// removeIfExists removes path (recursively when requested), treating a missing
// path as success. Kept private to avoid a transaction -> helpers -> transaction
// import cycle.
func removeIfExists(fs afero.Fs, path string, recursive bool) error {
	if path == "" {
		return nil
	}
	var err error
	if recursive {
		err = fs.RemoveAll(path)
	} else {
		err = fs.Remove(path)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove %q: %w", path, err)
	}
	return nil
}

// BackupForReplace renames an existing target to a sibling backup path before it
// is replaced, registering both a rollback (restore the backup on failure) and a
// commit finalizer (drop the backup on success) BEFORE performing the rename.
//
// If target does not exist, it is a no-op. The hooks are idempotent: they probe
// the backup's existence at execution time, so a failed rename leaves only safe
// no-op hooks behind. There is no copy-based fallback: a rename failure aborts.
func (m *Manager) BackupForReplace(fs afero.Fs, target string) error {
	exists, err := pathExists(fs, target)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	dir := filepath.Dir(target)
	base := filepath.Base(target)
	backup := filepath.Join(dir, fmt.Sprintf(".%s.upkg-backup-%s", base, uuid.NewString()))

	// Register hooks BEFORE the mutation. Both consult live backup existence so
	// they stay safe no-ops if the rename below never happens.
	m.Add("restore replaced target "+target, func() error {
		present, perr := pathExists(fs, backup)
		if perr != nil {
			return perr
		}
		if !present {
			return nil
		}
		// Clear whatever replacement was written, then restore the original.
		if rerr := removeIfExists(fs, target, true); rerr != nil {
			return rerr
		}
		if rerr := fs.Rename(backup, target); rerr != nil {
			return fmt.Errorf("failed to restore backup %q -> %q: %w", backup, target, rerr)
		}
		return nil
	})
	m.AddCommit("remove replacement backup "+backup, func() error {
		return removeIfExists(fs, backup, true)
	})

	if err := fs.Rename(target, backup); err != nil {
		return fmt.Errorf("failed to back up %q for replacement: %w", target, err)
	}
	return nil
}
