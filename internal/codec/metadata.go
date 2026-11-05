package codec

import (
	"bytes"
	"encoding/binary"
)

const (
	jpegICCTag = "ICC_PROFILE\x00"
	exifPrefix = "Exif\x00\x00"
	// A JPEG segment holds 65533 payload bytes, minus 14 for the ICC header.
	jpegICCChunk = 65519
)

func stripExifPrefix(b []byte) []byte {
	return bytes.TrimPrefix(b, []byte(exifPrefix))
}

// IsSRGB reports whether an ICC profile describes sRGB. Those can be dropped
// safely, since untagged images are displayed as sRGB anyway.
func IsSRGB(icc []byte) bool {
	return bytes.Contains(icc, []byte("sRGB")) ||
		bytes.Contains(icc, []byte("s\x00R\x00G\x00B")) ||
		bytes.Contains(icc, []byte("\x00s\x00R\x00G\x00B"))
}

// jpegSegments walks the marker segments before the image data.
func jpegSegments(data []byte, fn func(marker byte, payload []byte)) {
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return
		}
		m := data[i+1]
		if m == 0xFF { // Fill byte.
			i++
			continue
		}
		if m == 0xDA || m == 0xD9 { // Start of scan or end of image.
			return
		}
		n := int(binary.BigEndian.Uint16(data[i+2:]))
		if n < 2 || i+2+n > len(data) {
			return
		}
		fn(m, data[i+4:i+2+n])
		i += 2 + n
	}
}

// jpegICC reassembles an ICC profile from its APP2 chunks.
func jpegICC(data []byte) []byte {
	chunks := map[int][]byte{}
	total := 0
	jpegSegments(data, func(marker byte, p []byte) {
		if marker != 0xE2 || len(p) < 14 || !bytes.HasPrefix(p, []byte(jpegICCTag)) {
			return
		}
		chunks[int(p[12])] = p[14:]
		total = int(p[13])
	})
	if total == 0 || len(chunks) != total {
		return nil
	}
	var out []byte
	for i := 1; i <= total; i++ {
		c, ok := chunks[i]
		if !ok {
			return nil
		}
		out = append(out, c...)
	}
	return out
}

// insertJPEGSegments writes Exif and ICC segments after the SOI and any JFIF
// header. jpegli has no API for this, but the format makes it simple.
func insertJPEGSegments(data, icc, exif []byte) []byte {
	if len(icc) == 0 && len(exif) == 0 {
		return data
	}
	at := 2
	if len(data) > 6 && data[2] == 0xFF && data[3] == 0xE0 {
		at = 4 + int(binary.BigEndian.Uint16(data[4:])) // Marker plus APP0 length.
	}
	var seg bytes.Buffer
	write := func(marker byte, parts ...[]byte) {
		n := 2
		for _, p := range parts {
			n += len(p)
		}
		seg.Write([]byte{0xFF, marker, byte(n >> 8), byte(n)})
		for _, p := range parts {
			seg.Write(p)
		}
	}
	if len(exif) > 0 && len(exif)+len(exifPrefix)+2 <= 0xFFFF {
		write(0xE1, []byte(exifPrefix), exif)
	}
	total := (len(icc) + jpegICCChunk - 1) / jpegICCChunk
	if total <= 255 {
		for i := range total {
			end := min((i+1)*jpegICCChunk, len(icc))
			write(0xE2, []byte(jpegICCTag), []byte{byte(i + 1), byte(total)}, icc[i*jpegICCChunk:end])
		}
	}
	out := make([]byte, 0, len(data)+seg.Len())
	out = append(out, data[:at]...)
	out = append(out, seg.Bytes()...)
	return append(out, data[at:]...)
}

// resetOrientation sets the EXIF orientation tag to 1, because the pixels have
// already been rotated. A rotated image with the tag left in place would be
// turned a second time by viewers. The input is not modified.
func resetOrientation(tiff []byte) []byte {
	if len(tiff) < 8 {
		return tiff
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return tiff
	}
	ifd := int(order.Uint32(tiff[4:]))
	if ifd+2 > len(tiff) {
		return tiff
	}
	count := int(order.Uint16(tiff[ifd:]))
	for i := range count {
		e := ifd + 2 + i*12
		if e+12 > len(tiff) {
			return tiff
		}
		if order.Uint16(tiff[e:]) == 0x0112 {
			out := bytes.Clone(tiff)
			order.PutUint16(out[e+8:], 1)
			return out
		}
	}
	return tiff
}

type riffChunk struct {
	id   string
	data []byte
}

// webpChunks splits a WebP file into its RIFF chunks.
func webpChunks(data []byte) []riffChunk {
	var out []riffChunk
	for i := 12; i+8 <= len(data); {
		n := int(binary.LittleEndian.Uint32(data[i+4:]))
		if n < 0 || i+8+n > len(data) {
			break
		}
		out = append(out, riffChunk{string(data[i : i+4]), data[i+8 : i+8+n]})
		i += 8 + n + n&1
	}
	return out
}

func webpChunk(chunks []riffChunk, id string) []byte {
	for _, c := range chunks {
		if c.id == id {
			return c.data
		}
	}
	return nil
}

func webpAnimated(chunks []riffChunk) bool {
	x := webpChunk(chunks, "VP8X")
	return len(x) > 0 && x[0]&0x02 != 0
}

// muxWebP adds ICC and Exif chunks, which the encoder has no options for. Both
// need the extended VP8X header, so a simple file is converted to that form.
func muxWebP(data, icc, exif []byte, w, h int, alpha bool) []byte {
	if len(icc) == 0 && len(exif) == 0 {
		return data
	}
	chunks := webpChunks(data)
	x := bytes.Clone(webpChunk(chunks, "VP8X"))
	if x == nil {
		x = make([]byte, 10)
		putUint24(x[4:], w-1)
		putUint24(x[7:], h-1)
		if alpha {
			x[0] |= 0x10
		}
	}
	if len(icc) > 0 {
		x[0] |= 0x20
	}
	if len(exif) > 0 {
		x[0] |= 0x08
	}
	var body bytes.Buffer
	body.WriteString("WEBP")
	writeChunk(&body, "VP8X", x)
	if len(icc) > 0 {
		writeChunk(&body, "ICCP", icc)
	}
	for _, c := range chunks {
		if c.id != "VP8X" {
			writeChunk(&body, c.id, c.data)
		}
	}
	if len(exif) > 0 {
		writeChunk(&body, "EXIF", exif)
	}
	out := make([]byte, 8, 8+body.Len())
	copy(out, "RIFF")
	binary.LittleEndian.PutUint32(out[4:], uint32(body.Len()))
	return append(out, body.Bytes()...)
}

func writeChunk(b *bytes.Buffer, id string, data []byte) {
	b.WriteString(id)
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(data)))
	b.Write(n[:])
	b.Write(data)
	if len(data)&1 == 1 {
		b.WriteByte(0)
	}
}

func putUint24(b []byte, v int) {
	b[0], b[1], b[2] = byte(v), byte(v>>8), byte(v>>16)
}
