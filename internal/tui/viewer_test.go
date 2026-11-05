package tui

import (
	"errors"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jamaalsawyerd/squish/internal/codec"
	"github.com/jamaalsawyerd/squish/internal/engine"
)

func fakeEnv(goos string, env map[string]string, installed ...string) viewerEnv {
	return viewerEnv{
		goos:   goos,
		getenv: func(k string) string { return env[k] },
		lookPath: func(name string) (string, error) {
			for _, i := range installed {
				if i == name {
					return "/bin/" + name, nil
				}
			}
			return "", errors.New("not found")
		},
		winPath: func(p string) (string, error) { return `\\wsl\` + p, nil },
	}
}

func TestViewerCommand(t *testing.T) {
	wsl := map[string]string{"WSL_DISTRO_NAME": "Ubuntu"}
	desktop := map[string]string{"DISPLAY": ":0"}
	tests := []struct {
		name string
		env  viewerEnv
		want []string
	}{
		{"windows", fakeEnv("windows", nil), []string{"rundll32", "url.dll,FileProtocolHandler", "/p.jpg"}},
		{"mac", fakeEnv("darwin", nil), []string{"open", "/p.jpg"}},
		{"linux desktop", fakeEnv("linux", desktop, "xdg-open"), []string{"xdg-open", "/p.jpg"}},
		{"wayland", fakeEnv("linux", map[string]string{"WAYLAND_DISPLAY": "w"}, "xdg-open"), []string{"xdg-open", "/p.jpg"}},
		{"linux with nothing installed", fakeEnv("linux", desktop), nil},
		{"linux console", fakeEnv("linux", nil, "xdg-open"), nil},
		{"wsl prefers wslview", fakeEnv("linux", wsl, "wslview", "explorer.exe"), []string{"wslview", "/p.jpg"}},
		{"wsl falls back to explorer", fakeEnv("linux", wsl, "explorer.exe"), []string{"explorer.exe", `\\wsl\/p.jpg`}},
		{"wsl with interop off", fakeEnv("linux", wsl), nil},
		{"ssh to a desktop", fakeEnv("linux", map[string]string{"SSH_CONNECTION": "x", "DISPLAY": ":0"}, "xdg-open"), nil},
		{"ssh to a mac", fakeEnv("darwin", map[string]string{"SSH_TTY": "/dev/pts/1"}), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.env.command("/p.jpg")
			if tt.want == nil {
				if !errors.Is(err, errNoViewer) {
					t.Fatalf("got %v, %v, want errNoViewer", got, err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestViewerNotice(t *testing.T) {
	tests := []struct {
		msg  viewerMsg
		want []string
	}{
		{viewerMsg{path: "/a"}, []string{"Opened /a"}},
		{viewerMsg{path: "/a", err: errNoViewer}, []string{"Can't open a viewer", "/a"}},
		{viewerMsg{path: "/a", warn: "Careful."}, []string{"Careful.", "Opened /a"}},
		{viewerMsg{path: "/a", warn: "Careful.", err: errNoViewer}, []string{"Careful.", "/a"}},
		{viewerMsg{err: errors.New("disk full")}, []string{"disk full"}},
	}
	for _, tt := range tests {
		got := tt.msg.notice()
		for _, w := range tt.want {
			if !strings.Contains(got, w) {
				t.Errorf("notice %q is missing %q", got, w)
			}
		}
	}
}

// stubViewer records what would have been opened.
func stubViewer(t *testing.T, err error) *[]string {
	t.Helper()
	var opened []string
	old := openFile
	openFile = func(p string) error {
		opened = append(opened, p)
		return err
	}
	t.Cleanup(func() { openFile = old })
	return &opened
}

var pressO = tea.KeyPressMsg{Code: 'o', Text: "o"}

func tuneOn(t *testing.T, files []string, size image.Point) *model {
	t.Helper()
	m := newModel(Config{Options: engine.Options{}})
	t.Cleanup(m.removePreviews)
	m.width, m.height = size.X, size.Y
	pump(m, goTuneMsg{files})
	return m
}

func TestTuneOpensTheRealResult(t *testing.T) {
	opened := stubViewer(t, nil)
	files := sampleFiles(t, 1)
	before, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	m := tuneOn(t, files, image.Pt(120, 40))
	m.tune.size = 5 // Fit 800 px is ignored by a 240 px image, so it must stay whole.
	pump(m, pressO)
	if len(*opened) != 1 {
		t.Fatalf("opened = %v", *opened)
	}
	data, err := os.ReadFile((*opened)[0])
	if err != nil {
		t.Fatal(err)
	}
	dec, err := codec.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := dec.Image.Rect.Size(); got != image.Pt(240, 160) {
		t.Errorf("preview is %v, want the whole 240x160 image", got)
	}
	if !strings.Contains(m.content(), "Opened") {
		t.Error("the screen doesn't say what happened")
	}
	if m.tune.opening {
		t.Error("still marked as opening")
	}

	after, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Error("the original changed")
	}
	entries, err := os.ReadDir(filepath.Dir(files[0]))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("wrote %d files beside the original", len(entries)-1)
	}

	pump(m, pressO)
	if len(*opened) != 2 || (*opened)[0] == (*opened)[1] {
		t.Errorf("a second look should get a new file: %v", *opened)
	}
	m.removePreviews()
	if _, err := os.Stat(m.tempDir); !os.IsNotExist(err) {
		t.Errorf("temporary folder survived: %v", err)
	}
}

func TestTuneFollowsTheSettings(t *testing.T) {
	opened := stubViewer(t, nil)
	m := tuneOn(t, sampleFiles(t, 1), image.Pt(120, 40))
	m.tune.format = 4 // WebP.
	pump(m, pressO)
	if len(*opened) != 1 || filepath.Ext((*opened)[0]) != ".webp" {
		t.Fatalf("opened = %v, want a WebP", *opened)
	}
	data, err := os.ReadFile((*opened)[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := codec.Sniff(data); got != codec.WebP {
		t.Errorf("format = %v", got)
	}
}

func TestTuneWarnsWhenNothingWouldBeSaved(t *testing.T) {
	stubViewer(t, nil)
	m := tuneOn(t, sampleFiles(t, 1), image.Pt(200, 40))
	m.tune.fileSize = 1 // Everything is bigger than this.
	pump(m, pressO)
	if !strings.Contains(m.tune.notice, "keeps the original") {
		t.Errorf("notice = %q", m.tune.notice)
	}
}

func TestTuneShowsThePathWithoutAViewer(t *testing.T) {
	stubViewer(t, errNoViewer)
	m := tuneOn(t, sampleFiles(t, 1), image.Pt(200, 40))
	pump(m, pressO)
	if !strings.Contains(m.tune.notice, m.tempDir) {
		t.Errorf("notice = %q, want the path in %s", m.tune.notice, m.tempDir)
	}
}

// assertShows checks a whole path survives wrapping and the footer stayed on
// screen. Content is clipped to the screen height, so measuring it isn't enough.
func assertShows(t *testing.T, m *model, label, path, footer string) {
	t.Helper()
	assertFits(t, m, label)
	view := ansi.Strip(m.content())
	var flat strings.Builder
	for line := range strings.SplitSeq(view, "\n") {
		flat.WriteString(strings.TrimSpace(line))
	}
	if !strings.Contains(flat.String(), path) {
		t.Errorf("%s: path %q was cut off:\n%s", label, path, view)
	}
	if !strings.Contains(view, footer) {
		t.Errorf("%s: footer %q fell off:\n%s", label, footer, view)
	}
}

func TestLongPathsFitTheSmallestScreens(t *testing.T) {
	path := "/tmp/" + strings.Repeat("a-rather-long-folder/", 8) + "photo-1.jpg"
	stubViewer(t, errNoViewer)
	m := tuneOn(t, sampleFiles(t, 12), image.Pt(minWidth, minHeight))
	pump(m, viewerMsg{path: path, err: errNoViewer})
	assertShows(t, m, "tune", path, "squish!")

	opts := m.tune.options()
	opts.OutDir = t.TempDir()
	pump(m, goCrunchMsg{opts})
	pump(m, viewerMsg{path: path, err: errNoViewer})
	assertShows(t, m, "results", path, "new batch")
}

func TestResultsOpenTheSavedFile(t *testing.T) {
	opened := stubViewer(t, nil)
	m := newModel(Config{})
	m.width, m.height = 120, 40
	pump(m, goResultsMsg{&crunchModel{results: []engine.Result{
		{Input: "a.jpg", Output: "out/a.jpg", Status: engine.StatusDone, BytesIn: 100, BytesOut: 50},
		{Input: "b.jpg", Status: engine.StatusSkipped, Reason: "already small"},
	}}})
	pump(m, pressO)
	if !reflect.DeepEqual(*opened, []string{"out/a.jpg"}) {
		t.Errorf("opened = %v", *opened)
	}
	pump(m, tea.KeyPressMsg{Code: tea.KeyDown})
	pump(m, pressO)
	if len(*opened) != 1 {
		t.Errorf("a skipped file has no output to open: %v", *opened)
	}
}
