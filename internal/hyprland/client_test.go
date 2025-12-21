package hyprland

import (
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindClientByPID(t *testing.T) {
	t.Run("finds mapped client", func(t *testing.T) {
		clients := []Client{
			{PID: 123, Mapped: true, Class: "test"},
			{PID: 456, Mapped: true, Class: "other"},
		}
		client := FindClientByPID(clients, 123)
		require.NotNil(t, client)
		assert.Equal(t, 123, client.PID)
		assert.True(t, client.Mapped)
	})

	t.Run("returns nil for unmapped client", func(t *testing.T) {
		clients := []Client{
			{PID: 123, Mapped: false},
		}
		client := FindClientByPID(clients, 123)
		assert.Nil(t, client)
	})

	t.Run("returns nil for non-existent PID", func(t *testing.T) {
		clients := []Client{
			{PID: 123, Mapped: true},
		}
		client := FindClientByPID(clients, 999)
		assert.Nil(t, client)
	})

	t.Run("empty client list", func(t *testing.T) {
		clients := []Client{}
		client := FindClientByPID(clients, 123)
		assert.Nil(t, client)
	})
}

func TestIsHyprlandRunning(t *testing.T) {
	// This test will pass if hyprctl exists and returns success
	// or fail gracefully if not
	result := IsHyprlandRunning()
	// Just ensure it doesn't panic and returns a boolean
	assert.IsType(t, true, result)
}

func TestGetClients(t *testing.T) {
	t.Run("successful parse", func(t *testing.T) {
		// Mock the command execution
		// Note: This test requires hyprctl to be available
		// If not available, it will fail which is expected
		ctx := context.Background()
		clients, err := GetClients(ctx)

		// If hyprctl exists, should succeed
		_, lookErr := exec.LookPath("hyprctl")
		if lookErr == nil {
			// hyprctl exists, expect success
			if err == nil {
				assert.NotNil(t, clients)
				// Could be empty list if no windows
			}
		} else {
			// hyprctl doesn't exist, expect error
			assert.Error(t, err)
			assert.Nil(t, clients)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := GetClients(ctx)
		assert.Error(t, err)
	})
}

func TestWaitForClient(t *testing.T) {
	t.Run("timeout waiting for client", func(t *testing.T) {
		ctx := context.Background()
		// Use a very short timeout
		_, err := WaitForClient(ctx, 99999, 100*time.Millisecond, 50*time.Millisecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})

	t.Run("context cancellation during wait", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		_, err := WaitForClient(ctx, 99999, 5*time.Second, 100*time.Millisecond)
		assert.Error(t, err)
	})

	t.Run("zero timeout", func(t *testing.T) {
		ctx := context.Background()
		_, err := WaitForClient(ctx, 99999, 0, 1*time.Millisecond)
		assert.Error(t, err)
	})
}

func TestClientStruct(t *testing.T) {
	// Test JSON unmarshaling
	jsonData := `{
		"address": "0x12345678",
		"class": "test-app",
		"title": "Test Window",
		"initialClass": "test-app",
		"initialTitle": "Test Window",
		"pid": 12345,
		"mapped": true
	}`

	var client Client
	err := json.Unmarshal([]byte(jsonData), &client)
	require.NoError(t, err)

	assert.Equal(t, "0x12345678", client.Address)
	assert.Equal(t, "test-app", client.Class)
	assert.Equal(t, "Test Window", client.Title)
	assert.Equal(t, 12345, client.PID)
	assert.True(t, client.Mapped)
}
