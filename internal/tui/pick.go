package tui

import (
	"cmp"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jamaalsawyerd/squish/internal/codec"
	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/units"
)

type pickEntry struct {
	name string
	path string
	dir  bool
	size int64
}

type thumbMsg struct {
	path   string
	img    *image.NRGBA
	format codec.Format
	w, h   int
	err    error
}

type dirMsg struct {
	dir     string
	entries []pickEntry
	err     error
}

const (
	thumbSize  = 128
	introStart = 1.5
)

type pickModel struct {
	app        *model
	dir        string
	entries    []pickEntry
	cursor     int
	offset     int
	selected   map[string]int64
	order      []string
	showHidden bool
	typing     bool
	input      textinput.Model
	status     string
	thumbs     map[string]thumbMsg
	logo       spring
}

func newPick(app *model) *pickModel {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	in := textinput.New()
	in.Placeholder = "/path/to/photos"
	in.Prompt = "go to: "
	p := &pickModel{
		app: app, dir: dir, selected: map[string]int64{}, input: in,
		thumbs: map[string]thumbMsg{}, logo: newSpring(5, 0.28, introStart),
	}
	if app.cfg.NoAnim {
		p.logo.pos = 1
	}
	return p
}

func (p *pickModel) init() tea.Cmd { return p.readDir(p.dir) }

func (p *pickModel) animating() bool { return !p.logo.settled(1) }

func (p *pickModel) step() { p.logo.to(1) }

func (p *pickModel) readDir(dir string) tea.Cmd {
	hidden := p.showHidden
	return func() tea.Msg {
		es, err := os.ReadDir(dir)
		if err != nil {
			return dirMsg{dir: dir, err: err}
		}
		var out []pickEntry
		for _, e := range es {
			name := e.Name()
			if !hidden && strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(dir, name)
			st, err := os.Stat(path) // Follows links, so linked folders work.
			if err != nil {
				continue
			}
			if st.IsDir() {
				out = append(out, pickEntry{name: name, path: path, dir: true})
			} else if engine.IsImageName(name) {
				out = append(out, pickEntry{name: name, path: path, size: st.Size()})
			}
		}
		slices.SortFunc(out, func(a, b pickEntry) int {
			if a.dir != b.dir {
				if a.dir {
					return -1
				}
				return 1
			}
			return cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
		})
		return dirMsg{dir: dir, entries: out}
	}
}

func (p *pickModel) current() *pickEntry {
	if p.cursor < len(p.entries) {
		return &p.entries[p.cursor]
	}
	return nil
}

func loadThumb(path string) tea.Cmd {
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return thumbMsg{path: path, err: err}
		}
		info, err := codec.Probe(data)
		if err != nil {
			return thumbMsg{path: path, err: err}
		}
		img, f, err := codec.DecodePreview(data, thumbSize)
		return thumbMsg{path: path, img: img, format: f, w: info.Width, h: info.Height, err: err}
	}
}

func (p *pickModel) wantThumb() tea.Cmd {
	e := p.current()
	if e == nil || e.dir {
		return nil
	}
	if _, ok := p.thumbs[e.path]; ok {
		return nil
	}
	return loadThumb(e.path)
}

func (p *pickModel) toggle(path string, size int64) {
	if _, ok := p.selected[path]; ok {
		delete(p.selected, path)
		p.order = slices.DeleteFunc(p.order, func(s string) bool { return s == path })
		return
	}
	p.selected[path] = size
	p.order = append(p.order, path)
}

func (p *pickModel) add(path string, size int64) {
	if _, ok := p.selected[path]; !ok {
		p.toggle(path, size)
	}
}

func (p *pickModel) toggleAll() {
	all := true
	for _, e := range p.entries {
		if !e.dir {
			if _, ok := p.selected[e.path]; !ok {
				all = false
			}
		}
	}
	for _, e := range p.entries {
		if e.dir {
			continue
		}
		if _, on := p.selected[e.path]; on == all {
			p.toggle(e.path, e.size)
		}
	}
}

// addPaths handles pasted or dragged paths. Folders are opened, and files are
// selected.
func (p *pickModel) addPaths(paths []string) tea.Cmd {
	var cmd tea.Cmd
	added := 0
	for _, path := range paths {
		st, err := os.Stat(path)
		if err != nil {
			p.status = "Can't find " + path
			continue
		}
		if st.IsDir() {
			cmd = p.readDir(path)
			continue
		}
		p.add(path, st.Size())
		added++
	}
	if added > 0 {
		p.status = "Added " + units.Count(added, "file")
	}
	return cmd
}

func (p *pickModel) proceed() tea.Cmd {
	if len(p.selected) == 0 {
		if e := p.current(); e != nil && !e.dir {
			p.toggle(e.path, e.size)
		} else {
			p.status = "Select some images first"
			return nil
		}
	}
	files := slices.Clone(p.order)
	return func() tea.Msg { return goTuneMsg{files} }
}

func (p *pickModel) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case dirMsg:
		if msg.err != nil {
			p.status = "Can't open " + msg.dir
			return nil
		}
		p.dir, p.entries, p.cursor, p.offset = msg.dir, msg.entries, 0, 0
		return p.wantThumb()
	case thumbMsg:
		p.thumbs[msg.path] = msg
		return nil
	case tea.PasteMsg:
		if !p.typing {
			return p.addPaths(parsePaths(msg.Content))
		}
	case tea.KeyPressMsg:
		if p.typing {
			return p.updateInput(msg)
		}
		return p.updateKeys(msg)
	}
	if p.typing {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		return cmd
	}
	return nil
}

func (p *pickModel) updateInput(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		p.typing = false
		p.input.Blur()
		return nil
	case "enter":
		p.typing = false
		p.input.Blur()
		paths := parsePaths(p.input.Value())
		p.input.SetValue("")
		return p.addPaths(paths)
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return cmd
}

func (p *pickModel) updateKeys(msg tea.KeyPressMsg) tea.Cmd {
	p.status = ""
	switch {
	case key.Matches(msg, keys.Quit):
		return tea.Quit
	case key.Matches(msg, keys.Up):
		p.move(-1)
		return p.wantThumb()
	case key.Matches(msg, keys.Down):
		p.move(1)
		return p.wantThumb()
	case key.Matches(msg, keys.Toggle):
		if e := p.current(); e != nil && !e.dir {
			p.toggle(e.path, e.size)
			p.move(1)
			return p.wantThumb()
		}
	case key.Matches(msg, keys.All):
		p.toggleAll()
	case key.Matches(msg, keys.Open):
		e := p.current()
		switch {
		case e == nil:
		case e.dir:
			return p.readDir(e.path)
		default:
			return p.proceed()
		}
	case key.Matches(msg, keys.Parent):
		return p.readDir(filepath.Dir(p.dir))
	case key.Matches(msg, keys.Next):
		return p.proceed()
	case key.Matches(msg, keys.Path):
		p.typing = true
		return p.input.Focus()
	case key.Matches(msg, keys.Hidden):
		p.showHidden = !p.showHidden
		return p.readDir(p.dir)
	}
	return nil
}

func (p *pickModel) move(d int) {
	if len(p.entries) > 0 {
		p.cursor = max(0, min(p.cursor+d, len(p.entries)-1))
	}
}

func (p *pickModel) selectedSize() int64 {
	var n int64
	for _, s := range p.selected {
		n += s
	}
	return n
}

func (p *pickModel) view(w, h int) string {
	th := p.app.th
	logo := drawLogo(clamp01(p.logo.pos/1.0*0.5 + 0.5))
	if p.logo.pos >= 1 {
		logo = drawLogo(1 / p.logo.pos)
	}
	tagline := th.Muted.Render("Pick some images to squish. Drag files in, or browse below.")
	header := lipgloss.JoinVertical(lipgloss.Left, logo, " "+tagline)

	footerText := p.app.footer(keys.Toggle, keys.All, keys.Open, keys.Parent, keys.Path, keys.Next, keys.Quit)
	status := th.Muted.Render(fmt.Sprintf(" %d selected · %s", len(p.selected), units.Bytes(p.selectedSize())))
	if p.status != "" {
		status += th.Warn.Render("   " + p.status)
	}
	if p.typing {
		status = " " + p.input.View()
	}
	bodyH := h - lipgloss.Height(header) - 2
	listW := w * 3 / 5
	rightW := w - listW
	list := p.app.panel(shortenPath(p.dir, listW-4), p.listBody(listW-4, bodyH-3), listW, bodyH, true)
	info := p.app.panel("Preview", p.infoBody(rightW-4, bodyH-3), rightW, bodyH, false)
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, info)
	return lipgloss.JoinVertical(lipgloss.Left, header, body, status, footerText)
}

func (p *pickModel) listBody(w, rows int) string {
	th := p.app.th
	if len(p.entries) == 0 {
		return th.Faint.Render("\nNo images or folders here.\nPress backspace to go up a folder.")
	}
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.cursor >= p.offset+rows {
		p.offset = p.cursor - rows + 1
	}
	var lines []string
	for i := p.offset; i < min(p.offset+rows, len(p.entries)); i++ {
		e := p.entries[i]
		mark := "  "
		if _, on := p.selected[e.path]; on {
			mark = th.Good.Render("✓ ")
		}
		name, size := e.name, ""
		if e.dir {
			name += "/"
		} else {
			size = units.Bytes(e.size)
		}
		name = truncateName(name, w-12)
		style := th.Text
		if e.dir {
			style = th.Accent
		}
		pointer := "  "
		if i == p.cursor {
			pointer = th.Key.Render("▸ ")
			style = style.Bold(true)
		}
		left := pointer + mark + style.Render(name)
		gap := max(w-lipgloss.Width(left)-lipgloss.Width(size), 1)
		lines = append(lines, left+strings.Repeat(" ", gap)+th.Faint.Render(size))
	}
	return strings.Join(lines, "\n")
}

func (p *pickModel) infoBody(w, rows int) string {
	th := p.app.th
	e := p.current()
	portrait := func() string {
		mh := min(rows-2, w/2)
		return lipgloss.Place(w, rows, lipgloss.Center, lipgloss.Center,
			lipgloss.JoinVertical(lipgloss.Center, mascotPortrait(w*2/3, mh, moodHappy)))
	}
	if e == nil || e.dir {
		return portrait()
	}
	t, ok := p.thumbs[e.path]
	if !ok {
		return th.Faint.Render("Loading...")
	}
	if t.err != nil {
		return th.Bad.Render("Can't read this image")
	}
	imgRows := max(rows-3, 2)
	pic := thumbnail(t.img, false, w, imgRows)
	iw, ih := t.w, t.h
	if tb := t.img.Bounds(); (tb.Dx() > tb.Dy()) != (iw > ih) {
		iw, ih = ih, iw // The header size ignores EXIF rotation.
	}
	meta := fmt.Sprintf("%s · %d×%d · %s", t.format, iw, ih, units.Bytes(e.size))
	return lipgloss.JoinVertical(lipgloss.Left, pic, "", th.Muted.Render(meta))
}

// shortenPath keeps the end of a path, which is the part that matters.
func shortenPath(path string, w int) string {
	r := []rune(path)
	if len(r) <= w {
		return path
	}
	return "…" + string(r[len(r)-w+1:])
}
