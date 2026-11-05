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
	tuneListW     = 44
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

type estimateMsg struct {
	gen int
	est engine.Output
}

type tuneModel struct {
	app        *model
	files      []string
	index      int
	row        int
	confirming bool

	format, level, size, save int
	startLevel                int
	keepMeta                  bool

	dec        *codec.Decoded
	fileSize   int64
	loadErr    string
	thumb      *image.NRGBA
	gen        int
	est        engine.Output
	hasEst     bool
	estimating bool
	opening    bool
	notice     string
}

func newTune(app *model, files []string) *tuneModel {
	t := &tuneModel{app: app, files: files}
	o := app.opts
	t.level = o.Level
	if t.level == 0 {
		t.level = codec.LevelBalanced
	}
	t.startLevel = t.level
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
	t.dec, t.thumb, t.loadErr, t.hasEst = nil, nil, "", false
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
	if t.level != t.startLevel {
		o.Quality = 0 // Moving the slider replaces any --quality from the command line.
	}
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

// refresh asks for a new estimate after a short pause, so that holding a key
// does not queue up a pile of encodes.
func (t *tuneModel) refresh() tea.Cmd {
	t.gen++
	t.estimating = true
	gen := t.gen
	return tea.Tick(debounce, func(time.Time) tea.Msg { return debounceMsg{gen} })
}

func (t *tuneModel) estimate() tea.Cmd {
	gen, dec, opts := t.gen, t.dec, t.options()
	return func() tea.Msg { return estimateMsg{gen, engine.Estimate(dec, opts)} }
}

// showResult opens the current file, with the current settings, in a real
// image viewer. It is the only place squish shows what the output looks like.
func (t *tuneModel) showResult() tea.Cmd {
	if t.dec == nil || t.opening {
		return nil
	}
	dir, err := t.app.previewDir()
	if err != nil {
		t.notice = "Can't make a temporary folder: " + err.Error()
		return nil
	}
	input := t.files[t.index]
	stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
	t.app.previews++
	t.opening, t.notice = true, ""
	path := filepath.Join(dir, fmt.Sprintf("%s-%d", stem, t.app.previews))
	return showPreview(t.dec, t.options(), t.fileSize, path)
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
		t.thumb = codec.Shrink(msg.dec.Image, thumbSize, thumbSize)
		return t.refresh()
	case debounceMsg:
		if msg.gen == t.gen && t.dec != nil {
			return t.estimate()
		}
	case estimateMsg:
		if msg.gen != t.gen {
			return nil
		}
		t.estimating = false
		t.est, t.hasEst = msg.est, msg.est.Err == nil
	case viewerMsg:
		t.opening = false
		t.notice = msg.notice()
	case tea.KeyPressMsg:
		return t.updateKeys(msg)
	}
	return nil
}

func (t *tuneModel) updateKeys(msg tea.KeyPressMsg) tea.Cmd {
	t.notice = ""
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
	case key.Matches(msg, keys.View):
		return t.showResult()
	case key.Matches(msg, keys.Prev):
		return t.stepFile(-1)
	case key.Matches(msg, keys.NextFile):
		return t.stepFile(1)
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
		if i == t.row {
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

func (t *tuneModel) status(w int) []string {
	switch {
	case t.opening:
		return []string{" " + t.app.th.Faint.Render("Compressing the full image to show it...")}
	case t.notice != "":
		return noticeLines(t.app.th, t.notice, w)
	}
	return []string{""}
}

func (t *tuneModel) view(w, h int) string {
	th := t.app.th
	title := " " + th.Key.Render("squish") + th.Muted.Render(" › tune it")
	status := t.status(w)
	bodyH := h - 4 - len(status)
	left := t.app.panel("Settings", "\n"+t.settings(tuneListW-4), tuneListW, bodyH, true)

	rightW := w - tuneListW
	label := th.Faint.Render("press ") + th.Key.Render("o") + th.Faint.Render(" to see the real result")
	right := t.app.panel("Preview   "+label, thumbnail(t.thumb, t.dec != nil && !t.dec.Opaque, rightW-4, bodyH-3), rightW, bodyH, false)

	bindings := []key.Binding{keys.Up, keys.Left, keys.View}
	if len(t.files) > 1 {
		bindings = append(bindings, keys.NextFile)
	}
	bindings = append(bindings, keys.Start, keys.Back, keys.Quit)
	footer := t.app.footer(bindings...)
	if t.confirming {
		footer = " " + th.Warn.Render(fmt.Sprintf("Replace up to %d originals with smaller versions? ", len(t.files))) +
			th.Key.Render("y") + th.Muted.Render(" yes  ") + th.Key.Render("n") + th.Muted.Render(" no")
	}
	parts := []string{title, lipgloss.JoinHorizontal(lipgloss.Top, left, right), " " + t.summaryLine()}
	parts = append(parts, status...)
	return lipgloss.JoinVertical(lipgloss.Left, append(parts, footer)...)
}
