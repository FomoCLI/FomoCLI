package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Align of a table column.
type Align int

const (
	Left Align = iota
	Right
)

// Col describes a table column.
type Col struct {
	Title string
	Align Align
	Max   int  // max display width (0 = unlimited)
	Flex  bool // this column shrinks first when the terminal is narrow
}

// Table is a lightweight, ANSI-aware table: headers, aligned columns, a rule.
type Table struct {
	Cols []Col
	Rows [][]string
}

// NewTable creates a table with the given columns.
func NewTable(cols ...Col) *Table { return &Table{Cols: cols} }

// Add appends a row. Missing cells are blank.
func (t *Table) Add(cells ...string) { t.Rows = append(t.Rows, cells) }

// Len is the number of rows.
func (t *Table) Len() int { return len(t.Rows) }

// Render returns the table as a string sized for the terminal.
func (t *Table) Render() string {
	if len(t.Rows) == 0 {
		return Muted.Render("  (no results)") + "\n"
	}
	n := len(t.Cols)
	w := make([]int, n)
	for i, c := range t.Cols {
		w[i] = lipgloss.Width(c.Title)
	}
	for _, r := range t.Rows {
		for i := 0; i < n && i < len(r); i++ {
			cw := lipgloss.Width(r[i])
			if t.Cols[i].Max > 0 && cw > t.Cols[i].Max {
				cw = t.Cols[i].Max
			}
			if cw > w[i] {
				w[i] = cw
			}
		}
	}

	// Shrink flex columns to fit the terminal.
	const gap = 2
	total := func() int {
		s := 2
		for _, x := range w {
			s += x + gap
		}
		return s
	}
	avail := Width()
	for total() > avail {
		shrunk := false
		for i, c := range t.Cols {
			if c.Flex && w[i] > 8 && total() > avail {
				w[i]--
				shrunk = true
			}
		}
		if !shrunk {
			break
		}
	}

	var b strings.Builder
	line := func(cells []string, header bool) {
		b.WriteString("  ")
		for i := 0; i < n; i++ {
			cell := ""
			if i < len(cells) {
				cell = cells[i]
			}
			if lipgloss.Width(cell) > w[i] {
				cell = truncateANSI(cell, w[i])
			}
			pad := w[i] - lipgloss.Width(cell)
			if header {
				cell = Header.Render(cell)
			}
			if t.Cols[i].Align == Right {
				b.WriteString(strings.Repeat(" ", pad) + cell)
			} else if i < n-1 {
				b.WriteString(cell + strings.Repeat(" ", pad))
			} else {
				b.WriteString(cell)
			}
			if i < n-1 {
				b.WriteString(strings.Repeat(" ", gap))
			}
		}
		b.WriteString("\n")
	}

	titles := make([]string, n)
	for i, c := range t.Cols {
		titles[i] = strings.ToUpper(c.Title)
	}
	line(titles, true)
	ruleW := total() - 2 - gap
	b.WriteString("  " + Muted.Render(strings.Repeat("─", max(ruleW, 0))) + "\n")
	for _, r := range t.Rows {
		line(r, false)
	}
	return b.String()
}

// truncateANSI trims a possibly-styled string to n cells. Styling is dropped
// for truncated cells so we never cut an escape sequence in half.
func truncateANSI(s string, n int) string {
	plain := stripANSI(s)
	return Truncate(plain, n)
}

func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			in = true
		case in && ((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')):
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// KV renders aligned key/value lines, skipping empty values.
func KV(pairs ...[2]string) string {
	kw := 0
	for _, p := range pairs {
		if p[1] != "" && lipgloss.Width(p[0]) > kw {
			kw = lipgloss.Width(p[0])
		}
	}
	var b strings.Builder
	for _, p := range pairs {
		if p[1] == "" {
			continue
		}
		fmt.Fprintf(&b, "%s  %s\n", Muted.Render(p[0]+strings.Repeat(" ", kw-lipgloss.Width(p[0]))), p[1])
	}
	return strings.TrimRight(b.String(), "\n")
}

// Section prints a titled section heading.
func Section(title string) string {
	return "\n" + Title.Render("▌"+title)
}

// Stat renders a big-number tile: label above, value below.
func Stat(label, value string) string {
	return lipgloss.JoinVertical(lipgloss.Left, Muted.Render(strings.ToUpper(label)), Bold.Render(value))
}

// Tiles lays out stat tiles horizontally with spacing, wrapping to the width.
func Tiles(tiles ...string) string {
	avail := Width() - 4
	var rows []string
	var cur []string
	curW := 0
	for _, t := range tiles {
		tw := lipgloss.Width(t) + 4
		if curW+tw > avail && len(cur) > 0 {
			rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cur...))
			cur, curW = nil, 0
		}
		cur = append(cur, lipgloss.NewStyle().PaddingRight(4).Render(t))
		curW += tw
	}
	if len(cur) > 0 {
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cur...))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// VisibleWidth is the display width of s, ignoring ANSI escapes.
func VisibleWidth(s string) int { return lipgloss.Width(s) }
