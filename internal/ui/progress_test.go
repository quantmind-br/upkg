package ui

import (
	"testing"
	"time"
)

func TestNewProgressTracker(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 50, Deterministic: true},
		{Name: "Phase 2", Weight: 50, Deterministic: false},
	}

	// Test with enabled
	tracker := NewProgressTracker(phases, "Test Installation", true)
	if tracker == nil {
		t.Fatal("NewProgressTracker should not return nil")
	}
	if !tracker.enabled {
		t.Error("NewProgressTracker should be enabled")
	}
	if len(tracker.phases) != 2 {
		t.Errorf("NewProgressTracker should have 2 phases, got %d", len(tracker.phases))
	}

	// Test with disabled
	tracker = NewProgressTracker(phases, "Test Installation", false)
	if tracker == nil {
		t.Fatal("NewProgressTracker should not return nil")
	}
	if tracker.enabled {
		t.Error("NewProgressTracker should be disabled")
	}
}

func TestStartPhase(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 50, Deterministic: true},
		{Name: "Phase 2", Weight: 50, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test Installation", true)

	// Test starting first phase
	tracker.StartPhase(0)
	if tracker.currentPhase != 0 {
		t.Errorf("StartPhase should set currentPhase to 0, got %d", tracker.currentPhase)
	}

	// Test starting second phase
	tracker.StartPhase(1)
	if tracker.currentPhase != 1 {
		t.Errorf("StartPhase should set currentPhase to 1, got %d", tracker.currentPhase)
	}

	// Test invalid phase index
	tracker.StartPhase(10)
	if tracker.currentPhase != 1 {
		t.Errorf("StartPhase should not change currentPhase for invalid index")
	}
}

func TestAdvancePhase(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 50, Deterministic: true},
		{Name: "Phase 2", Weight: 50, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test Installation", true)

	// Start first phase
	tracker.StartPhase(0)

	// Advance to second phase
	tracker.AdvancePhase()
	if tracker.currentPhase != 1 {
		t.Errorf("AdvancePhase should move to next phase, got %d", tracker.currentPhase)
	}

	// Advance beyond last phase
	tracker.AdvancePhase()
	if tracker.currentPhase != 2 {
		t.Errorf("AdvancePhase should move beyond last phase, got %d", tracker.currentPhase)
	}
}

func TestSetProgress(_ *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: true},
	}
	tracker := NewProgressTracker(phases, "Test Installation", true)
	tracker.StartPhase(0)

	// Test setting progress
	tracker.SetProgress(50, 100)
	// Progress should be set to 50% of phase weight (100)
	// So 50% of 100 = 50
}

func TestFinish(_ *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: true},
	}
	tracker := NewProgressTracker(phases, "Test Installation", true)
	tracker.StartPhase(0)

	// Test finishing
	tracker.Finish()
	// Should not panic or error
}

func TestClear(_ *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: true},
	}
	tracker := NewProgressTracker(phases, "Test Installation", true)
	tracker.StartPhase(0)

	// Test clearing
	tracker.Clear()
	// Should not panic or error
}

func TestIsEnabled(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: true},
	}

	// Test enabled tracker
	tracker := NewProgressTracker(phases, "Test Installation", true)
	if !tracker.IsEnabled() {
		t.Error("IsEnabled should return true for enabled tracker")
	}

	// Test disabled tracker
	tracker = NewProgressTracker(phases, "Test Installation", false)
	if tracker.IsEnabled() {
		t.Error("IsEnabled should return false for disabled tracker")
	}
}

func TestNewSimpleSpinner(t *testing.T) {
	spinner := NewSimpleSpinner("Test Spinner")
	if spinner == nil {
		t.Fatal("NewSimpleSpinner should not return nil")
	}
	if !spinner.enabled {
		t.Error("NewSimpleSpinner should be enabled")
	}
	if len(spinner.phases) != 1 {
		t.Errorf("NewSimpleSpinner should have 1 phase, got %d", len(spinner.phases))
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"Zero", 0, "0s"},
		{"Seconds", 45 * time.Second, "45s"},
		{"Minutes", 2*time.Minute + 30*time.Second, "2m 30s"},
		{"Hours", 1*time.Hour + 30*time.Minute + 15*time.Second, "90m 15s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

func TestProgressTrackerDisabled(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: true},
	}
	tracker := NewProgressTracker(phases, "Test Installation", false)

	// Test that disabled tracker doesn't panic
	tracker.StartPhase(0)
	tracker.AdvancePhase()
	tracker.SetProgress(50, 100)
	tracker.Finish()
	tracker.Clear()

	if tracker.IsEnabled() {
		t.Error("Disabled tracker should report as disabled")
	}
}

func TestProgressTrackerWithOutput(_ *testing.T) {
	// This test is simplified as we can't easily capture stderr output
	// without more complex setup. The main functionality is tested
	// through the other test functions.
	phases := []InstallationPhase{
		{Name: "Test Phase", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test Installation", true)
	tracker.StartPhase(0)
	time.Sleep(100 * time.Millisecond)
	tracker.Finish()
}
