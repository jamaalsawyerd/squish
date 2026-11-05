package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"

	"github.com/gen2brain/jpegn"
)

// noise makes a photo-like image: it has too many colors for a palette and no
// long runs of identical pixels.
func noise(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	v := uint32(1)
	for i := 0; i < len(m.Pix); i += 4 {
		v = v*1664525 + 1013904223
		x := i / 4 % w
		y := i / 4 / w
		m.Pix[i] = uint8(x*255/w) + uint8(v>>28)
		m.Pix[i+1] = uint8(y*255/h) + uint8(v>>24)&15
		m.Pix[i+2] = uint8((x+y)*255/(w+h)) + uint8(v>>20)&15
		m.Pix[i+3] = 255
	}
	return m
}

// tiffOrientation builds a minimal EXIF payload.
func tiffOrientation(v uint16) []byte {
	var b bytes.Buffer
	b.WriteString("II*\x00")
	_ = binary.Write(&b, binary.LittleEndian, uint32(8))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, []uint16{0x0112, 3})
	_ = binary.Write(&b, binary.LittleEndian, uint32(1))
	_ = binary.Write(&b, binary.LittleEndian, []uint16{v, 0})
	_ = binary.Write(&b, binary.LittleEndian, uint32(0))
	return b.Bytes()
}

func readOrientation(t *testing.T, tiff []byte) uint16 {
	t.Helper()
	if len(tiff) < 20 {
		t.Fatalf("exif too short: %d bytes", len(tiff))
	}
	return binary.LittleEndian.Uint16(tiff[18:])
}

func TestSniff(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want Format
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, JPEG},
		{"png", []byte("\x89PNG\r\n\x1a\nxxxx"), PNG},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), WebP},
		{"avif", []byte("\x00\x00\x00\x20ftypavif"), AVIF},
		{"gif", []byte("GIF89axxxx"), GIF},
		{"text", []byte("hello world, not an image"), Unknown},
		{"empty", nil, Unknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sniff(tt.data); got != tt.want {
				t.Errorf("Sniff() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	src := noise(64, 48)
	for _, f := range []Format{JPEG, PNG, WebP, AVIF} {
		t.Run(f.String(), func(t *testing.T) {
			data, err := Encode(src, true, Options{Format: f, Level: LevelHigh})
			if err != nil {
				t.Fatal(err)
			}
			if got := Sniff(data); got != f {
				t.Fatalf("wrote %v, want %v", got, f)
			}
			d, err := Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			if d.Image.Rect != src.Rect {
				t.Errorf("bounds = %v, want %v", d.Image.Rect, src.Rect)
			}
			if s := SSIM(src, d.Image); s < 0.5 {
				t.Errorf("SSIM = %.3f, image was badly damaged", s)
			}
		})
	}
}

func TestDecodeAppliesRotationAndResetsTag(t *testing.T) {
	var buf bytes.Buffer
	err := jpegn.Encode(&buf, noise(64, 32), &jpegn.EncodeOptions{Quality: 90})
	if err != nil {
		t.Fatal(err)
	}
	data := insertJPEGSegments(buf.Bytes(), nil, tiffOrientation(6))
	d, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Image.Rect.Size(); got != image.Pt(32, 64) {
		t.Errorf("size = %v, want rotated 32x64", got)
	}
	if got := readOrientation(t, d.Exif); got != 1 {
		t.Errorf("orientation = %d, want 1 so viewers do not rotate it twice", got)
	}
}

func TestICCSurvivesEachFormat(t *testing.T) {
	icc := bytes.Repeat([]byte("display-p3-profile"), 500)
	src := noise(32, 32)
	for _, f := range []Format{JPEG, PNG, WebP} {
		t.Run(f.String(), func(t *testing.T) {
			data, err := Encode(src, true, Options{Format: f, Level: LevelHigh, ICC: icc})
			if err != nil {
				t.Fatal(err)
			}
			d, err := Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(d.ICC, icc) {
				t.Errorf("ICC profile changed: got %d bytes, want %d", len(d.ICC), len(icc))
			}
		})
	}
}

func TestJPEGICCSpansSegments(t *testing.T) {
	icc := bytes.Repeat([]byte{7}, jpegICCChunk*2+100)
	data, err := Encode(noise(16, 16), true, Options{Format: JPEG, Level: 5, ICC: icc})
	if err != nil {
		t.Fatal(err)
	}
	if got := jpegICC(data); !bytes.Equal(got, icc) {
		t.Errorf("got %d bytes, want %d", len(got), len(icc))
	}
}

func TestExifSurvivesEachFormat(t *testing.T) {
	exif := tiffOrientation(1)
	for _, f := range []Format{JPEG, PNG, WebP} {
		t.Run(f.String(), func(t *testing.T) {
			data, err := Encode(noise(16, 16), true, Options{Format: f, Level: LevelHigh, Exif: exif})
			if err != nil {
				t.Fatal(err)
			}
			d, err := Decode(data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(d.Exif, exif) {
				t.Errorf("exif = %x, want %x", d.Exif, exif)
			}
		})
	}
}

func TestWebPMuxKeepsAlpha(t *testing.T) {
	src := noise(16, 16)
	src.Pix[3] = 0
	data, err := Encode(src, false, Options{Format: WebP, Level: LevelHigh, ICC: []byte("profile")})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if d.Opaque {
		t.Error("transparency was lost")
	}
}

func TestPNGPaletteIsExactForFewColors(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = uint8(i/4%200), 9, 80, 128
	}
	data, err := Encode(src, false, Options{Format: PNG, Level: LevelSmall})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Image.Pix, src.Pix) {
		t.Error("palette PNG changed pixel values")
	}
}

func TestPNGRefusesBadPalette(t *testing.T) {
	src := noise(256, 256)
	data, err := Encode(src, true, Options{Format: PNG, Level: MinLevel})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if s := SSIM(src, d.Image); s < TargetSSIM(MinLevel) {
		t.Errorf("SSIM = %.3f, below the target of %.3f", s, TargetSSIM(MinLevel))
	}
}

func TestJPEGFlattensTransparency(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 32, 32)) // Fully transparent.
	data, err := Encode(src, false, Options{
		Format: JPEG, Quality: 95, Background: color.NRGBA{255, 0, 0, 255},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if c := d.Image.NRGBAAt(8, 8); c.R < 240 || c.G > 15 || c.B > 15 {
		t.Errorf("got %v, want the red background", c)
	}
}

func TestQualityScale(t *testing.T) {
	for _, f := range []Format{JPEG, WebP, AVIF} {
		for l := MinLevel + 1; l <= MaxLevel; l++ {
			if Quality(f, l) > Quality(f, l-1) {
				t.Errorf("%v: level %d has higher quality than level %d", f, l, l-1)
			}
		}
	}
	if Quality(JPEG, 99) != Quality(JPEG, MaxLevel) {
		t.Error("levels should clamp")
	}
}

func TestAnimatedGIFIsRefused(t *testing.T) {
	frame := image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	err := gif.EncodeAll(&buf, &gif.GIF{Image: []*image.Paletted{frame, frame}, Delay: []int{1, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(buf.Bytes()); !errors.Is(err, ErrAnimated) {
		t.Errorf("err = %v, want ErrAnimated", err)
	}
}

func TestTruncatedJPEGIsFlagged(t *testing.T) {
	var buf bytes.Buffer
	err := jpegn.Encode(&buf, noise(128, 128), &jpegn.EncodeOptions{Quality: 90})
	if err != nil {
		t.Fatal(err)
	}
	whole, err := Decode(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if whole.Damaged {
		t.Error("complete file was flagged as damaged")
	}
	cut, err := Decode(buf.Bytes()[:buf.Len()*2/3])
	if err != nil {
		t.Fatal(err)
	}
	if !cut.Damaged {
		t.Error("truncated file was not flagged")
	}
}

func TestTruncatedProgressiveJPEGFails(t *testing.T) {
	data, err := Encode(noise(128, 128), true, Options{Format: JPEG, Quality: 90})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data[:len(data)*2/3]); !errors.Is(err, ErrDamaged) {
		t.Errorf("err = %v, want ErrDamaged", err)
	}
}

func TestHugeDeclaredSizeIsRefused(t *testing.T) {
	// A tiny GIF claiming 65535×65535, with no pixels behind it.
	data := []byte("GIF89a\xFF\xFF\xFF\xFF\x00\x00\x00\x2C")
	if _, err := Decode(data); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

func TestBadHeaderIsReportedAsDamage(t *testing.T) {
	data := []byte("\x89PNG\r\n\x1a\nnot really a png at all, just text")
	if _, err := Decode(data); !errors.Is(err, ErrDamaged) {
		t.Errorf("err = %v, want ErrDamaged", err)
	}
}

func TestIsGraphic(t *testing.T) {
	flat := image.NewNRGBA(image.Rect(0, 0, 200, 200))
	for i := 3; i < len(flat.Pix); i += 4 {
		flat.Pix[i] = 255
	}
	if !IsGraphic(flat) {
		t.Error("flat image should count as a graphic")
	}
	if IsGraphic(noise(200, 200)) {
		t.Error("noisy image should not count as a graphic")
	}
}

func TestFit(t *testing.T) {
	tests := []struct{ w, h, bw, bh, ww, wh int }{
		{4000, 3000, 2000, 2000, 2000, 1500},
		{3000, 4000, 2000, 2000, 1500, 2000},
		{100, 100, 2000, 2000, 100, 100},
		{5000, 1, 100, 100, 100, 1},
	}
	for _, tt := range tests {
		w, h := fit(tt.w, tt.h, tt.bw, tt.bh)
		if w != tt.ww || h != tt.wh {
			t.Errorf("fit(%d,%d) = %d,%d, want %d,%d", tt.w, tt.h, w, h, tt.ww, tt.wh)
		}
	}
}
