package tui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jamaalsawyerd/squish/internal/codec"
	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/units"
)

const (
	settingFormat = iota
	settingLevel
	settingSize
	settingMeta
	settingSave
	settingCount
)

const (
	saveBeside = iota
	saveFolder
	saveReplace
	saveCount
)

const (
	outFolderName = "squished"
	debounce      = 140 * time.Millisecond
	previewMax    = 1024
	tuneListW     = 44
	wipeStep      = 0.05
	panStep       = 0.04
	regionPad     = 8
)

var (
	formatChoices = []engine.Target{
		engine.TargetSame, engine.TargetAuto, engine.TargetJPEG,
		engine.TargetPNG, engine.TargetWebP, engine.TargetAVIF,
	}
	sizeChoices = []int{0, 3840, 2560, 1920, 1280, 800}
)

type decodedMsg struct {
	path string
	dec  *codec.Decoded
	size int64
	err  error
}

type debounceMsg struct{ gen int }

type previewMsg struct {
	gen           int
	before, after *image.NRGBA
	est           engine.Output
}

type tuneModel struct {
	app        *model
	files      []string
	index      int
	row        int
	wiping     bool
	confirming bool

	format, level, size, save int
	keepMeta                  bool

	dec        *codec.Decoded
	fileSize   int64
	loadErr    string
	pane       pane
	focusX     float64
	focusY     float64
	gen        int
	est        engine.Output
	hasEst     bool
	estimating bool
	paneCols   int
	paneRows   int
	paneOrigin image.Point
}

func newTune(app *model, files []string) *tuneModel {
	t := &tuneModel{app: app, files: files, pane: newPane(), focusX: 0.5, focusY: 0.5}
	o := app.opts
	t.level = o.Level
	if t.level == 0 {
		t.level = codec.LevelBalanced
	}
	for i, f := range formatChoices {
		if f == o.Target {
			t.format = i
		}
	}
	for i, s := range sizeChoices {
		if s == o.MaxDim {
			t.size = i
		}
	}
	t.keepMeta = o.KeepMetadata
	switch {
	case o.InPlace:
		t.save = saveReplace
	case o.OutDir != "":
		t.save = saveFolder
	}
	return t
}

func (t *tuneModel) init() tea.Cmd { return t.loadFile() }

func (t *tuneModel) loadFile() tea.Cmd {
	path := t.files[t.index]
	t.dec, t.loadErr, t.hasEst = nil, "", false
	t.pane.before, t.pane.after = nil, nil
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return decodedMsg{path: path, err: err}
		}
		dec, err := codec.Decode(data)
		return decodedMsg{path: path, dec: dec, size: int64(len(data)), err: err}
	}
}

// options turns the on-screen settings into engine options.
func (t *tuneModel) options() engine.Options {
	o := t.app.opts
	o.Target = formatChoices[t.format]
	o.Level = t.level
	o.Quality = 0
	o.MaxDim = sizeChoices[t.size]
	o.KeepMetadata = t.keepMeta
	o.OutDir, o.InPlace = "", false
	switch t.save {
	case saveFolder:
		o.OutDir = filepath.Join(filepath.Dir(t.files[0]), outFolderName)
	case saveReplace:
		o.InPlace = true
	}
	o.ThumbSize = thumbSize
	return o
}

// refresh asks for a new preview after a short pause, so that holding a key
// does not queue up a pile of encodes.
func (t *tuneModel) refresh() tea.Cmd {
	t.gen++
	t.estimating = true
	gen := t.gen
	return tea.Tick(debounce, func(time.Time) tea.Msg { return debounceMsg{gen} })
}

func (t *tuneModel) buildPreview() tea.Cmd {
	gen, dec, opts := t.gen, t.dec, t.options()
	zoom := t.pane.zoomFactor()
	fx, fy, cols, rows := t.focusX, t.focusY, t.paneCols, t.paneRows
	return func() tea.Msg {
		// Resize first, so the preview shows what the output will really be.
		base := dec.Image
		if opts.MaxDim > 0 {
			base = codec.Shrink(base, opts.MaxDim, opts.MaxDim)
		}
		var src *image.NRGBA
		if zoom == 0 {
			src = codec.Shrink(base, previewMax, previewMax)
		} else {
			src = cropAt(base, fx, fy, cols/zoom+regionPad, rows*2/zoom+regionPad)
		}
		sample := *dec
		sample.Image, sample.ICC, sample.Exif = src, nil, nil
		o := opts
		o.MaxDim = 0
		msg := previewMsg{gen: gen, before: src, est: engine.Estimate(dec, opts)}
		if out := engine.Compress(&sample, o); out.Err == nil {
			if d, err := codec.Decode(out.Data); err == nil {
				msg.after = d.Image
			}
		}
		return msg
	}
}

// cropAt cuts out a region centered on a relative position.
func cropAt(m *image.NRGBA, fx, fy float64, w, h int) *image.NRGBA {
	iw, ih := m.Rect.Dx(), m.Rect.Dy()
	w, h = min(w, iw), min(h, ih)
	x := max(0, min(int(fx*float64(iw))-w/2, iw-w))
	y := max(0, min(int(fy*float64(ih))-h/2, ih-h))
	return codec.Crop(m, image.Rect(x, y, x+w, y+h))
}

func (t *tuneModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case decodedMsg:
		if msg.path != t.files[t.index] {
			return nil
		}
		if msg.err != nil {
			t.loadErr = "Can't preview this file, but it will still be tried."
			return nil
		}
		t.dec, t.fileSize = msg.dec, msg.size
		t.pane.before = codec.Shrink(msg.dec.Image, previewMax, previewMax)
		t.pane.alpha = !msg.dec.Opaque
		return t.refresh()
	case debounceMsg:
		if msg.gen == t.gen && t.dec != nil {
			return t.buildPreview()
		}
	case previewMsg:
		if msg.gen != t.gen {
			return nil
		}
		t.estimating = false
		t.pane.before, t.pane.after = msg.before, msg.after
		t.est, t.hasEst = msg.est, msg.est.Err == nil
	case tea.KeyPressMsg:
		return t.updateKeys(msg)
	case tea.MouseClickMsg:
		t.dragWipe(msg.Mouse().X, msg.Mouse().Y)
	case tea.MouseMotionMsg:
		if msg.Mouse().Button == tea.MouseLeft {
			t.dragWipe(msg.Mouse().X, msg.Mouse().Y)
		}
	case tea.MouseWheelMsg:
		t.pane.cycleZoom()
		return t.refresh()
	}
	return nil
}

func (t *tuneModel) dragWipe(x, y int) {
	if t.paneCols == 0 {
		return
	}
	dx, dy := x-t.paneOrigin.X, y-t.paneOrigin.Y
	if dy >= 0 && dy < t.paneRows && dx >= 0 && dx < t.paneCols {
		t.pane.split = float64(dx) / float64(t.paneCols)
	}
}

func (t *tuneModel) updateKeys(msg tea.KeyPressMsg) tea.Cmd {
	if t.confirming {
		switch {
		case key.Matches(msg, keys.Yes):
			t.confirming = false
			return t.start()
		case key.Matches(msg, keys.No):
			t.confirming = false
		}
		return nil
	}
	switch {
	case msg.String() == "ctrl+c":
		return tea.Quit
	case key.Matches(msg, keys.Quit):
		return tea.Quit
	case key.Matches(msg, keys.Back):
		return func() tea.Msg { return goPickMsg{} }
	case key.Matches(msg, keys.Start):
		if t.save == saveReplace {
			t.confirming = true
			return nil
		}
		return t.start()
	case key.Matches(msg, keys.Wipe):
		t.wiping = !t.wiping
	case key.Matches(msg, keys.Zoom):
		t.pane.cycleZoom()
		return t.refresh()
	case key.Matches(msg, keys.Prev):
		return t.stepFile(-1)
	case key.Matches(msg, keys.NextFile):
		return t.stepFile(1)
	}
	if t.wiping {
		return t.updateWipe(msg)
	}
	switch {
	case key.Matches(msg, keys.Up):
		t.row = (t.row + settingCount - 1) % settingCount
	case key.Matches(msg, keys.Down):
		t.row = (t.row + 1) % settingCount
	case key.Matches(msg, keys.Left):
		return t.change(-1)
	case key.Matches(msg, keys.Right):
		return t.change(1)
	}
	return nil
}

func (t *tuneModel) updateWipe(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, keys.Left):
		t.pane.nudgeSplit(-wipeStep)
	case key.Matches(msg, keys.Right):
		t.pane.nudgeSplit(wipeStep)
	case key.Matches(msg, keys.PanUp):
		return t.pan(0, -panStep)
	case key.Matches(msg, keys.PanDown):
		return t.pan(0, panStep)
	case key.Matches(msg, keys.PanLeft):
		return t.pan(-panStep, 0)
	case key.Matches(msg, keys.PanRight):
		return t.pan(panStep, 0)
	}
	return nil
}

func (t *tuneModel) pan(dx, dy float64) tea.Cmd {
	if t.pane.zoomFactor() == 0 {
		return nil
	}
	t.focusX, t.focusY = clamp01(t.focusX+dx), clamp01(t.focusY+dy)
	return t.refresh()
}

func (t *tuneModel) stepFile(d int) tea.Cmd {
	n := len(t.files)
	if n < 2 {
		return nil
	}
	t.index = (t.index + d + n) % n
	return t.loadFile()
}

func wrap(v, d, n int) int { return (v + d + n) % n }

func (t *tuneModel) change(d int) tea.Cmd {
	switch t.row {
	case settingFormat:
		t.format = wrap(t.format, d, len(formatChoices))
	case settingLevel:
		t.level = max(codec.MinLevel, min(t.level+d, codec.MaxLevel))
	case settingSize:
		t.size = wrap(t.size, d, len(sizeChoices))
	case settingMeta:
		t.keepMeta = !t.keepMeta
	case settingSave:
		t.save = wrap(t.save, d, saveCount)
		return nil
	}
	return t.refresh()
}

func (t *tuneModel) start() tea.Cmd {
	opts := t.options()
	return func() tea.Msg { return goCrunchMsg{opts} }
}

func levelName(l int) string {
	switch l {
	case codec.LevelHigh:
		return "High quality"
	case codec.LevelBalanced:
		return "Balanced"
	case codec.LevelSmall:
		return "Small"
	}
	if l < codec.LevelHigh {
		return "Gentle"
	}
	if l > codec.LevelSmall {
		return "Crushed"
	}
	return "" // Between the named presets.
}

func (t *tuneModel) saveName() string {
	switch t.save {
	case saveFolder:
		return outFolderName + "/ folder"
	case saveReplace:
		return "Replace originals"
	}
	return "Beside originals"
}

func (t *tuneModel) sizeName() string {
	if s := sizeChoices[t.size]; s != 0 {
		return fmt.Sprintf("Fit %d px", s)
	}
	return "Original size"
}

func (t *tuneModel) metaName() string {
	if t.keepMeta {
		return "Keep EXIF"
	}
	return "Strip EXIF"
}

func (t *tuneModel) slider(w int) string {
	th := t.app.th
	var b strings.Builder
	stops := pickGradient(codec.MaxLevel)
	seg := max(w/codec.MaxLevel, 1)
	for l := codec.MinLevel; l <= codec.MaxLevel; l++ {
		s := lipgloss.NewStyle().Foreground(stops[l-1])
		glyph := strings.Repeat("━", seg)
		if l > t.level {
			s = lipgloss.NewStyle().Foreground(th.TrackColor)
		}
		if l == t.level {
			glyph = strings.Repeat("━", seg-1) + "●"
		}
		b.WriteString(s.Render(glyph))
	}
	return b.String()
}

func (t *tuneModel) settings(w int) string {
	th := t.app.th
	row := func(i int, label, value string) string {
		style := th.Muted
		pointer := "  "
		v := th.Text.Render(value)
		if i == t.row && !t.wiping {
			style = th.Key
			pointer = th.Key.Render("▸ ")
			v = th.Accent.Render("‹ ") + th.Bold.Render(value) + th.Accent.Render(" ›")
		}
		return pointer + pad(style.Render(label), 10) + v
	}
	lines := []string{
		row(settingFormat, "Format", formatChoices[t.format].String()),
		"",
		row(settingLevel, "Squeeze", fmt.Sprintf("%d %s", t.level, levelName(t.level))),
		"            " + t.slider(w-14),
		"",
		row(settingSize, "Size", t.sizeName()),
		"",
		row(settingMeta, "Metadata", t.metaName()),
		"",
		row(settingSave, "Save to", t.saveName()),
		"",
	}
	lines = append(lines, t.advice()...)
	return strings.Join(lines, "\n")
}

func (t *tuneModel) advice() []string {
	th := t.app.th
	var out []string
	if formatChoices[t.format] == engine.TargetAVIF {
		out = append(out, th.Warn.Render("AVIF is the smallest, but slow."), th.Warn.Render("Expect several seconds per photo."))
	}
	if t.keepMeta {
		out = append(out, th.Warn.Render("EXIF can include GPS location."))
	}
	if t.save == saveReplace {
		out = append(out, th.Warn.Render("Originals are replaced, but only"), th.Warn.Render("by smaller files."))
	}
	return out
}

func (t *tuneModel) summaryLine() string {
	th := t.app.th
	name := truncateName(filepath.Base(t.files[t.index]), 34)
	head := th.Bold.Render(name)
	if n := len(t.files); n > 1 {
		head += th.Muted.Render(fmt.Sprintf("  file %d of %d", t.index+1, n))
	}
	switch {
	case t.loadErr != "":
		return head + "  " + th.Warn.Render(t.loadErr)
	case !t.hasEst:
		return head + "  " + th.Faint.Render("working it out...")
	}
	est := int64(len(t.est.Data))
	pct := units.Percent(t.fileSize, est)
	num := fmt.Sprintf("%s → about %s", units.Bytes(t.fileSize), units.Bytes(est))
	style := th.Good
	label := units.PercentLabel(pct) + " smaller"
	if pct <= 0 {
		style = th.Warn
		label = "no savings, original kept"
	}
	return head + "  " + th.Text.Render(num) + "  " + style.Render(label) + "  " + th.Muted.Render(t.est.Format.String())
}

func (t *tuneModel) view(w, h int) string {
	th := t.app.th
	title := " " + th.Key.Render("squish") + th.Muted.Render(" › tune it")
	bodyH := h - 5
	left := t.app.panel("Settings", "\n"+t.settings(tuneListW-4), tuneListW, bodyH, !t.wiping)

	rightW := w - tuneListW
	t.paneCols, t.paneRows = rightW-4, bodyH-3
	t.paneOrigin = image.Pt(tuneListW+2, 1+2)
	paneTitle := "Preview"
	if z := t.pane.zoomFactor(); z > 0 {
		paneTitle = fmt.Sprintf("Preview at %d:1", z)
	}
	label := th.Muted.Render("before ◂") + th.Faint.Render(" drag ") + th.Muted.Render("▸ after")
	right := t.app.panel(paneTitle+"   "+label, t.pane.view(t.paneCols, t.paneRows), rightW, bodyH, t.wiping)

	bindings := []key.Binding{keys.Up, keys.Left, keys.Wipe, keys.Zoom}
	if t.wiping {
		bindings = []key.Binding{keys.Left, keys.PanUp, keys.Zoom, keys.Wipe}
	}
	if len(t.files) > 1 {
		bindings = append(bindings, keys.NextFile)
	}
	bindings = append(bindings, keys.Start, keys.Back, keys.Quit)
	footer := t.app.footer(bindings...)
	if t.confirming {
		footer = " " + th.Warn.Render(fmt.Sprintf("Replace up to %d originals with smaller versions? ", len(t.files))) +
			th.Key.Render("y") + th.Muted.Render(" yes  ") + th.Key.Render("n") + th.Muted.Render(" no")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		title, lipgloss.JoinHorizontal(lipgloss.Top, left, right), " "+t.summaryLine(), "", footer)
}
