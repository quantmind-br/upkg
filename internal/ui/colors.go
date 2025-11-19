package ui

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

// Color scheme for upkg
var (
	// Primary actions
	Success = color.New(color.FgGreen)
	Error   = color.New(color.FgRed, color.Bold)
	Warning = color.New(color.FgYellow)
	Info    = color.New(color.FgCyan)

	// Secondary actions
	Highlight = color.New(color.FgHiCyan, color.Bold)
	Muted     = color.New(color.Faint)
	Bold      = color.New(color.Bold)

	// Status indicators
	CheckMark = color.GreenString("✓")
	CrossMark = color.RedString("✗")
	Arrow     = color.CyanString("→")
	Bullet    = color.HiBlackString("•")

	// Package type colors
	TypeAppImage = color.New(color.FgMagenta)
	TypeBinary   = color.New(color.FgBlue)
	TypeTarball  = color.New(color.FgYellow)
	TypeDEB      = color.New(color.FgCyan)
	TypeRPM      = color.New(color.FgRed)
)

// InitColors initializes color settings based on environment
func InitColors() {
	// Respect NO_COLOR environment variable
	if os.Getenv("NO_COLOR") != "" {
		color.NoColor = true
	}

	// Respect TERM environment variable
	if os.Getenv("TERM") == "dumb" {
		color.NoColor = true
	}
}

// PrintSuccess prints a success message
func PrintSuccess(format string, args ...interface{}) {
	Success.Fprintf(os.Stdout, "%s %s\n", CheckMark, fmt.Sprintf(format, args...))
}

// PrintError prints an error message
func PrintError(format string, args ...interface{}) {
	Error.Fprintf(os.Stderr, "%s Error: %s\n", CrossMark, fmt.Sprintf(format, args...))
}

// PrintWarning prints a warning message
func PrintWarning(format string, args ...interface{}) {
	Warning.Fprintf(os.Stderr, "Warning: %s\n", fmt.Sprintf(format, args...))
}

// PrintInfo prints an info message
func PrintInfo(format string, args ...interface{}) {
	Info.Fprintf(os.Stdout, "%s %s\n", Arrow, fmt.Sprintf(format, args...))
}

// PrintStep prints a step indicator
func PrintStep(step, total int, format string, args ...interface{}) {
	Highlight.Fprintf(os.Stdout, "[%d/%d] ", step, total)
	fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// PrintKeyValue prints a key-value pair with color
func PrintKeyValue(key, value string) {
	Bold.Fprintf(os.Stdout, "%s: ", key)
	fmt.Fprintln(os.Stdout, value)
}

// PrintKeyValueColor prints a key-value pair with custom color for value
func PrintKeyValueColor(key string, value string, valueColor *color.Color) {
	Bold.Fprintf(os.Stdout, "%s: ", key)
	valueColor.Fprintln(os.Stdout, value)
}

// PrintSeparator prints a separator line
func PrintSeparator() {
	Muted.Fprintln(os.Stdout, "────────────────────────────────────────")
}

// PrintHeader prints a section header
func PrintHeader(text string) {
	fmt.Fprintln(os.Stdout)
	Bold.Fprintln(os.Stdout, text)
	Muted.Fprintln(os.Stdout, "────────────────────────────────────────")
}

// PrintSubheader prints a subsection header
func PrintSubheader(text string) {
	fmt.Fprintln(os.Stdout)
	Highlight.Fprintln(os.Stdout, text)
}

// ColorizePackageType returns a colored package type string
func ColorizePackageType(pkgType string) string {
	switch pkgType {
	case "appimage":
		return TypeAppImage.Sprint(pkgType)
	case "binary":
		return TypeBinary.Sprint(pkgType)
	case "tarball":
		return TypeTarball.Sprint(pkgType)
	case "deb":
		return TypeDEB.Sprint(pkgType)
	case "rpm":
		return TypeRPM.Sprint(pkgType)
	default:
		return pkgType
	}
}

// PrintList prints a bulleted list
func PrintList(items []string) {
	for _, item := range items {
		fmt.Fprintf(os.Stdout, "  %s %s\n", Bullet, item)
	}
}

// PrintNumberedList prints a numbered list
func PrintNumberedList(items []string) {
	for i, item := range items {
		Bold.Fprintf(os.Stdout, "%d. ", i+1)
		fmt.Fprintln(os.Stdout, item)
	}
}

// Confirm prints a confirmation message (for use before prompts)
func Confirm(message string) {
	Warning.Fprintf(os.Stdout, "⚠  %s ", message)
}

// SprintSuccess returns a success string without printing
func SprintSuccess(format string, args ...interface{}) string {
	return fmt.Sprintf("%s %s", CheckMark, fmt.Sprintf(format, args...))
}

// SprintError returns an error string without printing
func SprintError(format string, args ...interface{}) string {
	return fmt.Sprintf("%s Error: %s", CrossMark, fmt.Sprintf(format, args...))
}

// SprintWarning returns a warning string without printing
func SprintWarning(format string, args ...interface{}) string {
	return fmt.Sprintf("Warning: %s", fmt.Sprintf(format, args...))
}

// SprintInfo returns an info string without printing
func SprintInfo(format string, args ...interface{}) string {
	return fmt.Sprintf("%s %s", Arrow, fmt.Sprintf(format, args...))
}

// DisableColors disables all color output
func DisableColors() {
	color.NoColor = true
}

// EnableColors enables color output
func EnableColors() {
	color.NoColor = false
}

// AreColorsEnabled returns whether colors are currently enabled
func AreColorsEnabled() bool {
	return !color.NoColor
}
