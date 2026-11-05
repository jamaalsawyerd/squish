// Package gfx draws small raster images and renders them to the terminal.
// Each character cell shows two pixels using the half block glyph, which is why
// the image thumbnails and the animated press can share one code path.
package gfx

import (
	"image"
	"image/color"
	"strconv"
	"strings"
)

const (
	upperHalf = "▀"
	lowerHalf = "▄"
	reset     = "\x1b[m"
	// Pixels below this alpha count as empty, showing the terminal's background.
	alphaCutoff = 128
)

// New makes a transparent canvas.
func New(w, h int) *image.NRGBA {
	return image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
}

func appendColor(b *strings.Builder, prefix string, p []uint8) {
	b.WriteString(prefix)
	var num [8]byte
	for i := range 3 {
		b.WriteByte(';')
		b.Write(strconv.AppendUint(num[:0], uint64(p[i]), 10))
	}
	b.WriteByte('m')
}

// mono switches rendering to plain characters, for terminals without color.
var mono bool

// SetMono turns monochrome rendering on or off. Half blocks are meaningless
// without color, while shaded characters still show what an image looks like.
func SetMono(on bool) { mono = on }

// shades runs from empty to solid.
const shades = " .:-=+*#%@"

func renderMono(m *image.NRGBA) string {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	var b strings.Builder
	for y := 0; y < h; y += 2 {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := range w {
			p := m.Pix[m.PixOffset(x, y):]
			luma := (299*int(p[0]) + 587*int(p[1]) + 114*int(p[2])) / 1000
			if p[3] < alphaCutoff {
				luma = 0
			}
			b.WriteByte(shades[luma*(len(shades)-1)/255])
		}
	}
	return b.String()
}

// Render turns a canvas into text, one row of cells per two rows of pixels.
func Render(m *image.NRGBA) string {
	if mono {
		return renderMono(m)
	}
	w, h := m.Rect.Dx(), m.Rect.Dy()
	var b strings.Builder
	b.Grow(w * h * 12)
	for y := 0; y < h; y += 2 {
		if y > 0 {
			b.WriteByte('\n')
		}
		top := m.Pix[y*m.Stride:]
		var bottom []uint8
		if y+1 < h {
			bottom = m.Pix[(y+1)*m.Stride:]
		}
		var lastFG, lastBG uint32 = 1 << 31, 1 << 31
		for x := range w {
			t := top[x*4 : x*4+4]
			hasTop := t[3] >= alphaCutoff
			var o []uint8
			hasBottom := false
			if bottom != nil {
				o = bottom[x*4 : x*4+4]
				hasBottom = o[3] >= alphaCutoff
			}
			switch {
			case !hasTop && !hasBottom:
				if lastFG != 1<<31 || lastBG != 1<<31 {
					b.WriteString(reset)
					lastFG, lastBG = 1<<31, 1<<31
				}
				b.WriteByte(' ')
			case hasTop && hasBottom:
				if c := rgbKey(t); c != lastFG {
					appendColor(&b, "\x1b[38;2", t)
					lastFG = c
				}
				if c := rgbKey(o); c != lastBG {
					appendColor(&b, "\x1b[48;2", o)
					lastBG = c
				}
				b.WriteString(upperHalf)
			default:
				if lastBG != 1<<31 {
					b.WriteString(reset)
					lastFG, lastBG = 1<<31, 1<<31
				}
				p, glyph := t, upperHalf
				if hasBottom {
					p, glyph = o, lowerHalf
				}
				if c := rgbKey(p); c != lastFG {
					appendColor(&b, "\x1b[38;2", p)
					lastFG = c
				}
				b.WriteString(glyph)
			}
		}
		b.WriteString(reset)
	}
	return b.String()
}

func rgbKey(p []uint8) uint32 {
	return uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
}

// Set draws one pixel, ignoring anything outside the canvas.
func Set(m *image.NRGBA, x, y int, c color.NRGBA) {
	if (image.Point{x, y}).In(m.Rect) {
		m.SetNRGBA(x, y, c)
	}
}

// FillRect draws a filled rectangle.
func FillRect(m *image.NRGBA, r image.Rectangle, c color.NRGBA) {
	r = r.Intersect(m.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			m.SetNRGBA(x, y, c)
		}
	}
}

// Shade multiplies a color's brightness, keeping alpha.
func Shade(c color.NRGBA, f float64) color.NRGBA {
	s := func(v uint8) uint8 {
		return uint8(min(float64(v)*f, 255))
	}
	return color.NRGBA{s(c.R), s(c.G), s(c.B), c.A}
}

// FillEllipse draws a filled ellipse, passing each pixel's height within the
// shape, from 0 at the top to 1 at the bottom, to the color function.
func FillEllipse(m *image.NRGBA, cx, cy, rx, ry float64, fill func(t float64) color.NRGBA) {
	if rx <= 0 || ry <= 0 {
		return
	}
	for y := int(cy - ry); y <= int(cy+ry); y++ {
		for x := int(cx - rx); x <= int(cx+rx); x++ {
			dx := (float64(x) + 0.5 - cx) / rx
			dy := (float64(y) + 0.5 - cy) / ry
			if dx*dx+dy*dy <= 1 {
				Set(m, x, y, fill((dy+1)/2))
			}
		}
	}
}

// Blit copies src into the rectangle, scaling with nearest neighbor so that
// scaled pixels stay crisp.
func Blit(dst *image.NRGBA, src *image.NRGBA, r image.Rectangle) {
	if src == nil || r.Empty() {
		return
	}
	sw, sh := src.Rect.Dx(), src.Rect.Dy()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		sy := (y - r.Min.Y) * sh / r.Dy()
		for x := r.Min.X; x < r.Max.X; x++ {
			sx := (x - r.Min.X) * sw / r.Dx()
			c := src.NRGBAAt(src.Rect.Min.X+sx, src.Rect.Min.Y+sy)
			if c.A > 0 {
				Set(dst, x, y, c)
			}
		}
	}
}

// Checkerboard fills the canvas with the classic transparency pattern.
func Checkerboard(m *image.NRGBA, r image.Rectangle) {
	const size = 4
	light := color.NRGBA{120, 120, 128, 255}
	dark := color.NRGBA{88, 88, 96, 255}
	r = r.Intersect(m.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if (x/size+y/size)%2 == 0 {
				m.SetNRGBA(x, y, light)
			} else {
				m.SetNRGBA(x, y, dark)
			}
		}
	}
}
