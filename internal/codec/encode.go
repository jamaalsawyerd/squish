package codec

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/jpegli"
	vpxwebp "github.com/gen2brain/vpx/webp"
)

// avifSpeed gets most of AVIF's savings at a quarter of the time of speed 8.
const avifSpeed = 10

// webpMethod is libwebp's default balance of speed and size.
const webpMethod = 4

// Options controls one encode.
type Options struct {
	Format   Format
	Level    int  // Squeeze level, MinLevel to MaxLevel. Used when Quality is 0.
	Quality  int  // Native encoder quality, overriding Level when not 0.
	Lossless bool // WebP only.
	Graphic  bool // Keep full chroma resolution, for sharp edges and text.

	Background color.NRGBA // Replaces transparency in formats without alpha.
	ICC        []byte
	Exif       []byte
}

func (o Options) quality() int {
	if o.Quality > 0 {
		return min(o.Quality, 100)
	}
	return Quality(o.Format, o.Level)
}

// Encode compresses m in the requested format. Set opaque when m has no
// transparency, so that encoders can use their faster paths.
func Encode(m *image.NRGBA, opaque bool, o Options) ([]byte, error) {
	var (
		out []byte
		err error
	)
	switch o.Format {
	case JPEG:
		out, err = encodeJPEG(m, opaque, o)
	case PNG:
		out, err = encodePNG(m, opaque, o)
	case WebP:
		out, err = encodeWebP(m, opaque, o)
	case AVIF:
		out, err = encodeAVIF(m, opaque, o)
	default:
		return nil, fmt.Errorf("cannot encode %s", o.Format)
	}
	if err != nil {
		return nil, fmt.Errorf("encoding %s: %w", o.Format, err)
	}
	return out, nil
}

// pixels returns the view of m that encoders should read.
func pixels(m *image.NRGBA, opaque bool) image.Image {
	if opaque {
		return asRGBA(m)
	}
	return m
}

func encodeJPEG(m *image.NRGBA, opaque bool, o Options) ([]byte, error) {
	src := asRGBA(m)
	if !opaque {
		src = flatten(m, o.Background)
	}
	chroma := image.YCbCrSubsampleRatio420
	if o.Graphic {
		chroma = image.YCbCrSubsampleRatio444
	}
	// Passing options replaces every default, so the ones that make jpegli
	// worthwhile must be switched back on here.
	var buf bytes.Buffer
	err := jpegli.Encode(&buf, src, &jpegli.EncodingOptions{
		Quality:              o.quality(),
		ChromaSubsampling:    chroma,
		ProgressiveLevel:     2,
		OptimizeCoding:       true,
		AdaptiveQuantization: true,
	})
	if err != nil {
		return nil, err
	}
	return insertJPEGSegments(buf.Bytes(), o.ICC, o.Exif), nil
}

func encodeWebP(m *image.NRGBA, opaque bool, o Options) ([]byte, error) {
	var buf bytes.Buffer
	err := vpxwebp.Encode(&buf, pixels(m, opaque), vpxwebp.EncodeOptions{
		Quality:  o.quality(),
		Lossless: o.Lossless,
		Method:   webpMethod,
		Threads:  1, // The engine already runs one encode per core.
	})
	if err != nil {
		return nil, err
	}
	b := m.Bounds()
	return muxWebP(buf.Bytes(), o.ICC, o.Exif, b.Dx(), b.Dy(), !opaque), nil
}

func encodeAVIF(m *image.NRGBA, opaque bool, o Options) ([]byte, error) {
	chroma := image.YCbCrSubsampleRatio420
	if o.Graphic {
		chroma = image.YCbCrSubsampleRatio444
	}
	q := o.quality()
	var buf bytes.Buffer
	err := avif.Encode(&buf, pixels(m, opaque), avif.Options{
		Quality:           q,
		QualityAlpha:      q,
		Speed:             avifSpeed,
		ChromaSubsampling: chroma,
	})
	return buf.Bytes(), err
}

// flatten composites m onto a solid background.
func flatten(m *image.NRGBA, bg color.NRGBA) *image.RGBA {
	out := image.NewRGBA(m.Rect)
	draw.Draw(out, out.Rect, image.NewUniform(bg), image.Point{}, draw.Src)
	draw.Draw(out, out.Rect, m, m.Rect.Min, draw.Over)
	return out
}
