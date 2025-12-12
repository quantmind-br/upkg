package ui

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
)

const (
	ansiClearLine                = "\r\033[2K"
	deterministicRefreshInterval = time.Second
)

// InstallationPhase represents a phase in the installation process
type InstallationPhase struct {
	Name          string
	Weight        int  // Relative weight (sum should be 100)
	Deterministic bool // true = progress bar | false = spinner
}

// ProgressTracker manages installation progress with hybrid approach
type ProgressTracker struct {
	bar            *progressbar.ProgressBar
	currentPhase   int
	phases         []InstallationPhase
	totalWeight    int
	startTime      time.Time
	enabled        bool
	lastUpdate     time.Time
	spinnerFrames  []string
	spinnerIndex   int
	inSpinnerMode  bool
	originalWriter io.Writer
	refreshStop    chan struct{}
}

// NewProgressTracker creates a new progress tracker with phases
func NewProgressTracker(phases []InstallationPhase, description string, enabled bool) *ProgressTracker {
	if !enabled {
		return &ProgressTracker{
			enabled: false,
			phases:  phases,
		}
	}

	totalWeight := 0
	for _, p := range phases {
		totalWeight += p.Weight
	}

	writer := os.Stderr

	bar := progressbar.NewOptions(totalWeight,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWriter(writer),
		progressbar.OptionSetWidth(40),
		progressbar.OptionUseANSICodes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "■",
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(writer, "\n")
		}),
	)

	return &ProgressTracker{
		bar:         bar,
		phases:      phases,
		totalWeight: totalWeight,
		startTime:   time.Now(),
		enabled:     true,
		lastUpdate:  time.Now(),
		spinnerFrames: []string{
			"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏",
		},
		spinnerIndex:   0,
		inSpinnerMode:  false,
		originalWriter: writer,
	}
}

// StartPhase starts a new installation phase
func (p *ProgressTracker) StartPhase(phaseIndex int) {
	if !p.enabled {
		return
	}

	if phaseIndex < 0 || phaseIndex >= len(p.phases) {
		return
	}

	phase := p.phases[phaseIndex]
	p.currentPhase = phaseIndex

	if phase.Deterministic {
		// Restore progressbar if coming from spinner mode
		if p.inSpinnerMode {
			p.bar.ChangeMax(p.totalWeight) // Reset progressbar
			p.inSpinnerMode = false
		}
		p.bar.Describe(phase.Name)
		p.startDeterministicRefresh(phase.Name)
	} else {
		p.stopDeterministicRefresh()
		// Entering spinner mode - allocate dedicated line beneath progress bar
		if !p.inSpinnerMode {
			fmt.Fprint(p.originalWriter, "\n")
			p.inSpinnerMode = true
		}

		// Spinner mode for indeterminate phases
		p.spinnerIndex = 0
		p.lastUpdate = time.Now()
		// Print initial spinner message directly
		p.clearLine()
		line := fmt.Sprintf("%s %s", p.getSpinner(), phase.Name)
		fmt.Fprint(p.originalWriter, line)
	}
}

// AdvancePhase completes current phase and moves to next
func (p *ProgressTracker) AdvancePhase() {
	if !p.enabled {
		return
	}

	if p.currentPhase < 0 || p.currentPhase >= len(p.phases) {
		return
	}

	// If completing an indeterminate phase, clear spinner line and move on
	if p.inSpinnerMode {
		p.clearLine()
		fmt.Fprint(p.originalWriter, "\n")
		p.inSpinnerMode = false
	} else {
		p.stopDeterministicRefresh()
		// Add weight of completed phase only if not in spinner mode
		currentWeight := p.phases[p.currentPhase].Weight
		if addErr := p.bar.Add(currentWeight); addErr != nil {
			// Best-effort progress update; ignore render errors.
			_ = addErr
		}
	}

	// Move to next phase
	p.currentPhase++
	if p.currentPhase < len(p.phases) {
		p.StartPhase(p.currentPhase)
	}
}

// UpdateIndeterminate updates message for indeterminate phases (with spinner animation)
func (p *ProgressTracker) UpdateIndeterminate(message string) {
	if !p.enabled {
		return
	}

	// Throttle updates to avoid excessive rendering
	now := time.Now()
	if now.Sub(p.lastUpdate) < 100*time.Millisecond {
		return
	}
	p.lastUpdate = now

	// Update spinner animation
	p.spinnerIndex = (p.spinnerIndex + 1) % len(p.spinnerFrames)

	elapsed := time.Since(p.startTime)

	// Clear previous line and write spinner update in-place
	p.clearLine()
	fmt.Fprintf(p.originalWriter, "%s %s (elapsed: %s)",
		p.getSpinner(),
		message,
		formatDuration(elapsed))
}

// UpdateIndeterminateWithElapsed updates with custom elapsed time display
func (p *ProgressTracker) UpdateIndeterminateWithElapsed(message string, elapsed time.Duration) {
	if !p.enabled {
		return
	}

	now := time.Now()
	if now.Sub(p.lastUpdate) < 100*time.Millisecond {
		return
	}
	p.lastUpdate = now

	p.spinnerIndex = (p.spinnerIndex + 1) % len(p.spinnerFrames)

	// Clear previous line and write spinner update in-place
	p.clearLine()
	fmt.Fprintf(p.originalWriter, "%s %s (elapsed: %s)",
		p.getSpinner(),
		message,
		formatDuration(elapsed))
}

// SetProgress sets progress for deterministic phases
func (p *ProgressTracker) SetProgress(current, total int) {
	if !p.enabled || p.currentPhase < 0 || p.currentPhase >= len(p.phases) {
		return
	}

	phase := p.phases[p.currentPhase]
	if !phase.Deterministic {
		return
	}

	// Calculate progress within current phase's weight
	if total > 0 {
		phaseProgress := (current * phase.Weight) / total
		if setErr := p.bar.Set(p.getCompletedWeight() + phaseProgress); setErr != nil {
			// Best-effort progress update; ignore render errors.
			_ = setErr
		}
	}
}

// Finish completes the progress bar
func (p *ProgressTracker) Finish() {
	if !p.enabled {
		return
	}

	p.stopDeterministicRefresh()

	// If finishing in spinner mode, just add newline
	if p.inSpinnerMode {
		p.clearLine()
		fmt.Fprintln(p.originalWriter)
		p.inSpinnerMode = false
	} else {
		if finishErr := p.bar.Finish(); finishErr != nil {
			// Best-effort progress update; ignore render errors.
			_ = finishErr
		}
	}
}

// Clear clears the progress bar from terminal
func (p *ProgressTracker) Clear() {
	if !p.enabled {
		return
	}

	p.stopDeterministicRefresh()
	if clearErr := p.bar.Clear(); clearErr != nil {
		// Best-effort progress update; ignore render errors.
		_ = clearErr
	}
}

// IsEnabled returns whether progress tracking is enabled
func (p *ProgressTracker) IsEnabled() bool {
	return p.enabled
}

// getSpinner returns current spinner frame
func (p *ProgressTracker) getSpinner() string {
	if p.spinnerIndex < 0 || p.spinnerIndex >= len(p.spinnerFrames) {
		p.spinnerIndex = 0
	}
	return p.spinnerFrames[p.spinnerIndex]
}

// getCompletedWeight calculates total weight of completed phases
func (p *ProgressTracker) getCompletedWeight() int {
	if p.currentPhase < 0 {
		return 0
	}

	total := 0
	for i := 0; i < p.currentPhase && i < len(p.phases); i++ {
		total += p.phases[i].Weight
	}
	return total
}

// formatDuration formats duration in human-readable form
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)

	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60

	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// NewSimpleSpinner creates a simple spinner for quick operations
func NewSimpleSpinner(message string) *ProgressTracker {
	phases := []InstallationPhase{
		{Name: message, Weight: 100, Deterministic: false},
	}
	tracker := NewProgressTracker(phases, "", true)
	tracker.StartPhase(0)
	return tracker
}

// clearLine erases the current terminal line without creating newlines
func (p *ProgressTracker) clearLine() {
	if !p.enabled || p.originalWriter == nil {
		return
	}
	fmt.Fprint(p.originalWriter, ansiClearLine)
}

func (p *ProgressTracker) startDeterministicRefresh(description string) {
	if !p.enabled || p.bar == nil {
		return
	}

	p.stopDeterministicRefresh()

	stopCh := make(chan struct{})
	p.refreshStop = stopCh
	ticker := time.NewTicker(deterministicRefreshInterval)

	go func() {
		for {
			select {
			case <-ticker.C:
				p.bar.Describe(description)
			case <-stopCh:
				ticker.Stop()
				return
			}
		}
	}()
}

func (p *ProgressTracker) stopDeterministicRefresh() {
	if p.refreshStop == nil {
		return
	}
	close(p.refreshStop)
	p.refreshStop = nil
}
