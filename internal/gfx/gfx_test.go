package gfx

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderDimensions(t *testing.T) {
	m := New(10, 6)
	FillRect(m, m.Rect, color.NRGBA{200, 10, 20, 255})
	lines := strings.Split(Render(m), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d rows, want 3", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 10 {
			t.Errorf("row %d is %d wide, want 10", i, w)
		}
	}
}

func TestRenderOddHeightKeepsLastRow(t *testing.T) {
	m := New(2, 3)
	FillRect(m, m.Rect, color.NRGBA{1, 2, 3, 255})
	if got := len(strings.Split(Render(m), "\n")); got != 2 {
		t.Errorf("got %d rows, want 2", got)
	}
}

func TestRenderTransparency(t *testing.T) {
	m := New(3, 2)
	c := color.NRGBA{9, 9, 9, 255}
	m.SetNRGBA(1, 0, c) // Top half only.
	m.SetNRGBA(2, 1, c) // Bottom half only.
	got := ansi.Strip(Render(m))
	if want := " " + upperHalf + lowerHalf; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDrawingClipsToCanvas(t *testing.T) {
	m := New(4, 4)
	c := color.NRGBA{9, 9, 9, 255}
	Set(m, -1, 99, c)
	FillRect(m, image.Rect(-5, -5, 50, 50), c)
	FillEllipse(m, 2, 2, 40, 40, func(float64) color.NRGBA { return c })
	Blit(m, m, image.Rect(-3, -3, 9, 9))
}
