// Package ui holds fomocli's terminal look: palette, formatters, tables, cards,
// the banner and the spinner.
package ui

import (
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

// Palette - taken from the FomoCLI logo: ice white on near-black, with cool
// greys for secondary text. Green and red are kept strictly for gains/losses.
var (
	ColIce    = lipgloss.AdaptiveColor{Light: "#0b0d1a", Dark: "#eef1fc"} // logo white
	ColSteel  = lipgloss.AdaptiveColor{Light: "#3b4266", Dark: "#b4bbdc"} // secondary
	ColGreen  = lipgloss.AdaptiveColor{Light: "#0f8a4b", Dark: "#5ee6a0"}
	ColRed    = lipgloss.AdaptiveColor{Light: "#c4243f", Dark: "#ff6b81"}
	ColAccent = ColIce
	ColPink   = lipgloss.AdaptiveColor{Light: "#a3346b", Dark: "#f5a3c7"}
	ColGold   = lipgloss.AdaptiveColor{Light: "#8a6100", Dark: "#f2d28b"}
	ColCyan   = ColSteel
	ColMuted  = lipgloss.AdaptiveColor{Light: "#6b7190", Dark: "#6e7596"}
	ColText   = ColIce
	ColBorder = lipgloss.AdaptiveColor{Light: "#c9cde0", Dark: "#2b3048"}
	colInk    = lipgloss.Color("#05060f") // logo background
)

var (
	Bold    = lipgloss.NewStyle().Bold(true).Foreground(ColIce)
	Muted   = lipgloss.NewStyle().Foreground(ColMuted)
	Up      = lipgloss.NewStyle().Foreground(ColGreen)
	Down    = lipgloss.NewStyle().Foreground(ColRed)
	Accent  = lipgloss.NewStyle().Foreground(ColAccent)
	Pink    = lipgloss.NewStyle().Foreground(ColPink)
	Gold    = lipgloss.NewStyle().Foreground(ColGold)
	Cyan    = lipgloss.NewStyle().Foreground(ColCyan)
	Title   = lipgloss.NewStyle().Bold(true).Foreground(ColIce)
	Heading = lipgloss.NewStyle().Bold(true).Foreground(ColText)
	Header  = lipgloss.NewStyle().Bold(true).Foreground(ColMuted)
	Handle  = lipgloss.NewStyle().Bold(true).Foreground(ColSteel)
	Badge   = lipgloss.NewStyle().Bold(true).Foreground(colInk).Background(ColIce).Padding(0, 1)
	BadgeW  = lipgloss.NewStyle().Bold(true).Foreground(colInk).Background(ColGold).Padding(0, 1)
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
