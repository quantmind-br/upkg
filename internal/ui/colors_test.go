package ui

import (
	"bytes"
	"os"
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
)

func TestInitColors(t *testing.T) {
	t.Run("with NO_COLOR", func(t *testing.T) {
		os.Setenv("NO_COLOR", "1")
		defer os.Unsetenv("NO_COLOR")

		color.NoColor = false
		InitColors()

		assert.True(t, color.NoColor)
	})

	t.Run("with TERM=dumb", func(t *testing.T) {
		os.Setenv("TERM", "dumb")
		defer os.Unsetenv("TERM")

		color.NoColor = false
		InitColors()

		assert.True(t, color.NoColor)
	})

	t.Run("normal terminal", func(_ *testing.T) {
		os.Unsetenv("NO_COLOR")
		os.Unsetenv("TERM")

		// Just ensure it doesn't panic
		InitColors()
		// Can't assert on color.NoColor as it depends on terminal detection
	})
}

func TestPrintFunctions(t *testing.T) {
	// Disable colors for consistent testing
	DisableColors()
	defer EnableColors()

	t.Run("PrintSuccess", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintSuccess("test %s", "message")

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "✓")
		assert.Contains(t, output, "test message")
	})

	t.Run("PrintError", func(t *testing.T) {
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		PrintError("test %s", "error")

		w.Close()
		os.Stderr = oldStderr

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "✗")
		assert.Contains(t, output, "Error:")
		assert.Contains(t, output, "test error")
	})

	t.Run("PrintWarning", func(t *testing.T) {
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		PrintWarning("test %s", "warning")

		w.Close()
		os.Stderr = oldStderr

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "Warning:")
		assert.Contains(t, output, "test warning")
	})

	t.Run("PrintInfo", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintInfo("test %s", "info")

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "→")
		assert.Contains(t, output, "test info")
	})

	t.Run("PrintStep", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintStep(1, 3, "step %d", 1)

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "[1/3]")
		assert.Contains(t, output, "step 1")
	})

	t.Run("PrintKeyValue", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintKeyValue("key", "value")

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "key:")
		assert.Contains(t, output, "value")
	})

	t.Run("PrintKeyValueColor", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintKeyValueColor("key", "value", Success)

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "key:")
		assert.Contains(t, output, "value")
	})

	t.Run("PrintSeparator", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintSeparator()

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "─")
	})

	t.Run("PrintHeader", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintHeader("Header")

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "Header")
	})

	t.Run("PrintSubheader", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintSubheader("Subheader")

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "Subheader")
	})

	t.Run("PrintList", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintList([]string{"item1", "item2"})

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "item1")
		assert.Contains(t, output, "item2")
		assert.Contains(t, output, "•")
	})

	t.Run("PrintNumberedList", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		PrintNumberedList([]string{"first", "second"})

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "1. first")
		assert.Contains(t, output, "2. second")
	})

	t.Run("Confirm", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		Confirm("Are you sure?")

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		buf.ReadFrom(r)
		output := buf.String()

		assert.Contains(t, output, "⚠")
		assert.Contains(t, output, "Are you sure?")
	})
}

func TestSprintFunctions(t *testing.T) {
	DisableColors()
	defer EnableColors()

	t.Run("SprintSuccess", func(t *testing.T) {
		result := SprintSuccess("test %s", "message")
		assert.Contains(t, result, "✓")
		assert.Contains(t, result, "test message")
	})

	t.Run("SprintError", func(t *testing.T) {
		result := SprintError("test %s", "error")
		assert.Contains(t, result, "✗")
		assert.Contains(t, result, "Error:")
		assert.Contains(t, result, "test error")
	})

	t.Run("SprintWarning", func(t *testing.T) {
		result := SprintWarning("test %s", "warning")
		assert.Contains(t, result, "Warning:")
		assert.Contains(t, result, "test warning")
	})

	t.Run("SprintInfo", func(t *testing.T) {
		result := SprintInfo("test %s", "info")
		assert.Contains(t, result, "→")
		assert.Contains(t, result, "test info")
	})
}

func TestColorizePackageType(t *testing.T) {
	DisableColors()
	defer EnableColors()

	tests := []struct {
		name     string
		pkgType  string
		expected string
	}{
		{"appimage", "appimage", "appimage"},
		{"binary", "binary", "binary"},
		{"tarball", "tarball", "tarball"},
		{"deb", "deb", "deb"},
		{"rpm", "rpm", "rpm"},
		{"unknown", "unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ColorizePackageType(tt.pkgType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestColorControls(t *testing.T) {
	t.Run("DisableColors", func(t *testing.T) {
		color.NoColor = false
		DisableColors()
		assert.True(t, color.NoColor)
	})

	t.Run("EnableColors", func(t *testing.T) {
		color.NoColor = true
		EnableColors()
		assert.False(t, color.NoColor)
	})

	t.Run("AreColorsEnabled", func(t *testing.T) {
		color.NoColor = true
		assert.False(t, AreColorsEnabled())

		color.NoColor = false
		assert.True(t, AreColorsEnabled())
	})
}
