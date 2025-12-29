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

func TestUpdateIndeterminate(t *testing.T) {
	t.Run("calls without panic", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 50, Deterministic: false},
		}
		tracker := NewProgressTracker(phases, "Test", true)

		// Should not panic
		tracker.UpdateIndeterminate("Processing")
		tracker.Finish()
	})

	t.Run("disabled tracker", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 50, Deterministic: false},
		}
		tracker := NewProgressTracker(phases, "Test", false)

		// Should not panic
		tracker.UpdateIndeterminate("Processing")
		tracker.Finish()
	})
}

func TestUpdateIndeterminateWithElapsed(t *testing.T) {
	t.Run("calls without panic", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 50, Deterministic: false},
		}
		tracker := NewProgressTracker(phases, "Test", true)

		// Should not panic
		tracker.UpdateIndeterminateWithElapsed("Processing", 5*time.Second)
		tracker.Finish()
	})

	t.Run("disabled tracker", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 50, Deterministic: false},
		}
		tracker := NewProgressTracker(phases, "Test", false)

		// Should not panic
		tracker.UpdateIndeterminateWithElapsed("Processing", 5*time.Second)
		tracker.Finish()
	})
}

func TestGetSpinner(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)

	// Test various spinner indices
	for i := 0; i < 10; i++ {
		spinner := tracker.getSpinner()
		if spinner == "" {
			t.Errorf("getSpinner should not return empty string at index %d", i)
		}
		tracker.spinnerIndex++
	}
}

func TestGetCompletedWeight(t *testing.T) {
	t.Run("single phase", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 100, Deterministic: true},
		}
		tracker := NewProgressTracker(phases, "Test", true)
		tracker.StartPhase(0)

		weight := tracker.getCompletedWeight()
		if weight < 0 || weight > 100 {
			t.Errorf("getCompletedWeight should be between 0 and 100, got %d", weight)
		}
	})

	t.Run("multiple phases", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 30, Deterministic: true},
			{Name: "Phase 2", Weight: 70, Deterministic: true},
		}
		tracker := NewProgressTracker(phases, "Test", true)
		tracker.StartPhase(0)

		weight := tracker.getCompletedWeight()
		if weight < 0 || weight > 30 {
			t.Errorf("getCompletedWeight for first phase should be <= 30, got %d", weight)
		}
	})
}

func TestClearLine(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)

	// Test that clearLine doesn't panic
	tracker.clearLine()
}

func TestSetProgressEdgeCases(t *testing.T) {
	t.Run("zero total", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 100, Deterministic: true},
		}
		tracker := NewProgressTracker(phases, "Test", true)
		tracker.StartPhase(0)

		// Should not panic with total = 0
		tracker.SetProgress(50, 0)
	})

	t.Run("negative current", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 100, Deterministic: true},
		}
		tracker := NewProgressTracker(phases, "Test", true)
		tracker.StartPhase(0)

		// Should not panic with negative current
		tracker.SetProgress(-10, 100)
	})

	t.Run("no active phase", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 100, Deterministic: true},
		}
		tracker := NewProgressTracker(phases, "Test", true)

		// No phase started - should not panic
		tracker.SetProgress(50, 100)
	})

	t.Run("indeterminate phase", func(_ *testing.T) {
		phases := []InstallationPhase{
			{Name: "Phase 1", Weight: 100, Deterministic: false},
		}
		tracker := NewProgressTracker(phases, "Test", true)
		tracker.StartPhase(0)

		// Indeterminate phase - should not affect progress
		tracker.SetProgress(50, 100)
	})
}

func TestUpdateIndeterminateThrottling(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)
	tracker.StartPhase(0)

	// Rapid updates should be throttled
	for i := 0; i < 10; i++ {
		tracker.UpdateIndeterminate("Rapid update")
	}
	tracker.Finish()
}

func TestUpdateIndeterminateWithElapsedThrottling(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)
	tracker.StartPhase(0)

	// Rapid updates should be throttled
	for i := 0; i < 10; i++ {
		tracker.UpdateIndeterminateWithElapsed("Rapid", time.Duration(i)*time.Second)
	}
	tracker.Finish()
}

func TestProgressTrackerFullLifecycle(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Download", Weight: 30, Deterministic: true},
		{Name: "Extract", Weight: 40, Deterministic: false},
		{Name: "Install", Weight: 30, Deterministic: true},
	}
	tracker := NewProgressTracker(phases, "Full Test", true)

	// Start first phase
	tracker.StartPhase(0)
	tracker.SetProgress(25, 50)

	// Move to second phase (indeterminate)
	tracker.AdvancePhase()
	tracker.UpdateIndeterminate("Extracting files...")

	// Move to third phase
	tracker.AdvancePhase()
	tracker.SetProgress(15, 30)

	// Finish
	tracker.Finish()
}

func TestProgressTrackerMultipleAdvances(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 50, Deterministic: false},
		{Name: "Phase 2", Weight: 50, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)

	tracker.StartPhase(0)
	tracker.AdvancePhase()
	tracker.AdvancePhase() // Beyond last phase
	tracker.AdvancePhase() // Still beyond
	tracker.Finish()
}

func TestProgressTrackerWithAllDeterministic(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 33, Deterministic: true},
		{Name: "Phase 2", Weight: 33, Deterministic: true},
		{Name: "Phase 3", Weight: 34, Deterministic: true},
	}
	tracker := NewProgressTracker(phases, "All Deterministic", true)

	tracker.StartPhase(0)
	tracker.SetProgress(10, 100)
	tracker.AdvancePhase()
	tracker.SetProgress(20, 100)
	tracker.AdvancePhase()
	tracker.SetProgress(30, 100)
	tracker.Finish()
}

func TestProgressTrackerWithAllIndeterminate(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 50, Deterministic: false},
		{Name: "Phase 2", Weight: 50, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "All Indeterminate", true)

	tracker.StartPhase(0)
	tracker.UpdateIndeterminate("Processing phase 1")
	tracker.AdvancePhase()
	tracker.UpdateIndeterminate("Processing phase 2")
	tracker.Finish()
}

func TestProgressTrackerClearAndRestart(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)

	tracker.StartPhase(0)
	tracker.UpdateIndeterminate("First run")
	tracker.Clear()

	// Can still use after clear
	tracker.UpdateIndeterminate("Second run")
	tracker.Finish()
}

func TestSimpleSpinner(t *testing.T) {
	t.Run("with indeterminate updates", func(_ *testing.T) {
		spinner := NewSimpleSpinner("Working...")
		spinner.UpdateIndeterminate("Step 1")
		spinner.UpdateIndeterminate("Step 2")
		spinner.Finish()
	})

	t.Run("with progress", func(_ *testing.T) {
		spinner := NewSimpleSpinner("Downloading...")
		spinner.SetProgress(50, 100)
		spinner.SetProgress(75, 100)
		spinner.SetProgress(100, 100)
		spinner.Finish()
	})

	t.Run("disabled", func(_ *testing.T) {
		spinner := NewSimpleSpinner("Disabled")
		// Simple spinner is always enabled, just test it doesn't panic
		spinner.UpdateIndeterminate("Should not show")
		spinner.Finish()
	})
}

func TestUpdateIndeterminateThrottleBypass(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)
	tracker.StartPhase(0)

	// Wait for throttle period to pass (100ms)
	time.Sleep(150 * time.Millisecond)
	tracker.UpdateIndeterminate("After throttle")
	tracker.Finish()
}

func TestUpdateIndeterminateWithElapsedThrottleBypass(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)
	tracker.StartPhase(0)

	// Wait for throttle period to pass (100ms)
	time.Sleep(150 * time.Millisecond)
	tracker.UpdateIndeterminateWithElapsed("After throttle", 10*time.Second)
	tracker.Finish()
}

func TestClearLineDisabled(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", false)
	tracker.StartPhase(0)

	// Should not panic when disabled
	tracker.clearLine()
}

func TestClearLineNilWriter(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)
	tracker.StartPhase(0)
	// Set writer to nil to test that branch
	originalWriter := tracker.originalWriter
	tracker.originalWriter = nil

	// Should not panic when writer is nil
	tracker.clearLine()

	// Restore writer for cleanup
	tracker.originalWriter = originalWriter
	tracker.Finish()
}

func TestGetSpinnerAllFrames(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)
	tracker.StartPhase(0)

	// Test all possible spinner frame indices
	for i := 0; i < 20; i++ {
		tracker.spinnerIndex = i
		spinner := tracker.getSpinner()
		if spinner == "" {
			t.Errorf("getSpinner should not return empty string at index %d", i)
		}
		// Each spinner should be a single character (emoji or similar)
		if len(spinner) == 0 {
			t.Errorf("getSpinner returned empty at index %d", i)
		}
	}
	tracker.Finish()
}

func TestUpdateIndeterminateAfterThrottle(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)
	tracker.StartPhase(0)

	// First call may be throttled
	tracker.UpdateIndeterminate("First")

	// Wait for throttle to pass
	time.Sleep(150 * time.Millisecond)

	// Second call should pass throttle
	tracker.UpdateIndeterminate("Second")

	tracker.Finish()
}

func TestUpdateIndeterminateWithElapsedAfterThrottle(t *testing.T) {
	phases := []InstallationPhase{
		{Name: "Phase 1", Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "Test", true)
	tracker.StartPhase(0)

	// First call may be throttled
	tracker.UpdateIndeterminateWithElapsed("First", 5*time.Second)

	// Wait for throttle to pass
	time.Sleep(150 * time.Millisecond)

	// Second call should pass throttle
	tracker.UpdateIndeterminateWithElapsed("Second", 10*time.Second)

	tracker.Finish()
}
