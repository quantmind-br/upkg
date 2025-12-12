package transaction

import (
	"errors"
	"fmt"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestNewManager(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)
	assert.NotNil(t, manager)
	assert.NotNil(t, manager.rollbacks)
	assert.Equal(t, 0, len(manager.rollbacks))
}

func TestAdd(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)

	rollbackFunc := func() error {
		return nil
	}

	manager.Add("test-operation", rollbackFunc)
	assert.Equal(t, 1, len(manager.rollbacks))
}

func TestCommit(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)

	rollbackFunc := func() error {
		return nil
	}

	manager.Add("test-operation", rollbackFunc)
	assert.Equal(t, 1, len(manager.rollbacks))

	manager.Commit()
	assert.Equal(t, 0, len(manager.rollbacks))
}

func TestRollback(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)

	var operations []string
	rollbackFunc1 := func() error {
		operations = append(operations, "op1")
		return nil
	}
	rollbackFunc2 := func() error {
		operations = append(operations, "op2")
		return nil
	}

	manager.Add("op1", rollbackFunc1)
	manager.Add("op2", rollbackFunc2)

	err := manager.Rollback()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(manager.rollbacks))
	assert.Equal(t, []string{"op2", "op1"}, operations)
}

func TestRollbackWithErrors(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)

	expectedErr := errors.New("rollback error")
	rollbackFunc1 := func() error {
		return nil
	}
	rollbackFunc2 := func() error {
		return expectedErr
	}

	manager.Add("op1", rollbackFunc1)
	manager.Add("op2", rollbackFunc2)

	err := manager.Rollback()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rollback completed with errors")
	assert.Equal(t, 0, len(manager.rollbacks))
}

func TestRollbackEmpty(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)

	err := manager.Rollback()
	assert.NoError(t, err)
	assert.Equal(t, 0, len(manager.rollbacks))
}

func TestConcurrentOperations(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)

	// Test concurrent Add operations
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			manager.Add(fmt.Sprintf("op-%d", i), func() error {
				return nil
			})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			manager.Add(fmt.Sprintf("op-%d", i+100), func() error {
				return nil
			})
		}
		done <- true
	}()

	<-done
	<-done

	assert.Equal(t, 200, len(manager.rollbacks))

	// Test concurrent Rollback
	go func() {
		manager.Rollback()
		done <- true
	}()

	<-done
	assert.Equal(t, 0, len(manager.rollbacks))
}

func TestRollbackOrder(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)

	var executionOrder []string
	rollbackFunc1 := func() error {
		executionOrder = append(executionOrder, "op1")
		return nil
	}
	rollbackFunc2 := func() error {
		executionOrder = append(executionOrder, "op2")
		return nil
	}
	rollbackFunc3 := func() error {
		executionOrder = append(executionOrder, "op3")
		return nil
	}

	manager.Add("op1", rollbackFunc1)
	manager.Add("op2", rollbackFunc2)
	manager.Add("op3", rollbackFunc3)

	err := manager.Rollback()
	assert.NoError(t, err)
	assert.Equal(t, []string{"op3", "op2", "op1"}, executionOrder)
}

func TestRollbackWithMultipleErrors(t *testing.T) {
	logger := zerolog.Nop()
	manager := NewManager(&logger)

	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	rollbackFunc1 := func() error {
		return err1
	}
	rollbackFunc2 := func() error {
		return err2
	}
	rollbackFunc3 := func() error {
		return nil
	}

	manager.Add("op1", rollbackFunc1)
	manager.Add("op2", rollbackFunc2)
	manager.Add("op3", rollbackFunc3)

	err := manager.Rollback()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rollback completed with errors")
	assert.Equal(t, 0, len(manager.rollbacks))
}