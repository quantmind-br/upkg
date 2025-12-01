// Package hyprland provides utilities for interacting with the Hyprland compositor.
package hyprland

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// Client represents a Hyprland window client.
type Client struct {
	Address      string `json:"address"`
	Class        string `json:"class"`
	Title        string `json:"title"`
	InitialClass string `json:"initialClass"`
	InitialTitle string `json:"initialTitle"`
	PID          int    `json:"pid"`
	Mapped       bool   `json:"mapped"`
}

// GetClients returns all current Hyprland clients.
func GetClients(ctx context.Context) ([]Client, error) {
	cmd := exec.CommandContext(ctx, "hyprctl", "clients", "-j")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get hyprctl clients: %w", err)
	}

	var clients []Client
	if err := json.Unmarshal(output, &clients); err != nil {
		return nil, fmt.Errorf("failed to parse hyprctl output: %w", err)
	}

	return clients, nil
}

// FindClientByPID finds a client by its process ID.
func FindClientByPID(clients []Client, pid int) *Client {
	for i := range clients {
		if clients[i].PID == pid && clients[i].Mapped {
			return &clients[i]
		}
	}
	return nil
}

// WaitForClient waits for a client with the given PID to appear.
// It polls every pollInterval until timeout is reached.
func WaitForClient(ctx context.Context, pid int, timeout, pollInterval time.Duration) (*Client, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		clients, err := GetClients(ctx)
		if err != nil {
			// Hyprctl might not be available, continue waiting
			time.Sleep(pollInterval)
			continue
		}

		if client := FindClientByPID(clients, pid); client != nil {
			return client, nil
		}

		time.Sleep(pollInterval)
	}

	return nil, fmt.Errorf("timeout waiting for client with PID %d", pid)
}

// IsHyprlandRunning checks if Hyprland compositor is running.
func IsHyprlandRunning() bool {
	cmd := exec.Command("hyprctl", "version")
	return cmd.Run() == nil
}
