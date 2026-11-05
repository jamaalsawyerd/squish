package codec

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"

	"github.com/ericpauley/go-quantize/quantize"
	"github.com/gen2brain/pngn"
)

const maxPalette = 256

// encodePNG picks the smallest PNG that still looks right. Plain re-encoding
// saves almost nothing, so the real savings come from using a palette: exact
// if the image has few colors, approximate if that passes a quality check.
func encodePNG(m *image.NRGBA, _ bool, o Options) ([]byte, error) {
	meta := pngMetadata(o)
	if pal := exactPalette(m); pal != nil {
		return writePNG(pal, meta)
	}
	if !o.Lossless {
		pal := quantized(m)
		if SSIM(m, pal) >= TargetSSIM(o.Level) {
			return writePNG(pal, meta)
		}
	}
	return writePNG(m, meta)
}

func pngMetadata(o Options) *pngn.Metadata {
	if len(o.ICC) == 0 && len(o.Exif) == 0 {
		return nil
	}
	meta := &pngn.Metadata{Exif: o.Exif}
	if len(o.ICC) > 0 {
		meta.ICCProfile = &pngn.ICCProfile{Name: "icc", Profile: o.ICC}
	}
	return meta
}

func writePNG(m image.Image, meta *pngn.Metadata) ([]byte, error) {
	var buf bytes.Buffer
	err := pngn.Encode(&buf, m, &pngn.EncodeOptions{
		CompressionLevel: pngn.BestCompression,
		Metadata:         meta,
	})
	return buf.Bytes(), err
}

func packColor(p []uint8) uint32 {
	return uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16 | uint32(p[3])<<24
}

// exactPalette returns a lossless paletted copy of m, or nil if it has too
// many colors for one.
func exactPalette(m *image.NRGBA) *image.Paletted {
	index := make(map[uint32]uint8, maxPalette)
	var pal color.Palette
	w, h := m.Rect.Dx(), m.Rect.Dy()
	out := image.NewPaletted(image.Rect(0, 0, w, h), nil)
	for y := range h {
		row := m.Pix[y*m.Stride : y*m.Stride+w*4]
		dst := out.Pix[y*out.Stride : y*out.Stride+w]
		for x := range w {
			p := row[x*4 : x*4+4]
			c := packColor(p)
			i, ok := index[c]
			if !ok {
				if len(pal) == maxPalette {
					return nil
				}
				i = uint8(len(pal))
				index[c] = i
				pal = append(pal, color.NRGBA{p[0], p[1], p[2], p[3]})
			}
			dst[x] = i
		}
	}
	out.Palette = pal
	return out
}

// quantized maps m onto a 256 color palette. It does not dither: in testing
// that made files larger and scored worse on both photos and logos.
func quantized(m *image.NRGBA) *image.Paletted {
	pal := quantize.MedianCutQuantizer{}.Quantize(make(color.Palette, 0, maxPalette), m)
	out := image.NewPaletted(m.Rect, pal)
	draw.Draw(out, m.Rect, m, m.Rect.Min, draw.Src)
	return out
}
