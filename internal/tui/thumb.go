package tui

import (
	"image"

	"charm.land/lipgloss/v2"
	"github.com/jamaalsawyerd/squish/internal/codec"
	"github.com/jamaalsawyerd/squish/internal/gfx"
)

// thumbnail draws an image centered in an area of terminal cells. It is only
// good for telling images apart: half blocks are far too coarse to judge
// quality, which is what the image viewer is for.
func thumbnail(m *image.NRGBA, alpha bool, cols, rows int) string {
	if m == nil {
		return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, "loading...")
	}
	w, h := cols, rows*2
	c := gfx.New(w, h)
	fitted := codec.Shrink(m, w, h)
	iw, ih := fitted.Rect.Dx(), fitted.Rect.Dy()
	x, y := (w-iw)/2, (h-ih)/2
	r := image.Rect(x, y, x+iw, y+ih)
	if alpha {
		gfx.Checkerboard(c, r)
	}
	gfx.Blit(c, fitted, r)
	return gfx.Render(c)
}
