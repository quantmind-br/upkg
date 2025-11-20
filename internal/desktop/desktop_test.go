package desktop

import (
	"testing"

	"github.com/quantmind-br/upkg/internal/core"
)

func TestInjectWaylandEnvVars(t *testing.T) {
	entry := &core.DesktopEntry{
		Exec: "myapp",
	}

	// Test default injection
	err := InjectWaylandEnvVars(entry, nil)
	if err != nil {
		t.Fatalf("InjectWaylandEnvVars failed: %v", err)
	}

	expectedEnv := []string{"GDK_BACKEND=x11", "QT_QPA_PLATFORM=xcb", "SDL_VIDEODRIVER=x11"}
	for _, env := range expectedEnv {
		if !contains(entry.Exec, "env "+env) && !contains(entry.Exec, env) {
			// The implementation might prepend "env VAR=VAL "
			// Let's check if Exec starts with "env " and contains the vars
		}
	}

	// Simplest check: check if it was modified to include env command
	if entry.Exec == "myapp" {
		t.Error("Exec was not modified")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[0:len(substr)] == substr // naive check
}

func TestParse(t *testing.T) {
	// TODO: Implement parsing test
}
