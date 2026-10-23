package ui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chriha/vidl/internal/config"
	"github.com/chriha/vidl/internal/ytdlp"
)

type settingKind int

const (
	kindText settingKind = iota // edited inline with a text input
	kindInt                     // h/l decrement/increment within [lo, hi]
	kindEnum                    // h/l cycle through options
	kindBool                    // space/enter toggle
)

// settingLabelW is the width of the label column in the settings form.
const settingLabelW = 23

var (
	errMissingDir = errors.New("folder doesn't exist")
	rateLimitRe   = regexp.MustCompile(`^\d+(\.\d+)?[KMG]?$`)
)

// setting is one editable config field. Values are passed around as strings
// so every kind shares the same get/set.
type setting struct {
	label    string
	help     string
	kind     settingKind
	get      func(config.Config) string
	set      func(*config.Config, string)
	options  []string                     // kindEnum
	lo, hi   int                          // kindInt
	empty    string                       // shown for an empty value
	show     func(string) string          // optional display form of a value
	validate func(string) (string, error) // kindText: the value to store
}

var settings = []setting{
	{
		label: "Download folder", help: "Where files are saved. ~ is expanded.", kind: kindText,
		get:      func(c config.Config) string { return c.DownloadDir },
		set:      func(c *config.Config, v string) { c.DownloadDir = v },
		validate: validateDir,
	},
	{
		label: "Concurrent downloads", help: "Downloads running at the same time.", kind: kindInt,
		lo: config.MinConcurrency, hi: config.MaxConcurrency,
		get: func(c config.Config) string { return strconv.Itoa(c.Concurrency) },
		set: func(c *config.Config, v string) { c.Concurrency, _ = strconv.Atoi(v) },
	},
	{
		label: "Default resolution", help: "Maximum video height for new downloads.", kind: kindEnum,
		options: ytdlp.Resolutions, show: resolutionName,
		get: func(c config.Config) string { return c.DefaultResolution },
		set: func(c *config.Config, v string) { c.DefaultResolution = v },
	},
	{
		label: "Default file type", help: "mp4, mkv, webm, or audio only: mp3, m4a, opus.", kind: kindEnum,
		options: formatNames(),
		get:     func(c config.Config) string { return string(c.DefaultFormat) },
		set:     func(c *config.Config, v string) { c.DefaultFormat = ytdlp.Format(v) },
	},
	{
		label: "Filename template", help: "yt-dlp output template, relative to the download folder.", kind: kindText,
		get:      func(c config.Config) string { return c.FilenameTemplate },
		set:      func(c *config.Config, v string) { c.FilenameTemplate = v },
		validate: notEmpty,
	},
	{
		label: "Embed subtitles", help: "Embed subtitles into video files.", kind: kindBool,
		get: func(c config.Config) string { return strconv.FormatBool(c.EmbedSubtitles) },
		set: func(c *config.Config, v string) { c.EmbedSubtitles = v == "true" },
	},
	{
		label: "Subtitle languages", help: "yt-dlp --sub-langs syntax, e.g. en.*,de", kind: kindText,
		empty:    "yt-dlp default",
		get:      func(c config.Config) string { return c.SubtitleLangs },
		set:      func(c *config.Config, v string) { c.SubtitleLangs = v },
		validate: func(v string) (string, error) { return strings.TrimSpace(v), nil },
	},
	{
		label: "Embed thumbnail", help: "Embed the thumbnail as cover art.", kind: kindBool,
		get: func(c config.Config) string { return strconv.FormatBool(c.EmbedThumbnail) },
		set: func(c *config.Config, v string) { c.EmbedThumbnail = v == "true" },
	},
	{
		label: "Embed metadata", help: "Write title, uploader, date, etc. into the file.", kind: kindBool,
		get: func(c config.Config) string { return strconv.FormatBool(c.EmbedMetadata) },
		set: func(c *config.Config, v string) { c.EmbedMetadata = v == "true" },
	},
	{
		label: "Rate limit", help: "Maximum rate per download, e.g. 500K or 5M. Empty for unlimited.", kind: kindText,
		empty:    "unlimited",
		get:      func(c config.Config) string { return c.RateLimit },
		set:      func(c *config.Config, v string) { c.RateLimit = v },
		validate: validateRateLimit,
	},
	{
		label: "Cookies from browser", help: "For private, members-only or age-restricted videos.", kind: kindEnum,
		options: config.Browsers, empty: "none",
		get: func(c config.Config) string { return c.CookiesFromBrowser },
		set: func(c *config.Config, v string) { c.CookiesFromBrowser = v },
	},
	{
		label: "Desktop notifications", help: "Notify when a download finishes or fails.", kind: kindBool,
		get: func(c config.Config) string { return strconv.FormatBool(c.Notify) },
		set: func(c *config.Config, v string) { c.Notify = v == "true" },
	},
}

func formatNames() []string {
	names := make([]string, len(ytdlp.Formats))
	for i, f := range ytdlp.Formats {
		names[i] = string(f)
	}
	return names
}

func notEmpty(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", errors.New("can't be empty")
	}
	return v, nil
}

// validateDir accepts an absolute path (after expanding ~) to an existing
// folder; a missing one returns errMissingDir.
func validateDir(v string) (string, error) {
	v, err := notEmpty(v)
	if err != nil {
		return "", err
	}
	p := config.ExpandHome(v)
	if !filepath.IsAbs(p) {
		return "", errors.New("use an absolute path or one starting with ~")
	}
	fi, err := os.Stat(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return v, errMissingDir
	case err != nil:
		return "", err
	case !fi.IsDir():
		return "", errors.New("not a folder")
	}
	return v, nil
}

func validateRateLimit(v string) (string, error) {
	v = strings.ToUpper(strings.TrimSpace(v))
	if v != "" && !rateLimitRe.MatchString(v) {
		return "", errors.New("use a number with an optional K, M or G suffix, e.g. 500K")
	}
	return v, nil
}

// settingsForm is the state of the Settings tab.
type settingsForm struct {
	cursor int
	edit   *textinput.Model // inline editor for a text setting, nil when closed
	err    error            // validation error of the edited value
	mkdir  string           // download folder awaiting confirmation to create
}

// handleSettingsKey handles keys on the Settings tab; ok is false for keys
// it leaves to the global bindings.
func (m Model) handleSettingsKey(msg tea.KeyPressMsg) (_ Model, _ tea.Cmd, ok bool) {
	f := &m.settings
	s := settings[f.cursor]
	if msg.String() == "ctrl+c" {
		return m, nil, false
	}
	if f.mkdir != "" {
		switch msg.String() {
		case "y", "Y":
			dir := f.mkdir
			f.mkdir = ""
			if err := os.MkdirAll(config.ExpandHome(dir), 0o755); err != nil {
				f.err = err
				break
			}
			f.edit, f.err = nil, nil
			m.applySetting(s, dir)
		case "n", "N", "esc":
			f.mkdir, f.err = "", nil
		}
		return m, nil, true
	}
	if f.edit != nil {
		switch msg.String() {
		case "esc":
			f.edit, f.err = nil, nil
		case "enter":
			v, err := s.validate(f.edit.Value())
			switch {
			case errors.Is(err, errMissingDir):
				f.mkdir, f.err = v, nil
			case err != nil:
				f.err = err
			default:
				f.edit, f.err = nil, nil
				m.applySetting(s, v)
			}
		default:
			var cmd tea.Cmd
			*f.edit, cmd = f.edit.Update(msg)
			f.err = nil
			return m, cmd, true
		}
		return m, nil, true
	}

	k := m.keys
	switch {
	case key.Matches(msg, k.Up):
		f.cursor = max(f.cursor-1, 0)
	case key.Matches(msg, k.Down):
		f.cursor = min(f.cursor+1, len(settings)-1)
	case key.Matches(msg, k.Top):
		f.cursor = 0
	case key.Matches(msg, k.Bottom):
		f.cursor = len(settings) - 1
	case key.Matches(msg, k.Edit):
		if s.kind == kindText {
			return m, m.openSettingEditor(s), true
		}
		m.adjustSetting(s, 1)
	case key.Matches(msg, k.Toggle):
		if s.kind == kindBool {
			m.adjustSetting(s, 1)
		}
	case key.Matches(msg, k.Decrease):
		m.adjustSetting(s, -1)
	case key.Matches(msg, k.Increase):
		m.adjustSetting(s, 1)
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m *Model) openSettingEditor(s setting) tea.Cmd {
	in := textinput.New()
	in.Prompt = ""
	in.SetValue(s.get(m.mgr.Config()))
	in.CursorEnd()
	in.SetWidth(m.settingValueWidth() - 1)
	m.settings.edit, m.settings.err = &in, nil
	return m.settings.edit.Focus()
}

// adjustSetting steps a non-text setting by delta: integers are clamped,
// enums wrap around and booleans toggle.
func (m *Model) adjustSetting(s setting, delta int) {
	cur := s.get(m.mgr.Config())
	var v string
	switch s.kind {
	case kindInt:
		n, _ := strconv.Atoi(cur)
		v = strconv.Itoa(min(max(n+delta, s.lo), s.hi))
	case kindEnum:
		i := max(slices.Index(s.options, cur), 0)
		v = s.options[(i+delta+len(s.options))%len(s.options)]
	case kindBool:
		v = strconv.FormatBool(cur != "true")
	default:
		return
	}
	if v != cur {
		m.applySetting(s, v)
	}
}

// applySetting stores v, saves the config file and hands the config to the
// manager, which uses it for downloads started from now on.
func (m *Model) applySetting(s setting, v string) {
	cfg := m.mgr.Config()
	s.set(&cfg, v)
	cfg = cfg.Normalize()
	m.mgr.SetConfig(cfg)
	if err := config.Save(m.cfgPath, cfg); err != nil {
		m.report(fmt.Errorf("saving settings: %w", err))
		return
	}
	m.report(nil)
}

func (m Model) settingValueWidth() int {
	return max(m.width-2-settingLabelW-1, 8)
}

func (m Model) settingsView(height int) string {
	f := m.settings
	cfg := m.mgr.Config()
	valueW := m.settingValueWidth()

	var status string
	switch {
	case f.mkdir != "":
		status = errorStyle.Render("Folder doesn't exist.") + " Create it? y/n"
	case f.err != nil:
		status = errorStyle.Render(f.err.Error())
	case f.edit != nil:
		status = dimStyle.Render("enter save · esc cancel")
	default:
		status = dimStyle.Render(settings[f.cursor].help)
	}
	trailer := []string{
		"",
		" " + status,
		"",
		dimStyle.Render(" Applies to downloads started from now on."),
		dimStyle.Render(" Config file: " + m.cfgPath),
	}

	// Keep the cursor visible when the form doesn't fit.
	rows := min(max(height-len(trailer), 1), len(settings))
	start := min(max(f.cursor-rows/2, 0), len(settings)-rows)

	var lines []string
	for i := start; i < start+rows; i++ {
		s := settings[i]
		focused := i == f.cursor
		var value string
		if focused && f.edit != nil {
			value = f.edit.View()
		} else {
			value = ansi.Truncate(settingValue(s, cfg, focused), valueW, "…")
		}
		marker, base := "  ", lipgloss.NewStyle()
		if focused {
			marker, base = "› ", cursorStyle
		}
		label := base.Width(settingLabelW).Render(s.label)
		line := base.Render(marker) + label + base.Render(" ") + value
		if focused && f.edit == nil {
			line += base.Render(strings.Repeat(" ", max(m.width-lipgloss.Width(line), 0)))
		}
		lines = append(lines, ansi.Truncate(line, m.width, "…"))
	}
	for i, l := range trailer {
		trailer[i] = ansi.Truncate(l, m.width, "…")
	}
	return strings.Join(append(lines, trailer...), "\n")
}

// settingValue renders the current value of s; adjustable values of the
// focused setting get arrows hinting at h/l.
func settingValue(s setting, cfg config.Config, focused bool) string {
	v := s.get(cfg)
	style := lipgloss.NewStyle()
	if focused {
		style = cursorStyle
	}
	switch s.kind {
	case kindBool:
		if v == "true" {
			return style.Render("[x] on")
		}
		return style.Render("[ ] off")
	case kindEnum, kindInt:
		switch {
		case v == "":
			v = s.empty
		case s.show != nil:
			v = s.show(v)
		}
		if focused {
			return style.Render("‹ " + v + " ›")
		}
		return v
	}
	if v == "" {
		return dimStyle.Render(s.empty)
	}
	return style.Render(v)
}
