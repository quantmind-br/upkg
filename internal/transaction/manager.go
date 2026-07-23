package transaction

import (
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// RollbackFunc is a function that reverses an operation
type RollbackFunc func() error

// CommitFunc is a function that finalizes an operation once the transaction
// commits (e.g. removing a replacement backup that is no longer needed).
type CommitFunc func() error

type namedFunc[T ~func() error] struct {
	name string
	fn   T
}

// Manager manages a stack of rollback operations and a stack of commit
// finalizers. Rollbacks reverse mutations on failure; commit finalizers run
// only on a successful Commit to clean up resources kept alive for rollback.
type Manager struct {
	rollbacks []namedFunc[RollbackFunc]
	commits   []namedFunc[CommitFunc]
	mu        sync.Mutex
	logger    *zerolog.Logger
}

// NewManager creates a new transaction manager
func NewManager(logger *zerolog.Logger) *Manager {
	return &Manager{
		rollbacks: make([]namedFunc[RollbackFunc], 0),
		commits:   make([]namedFunc[CommitFunc], 0),
		logger:    logger,
	}
}

// Add adds a rollback function to the stack
func (m *Manager) Add(name string, fn RollbackFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollbacks = append(m.rollbacks, namedFunc[RollbackFunc]{name, fn})
}

// AddCommit adds a commit finalizer to the stack. Finalizers run in LIFO order
// during Commit and are discarded (never executed) during Rollback.
func (m *Manager) AddCommit(name string, fn CommitFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.commits = append(m.commits, namedFunc[CommitFunc]{name, fn})
}

// Rollback executes all registered rollback functions in reverse order (LIFO),
// discards any commit finalizers without executing them, and clears both stacks.
func (m *Manager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Commit finalizers are only meaningful on success; drop them.
	m.commits = nil

	if len(m.rollbacks) == 0 {
		return nil
	}

	if m.logger != nil {
		m.logger.Info().Msg("Rolling back transaction...")
	}

	var errs []error
	// Execute in reverse order (LIFO)
	for i := len(m.rollbacks) - 1; i >= 0; i-- {
		op := m.rollbacks[i]
		if m.logger != nil {
			m.logger.Debug().Str("operation", op.name).Msg("rolling back")
		}

		if err := op.fn(); err != nil {
			errMsg := fmt.Errorf("failed to rollback '%s': %w", op.name, err)
			errs = append(errs, errMsg)
			if m.logger != nil {
				m.logger.Error().Err(err).Str("operation", op.name).Msg("rollback failed")
			}
		}
	}

	// Clear after rollback
	m.rollbacks = nil

	if len(errs) > 0 {
		return fmt.Errorf("rollback completed with errors: %v", errs)
	}
	return nil
}

// Commit runs every commit finalizer in reverse order (LIFO), aggregates any
// failures, and clears both stacks even when a finalizer fails. Once Commit
// returns, the transaction can no longer be rolled back. A finalizer failure
// is reported to the caller but never undoes an already-persisted operation.
func (m *Manager) Commit() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// The transaction is confirmed; rollbacks are no longer eligible.
	m.rollbacks = nil

	if len(m.commits) == 0 {
		return nil
	}

	var errs []error
	for i := len(m.commits) - 1; i >= 0; i-- {
		op := m.commits[i]
		if m.logger != nil {
			m.logger.Debug().Str("operation", op.name).Msg("finalizing commit")
		}

		if err := op.fn(); err != nil {
			errs = append(errs, fmt.Errorf("failed to finalize '%s': %w", op.name, err))
			if m.logger != nil {
				m.logger.Error().Err(err).Str("operation", op.name).Msg("commit finalizer failed")
			}
		}
	}

	m.commits = nil

	if len(errs) > 0 {
		return fmt.Errorf("commit completed with errors: %w", errors.Join(errs...))
	}
	return nil
}
