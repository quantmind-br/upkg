package transaction

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog"
)

// RollbackFunc is a function that reverses an operation
type RollbackFunc func() error

// Manager manages a stack of rollback operations
type Manager struct {
	rollbacks []struct {
		name string
		fn   RollbackFunc
	}
	mu     sync.Mutex
	logger *zerolog.Logger
}

// NewManager creates a new transaction manager
func NewManager(logger *zerolog.Logger) *Manager {
	return &Manager{
		rollbacks: make([]struct {
			name string
			fn   RollbackFunc
		}, 0),
		logger: logger,
	}
}

// Add adds a rollback function to the stack
func (m *Manager) Add(name string, fn RollbackFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollbacks = append(m.rollbacks, struct {
		name string
		fn   RollbackFunc
	}{name, fn})
}

// Rollback executes all registered rollback functions in reverse order (LIFO)
func (m *Manager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()

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

// Commit clears the rollback stack, confirming the transaction
func (m *Manager) Commit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rollbacks = nil
}
