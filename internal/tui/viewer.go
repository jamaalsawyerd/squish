package tui

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jamaalsawyerd/squish/internal/codec"
	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/tui/theme"
)

var errNoViewer = errors.New("no image viewer here")

// viewerEnv is what choosing a viewer depends on, so tests can fake a machine.
type viewerEnv struct {
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
	winPath  func(string) (string, error)
}

func realViewerEnv() viewerEnv {
	return viewerEnv{
		goos:     runtime.GOOS,
		getenv:   os.Getenv,
		lookPath: exec.LookPath,
		winPath: func(path string) (string, error) {
			out, err := exec.Command("wslpath", "-w", path).Output()
			return strings.TrimSpace(string(out)), err
		},
	}
}

// command picks how to show a file. It returns errNoViewer when a window would
// open somewhere the user can't see it, or not at all.
func (e viewerEnv) command(path string) ([]string, error) {
	if e.getenv("SSH_CONNECTION") != "" || e.getenv("SSH_TTY") != "" {
		return nil, errNoViewer
	}
	switch e.goos {
	case "windows":
		// Unlike cmd's start, this involves no shell, so odd names are safe.
		return []string{"rundll32", "url.dll,FileProtocolHandler", path}, nil
	case "darwin":
		return []string{"open", path}, nil
	}
	if e.getenv("WSL_DISTRO_NAME") != "" {
		if _, err := e.lookPath("wslview"); err == nil {
			return []string{"wslview", path}, nil
		}
		if _, err := e.lookPath("explorer.exe"); err != nil {
			return nil, errNoViewer
		}
		win, err := e.winPath(path)
		if err != nil {
			return nil, errNoViewer
		}
		return []string{"explorer.exe", win}, nil
	}
	if e.getenv("DISPLAY") == "" && e.getenv("WAYLAND_DISPLAY") == "" {
		return nil, errNoViewer
	}
	if _, err := e.lookPath("xdg-open"); err != nil {
		return nil, errNoViewer
	}
	return []string{"xdg-open", path}, nil
}

// openInViewer hands a file to the system's default app and returns without
// waiting, since some openers only exit once the viewer is closed. That also
// means a viewer that rejects the file goes unnoticed here, so callers show the
// path as well.
func openInViewer(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	args, err := realViewerEnv().command(abs)
	if err != nil {
		return err
	}
	// With no stdio attached, nothing can scribble over the interface.
	cmd := exec.Command(args[0], args[1:]...)
	if err := cmd.Start(); err != nil {
		return errNoViewer
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openFile is a variable so that tests don't open windows on the desktop.
var openFile = openInViewer

// viewerMsg reports how showing a file went.
type viewerMsg struct {
	path string
	warn string // Something the picture alone won't tell you.
	err  error
}

// notice describes the outcome for one line of the screen. The path always
// appears, since an opened viewer may still have turned the file down.
func (v viewerMsg) notice() string {
	s := "Opened " + v.path
	switch {
	case errors.Is(v.err, errNoViewer):
		s = "Can't open a viewer here. The image is at " + v.path
	case v.err != nil:
		return "Can't show this image: " + v.err.Error()
	}
	if v.warn != "" {
		return v.warn + " " + s
	}
	return s
}

// noticeLines wraps a notice to the screen. It never truncates: the path is
// the point of it when no viewer can open.
func noticeLines(th theme.Theme, notice string, w int) []string {
	lines := strings.Split(ansi.Hardwrap(notice, max(w-2, 1), true), "\n")
	for i, l := range lines {
		lines[i] = " " + th.Muted.Render(l)
	}
	return lines
}

// previewDir makes the session's folder for preview images on first use. It
// must run on the update loop: a preview still encoding when the app quits
// should fail, not bring the folder back after cleanup.
func (m *model) previewDir() (string, error) {
	if m.tempDir == "" {
		dir, err := os.MkdirTemp("", "squish-")
		if err != nil {
			return "", err
		}
		m.tempDir = dir
	}
	return m.tempDir, nil
}

func (m *model) removePreviews() {
	if m.tempDir != "" {
		_ = os.RemoveAll(m.tempDir) // Whatever is left stays in the system's temp folder.
	}
}

// showPreview compresses a whole image exactly as a real run would, writes it
// to a temporary file and opens that. Each one gets a new name, so a viewer
// holding the last one open can neither block this nor show a stale copy.
func showPreview(dec *codec.Decoded, opts engine.Options, size int64, path string) tea.Cmd {
	return func() tea.Msg {
		out := engine.Compress(dec, opts)
		if out.Format == codec.AVIF {
			runtime.GC() // Its memory is outside the Go heap, so nothing else would free it soon.
		}
		if out.Err != nil {
			return viewerMsg{err: out.Err}
		}
		path += out.Format.Ext()
		if err := os.WriteFile(path, out.Data, 0o600); err != nil {
			return viewerMsg{err: err}
		}
		msg := viewerMsg{path: path, err: openFile(path)}
		if int64(len(out.Data)) >= size && (out.Format == dec.Format || opts.InPlace) {
			msg.warn = "This wouldn't get smaller, so a real run keeps the original."
		}
		return msg
	}
}
