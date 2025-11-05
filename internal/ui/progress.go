package ui

import (
	"fmt"
	"io"
	"os"

	"github.com/schollz/progressbar/v3"
)

// ProgressBar wraps progressbar/v3 with pkgctl styling
type ProgressBar struct {
	bar *progressbar.ProgressBar
}

// NewProgressBar creates a new progress bar for a known-length operation
func NewProgressBar(max int64, description string) *ProgressBar {
	bar := progressbar.NewOptions64(max,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWidth(15),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionOnCompletion(func() {
			fmt.Fprint(os.Stderr, "\n")
		}),
		progressbar.OptionSetRenderBlankState(true),
	)

	return &ProgressBar{bar: bar}
}

// NewProgressBarBytes creates a progress bar optimized for byte operations (downloads, etc.)
func NewProgressBarBytes(max int64, description string) *ProgressBar {
	bar := progressbar.DefaultBytes(max, description)
	return &ProgressBar{bar: bar}
}

// NewIndeterminateProgressBar creates a spinner for unknown-length operations
func NewIndeterminateProgressBar(description string) *ProgressBar {
	bar := progressbar.NewOptions(-1,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWidth(10),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionSetRenderBlankState(true),
	)

	return &ProgressBar{bar: bar}
}

// Write implements io.Writer for streaming operations
func (p *ProgressBar) Write(b []byte) (int, error) {
	return p.bar.Write(b)
}

// Add increments the progress bar by n
func (p *ProgressBar) Add(n int) error {
	return p.bar.Add(n)
}

// Add64 increments the progress bar by n (64-bit)
func (p *ProgressBar) Add64(n int64) error {
	return p.bar.Add64(n)
}

// Set sets the current progress to n
func (p *ProgressBar) Set(n int) error {
	return p.bar.Set(n)
}

// Set64 sets the current progress to n (64-bit)
func (p *ProgressBar) Set64(n int64) error {
	return p.bar.Set64(n)
}

// Finish completes the progress bar
func (p *ProgressBar) Finish() error {
	return p.bar.Finish()
}

// Clear clears the progress bar
func (p *ProgressBar) Clear() error {
	return p.bar.Clear()
}

// Describe changes the description of the progress bar
func (p *ProgressBar) Describe(description string) {
	p.bar.Describe(description)
}

// IsFinished returns true if the progress bar is finished
func (p *ProgressBar) IsFinished() bool {
	return p.bar.IsFinished()
}

// ProgressReader wraps an io.Reader with a progress bar
type ProgressReader struct {
	reader io.Reader
	bar    *ProgressBar
}

// NewProgressReader creates a new reader with progress tracking
func NewProgressReader(reader io.Reader, max int64, description string) *ProgressReader {
	return &ProgressReader{
		reader: reader,
		bar:    NewProgressBarBytes(max, description),
	}
}

// Read implements io.Reader with progress tracking
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.bar.Add(n)
	}
	return n, err
}

// Close closes the progress bar
func (pr *ProgressReader) Close() error {
	return pr.bar.Finish()
}

// ProgressWriter wraps an io.Writer with a progress bar
type ProgressWriter struct {
	writer io.Writer
	bar    *ProgressBar
}

// NewProgressWriter creates a new writer with progress tracking
func NewProgressWriter(writer io.Writer, max int64, description string) *ProgressWriter {
	return &ProgressWriter{
		writer: writer,
		bar:    NewProgressBarBytes(max, description),
	}
}

// Write implements io.Writer with progress tracking
func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if n > 0 {
		pw.bar.Add(n)
	}
	return n, err
}

// Close closes the progress bar
func (pw *ProgressWriter) Close() error {
	return pw.bar.Finish()
}

// MultiProgressBar manages multiple progress bars
type MultiProgressBar struct {
	bars []*ProgressBar
}

// NewMultiProgressBar creates a new multi-progress bar manager
func NewMultiProgressBar() *MultiProgressBar {
	return &MultiProgressBar{
		bars: make([]*ProgressBar, 0),
	}
}

// AddBar adds a progress bar to the manager
func (m *MultiProgressBar) AddBar(max int64, description string) *ProgressBar {
	bar := NewProgressBar(max, description)
	m.bars = append(m.bars, bar)
	return bar
}

// FinishAll finishes all progress bars
func (m *MultiProgressBar) FinishAll() error {
	for _, bar := range m.bars {
		if err := bar.Finish(); err != nil {
			return err
		}
	}
	return nil
}
