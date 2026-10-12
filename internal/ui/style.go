// Package ui holds fomo's terminal look: palette, formatters, tables, cards,
// the banner and the spinner.
package ui

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

// Palette - neon on dark, in the spirit of the FOMO app.
var (
	ColGreen  = lipgloss.AdaptiveColor{Light: "#008a3e", Dark: "#39ff88"}
	ColRed    = lipgloss.AdaptiveColor{Light: "#c4153a", Dark: "#ff4d6d"}
	ColAccent = lipgloss.AdaptiveColor{Light: "#6b2bd9", Dark: "#b388ff"}
	ColPink   = lipgloss.AdaptiveColor{Light: "#c2187a", Dark: "#ff5fd2"}
	ColGold   = lipgloss.AdaptiveColor{Light: "#9a6700", Dark: "#ffd166"}
	ColCyan   = lipgloss.AdaptiveColor{Light: "#00728f", Dark: "#5ce1ff"}
	ColMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#7c8591"}
	ColText   = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#e6edf3"}
	ColBorder = lipgloss.AdaptiveColor{Light: "#c9cfd6", Dark: "#3a4250"}
)

var (
	Bold    = lipgloss.NewStyle().Bold(true)
	Muted   = lipgloss.NewStyle().Foreground(ColMuted)
	Up      = lipgloss.NewStyle().Foreground(ColGreen)
	Down    = lipgloss.NewStyle().Foreground(ColRed)
	Accent  = lipgloss.NewStyle().Foreground(ColAccent)
	Pink    = lipgloss.NewStyle().Foreground(ColPink)
	Gold    = lipgloss.NewStyle().Foreground(ColGold)
	Cyan    = lipgloss.NewStyle().Foreground(ColCyan)
	Title   = lipgloss.NewStyle().Bold(true).Foreground(ColAccent)
	Heading = lipgloss.NewStyle().Bold(true).Foreground(ColText)
	Header  = lipgloss.NewStyle().Bold(true).Foreground(ColMuted)
	Handle  = lipgloss.NewStyle().Bold(true).Foreground(ColCyan)
	Badge   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0b0f14")).Background(ColGreen).Padding(0, 1)
	BadgeW  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0b0f14")).Background(ColGold).Padding(0, 1)
	BadgeE  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffffff")).Background(ColRed).Padding(0, 1)
	Box     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColBorder).Padding(0, 1)
)

// Out and Err are where commands write. Tests swap them.
var (
	Out io.Writer = os.Stdout
	Err io.Writer = os.Stderr
)

var colorOn = true

// Setup configures color output. Colors are disabled for --no-color,
// NO_COLOR, theme=none, or when stdout is not a terminal.
func Setup(noColor bool) {
	if noColor || os.Getenv("NO_COLOR") != "" || !IsTTY(os.Stdout) {
		DisableColor()
	}
}

// DisableColor turns all styling into plain text.
func DisableColor() {
	colorOn = false
	lipgloss.SetColorProfile(termenv.Ascii)
}

// ColorEnabled reports whether ANSI styling is active.
func ColorEnabled() bool { return colorOn }

// IsTTY reports whether f is an interactive terminal.
func IsTTY(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

// Width returns the terminal width (default 100).
func Width() int {
	w, _, err := termSize()
	if err != nil || w <= 0 {
		return 100
	}
	return w
}
