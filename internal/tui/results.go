package tui

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/gfx"
	"github.com/jamaalsawyerd/squish/internal/stats"
	"github.com/jamaalsawyerd/squish/internal/units"
)

const (
	bandRows      = 7
	barCols       = 18
	lowSavings    = 20.0
	jumpHeight    = 4.0
	jumpFramesPer = 18
	maxNameCols   = 44
)

type resultsModel struct {
	app      *model
	rows     []engine.Result
	cursor   int
	offset   int
	done     int
	skipped  int
	failed   int
	canceled int
	totalIn  int64
	totalOut int64
	elapsed  time.Duration
	earned   []stats.Achievement
	tip      string

	fx    confetti
	frame int

	notice string
}

func newResults(app *model, c *crunchModel) *resultsModel {
	r := &resultsModel{app: app, elapsed: c.elapsed, rows: c.results}
	var best float64
	for _, x := range c.results {
		switch x.Status {
		case engine.StatusDone:
			r.done++
			r.totalIn += x.BytesIn
			r.totalOut += x.BytesOut
			best = max(best, units.Percent(x.BytesIn, x.BytesOut))
		case engine.StatusSkipped:
			r.skipped++
		case engine.StatusFailed:
			r.failed++
		case engine.StatusCanceled:
			r.canceled++
		}
	}
	slices.SortStableFunc(r.rows, func(a, b engine.Result) int {
		if a.Status != b.Status {
			return cmp.Compare(a.Status, b.Status)
		}
		return cmp.Compare(b.Saved(), a.Saved())
	})
	r.tip = r.findTip()
	if r.done > 0 {
		r.earned = app.stats.Add(stats.Batch{
			Saved: r.totalIn - r.totalOut, Files: r.done,
			Problems: r.skipped + r.failed + r.canceled, BestPercent: best,
		})
		if app.cfg.StatsPath != "" {
			_ = app.stats.Save(app.cfg.StatsPath) // Losing stats is not worth interrupting anyone.
		}
		if !app.cfg.NoAnim {
			r.fx.burst(app.width)
		}
	}
	return r
}

// findTip suggests a better option when a batch did poorly.
func (r *resultsModel) findTip() string {
	if r.app.opts.Target != engine.TargetSame {
		return ""
	}
	low := 0
	for _, x := range r.rows {
		isPNG := strings.EqualFold(filepath.Ext(x.Input), ".png")
		if isPNG && (x.Status == engine.StatusSkipped ||
			x.Status == engine.StatusDone && units.Percent(x.BytesIn, x.BytesOut) < lowSavings) {
			low++
		}
	}
	if low > 0 {
		return fmt.Sprintf("Tip: %s barely shrank. Format: Auto usually does much better on those.",
			units.Count(low, "PNG file"))
	}
	return ""
}

func (r *resultsModel) animating() bool { return r.fx.active() }

func (r *resultsModel) step() {
	r.frame++
	r.fx.step()
}

func (r *resultsModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case viewerMsg:
		r.notice = msg.notice()
	case tea.KeyPressMsg:
		return r.updateKeys(msg)
	}
	return nil
}

// showOutput opens the saved file, which needs no temporary copy.
func (r *resultsModel) showOutput() tea.Cmd {
	if r.cursor >= len(r.rows) || r.rows[r.cursor].Status != engine.StatusDone {
		return nil
	}
	path := r.rows[r.cursor].Output
	return func() tea.Msg { return viewerMsg{path: path, err: openFile(path)} }
}

func (r *resultsModel) updateKeys(msg tea.KeyPressMsg) tea.Cmd {
	r.notice = ""
	switch {
	case key.Matches(msg, keys.Quit):
		return tea.Quit
	case key.Matches(msg, keys.Up):
		r.cursor = max(r.cursor-1, 0)
	case key.Matches(msg, keys.Down):
		r.cursor = min(r.cursor+1, max(len(r.rows)-1, 0))
	case key.Matches(msg, keys.View):
		return r.showOutput()
	case key.Matches(msg, keys.Retune):
		return func() tea.Msg { return goRetuneMsg{} }
	case key.Matches(msg, keys.New):
		return func() tea.Msg { return goPickMsg{} }
	}
	return nil
}

func (r *resultsModel) band(w int) string {
	c := gfx.New(w, bandRows*2)
	bounce := 0.0
	if r.fx.active() {
		t := float64(r.frame%jumpFramesPer) / jumpFramesPer
		bounce = jumpHeight * 4 * t * (1 - t)
	}
	cx := float64(w) / 2
	mood := moodHappy
	if r.done == 0 {
		mood = moodDizzy
	}
	drawMascot(c, cx, float64(bandRows*2)-1-bounce, 6.5, 4.5, mood)
	drawParticles(c, r.fx.parts)
	return gfx.Render(c)
}

func (r *resultsModel) headline() string {
	th := r.app.th
	if r.done == 0 {
		return th.Bold.Render("Nothing got squished this time.")
	}
	saved := r.totalIn - r.totalOut
	return th.Good.Bold(true).Render("Saved "+units.Bytes(saved)) +
		th.Text.Render("  ·  "+units.PercentLabel(units.Percent(r.totalIn, r.totalOut))+" smaller  ·  ") +
		th.Accent.Render("that's "+units.Fun(saved)) +
		th.Muted.Render("  ·  "+r.elapsed.Round(100*time.Millisecond).String())
}

func (r *resultsModel) rowLine(x engine.Result, w int, selected bool) string {
	th := r.app.th
	pointer := "  "
	if selected {
		pointer = th.Key.Render("▸ ")
	}
	nameW := min(max(w-barCols-34, 12), maxNameCols)
	name := pad(truncateName(filepath.Base(x.Input), nameW), nameW+1)
	switch x.Status {
	case engine.StatusDone:
		pct := units.Percent(x.BytesIn, x.BytesOut)
		bar := gradientBar(barCols, 1-clamp01(pct/100), th.TrackColor)
		note := ""
		if len(x.Warnings) > 0 {
			note = th.Warn.Render("  ! " + x.Warnings[0])
		}
		return pointer + th.Text.Render(name) + bar + th.Muted.Render(
			fmt.Sprintf(" %8s → %-8s ", units.Bytes(x.BytesIn), units.Bytes(x.BytesOut))) +
			th.Good.Render(fmt.Sprintf("%4s", units.PercentLabel(pct))) + note
	case engine.StatusSkipped:
		return pointer + th.Muted.Render(name) + th.Warn.Render("skipped: "+x.Reason)
	case engine.StatusCanceled:
		return pointer + th.Muted.Render(name) + th.Faint.Render("canceled")
	}
	return pointer + th.Muted.Render(name) + th.Bad.Render(x.Reason)
}

func (r *resultsModel) view(w, h int) string {
	th := r.app.th
	head := lipgloss.PlaceHorizontal(w, lipgloss.Center, r.headline())
	var extras []string
	for _, a := range r.earned {
		extras = append(extras, " "+th.Warn.Render("★ "+a.Name)+th.Muted.Render("  "+a.Blurb))
	}
	if r.tip != "" {
		extras = append(extras, " "+th.Accent.Render(r.tip))
	}
	if r.skipped+r.failed+r.canceled > 0 {
		extras = append(extras, " "+th.Muted.Render(fmt.Sprintf("%d skipped  ·  %d failed  ·  %d canceled",
			r.skipped, r.failed, r.canceled)))
	}
	if r.notice != "" {
		extras = append(extras, noticeLines(th, r.notice, w)...)
	}
	listH := h - bandRows - 3 - len(extras)
	rows := max(listH-3, 1)
	if r.cursor < r.offset {
		r.offset = r.cursor
	}
	if r.cursor >= r.offset+rows {
		r.offset = r.cursor - rows + 1
	}
	var lines []string
	for i := r.offset; i < min(r.offset+rows, len(r.rows)); i++ {
		lines = append(lines, r.rowLine(r.rows[i], w-4, i == r.cursor))
	}
	list := r.app.panel(units.Count(len(r.rows), "file"), strings.Join(lines, "\n"), w, listH, false)
	parts := []string{r.band(w), head, list}
	parts = append(parts, extras...)
	parts = append(parts, r.app.footer(keys.Up, keys.View, keys.Retune, keys.New, keys.Quit))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
