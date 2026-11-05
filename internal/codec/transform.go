package codec

import (
	"image"
	"image/draw"

	xdraw "golang.org/x/image/draw"
)

// fit returns dimensions that fit inside a box, keeping the aspect ratio. It
// never enlarges.
func fit(w, h, boxW, boxH int) (int, int) {
	if w <= boxW && h <= boxH {
		return w, h
	}
	nw, nh := boxW, h*boxW/w
	if nh > boxH {
		nw, nh = w*boxH/h, boxH
	}
	return max(nw, 1), max(nh, 1)
}

// Resize shrinks m so its longest side is at most maxDim.
func Resize(m *image.NRGBA, opaque bool, maxDim int) *image.NRGBA {
	return scale(m, opaque, maxDim, maxDim, xdraw.CatmullRom)
}

func scale(m *image.NRGBA, opaque bool, boxW, boxH int, k xdraw.Interpolator) *image.NRGBA {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	nw, nh := fit(w, h, boxW, boxH)
	if nw == w && nh == h {
		return m
	}
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	var dst draw.Image = out
	if opaque {
		dst = asRGBA(out) // Same bytes, and much faster to scale into.
	}
	k.Scale(dst, out.Rect, m, m.Rect, xdraw.Src, nil)
	return out
}

// IsGraphic guesses whether an image is a screenshot, logo or diagram rather
// than a photo. Those have long runs of identical pixels, where sensor noise
// means photos almost never do. They need different compression settings.
func IsGraphic(m *image.NRGBA) bool {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	if w < 2 || h < 1 {
		return false
	}
	same, total := 0, 0
	step := max(h/256, 1)
	for y := 0; y < h; y += step {
		row := m.Pix[y*m.Stride : y*m.Stride+w*4]
		for x := 4; x+4 <= len(row); x += 4 {
			if packColor(row[x:x+4]) == packColor(row[x-4:x]) {
				same++
			}
			total++
		}
	}
	return float64(same) >= graphicRunShare*float64(total)
}

// graphicRunShare is the share of pixels matching their left neighbor above
// which an image counts as a graphic.
const graphicRunShare = 0.4

// Shrink quickly reduces m to fit inside a box by averaging samples. It costs
// far less than a resampling filter on large photos, and is meant for previews.
func Shrink(m *image.NRGBA, boxW, boxH int) *image.NRGBA {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	nw, nh := fit(w, h, boxW, boxH)
	if nw == w && nh == h {
		return m
	}
	out := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	const samplesPerSide = 4
	for y := range nh {
		y0, y1 := y*h/nh, max((y+1)*h/nh, y*h/nh+1)
		stepY := max((y1-y0)/samplesPerSide, 1)
		for x := range nw {
			x0, x1 := x*w/nw, max((x+1)*w/nw, x*w/nw+1)
			stepX := max((x1-x0)/samplesPerSide, 1)
			var r, g, b, a, n int
			for sy := y0; sy < y1; sy += stepY {
				row := m.Pix[sy*m.Stride:]
				for sx := x0; sx < x1; sx += stepX {
					p := row[sx*4 : sx*4+4]
					pa := int(p[3])
					r += int(p[0]) * pa
					g += int(p[1]) * pa
					b += int(p[2]) * pa
					a += pa
					n++
				}
			}
			if a > 0 {
				i := out.PixOffset(x, y)
				out.Pix[i], out.Pix[i+1], out.Pix[i+2] = uint8(r/a), uint8(g/a), uint8(b/a)
				out.Pix[i+3] = uint8(a / n)
			}
		}
	}
	return out
}
