package tui

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/stats"
)

func writeImage(t *testing.T, path string, seed int) {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, 240, 160))
	for y := range 160 {
		for x := range 240 {
			m.SetNRGBA(x, y, color.NRGBA{uint8(x + seed), uint8(y * 3), uint8((x ^ y) * 7), 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(f, m); err != nil {
		t.Fatal(err)
	}
}

func sampleFiles(t *testing.T, n int) []string {
	t.Helper()
	dir := t.TempDir()
	var out []string
	for i := range n {
		p := filepath.Join(dir, fmt.Sprintf("photo with a rather long file name %d.png", i))
		writeImage(t, p, i)
		out = append(out, p)
	}
	return out
}

// pump delivers a message and then runs whatever commands come back, the way
// the Bubble Tea runtime would, but without a terminal.
func pump(m *model, msg tea.Msg) {
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		_, cmd := m.Update(next)
		queue = append(queue, runCmd(cmd)...)
	}
}

func runCmd(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, runCmd(c)...)
		}
		return out
	}
	switch msg.(type) {
	case nil, tickMsg, tea.QuitMsg:
		return nil // Frames would run forever, and nothing should quit here.
	}
	return []tea.Msg{msg}
}

func assertFits(t *testing.T, m *model, label string) {
	t.Helper()
	s := m.content()
	w, h := lipgloss.Size(s)
	if w > m.width || h > m.height {
		t.Errorf("%s at %dx%d: rendered %dx%d", label, m.width, m.height, w, h)
	}
}

func TestEveryScreenFitsEverySize(t *testing.T) {
	files := sampleFiles(t, 12)
	sizes := []image.Point{{74, 22}, {80, 24}, {100, 30}, {120, 40}, {200, 60}, {300, 100}, {40, 10}, {1, 1}}
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			cfg := Config{Options: engine.Options{OutDir: t.TempDir()}, StatsPath: filepath.Join(t.TempDir(), "s.json")}
			m := newModel(cfg)
			m.width, m.height = size.X, size.Y
			assertFits(t, m, "pick before any data")

			pump(m, m.pick.init()())
			assertFits(t, m, "pick")

			pump(m, goTuneMsg{files})
			assertFits(t, m, "tune")

			pump(m, goCrunchMsg{m.tune.options()})
			if m.screen != screenResults {
				t.Fatalf("screen = %v, want results", m.screen)
			}
			if m.results.done != len(files) {
				t.Errorf("done = %d, want %d", m.results.done, len(files))
			}
			assertFits(t, m, "results")
		})
	}
}

func TestCrunchFitsWhileRunning(t *testing.T) {
	files := sampleFiles(t, 3)
	m := newModel(Config{Options: engine.Options{OutDir: t.TempDir()}})
	m.width, m.height = 90, 26
	m.screen = screenCrunch
	m.files = files
	m.crunch = newCrunch(m, files, engine.Options{OutDir: t.TempDir()})
	t.Cleanup(m.crunch.cancel)
	assertFits(t, m, "crunch")
	for range 30 {
		m.step()
		assertFits(t, m, "crunch animating")
	}
}

func TestResultsShowEarnedAchievement(t *testing.T) {
	m := newModel(Config{StatsPath: filepath.Join(t.TempDir(), "s.json")})
	m.width, m.height = 120, 40
	c := &crunchModel{results: []engine.Result{{
		Input: "a.jpg", Output: "b.jpg", Status: engine.StatusDone, BytesIn: 1000, BytesOut: 50,
	}}}
	pump(m, goResultsMsg{c})
	view := m.content()
	for _, want := range []string{"First squish", "The 90 club", "95%"} {
		if !strings.Contains(view, want) {
			t.Errorf("results screen is missing %q", want)
		}
	}
	if got := stats.Load(m.cfg.StatsPath); got.Files != 1 {
		t.Errorf("saved stats = %+v", got)
	}
}

func TestCanceledRunEarnsNoSpotless(t *testing.T) {
	m := newModel(Config{})
	rs := []engine.Result{{Status: engine.StatusCanceled, Input: "x.jpg"}}
	for i := range 12 {
		rs = append(rs, engine.Result{
			Input: fmt.Sprint(i), Status: engine.StatusDone, BytesIn: 100, BytesOut: 50,
		})
	}
	r := newResults(m, &crunchModel{results: rs})
	for _, a := range r.earned {
		if a.ID == "clean" {
			t.Error("a canceled batch earned Spotless")
		}
	}
}

func TestPickDropsPastedPaths(t *testing.T) {
	files := sampleFiles(t, 2)
	m := newModel(Config{})
	pump(m, m.pick.init()())
	pump(m, tea.PasteMsg{Content: `"` + files[0] + `" "` + files[1] + `"`})
	if len(m.pick.selected) != 2 {
		t.Errorf("selected = %d, want 2", len(m.pick.selected))
	}
}

func TestTruncateNameKeepsExtension(t *testing.T) {
	got := truncateName("a very long name indeed for one photo.jpeg", 20)
	if !strings.HasSuffix(got, ".jpeg") || lipgloss.Width(got) > 20 {
		t.Errorf("got %q", got)
	}
}
