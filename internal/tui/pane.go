package tui

import (
	"image"
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/jamaalsawyerd/squish/internal/codec"
	"github.com/jamaalsawyerd/squish/internal/gfx"
)

// Zoom levels for the preview. Half blocks are low resolution, so judging
// compression artifacts needs real pixels, shown 1:1 or larger.
var zoomLevels = []int{0, 1, 2, 4} // 0 means fit to the pane.

// pane shows a before and after image with a draggable wipe between them.
type pane struct {
	before, after *image.NRGBA
	split         float64
	zoom          int // Index into zoomLevels.
	alpha         bool

	// Cached scaled versions, so dragging the wipe does not rescale.
	cacheW, cacheH       int
	cacheSrcB, cacheSrcA *image.NRGBA
	fitB, fitA           *image.NRGBA
}

func newPane() pane { return pane{split: 0.5} }

func (p *pane) zoomFactor() int { return zoomLevels[p.zoom] }

func (p *pane) cycleZoom() {
	p.zoom = (p.zoom + 1) % len(zoomLevels)
}

func (p *pane) nudgeSplit(d float64) {
	p.split = clamp01(p.split + d)
}

func (p *pane) scaled(w, h int) (*image.NRGBA, *image.NRGBA) {
	if p.cacheW == w && p.cacheH == h && p.cacheSrcB == p.before && p.cacheSrcA == p.after {
		return p.fitB, p.fitA
	}
	p.cacheW, p.cacheH, p.cacheSrcB, p.cacheSrcA = w, h, p.before, p.after
	p.fitB, p.fitA = nil, nil
	if p.before != nil {
		p.fitB = codec.Shrink(p.before, w, h)
	}
	if p.after != nil {
		p.fitA = codec.Shrink(p.after, w, h)
	}
	return p.fitB, p.fitA
}

// target is where an image lands in the pane, centered.
func target(iw, ih, w, h int) image.Rectangle {
	x := (w - iw) / 2
	y := (h - ih) / 2
	return image.Rect(x, y, x+iw, y+ih)
}

// view renders the pane at a size in terminal cells.
func (p *pane) view(cols, rows int) string {
	w, h := cols, rows*2
	c := gfx.New(w, h)
	if p.before == nil {
		return lipgloss.Place(cols, rows, lipgloss.Center, lipgloss.Center, "loading...")
	}
	var b, a *image.NRGBA
	var r image.Rectangle
	if z := p.zoomFactor(); z == 0 {
		b, a = p.scaled(w, h)
		r = target(b.Rect.Dx(), b.Rect.Dy(), w, h)
	} else {
		// Crops are prepared at real size. Show them magnified and centered.
		b, a = p.before, p.after
		iw, ih := b.Rect.Dx()*z, b.Rect.Dy()*z
		r = target(iw, ih, w, h)
	}
	if p.alpha {
		gfx.Checkerboard(c, r)
	}
	splitX := int(float64(w) * p.split)
	left := r
	left.Max.X = min(left.Max.X, splitX)
	right := r
	right.Min.X = max(right.Min.X, splitX)
	blitClipped(c, b, r, left)
	if a != nil {
		blitClipped(c, a, r, right)
	} else {
		blitClipped(c, b, r, right)
	}
	if a != nil && splitX > 0 && splitX < w {
		line := color.NRGBA{255, 255, 255, 255}
		gfx.FillRect(c, image.Rect(splitX, 0, splitX+1, h), line)
		mid := h / 2
		gfx.FillRect(c, image.Rect(splitX-1, mid-2, splitX+2, mid+2), line)
	}
	return gfx.Render(c)
}

// blitClipped draws src scaled to r, showing only the part inside clip.
func blitClipped(dst, src *image.NRGBA, r, clip image.Rectangle) {
	clip = clip.Intersect(r).Intersect(dst.Rect)
	if clip.Empty() {
		return
	}
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	for y := clip.Min.Y; y < clip.Max.Y; y++ {
		sy := (y - r.Min.Y) * sh / r.Dy()
		for x := clip.Min.X; x < clip.Max.X; x++ {
			sx := (x - r.Min.X) * sw / r.Dx()
			c := src.NRGBAAt(sx, sy)
			if c.A > 0 {
				dst.SetNRGBA(x, y, c)
			}
		}
	}
}
