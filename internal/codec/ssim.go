package codec

import (
	"image"
	"image/color"
	"image/draw"
)

// SSIM scores how close b is to a, from 0 to 1, on luma. Transparent pixels
// are judged over mid gray so that alpha errors count.
func SSIM(a image.Image, b image.Image) float64 {
	w, h := a.Bounds().Dx(), a.Bounds().Dy()
	return ssim(luma(a), luma(b), w, h)
}

func luma(m image.Image) []float32 {
	b := m.Bounds()
	w, h := b.Dx(), b.Dy()
	flat := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(flat, flat.Rect, image.NewUniform(color.Gray{128}), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Rect, m, b.Min, draw.Over)
	out := make([]float32, w*h)
	for y := range h {
		row := flat.Pix[y*flat.Stride : y*flat.Stride+w*4]
		for x := range w {
			p := row[x*4 : x*4+3]
			out[y*w+x] = 0.2126*float32(p[0]) + 0.7152*float32(p[1]) + 0.0722*float32(p[2])
		}
	}
	return out
}

// ssim averages over 8x8 windows. The step is coprime with 8 so that windows
// do not all line up with JPEG blocks.
func ssim(a, b []float32, w, h int) float64 {
	const (
		win, step = 8, 5
		n         = win * win
		c1, c2    = 6.5025, 58.5225
	)
	var sum float64
	var count int
	for y := 0; y+win <= h; y += step {
		for x := 0; x+win <= w; x += step {
			var sa, sb, saa, sbb, sab float64
			for j := range win {
				ra := a[(y+j)*w+x : (y+j)*w+x+win]
				rb := b[(y+j)*w+x : (y+j)*w+x+win]
				for i := range win {
					pa, pb := float64(ra[i]), float64(rb[i])
					sa += pa
					sb += pb
					saa += pa * pa
					sbb += pb * pb
					sab += pa * pb
				}
			}
			ma, mb := sa/n, sb/n
			va, vb, cov := saa/n-ma*ma, sbb/n-mb*mb, sab/n-ma*mb
			sum += ((2*ma*mb + c1) * (2*cov + c2)) / ((ma*ma + mb*mb + c1) * (va + vb + c2))
			count++
		}
	}
	if count == 0 {
		return 1
	}
	return sum / float64(count)
}
