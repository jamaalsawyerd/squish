package tui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/tui/theme"
	"github.com/jamaalsawyerd/squish/internal/units"
)

type fileState int

const (
	fileWaiting fileState = iota
	fileActive
	fileDone
)

type engineMsg struct {
	ev engine.Event
	ok bool
}

const (
	etaMinProgress   = 0.25
	crunchStatusCols = 18 // Room for the marker and the size or status.
)

var quips = []string{
	"squish!", "pfffft", "so compact", "bytes begone", "hold my pixels",
	"mmph", "tiny now", "one more", "nearly flat", "whoosh",
}

type crunchModel struct {
	app      *model
	files    []string
	state    []fileState
	results  []engine.Result
	byIndex  map[int]engine.Result
	events   <-chan engine.Event
	cancel   context.CancelFunc
	stopping bool
	closed   bool
	start    time.Time
	elapsed  time.Duration

	done       int
	totalBytes int64
	doneBytes  int64
	savedBytes int64
	inBytes    int64
	outBytes   int64

	press   press
	counter spring
	bar     spring
	quip    string
	quipAt  int
}

func newCrunch(app *model, files []string, opts engine.Options) *crunchModel {
	ctx, cancel := context.WithCancel(context.Background())
	c := &crunchModel{
		app: app, files: files, state: make([]fileState, len(files)),
		byIndex: map[int]engine.Result{}, cancel: cancel, start: time.Now(),
		events: engine.Run(ctx, files, opts), press: newPress(),
		counter: newSpring(3, 0.9, 0), bar: newSpring(4, 0.9, 0), quip: quips[0],
	}
	c.press.running = true
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			c.totalBytes += st.Size()
		}
	}
	return c
}

func (c *crunchModel) waitEvent() tea.Cmd {
	ch := c.events
	return func() tea.Msg {
		ev, ok := <-ch
		return engineMsg{ev, ok}
	}
}

func (c *crunchModel) init() tea.Cmd { return c.waitEvent() }

func (c *crunchModel) animating() bool { return !c.closed }

func (c *crunchModel) step() {
	c.press.step()
	c.counter.to(float64(c.savedBytes))
	c.bar.to(c.progress())
}

func (c *crunchModel) progress() float64 {
	if c.totalBytes == 0 {
		return 0
	}
	return float64(c.doneBytes) / float64(c.totalBytes)
}

func (c *crunchModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case engineMsg:
		if !msg.ok {
			c.closed = true
			c.press.running = false
			c.elapsed = time.Since(c.start)
			return func() tea.Msg { return goResultsMsg{c} }
		}
		c.handle(msg.ev)
		return c.waitEvent()
	case tea.KeyPressMsg:
		if key.Matches(msg, keys.Quit) || key.Matches(msg, keys.Back) {
			c.stopping = true
			c.cancel()
		}
	}
	return nil
}

func (c *crunchModel) handle(ev engine.Event) {
	switch ev.Type {
	case engine.FileStarted:
		c.state[ev.Index] = fileActive
		if ev.Thumb != nil {
			c.press.next = ev.Thumb
			if c.press.thumb == nil {
				c.press.thumb, c.press.next = ev.Thumb, nil
			}
		}
	case engine.FileFinished:
		r := ev.Result
		c.state[ev.Index] = fileDone
		c.byIndex[ev.Index] = r
		c.results = append(c.results, r)
		c.done++
		c.doneBytes += r.BytesIn
		c.savedBytes += r.Saved()
		if r.Status == engine.StatusDone {
			c.inBytes += r.BytesIn
			c.outBytes += r.BytesOut
		}
		c.quipAt = (c.quipAt + 1) % len(quips)
		c.quip = quips[c.quipAt]
		if c.app.cfg.NoAnim {
			c.counter.pos, c.bar.pos = float64(c.savedBytes), c.progress()
		}
	}
}

// gradientBar draws a progress bar that fades from hot to cool.
func gradientBar(w int, f float64, track color.Color) string {
	filled := int(float64(w) * clamp01(f))
	stops := theme.Gradient(w)
	var b strings.Builder
	for i := range filled {
		b.WriteString(lipgloss.NewStyle().Foreground(stops[i]).Render("█"))
	}
	b.WriteString(lipgloss.NewStyle().Foreground(track).Render(strings.Repeat("░", w-filled)))
	return b.String()
}

func pickGradient(n int) []color.Color { return theme.Gradient(n) }

func (c *crunchModel) eta() string {
	// Early on, one tiny file finishing says little about the time remaining.
	if c.closed || c.progress() < etaMinProgress {
		return ""
	}
	el := time.Since(c.start)
	left := time.Duration(float64(el) * (float64(c.totalBytes)/float64(c.doneBytes) - 1))
	return "about " + left.Round(time.Second).String() + " left"
}

func (c *crunchModel) listBody(w, rows int) string {
	th := c.app.th
	// Keep the work in progress in view as the list scrolls.
	first := 0
	for i, s := range c.state {
		if s != fileDone {
			first = i
			break
		}
		first = i
	}
	offset := max(0, min(first-rows/3, len(c.files)-rows))
	spin := []string{"◐", "◓", "◑", "◒"}[int(time.Since(c.start)/(120*time.Millisecond))%4]
	nameW := max(w-crunchStatusCols, 8)
	var lines []string
	for i := offset; i < min(offset+rows, len(c.files)); i++ {
		name := truncateName(filepath.Base(c.files[i]), nameW)
		nameCol := pad(name, nameW+1)
		switch c.state[i] {
		case fileWaiting:
			lines = append(lines, th.Faint.Render("· "+nameCol))
		case fileActive:
			lines = append(lines, th.Accent.Render(spin+" ")+th.Text.Render(nameCol)+th.Faint.Render("squishing"))
		case fileDone:
			r := c.byIndex[i]
			switch r.Status {
			case engine.StatusDone:
				lines = append(lines, th.Good.Render("✓ ")+th.Muted.Render(nameCol)+
					th.Text.Render(units.Bytes(r.BytesOut))+
					th.Good.Render("  -"+units.PercentLabel(units.Percent(r.BytesIn, r.BytesOut))))
			case engine.StatusSkipped:
				lines = append(lines, th.Warn.Render("- ")+th.Muted.Render(nameCol)+th.Faint.Render(r.Reason))
			case engine.StatusCanceled:
				lines = append(lines, th.Faint.Render("· "+nameCol+"canceled"))
			default:
				lines = append(lines, th.Bad.Render("✗ ")+th.Muted.Render(nameCol)+th.Bad.Render(r.Reason))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (c *crunchModel) view(w, h int) string {
	th := c.app.th
	title := " " + th.Key.Render("squish") + th.Muted.Render(" › squishing")
	saved := units.Bytes(int64(c.counter.pos))
	right := th.Bold.Render("saved " + saved)
	head := pad(title, w-lipgloss.Width(right)-1) + right

	bodyH := h - 5
	pressW := min(w/2, 50)
	listW := w - pressW
	list := c.app.panel(fmt.Sprintf("%d of %d", c.done, len(c.files)), c.listBody(listW-4, bodyH-3), listW, bodyH, false)

	cols, rows := pressW-4, bodyH-4
	c.press.w, c.press.h = cols, rows*2
	scene := c.press.view()
	caption := lipgloss.PlaceHorizontal(cols, lipgloss.Center, th.Accent.Render(c.quip))
	press := c.app.panel("", scene+"\n"+caption, pressW, bodyH, true)

	barW := w - 2
	info := th.Muted.Render(fmt.Sprintf(" %.0f%%  %s", c.bar.pos*100, c.eta()))
	if c.stopping {
		info = th.Warn.Render(" Stopping after the files in progress...")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		head, lipgloss.JoinHorizontal(lipgloss.Top, list, press),
		" "+gradientBar(barW, c.bar.pos, th.TrackColor), info, c.app.footer(keys.Back))
}
