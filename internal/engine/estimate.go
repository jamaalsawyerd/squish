package engine

import (
	"image"

	"github.com/jamaalsawyerd/squish/internal/codec"
)

// A real encode takes too long to run on every setting change, so estimates
// encode a mosaic of tiles sampled across the image and scale the result up.
const (
	estimateTile = 96
	estimateGrid = 8
)

// Estimate predicts the size of the output. Small images are simply encoded.
func Estimate(dec *codec.Decoded, o Options) Output {
	src := dec.Image
	if o.MaxDim > 0 {
		src = codec.Shrink(src, o.MaxDim, o.MaxDim)
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	o.MaxDim = 0
	side := estimateTile * estimateGrid
	if w <= side && h <= side {
		sample := *dec
		sample.Image = src
		return Compress(&sample, o)
	}
	mosaic := tileMosaic(src)
	sample := *dec
	sample.Image = mosaic
	sample.ICC, sample.Exif = nil, nil
	out := Compress(&sample, o)
	if out.Err != nil {
		return out
	}
	ratio := float64(w*h) / float64(mosaic.Rect.Dx()*mosaic.Rect.Dy())
	out.Data = make([]byte, int(float64(len(out.Data))*ratio))
	out.Width, out.Height = w, h
	return out
}

// tileMosaic stitches evenly spaced tiles into one image.
func tileMosaic(m *image.NRGBA) *image.NRGBA {
	w, h := m.Rect.Dx(), m.Rect.Dy()
	tw, th := min(estimateTile, w/estimateGrid), min(estimateTile, h/estimateGrid)
	out := image.NewNRGBA(image.Rect(0, 0, tw*estimateGrid, th*estimateGrid))
	for gy := range estimateGrid {
		for gx := range estimateGrid {
			sx := gx * (w - tw) / (estimateGrid - 1)
			sy := gy * (h - th) / (estimateGrid - 1)
			for y := range th {
				src := m.Pix[m.PixOffset(sx, sy+y):]
				dst := out.Pix[out.PixOffset(gx*tw, gy*th+y):]
				copy(dst[:tw*4], src[:tw*4])
			}
		}
	}
	return out
}
