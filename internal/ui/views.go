package ui

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strconv"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/chriha/vidl/internal/queue"
)

var statusColors = map[queue.Status]color.Color{
	queue.Queued:      lipgloss.Color("245"),
	queue.Downloading: lipgloss.Color("12"),
	queue.Paused:      lipgloss.Color("11"),
	queue.Failed:      lipgloss.Color("9"),
	queue.Cancelled:   lipgloss.Color("240"),
	queue.Finished:    lipgloss.Color("10"),
}

var downloadColumns = []column{
	{title: "#", width: 3, right: true},
	{title: "Title", flex: 1, min: 16},
	{title: "Status", width: 11, drop: 1}, // the bar's color still shows it
	{title: "Progress", width: 22},
	{title: "Speed", width: 10, right: true, drop: 2},
	{title: "ETA", width: 7, right: true, drop: 3},
	{title: "Size", width: 9, right: true, drop: 4},
}

func downloadCells(i int, it queue.Item) []cell {
	p := it.Progress
	fg := statusColors[it.Status]
	speed, eta, size := "", "", "–"
	if it.Status == queue.Downloading {
		speed, eta = humanSpeed(p.Speed), humanDuration(p.ETA)
	}
	if p.Total > 0 {
		size = humanBytes(p.Total)
	}
	cells := []cell{
		textCell(strconv.Itoa(i+1), dimStyle),
		textCell(it.DisplayTitle(), lipgloss.NewStyle()),
		textCell(it.Status.String(), lipgloss.NewStyle().Foreground(fg)),
		barCell(p.Percent, fg),
		textCell(speed, lipgloss.NewStyle()),
		textCell(eta, lipgloss.NewStyle()),
		textCell(size, lipgloss.NewStyle()),
	}
	if it.Status == queue.Downloading && p.Stage != "" {
		// Post-processing: the stage replaces speed and ETA.
		cells[4], cells[5] = textCell(p.Stage, dimStyle), nil
	}
	return cells
}

var finishedColumns = []column{
	{title: "Title", flex: 2, min: 16},
	{title: "Format", width: 6, drop: 1},
	{title: "Res", width: 5, right: true, drop: 2},
	{title: "Size", width: 9, right: true, drop: 3},
	{title: "Finished", width: 12},
	{title: "File", flex: 1, min: 12, drop: 4},
}

func finishedCells(_ int, it queue.Item) []cell {
	res, size, file := "–", "–", ""
	if l := resolutionLabel(it); l != "" {
		res = l
	}
	if it.Size > 0 {
		size = humanBytes(it.Size)
	}
	if it.FilePath != "" {
		file = filepath.Base(it.FilePath)
	}
	plain := lipgloss.NewStyle()
	return []cell{
		textCell(it.DisplayTitle(), plain),
		textCell(string(it.Format), plain),
		textCell(res, plain),
		textCell(size, plain),
		textCell(humanTime(it.FinishedAt, time.Now()), plain),
		textCell(file, dimStyle),
	}
}

// humanTime is a relative time for the last day, otherwise a local date.
func humanTime(t, now time.Time) string {
	if t.IsZero() {
		return "–"
	}
	switch d := now.Sub(t); {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case t.Year() == now.Year():
		return t.Local().Format("Jan 2 15:04")
	}
	return t.Local().Format("2006-01-02")
}
