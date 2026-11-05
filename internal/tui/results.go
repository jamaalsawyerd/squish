package tui

import (
	"cmp"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jamaalsawyerd/squish/internal/codec"
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

type compareLoadedMsg struct {
	path          string
	before, after *image.NRGBA
	alpha         bool
	err           error
}

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

	comparing      bool
	cmpName        string
	cmpFull        [2]*image.NRGBA
	cmp            pane
	focusX, focusY float64
	cmpErr         string
}

func newResults(app *model, c *crunchModel) *resultsModel {
	r := &resultsModel{app: app, elapsed: c.elapsed, rows: c.results, cmp: newPane(), focusX: 0.5, focusY: 0.5}
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

func loadCompare(res engine.Result) tea.Cmd {
	return func() tea.Msg {
		msg := compareLoadedMsg{path: res.Input}
		src, err := os.ReadFile(res.Input)
		if err != nil {
			msg.err = err
			return msg
		}
		before, err := codec.Decode(src)
		if err != nil {
			msg.err = err
			return msg
		}
		out, err := os.ReadFile(res.Output)
		if err != nil {
			msg.err = err
			return msg
		}
		after, err := codec.Decode(out)
		if err != nil {
			msg.err = err
			return msg
		}
		// Resized output is compared at its own, final size.
		msg.before = codec.Shrink(before.Image, after.Image.Rect.Dx(), after.Image.Rect.Dy())
		msg.after = after.Image
		msg.alpha = !before.Opaque
		return msg
	}
}

func (r *resultsModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case compareLoadedMsg:
		if !r.comparing {
			return nil
		}
		if msg.err != nil {
			r.cmpErr = "Can't open these files to compare them."
			return nil
		}
		r.cmpFull = [2]*image.NRGBA{msg.before, msg.after}
		r.cmp.alpha = msg.alpha
		r.setRegion()
	case tea.KeyPressMsg:
		if r.comparing {
			return r.updateCompare(msg)
		}
		return r.updateKeys(msg)
	case tea.MouseClickMsg:
		r.dragWipe(msg.Mouse().X)
	case tea.MouseMotionMsg:
		if msg.Mouse().Button == tea.MouseLeft {
			r.dragWipe(msg.Mouse().X)
		}
	}
	return nil
}

func (r *resultsModel) dragWipe(x int) {
	if r.comparing {
		r.cmp.split = clamp01(float64(x) / float64(r.app.width))
	}
}

// setRegion picks what the compare pane shows at the current zoom.
func (r *resultsModel) setRegion() {
	b, a := r.cmpFull[0], r.cmpFull[1]
	if b == nil {
		return
	}
	z := r.cmp.zoomFactor()
	if z == 0 {
		r.cmp.before, r.cmp.after = b, a
		return
	}
	w, h := (r.app.width-2)/z+regionPad, (r.app.height-3)*2/z+regionPad
	r.cmp.before = cropAt(b, r.focusX, r.focusY, w, h)
	r.cmp.after = cropAt(a, r.focusX, r.focusY, w, h)
}

func (r *resultsModel) updateCompare(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case msg.String() == "ctrl+c":
		return tea.Quit
	case key.Matches(msg, keys.Quit), key.Matches(msg, keys.Back), key.Matches(msg, keys.Compare):
		r.comparing = false
		r.cmpFull = [2]*image.NRGBA{}
		r.cmp = newPane()
	case key.Matches(msg, keys.Left):
		r.cmp.nudgeSplit(-wipeStep)
	case key.Matches(msg, keys.Right):
		r.cmp.nudgeSplit(wipeStep)
	case key.Matches(msg, keys.Zoom):
		r.cmp.cycleZoom()
		r.setRegion()
	case key.Matches(msg, keys.PanUp):
		r.focusY = clamp01(r.focusY - panStep)
		r.setRegion()
	case key.Matches(msg, keys.PanDown):
		r.focusY = clamp01(r.focusY + panStep)
		r.setRegion()
	case key.Matches(msg, keys.PanLeft):
		r.focusX = clamp01(r.focusX - panStep)
		r.setRegion()
	case key.Matches(msg, keys.PanRight):
		r.focusX = clamp01(r.focusX + panStep)
		r.setRegion()
	}
	return nil
}

func (r *resultsModel) updateKeys(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Quit):
		return tea.Quit
	case key.Matches(msg, keys.Up):
		r.cursor = max(r.cursor-1, 0)
	case key.Matches(msg, keys.Down):
		r.cursor = min(r.cursor+1, max(len(r.rows)-1, 0))
	case key.Matches(msg, keys.Compare):
		if r.cursor < len(r.rows) && r.rows[r.cursor].Status == engine.StatusDone {
			res := r.rows[r.cursor]
			r.comparing, r.cmpErr = true, ""
			r.cmpName = filepath.Base(res.Input)
			r.focusX, r.focusY = 0.5, 0.5
			return loadCompare(res)
		}
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
	if r.comparing {
		return r.compareView(w, h)
	}
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
	parts = append(parts, r.app.footer(keys.Up, keys.Compare, keys.Retune, keys.New, keys.Quit))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (r *resultsModel) compareView(w, h int) string {
	th := r.app.th
	title := " " + th.Key.Render("squish") + th.Muted.Render(" › compare › ") + th.Text.Render(r.cmpName)
	if z := r.cmp.zoomFactor(); z > 0 {
		title += th.Muted.Render(fmt.Sprintf("  at %d:1", z))
	}
	body := ""
	if r.cmpErr != "" {
		body = lipgloss.Place(w, h-2, lipgloss.Center, lipgloss.Center, th.Warn.Render(r.cmpErr))
	} else {
		body = r.cmp.view(w, h-2)
	}
	hint := r.app.footer(keys.Left, keys.Zoom, keys.PanUp, keys.Back)
	return lipgloss.JoinVertical(lipgloss.Left, title, body, hint)
}
