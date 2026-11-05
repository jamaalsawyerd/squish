// Package codec decodes and encodes images. It hides the differences between
// the underlying libraries behind a single straight-alpha pixel format.
package codec

import (
	"bytes"
	"errors"
)

// Format is an image file format.
type Format int

const (
	Unknown Format = iota
	JPEG
	PNG
	WebP
	AVIF
	GIF
)

// ErrAnimated reports a file with more than one frame. squish does not handle
// those yet, and flattening one to its first frame would lose data.
var ErrAnimated = errors.New("animated images are not supported yet")

// ErrUnknownFormat reports a file that is not a supported image.
var ErrUnknownFormat = errors.New("not a supported image")

func (f Format) String() string {
	switch f {
	case JPEG:
		return "JPEG"
	case PNG:
		return "PNG"
	case WebP:
		return "WebP"
	case AVIF:
		return "AVIF"
	case GIF:
		return "GIF"
	}
	return "unknown"
}

// Ext returns the file extension for f, including the dot.
func (f Format) Ext() string {
	switch f {
	case JPEG:
		return ".jpg"
	case PNG:
		return ".png"
	case WebP:
		return ".webp"
	case AVIF:
		return ".avif"
	case GIF:
		return ".gif"
	}
	return ""
}

// Sniff identifies a format from the start of a file, ignoring its extension.
func Sniff(b []byte) Format {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return JPEG
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return PNG
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return WebP
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return GIF
	case len(b) >= 12 && string(b[4:8]) == "ftyp" && isAVIFBrand(string(b[8:12])):
		return AVIF
	}
	return Unknown
}

func isAVIFBrand(s string) bool { return s == "avif" || s == "avis" }
