package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

var bannerArt = []string{
	"███████╗ ██████╗ ███╗   ███╗ ██████╗ ",
	"██╔════╝██╔═══██╗████╗ ████║██╔═══██╗",
	"█████╗  ██║   ██║██╔████╔██║██║   ██║",
	"██╔══╝  ██║   ██║██║╚██╔╝██║██║   ██║",
	"██║     ╚██████╔╝██║ ╚═╝ ██║╚██████╔╝",
	"╚═╝      ╚═════╝ ╚═╝     ╚═╝ ╚═════╝ ",
}

// Gradient colors each rune of s along a gradient from a to b.
func Gradient(s, a, b string) string {
	if !ColorEnabled() {
		return s
	}
	ca, _ := colorful.Hex(a)
	cb, _ := colorful.Hex(b)
	runes := []rune(s)
	var out strings.Builder
	for i, r := range runes {
		t := 0.0
		if len(runes) > 1 {
			t = float64(i) / float64(len(runes)-1)
		}
		c := ca.BlendLuv(cb, t).Clamped().Hex()
		out.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render(string(r)))
	}
	return out.String()
}

// Banner returns the big gradient FOMO logo with a tagline.
func Banner(version string) string {
	var b strings.Builder
	b.WriteString("\n")
	for _, l := range bannerArt {
		b.WriteString("  " + Gradient(l, "#39ff88", "#b388ff") + "\n")
	}
	tag := "  " + Bold.Render("fomo.family in your terminal") + Muted.Render("  ·  ") + Muted.Render(version)
	b.WriteString(tag + "\n")
	return b.String()
}

// Logo is the one-line wordmark used in headers.
func Logo() string {
	return Gradient("◆ fomo", "#39ff88", "#b388ff")
}
