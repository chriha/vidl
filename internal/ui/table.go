package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// column describes one table column. Fixed columns have a width; flexible
// ones (flex > 0) share the remaining space by weight.
type column struct {
	title string
	width int
	flex  int
	min   int // minimum width of a flexible column
	right bool
	drop  int // columns with a higher drop rank are hidden first; 0 never
}

// cell renders its content to exactly width cells, on top of base (which
// carries the row background and the column alignment).
type cell func(width int, base lipgloss.Style) string

const colGap = 1

var headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))

// layout returns the width of every column for a table total cells wide; 0
// marks a hidden column. Droppable columns are hidden, highest rank first,
// until the flexible columns get their minimum width.
func layout(cols []column, total int) []int {
	hidden := make([]bool, len(cols))
	for {
		fixed, flex, minFlex, visible := 0, 0, 0, 0
		for i, c := range cols {
			if hidden[i] {
				continue
			}
			visible++
			fixed += c.width
			flex += c.flex
			minFlex += c.min
		}
		avail := total - fixed - colGap*(visible+1)
		drop := -1
		for i, c := range cols {
			if !hidden[i] && c.drop > 0 && (drop < 0 || c.drop > cols[drop].drop) {
				drop = i
			}
		}
		if avail >= minFlex || drop < 0 {
			return distribute(cols, hidden, max(avail, 0), flex)
		}
		hidden[drop] = true
	}
}

func distribute(cols []column, hidden []bool, avail, flex int) []int {
	widths := make([]int, len(cols))
	last := -1
	for i, c := range cols {
		if hidden[i] {
			continue
		}
		widths[i] = c.width
		if c.flex > 0 && flex > 0 {
			widths[i] = max(avail*c.flex/flex, 1)
			last = i
		}
	}
	if last >= 0 {
		used := 0
		for i, c := range cols {
			if c.flex > 0 && !hidden[i] {
				used += widths[i]
			}
		}
		widths[last] = max(widths[last]+avail-used, 1)
	}
	return widths
}

// renderRow renders cells into one line exactly total cells wide. A nil cell
// is covered by the visible cell before it, which spans its width.
func renderRow(cols []column, widths []int, cells []cell, base lipgloss.Style, total int) string {
	gap := base.Render(strings.Repeat(" ", colGap))
	var b strings.Builder
	b.WriteString(gap)
	for i := 0; i < len(cols); i++ {
		if widths[i] == 0 || cells[i] == nil {
			continue
		}
		w := widths[i]
		for j := i + 1; j < len(cols) && cells[j] == nil; j++ {
			if widths[j] > 0 {
				w += colGap + widths[j]
			}
		}
		align := lipgloss.Left
		if cols[i].right {
			align = lipgloss.Right
		}
		b.WriteString(cells[i](w, base.Align(align)))
		b.WriteString(gap)
	}
	line := ansi.Truncate(b.String(), total, "")
	if pad := total - ansi.StringWidth(line); pad > 0 {
		line += base.Render(strings.Repeat(" ", pad))
	}
	return line
}

func headerRow(cols []column, widths []int, total int) string {
	cells := make([]cell, len(cols))
	for i, c := range cols {
		cells[i] = textCell(c.title, headerStyle)
	}
	return renderRow(cols, widths, cells, lipgloss.NewStyle(), total)
}

// textCell truncates s with an ellipsis and pads it to the cell width.
func textCell(s string, style lipgloss.Style) cell {
	return func(w int, base lipgloss.Style) string {
		return style.Inherit(base).Width(w).Render(ansi.Truncate(s, w, "…"))
	}
}

// barCell is a static progress bar followed by the percentage.
func barCell(percent float64, fill color.Color) cell {
	return func(w int, base lipgloss.Style) string {
		pct := fmt.Sprintf("%3.0f%%", percent)
		barW := w - len(pct) - 1
		if barW < 3 {
			return base.Width(w).Render(ansi.Truncate(pct, w, ""))
		}
		filled := min(max(int(percent/100*float64(barW)+0.5), 0), barW)
		return base.Foreground(fill).Render(strings.Repeat("█", filled)) +
			base.Foreground(lipgloss.Color("238")).Render(strings.Repeat("░", barW-filled)) +
			base.Render(" "+pct)
	}
}
