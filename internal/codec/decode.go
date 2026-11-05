package codec

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"

	"github.com/gen2brain/avif"
	"github.com/gen2brain/jpegn"
	"github.com/gen2brain/pngn"
	vpxwebp "github.com/gen2brain/vpx/webp"
)

// Decoded is an upright image plus the metadata worth carrying forward.
type Decoded struct {
	// Image always has straight alpha and starts at the origin. Premultiplied
	// images add rounding noise on soft edges, which can quadruple a PNG.
	Image  *image.NRGBA
	Format Format
	Opaque bool
	ICC    []byte // Raw ICC profile, if any.
	Exif   []byte // TIFF payload with no "Exif" prefix, orientation reset to 1.
	// Damaged means the file was cut short. What survived was still decoded,
	// which is the best that can be done, but the image will be incomplete.
	Damaged bool
}

// Info is what can be learned about a file without decoding its pixels.
type Info struct {
	Format        Format
	Width, Height int
}

// Probe reads a file's format and dimensions from its header.
func Probe(data []byte) (Info, error) {
	f := Sniff(data)
	var cfg image.Config
	var err error
	r := bytes.NewReader(data)
	switch f {
	case JPEG:
		cfg, err = jpegn.DecodeConfig(r)
	case PNG:
		cfg, err = pngn.DecodeConfig(r)
	case WebP:
		cfg, err = vpxwebp.DecodeConfig(r)
	case AVIF:
		cfg, err = avif.DecodeConfig(r)
	case GIF:
		cfg, err = gif.DecodeConfig(r)
	default:
		return Info{}, ErrUnknownFormat
	}
	if err != nil {
		return Info{Format: f}, fmt.Errorf("%w: reading %s header: %w", ErrDamaged, f, err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxPixels {
		return Info{Format: f}, fmt.Errorf("%w: %d×%d", ErrTooLarge, cfg.Width, cfg.Height)
	}
	return Info{Format: f, Width: cfg.Width, Height: cfg.Height}, nil
}

// Decode reads an image, applies any EXIF rotation and extracts metadata.
func Decode(data []byte) (*Decoded, error) {
	if _, err := Probe(data); err != nil {
		return nil, err
	}
	d := &Decoded{Format: Sniff(data)}
	var (
		m   image.Image
		err error
	)
	switch d.Format {
	case JPEG:
		m, err = decodeJPEG(data, d)
	case PNG:
		m, err = decodePNG(data, d)
	case WebP:
		m, err = decodeWebP(data, d)
	case AVIF:
		m, err = decodeAVIF(data)
	case GIF:
		m, err = decodeGIF(data)
	default:
		return nil, ErrUnknownFormat
	}
	if err != nil {
		return nil, err
	}
	d.Image = toNRGBA(m)
	d.Opaque = isOpaque(d.Image)
	d.Exif = resetOrientation(d.Exif)
	return d, nil
}

// DecodePreview decodes for display only, without metadata. JPEGs are decoded
// at reduced scale, which is much cheaper for large photos.
func DecodePreview(data []byte, maxDim int) (*image.NRGBA, Format, error) {
	f := Sniff(data)
	if f == JPEG {
		info, err := Probe(data)
		if err != nil {
			return nil, f, err
		}
		denom := 1
		for denom < 8 && max(info.Width, info.Height)/(denom*2) >= maxDim {
			denom *= 2
		}
		m, err := jpegn.Decode(bytes.NewReader(data), &jpegn.Options{
			ToRGBA: true, AutoRotate: true, ScaleDenom: denom,
		})
		if err != nil {
			return nil, f, err
		}
		return Shrink(toNRGBA(m), maxDim, maxDim), f, nil
	}
	d, err := Decode(data)
	if err != nil {
		return nil, f, err
	}
	return Shrink(d.Image, maxDim, maxDim), f, nil
}

func decodeJPEG(data []byte, d *Decoded) (image.Image, error) {
	m, err := jpegn.Decode(bytes.NewReader(data), &jpegn.Options{
		ToRGBA:         true,
		AutoRotate:     true,
		UpsampleMethod: jpegn.CatmullRom,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDamaged, err)
	}
	d.ICC = jpegICC(data)
	d.Damaged = !jpegComplete(data)
	if raw, err := jpegn.RawExif(bytes.NewReader(data)); err == nil {
		d.Exif = stripExifPrefix(raw)
	}
	return m, nil
}

// jpegComplete looks for the end of image marker. Some cameras and editors
// write padding after it, so it need not be the last two bytes.
func jpegComplete(data []byte) bool {
	const searchTail = 1 << 20
	tail := data[max(len(data)-searchTail, 0):]
	return bytes.LastIndex(tail, []byte{0xFF, 0xD9}) >= 0
}

func decodePNG(data []byte, d *Decoded) (image.Image, error) {
	if anim, err := pngn.Animated(bytes.NewReader(data)); err == nil && anim {
		return nil, ErrAnimated
	}
	var meta pngn.Metadata
	m, err := pngn.Decode(bytes.NewReader(data), &pngn.Options{Metadata: &meta, AutoRotate: true})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDamaged, err)
	}
	if meta.ICCProfile != nil {
		d.ICC = meta.ICCProfile.Profile
	}
	d.Exif = stripExifPrefix(meta.Exif)
	return m, nil
}

func decodeWebP(data []byte, d *Decoded) (image.Image, error) {
	chunks := webpChunks(data)
	if webpAnimated(chunks) {
		return nil, ErrAnimated
	}
	m, err := vpxwebp.Decode(bytes.NewReader(data), vpxwebp.Options{AutoRotate: true, Threads: 1})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDamaged, err)
	}
	d.ICC = webpChunk(chunks, "ICCP")
	d.Exif = stripExifPrefix(webpChunk(chunks, "EXIF"))
	return m, nil
}

func decodeAVIF(data []byte) (image.Image, error) {
	if len(data) >= 12 && string(data[8:12]) == "avis" {
		return nil, ErrAnimated
	}
	m, err := avif.Decode(bytes.NewReader(data), avif.Options{AutoRotate: true})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDamaged, err)
	}
	return m, nil
}

func decodeGIF(data []byte) (image.Image, error) {
	g, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDamaged, err)
	}
	if len(g.Image) > 1 {
		return nil, ErrAnimated
	}
	return g.Image[0], nil
}

// toNRGBA converts to straight alpha at the origin, reusing the pixels when it
// can. Opaque RGBA and NRGBA have identical bytes, so no copy is needed.
func toNRGBA(m image.Image) *image.NRGBA {
	b := m.Bounds()
	switch t := m.(type) {
	case *image.NRGBA:
		if b.Min == (image.Point{}) {
			return t
		}
	case *image.RGBA:
		if b.Min == (image.Point{}) && rgbaOpaque(t) {
			return &image.NRGBA{Pix: t.Pix, Stride: t.Stride, Rect: t.Rect}
		}
	case *image.Paletted:
		return palettedToNRGBA(t)
	}
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	if _, opaque := m.(*image.YCbCr); opaque {
		draw.Draw(asRGBA(out), out.Rect, m, b.Min, draw.Src)
		return out
	}
	for y := range b.Dy() {
		for x := range b.Dx() {
			out.SetNRGBA(x, y, color.NRGBAModel.Convert(m.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA))
		}
	}
	return out
}

func palettedToNRGBA(p *image.Paletted) *image.NRGBA {
	lut := make([]color.NRGBA, len(p.Palette))
	for i, c := range p.Palette {
		lut[i] = color.NRGBAModel.Convert(c).(color.NRGBA)
	}
	b := p.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := range b.Dy() {
		row := p.Pix[y*p.Stride : y*p.Stride+b.Dx()]
		for x, i := range row {
			out.SetNRGBA(x, y, lut[i])
		}
	}
	return out
}

func rgbaOpaque(m *image.RGBA) bool {
	return alphaAll255(m.Pix, m.Stride, m.Rect.Dx(), m.Rect.Dy())
}

func isOpaque(m *image.NRGBA) bool {
	return alphaAll255(m.Pix, m.Stride, m.Rect.Dx(), m.Rect.Dy())
}

func alphaAll255(pix []uint8, stride, w, h int) bool {
	for y := range h {
		row := pix[y*stride : y*stride+w*4]
		for i := 3; i < len(row); i += 4 {
			if row[i] != 255 {
				return false
			}
		}
	}
	return true
}

// asRGBA views an opaque NRGBA image as RGBA. The bytes are identical, and
// image/draw has fast paths for RGBA that it lacks for NRGBA.
func asRGBA(m *image.NRGBA) *image.RGBA {
	return &image.RGBA{Pix: m.Pix, Stride: m.Stride, Rect: m.Rect}
}
