package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/FomoCLI/FomoCLI/internal/jx"
)

// Dash is printed for unknown values.
const Dash = "—"

// Compact formats a number as 1.2K / 3.4M / 5.6B.
func Compact(f float64) string {
	a := math.Abs(f)
	sign := ""
	if f < 0 {
		sign = "-"
	}
	switch {
	case a >= 1e12:
		return sign + trim(a/1e12) + "T"
	case a >= 1e9:
		return sign + trim(a/1e9) + "B"
	case a >= 1e6:
		return sign + trim(a/1e6) + "M"
	case a >= 1e3:
		return sign + trim(a/1e3) + "K"
	case a == 0:
		return "0"
	case a < 1:
		return sign + strconv.FormatFloat(a, 'g', 3, 64)
	}
	return sign + trim(a)
}

func trim(f float64) string {
	s := strconv.FormatFloat(f, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0")
}

// USD formats a dollar amount compactly: $151.4K.
func USD(v jx.V) string {
	f, ok := v.Float()
	if !ok {
		return Dash
	}
	return USDf(f)
}

// USDf formats a float as compact dollars.
func USDf(f float64) string {
	if f < 0 {
		return "-$" + Compact(-f)
	}
	return "$" + Compact(f)
}

// Price formats a token price with enough precision for micro-caps.
func Price(v jx.V) string {
	f, ok := v.Float()
	if !ok {
		return Dash
	}
	a := math.Abs(f)
	switch {
	case a == 0:
		return "$0"
	case a >= 1000:
		return "$" + Commas(f, 0)
	case a >= 1:
		return "$" + strconv.FormatFloat(f, 'f', 2, 64)
	case a >= 0.0001:
		// four significant digits, trailing zeros trimmed: 0.0031, 0.004812
		dec := int(-math.Floor(math.Log10(a))) + 3
		s := strings.TrimRight(strconv.FormatFloat(f, 'f', dec, 64), "0")
		return "$" + strings.TrimSuffix(s, ".")
	}
	// 0.0₅123 style subscript zeros for tiny prices
	s := strconv.FormatFloat(a, 'f', 20, 64)[2:]
	zeros := len(s) - len(strings.TrimLeft(s, "0"))
	digits := strings.TrimLeft(s, "0")
	if len(digits) > 4 {
		digits = digits[:4]
	}
	digits = strings.TrimRight(digits, "0")
	return "$0.0" + subscript(zeros) + digits
}

func subscript(n int) string {
	sub := []rune("₀₁₂₃₄₅₆₇₈₉")
	var b strings.Builder
	for _, r := range strconv.Itoa(n) {
		b.WriteRune(sub[r-'0'])
	}
	return b.String()
}

// Signed formats a PnL in green/red with an explicit sign: +$8.2K / -$1.1K.
func Signed(v jx.V) string {
	f, ok := v.Float()
	if !ok {
		return Muted.Render(Dash)
	}
	return SignedF(f)
}

// SignedF is Signed for a float.
func SignedF(f float64) string {
	switch {
	case f > 0:
		return Up.Render("+$" + Compact(f))
	case f < 0:
		return Down.Render("-$" + Compact(-f))
	}
	return Muted.Render("$0")
}

// Pct formats a percentage change with arrow and color: ▲ 12.3%.
func Pct(v jx.V) string {
	f, ok := v.Float()
	if !ok {
		return Muted.Render(Dash)
	}
	s := strconv.FormatFloat(math.Abs(f), 'f', 1, 64) + "%"
	switch {
	case f > 0:
		return Up.Render("▲ " + s)
	case f < 0:
		return Down.Render("▼ " + s)
	}
	return Muted.Render("• " + s)
}

// Int formats an integer with thousands separators.
func Int(v jx.V) string {
	f, ok := v.Float()
	if !ok {
		return Dash
	}
	return Commas(f, 0)
}

// Commas formats a float with thousands separators and fixed decimals.
func Commas(f float64, dec int) string {
	s := strconv.FormatFloat(math.Abs(f), 'f', dec, 64)
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	var b strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String() + frac
	if f < 0 {
		out = "-" + out
	}
	return out
}

// Amount formats a token amount compactly.
func Amount(v jx.V) string {
	f, ok := v.Float()
	if !ok {
		return Dash
	}
	return Compact(f)
}

// Short shortens an address: 5Ahf…To62F.
func Short(s string) string {
	if len(s) <= 13 {
		return s
	}
	return s[:6] + "…" + s[len(s)-5:]
}

// Truncate cuts s to n display cells, adding an ellipsis.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > n {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// OneLine collapses whitespace so multi-line text fits in a table cell.
func OneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// Time parses an ISO string or a unix timestamp (s or ms).
func Time(v jx.V) (time.Time, bool) {
	if s := v.Str(); s != "" {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t, true
		}
	}
	if f, ok := v.Float(); ok && f > 0 {
		if f > 1e12 {
			return time.UnixMilli(int64(f)), true
		}
		return time.Unix(int64(f), 0), true
	}
	return time.Time{}, false
}

// Ago renders a timestamp as "5m ago".
func Ago(v jx.V) string {
	t, ok := Time(v)
	if !ok {
		return Dash
	}
	return AgoT(t)
}

// AgoT renders a time as "5m ago".
func AgoT(t time.Time) string {
	return Dur(time.Since(t)) + " ago"
}

// Dur renders a duration compactly: 45s, 12m, 3h, 2d.
func Dur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// Bar renders a horizontal bar proportional to value/max, colored by sign.
func Bar(value, max float64, width int) string {
	if max <= 0 || width <= 0 {
		return ""
	}
	ratio := math.Min(math.Abs(value)/max, 1)
	cells := ratio * float64(width)
	full := int(cells)
	parts := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	frac := parts[int((cells-float64(full))*8)]
	s := strings.Repeat("█", full) + frac
	if s == "" && value != 0 {
		s = "▏"
	}
	if value < 0 {
		return Down.Render(s)
	}
	return Up.Render(s)
}

// Spark renders a sparkline for a series.
func Spark(vals []float64) string {
	if len(vals) == 0 {
		return ""
	}
	ticks := []rune("▁▂▃▄▅▆▇█")
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	var b strings.Builder
	for _, v := range vals {
		i := 0
		if hi > lo {
			i = int((v - lo) / (hi - lo) * float64(len(ticks)-1))
		}
		b.WriteRune(ticks[i])
	}
	return b.String()
}

// Check renders a verified tick.
func Check(b bool) string {
	if b {
		return Cyan.Render("✔")
	}
	return ""
}

// Rank renders a leaderboard rank with medals for the podium.
func Rank(n int64) string {
	switch n {
	case 1:
		return "🥇"
	case 2:
		return "🥈"
	case 3:
		return "🥉"
	}
	return Muted.Render(strconv.FormatInt(n, 10))
}

// Side renders a buy/sell tag.
func Side(s string) string {
	switch strings.ToLower(s) {
	case "buy":
		return Up.Bold(true).Render("BUY ")
	case "sell":
		return Down.Bold(true).Render("SELL")
	case "thesis":
		return Accent.Bold(true).Render("THSS")
	case "":
		return Muted.Render(Dash)
	}
	return Gold.Bold(true).Render(strings.ToUpper(Truncate(s, 4)))
}

// Chain renders a chain name with a consistent color.
func Chain(s string) string {
	if s == "" {
		return Muted.Render(Dash)
	}
	c := map[string]lipgloss.Style{
		"solana":      lipgloss.NewStyle().Foreground(lipgloss.Color("#9945ff")),
		"robinhood":   lipgloss.NewStyle().Foreground(lipgloss.Color("#ccff00")),
		"base":        lipgloss.NewStyle().Foreground(lipgloss.Color("#3b82f6")),
		"bsc":         lipgloss.NewStyle().Foreground(lipgloss.Color("#f0b90b")),
		"eth":         lipgloss.NewStyle().Foreground(lipgloss.Color("#8c9eff")),
		"ethereum":    lipgloss.NewStyle().Foreground(lipgloss.Color("#8c9eff")),
		"monad":       lipgloss.NewStyle().Foreground(lipgloss.Color("#836ef9")),
		"arc":         lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8a65")),
		"hyperliquid": lipgloss.NewStyle().Foreground(lipgloss.Color("#50e3c2")),
	}
	if st, ok := c[strings.ToLower(s)]; ok {
		return st.Render(s)
	}
	return Muted.Render(s)
}

// ChainName maps a numeric network id to a friendly chain name.
func ChainName(id string) string {
	switch id {
	case "4663":
		return "robinhood"
	case "1399811149":
		return "solana"
	case "8453":
		return "base"
	case "56":
		return "bsc"
	case "1":
		return "eth"
	case "5042":
		return "arc"
	case "143", "10143":
		return "monad"
	}
	return id
}
