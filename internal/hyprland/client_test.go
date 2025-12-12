package hyprland

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFindClientByPID(t *testing.T) {
	clients := []Client{
		{Address: "0x123", Class: "Alacritty", Title: "Terminal", InitialClass: "Alacritty", InitialTitle: "Terminal", PID: 1234, Mapped: true},
		{Address: "0x456", Class: "Firefox", Title: "Browser", InitialClass: "Firefox", InitialTitle: "Browser", PID: 5678, Mapped: false},
	}

	// Test finding an existing client
	client := FindClientByPID(clients, 1234)
	assert.NotNil(t, client, "Client should be found")
	assert.Equal(t, "Alacritty", client.Class, "Client class should match")

	// Test finding a non-existing client
	client = FindClientByPID(clients, 9999)
	assert.Nil(t, client, "Client should not be found")
}
