// Package tui is squish's terminal interface.
package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/stats"
	"github.com/jamaalsawyerd/squish/internal/tui/theme"
	"github.com/jamaalsawyerd/squish/internal/units"
)

const (
	minWidth  = 74
	minHeight = 22
)

type screen int

const (
	screenPick screen = iota
	screenTune
	screenCrunch
	screenResults
)

// Config sets up the app.
type Config struct {
	Files     []string
	Options   engine.Options
	NoAnim    bool
	StatsPath string
}

// Summary is what the app leaves behind after it closes.
type Summary struct {
	Files   int
	In, Out int64
}

// String describes the session for printing after the app closes.
func (s Summary) String() string {
	if s.Files == 0 {
		return ""
	}
	return fmt.Sprintf("squish: saved %s across %d files (%s)",
		units.Bytes(s.In-s.Out), s.Files, units.Fun(s.In-s.Out))
}

// Run opens the app and blocks until it closes.
func Run(ctx context.Context, cfg Config) (Summary, error) {
	m := newModel(cfg)
	final, err := tea.NewProgram(m, tea.WithContext(ctx)).Run()
	if err != nil {
		return Summary{}, err
	}
	return final.(*model).summary, nil
}

type (
	goTuneMsg    struct{ files []string }
	goPickMsg    struct{}
	goCrunchMsg  struct{ opts engine.Options }
	goResultsMsg struct{ crunch *crunchModel }
	goRetuneMsg  struct{}
)

type model struct {
	cfg           Config
	th            theme.Theme
	help          help.Model
	screen        screen
	width, height int
	noColor       bool
	ticking       bool
	files         []string
	opts          engine.Options
	summary       Summary
	stats         stats.Stats

	pick    *pickModel
	tune    *tuneModel
	crunch  *crunchModel
	results *resultsModel
}

func newModel(cfg Config) *model {
	th := theme.New(true)
	m := &model{cfg: cfg, th: th, help: help.New(), opts: cfg.Options, width: 100, height: 30}
	m.applyTheme()
	m.pick = newPick(m)
	if cfg.StatsPath != "" {
		m.stats = stats.Load(cfg.StatsPath)
	}
	if len(cfg.Files) > 0 {
		m.files = cfg.Files
		m.tune = newTune(m, cfg.Files)
		m.screen = screenTune
	}
	return m
}

func (m *model) applyTheme() {
	s := m.help.Styles
	s.ShortKey = m.th.Key
	s.ShortDesc = m.th.Muted
	s.ShortSeparator = m.th.Faint
	m.help.Styles = s
}

func (m *model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.RequestBackgroundColor, m.startTick()}
	if m.screen == screenTune {
		cmds = append(cmds, m.tune.init())
	} else {
		cmds = append(cmds, m.pick.init())
	}
	return tea.Batch(cmds...)
}

// startTick runs the animation clock if something on screen is moving. Idle
// screens send no frames, so squish uses no CPU while waiting for a key.
func (m *model) startTick() tea.Cmd {
	if m.ticking || m.cfg.NoAnim || !m.animating() {
		return nil
	}
	m.ticking = true
	return tick()
}

func (m *model) animating() bool {
	switch m.screen {
	case screenPick:
		return m.pick.animating()
	case screenCrunch:
		return m.crunch.animating()
	case screenResults:
		return m.results.animating()
	}
	return false
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
	case tea.BackgroundColorMsg:
		m.th = theme.New(msg.IsDark())
		m.applyTheme()
	case tea.ColorProfileMsg:
		m.noColor = msg.Profile == colorprofile.NoTTY || msg.Profile == colorprofile.Ascii
	case tickMsg:
		m.ticking = false
		m.step()
		return m, m.startTick()
	case goTuneMsg:
		m.files = msg.files
		m.tune = newTune(m, msg.files)
		m.screen = screenTune
		return m, m.tune.init()
	case goPickMsg:
		m.screen = screenPick
		return m, m.pick.init()
	case goCrunchMsg:
		m.opts = msg.opts
		m.crunch = newCrunch(m, m.files, msg.opts)
		m.screen = screenCrunch
		return m, tea.Batch(m.crunch.init(), m.startTick())
	case goResultsMsg:
		m.results = newResults(m, msg.crunch)
		m.summary.Files += m.results.done
		m.summary.In += m.results.totalIn
		m.summary.Out += m.results.totalOut
		m.screen = screenResults
		return m, m.startTick()
	case goRetuneMsg:
		m.tune = newTune(m, m.files)
		m.screen = screenTune
		return m, m.tune.init()
	}

	var cmd tea.Cmd
	switch m.screen {
	case screenPick:
		cmd = m.pick.update(msg)
	case screenTune:
		cmd = m.tune.update(msg)
	case screenCrunch:
		cmd = m.crunch.update(msg)
	case screenResults:
		cmd = m.results.update(msg)
	}
	return m, cmd
}

func (m *model) step() {
	switch m.screen {
	case screenPick:
		m.pick.step()
	case screenCrunch:
		m.crunch.step()
	case screenResults:
		m.results.step()
	}
}

func (m *model) View() tea.View {
	v := tea.NewView(m.content())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "squish"
	return v
}

func (m *model) content() string {
	if m.width < minWidth || m.height < minHeight {
		return m.tooSmall()
	}
	var s string
	switch m.screen {
	case screenPick:
		s = m.pick.view(m.width, m.height)
	case screenTune:
		s = m.tune.view(m.width, m.height)
	case screenCrunch:
		s = m.crunch.view(m.width, m.height)
	case screenResults:
		s = m.results.view(m.width, m.height)
	}
	return lipgloss.NewStyle().MaxHeight(m.height).MaxWidth(m.width).Render(s)
}

func (m *model) tooSmall() string {
	msg := fmt.Sprintf("It's a bit squished in here!\nsquish needs %d×%d, and this is %d×%d.",
		minWidth, minHeight, m.width, m.height)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, mascotPortrait(14, 6, moodStrain), "", m.th.Text.Render(msg)))
}

func (m *model) footer(bindings ...key.Binding) string {
	return " " + m.help.ShortHelpView(bindings)
}

// panel draws a titled box of an exact outer size.
func (m *model) panel(title, body string, w, h int, active bool) string {
	style := m.th.Panel
	if active {
		style = m.th.PanelActive
	}
	inner := w - 4
	lines := strings.Split(body, "\n")
	if title != "" {
		lines = append([]string{m.th.PanelTitle.Render(title)}, lines...)
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, inner, "…")
	}
	if max := h - 2; len(lines) > max {
		lines = lines[:max]
	}
	return style.Width(w).Height(h).Render(strings.Join(lines, "\n"))
}

// truncateName shortens a file name, keeping its extension visible.
func truncateName(name string, w int) string {
	if ansi.StringWidth(name) <= w {
		return name
	}
	const tail = 6
	if w <= tail+1 {
		return ansi.Truncate(name, w, "…")
	}
	r := []rune(name)
	return ansi.Truncate(string(r[:len(r)-tail]), w-tail, "…") + string(r[len(r)-tail:])
}

// pad right-pads styled text to a width.
func pad(s string, w int) string {
	return s + strings.Repeat(" ", max(w-lipgloss.Width(s), 0))
}
