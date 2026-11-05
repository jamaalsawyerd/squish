// Package engine runs the compression pipeline. It has no UI, so the TUI and
// the plain command line share the same behavior.
package engine

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/jamaalsawyerd/squish/internal/codec"
	"github.com/pbnjay/memory"
	"golang.org/x/sync/semaphore"
)

// Target is the output format to aim for.
type Target int

const (
	// TargetSame keeps each file's format.
	TargetSame Target = iota
	// TargetAuto picks the best format for each image's content.
	TargetAuto
	TargetJPEG
	TargetPNG
	TargetWebP
	TargetAVIF
)

// ParseTarget converts a format name from the command line.
func ParseTarget(s string) (Target, error) {
	switch s {
	case "same", "":
		return TargetSame, nil
	case "auto":
		return TargetAuto, nil
	case "jpeg", "jpg":
		return TargetJPEG, nil
	case "png":
		return TargetPNG, nil
	case "webp":
		return TargetWebP, nil
	case "avif":
		return TargetAVIF, nil
	}
	return 0, fmt.Errorf("unknown format %q (use same, auto, jpeg, png, webp or avif)", s)
}

func (t Target) String() string {
	switch t {
	case TargetSame:
		return "Same as input"
	case TargetAuto:
		return "Auto"
	case TargetJPEG:
		return "JPEG"
	case TargetPNG:
		return "PNG"
	case TargetWebP:
		return "WebP"
	case TargetAVIF:
		return "AVIF"
	}
	return "unknown"
}

// Options configures a run.
type Options struct {
	Target       Target
	Level        int    // Squeeze level. Zero means balanced.
	Quality      int    // Native encoder quality, overriding Level when not 0.
	MaxDim       int    // Longest output side in pixels. Zero keeps the size.
	KeepMetadata bool   // Keep EXIF, including camera details and location.
	OutDir       string // Write here instead of beside the input.
	InPlace      bool   // Replace originals, but only when the output is smaller.
	Workers      int    // Zero picks a sensible number.
	Background   color.NRGBA
	ThumbSize    int // When not 0, Started events carry a thumbnail of this size.
}

// Status is how one file ended.
type Status int

const (
	StatusDone Status = iota
	StatusSkipped
	StatusFailed
	StatusCanceled
)

// Result describes one finished file.
type Result struct {
	Input    string
	Output   string // Empty when nothing was written.
	Status   Status
	Reason   string // Why the file was skipped or failed.
	Warnings []string
	BytesIn  int64
	BytesOut int64
	Format   codec.Format
	Width    int
	Height   int
	Elapsed  time.Duration
}

// Saved returns the bytes saved, or 0 if no smaller file was written.
func (r Result) Saved() int64 {
	if r.Status != StatusDone || r.BytesOut >= r.BytesIn {
		return 0
	}
	return r.BytesIn - r.BytesOut
}

// EventType says what an Event reports.
type EventType int

const (
	FileStarted EventType = iota
	FileFinished
)

// Event reports progress on a single file.
type Event struct {
	Type   EventType
	Index  int // Position in the input list.
	Input  string
	Thumb  *image.NRGBA // Set on FileStarted when Options.ThumbSize is set.
	Result Result       // Set on FileFinished.
}

// Bytes of memory used per pixel while encoding, measured on 12 MP photos.
// These include the decoded image and garbage not yet collected.
var pixelCost = map[codec.Format]int64{
	codec.JPEG: 40,
	codec.PNG:  12,
	codec.WebP: 35,
	codec.AVIF: avifCost,
}

const (
	maxDefaultWorkers = 8
	avifCost          = 300
)

func workerCount(n int) int {
	if n > 0 {
		return n
	}
	return min(runtime.NumCPU(), maxDefaultWorkers)
}

// memoryBudget limits concurrent encodes by what they will need. CPU count
// alone is not enough: AVIF needs over 1 GB for a single 12 MP photo.
func memoryBudget() int64 {
	const fallback = 4 << 30
	total := int64(memory.TotalMemory())
	if total <= 0 {
		return fallback
	}
	return total / 2
}

// Run compresses the inputs concurrently. The channel closes after every input
// has finished or been canceled.
func Run(ctx context.Context, inputs []string, o Options) <-chan Event {
	events := make(chan Event, 16)
	jobs := make(chan int)
	budget := memoryBudget()
	if os.Getenv("GOMEMLIMIT") == "" {
		debug.SetMemoryLimit(budget) // Without this, Go lets garbage pile up well past the budget.
	}
	sem := semaphore.NewWeighted(budget)
	var names nameClaims

	var wg sync.WaitGroup
	for range workerCount(o.Workers) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				r := process(ctx, i, inputs[i], o, sem, budget, &names, events)
				events <- Event{Type: FileFinished, Index: i, Input: inputs[i], Result: r}
			}
		}()
	}
	go func() {
		for i := range inputs {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(events)
	}()
	return events
}

// process handles one file from start to finish.
func process(ctx context.Context, index int, path string, o Options, sem *semaphore.Weighted,
	budget int64, names *nameClaims, events chan<- Event,
) Result {
	start := time.Now()
	r := Result{Input: path}
	finish := func(s Status, reason string) Result {
		r.Status, r.Reason, r.Elapsed = s, reason, time.Since(start)
		return r
	}
	if ctx.Err() != nil {
		return finish(StatusCanceled, "canceled")
	}
	st, err := os.Stat(path)
	if err != nil {
		return finish(StatusFailed, friendlyError(err))
	}
	r.BytesIn = st.Size()
	data, err := os.ReadFile(path)
	if err != nil {
		return finish(StatusFailed, friendlyError(err))
	}
	info, err := codec.Probe(data)
	if err != nil {
		return finish(StatusFailed, friendlyError(err))
	}

	target := outputFormat(o.Target, info.Format)
	cost := min(pixelCost[target]*int64(info.Width)*int64(info.Height), budget)
	if err := sem.Acquire(ctx, cost); err != nil {
		return finish(StatusCanceled, "canceled")
	}
	defer sem.Release(cost)

	dec, err := codec.Decode(data)
	if errors.Is(err, codec.ErrAnimated) {
		return finish(StatusSkipped, "animated")
	}
	if err != nil {
		return finish(StatusFailed, friendlyError(err))
	}
	if o.ThumbSize > 0 {
		events <- Event{
			Type: FileStarted, Index: index, Input: path,
			Thumb: codec.Shrink(dec.Image, o.ThumbSize, o.ThumbSize),
		}
	}

	out := Compress(dec, o)
	if out.Format == codec.AVIF {
		// AVIF's memory is outside the Go heap, so it is invisible to the memory
		// limit and stays allocated until a collection runs its finalizers.
		runtime.GC()
	}
	if out.Err != nil {
		return finish(StatusFailed, friendlyError(out.Err))
	}
	if ctx.Err() != nil {
		return finish(StatusCanceled, "canceled")
	}
	r.Format, r.Warnings = out.Format, out.Warnings
	r.Width, r.Height = out.Width, out.Height

	sameFormat := out.Format == info.Format
	larger := int64(len(out.Data)) >= r.BytesIn
	if larger && (sameFormat || o.InPlace) {
		return finish(StatusSkipped, "already small")
	}
	if larger {
		r.Warnings = append(r.Warnings, "larger than the original")
	}

	r.Output, err = writeOutput(path, st, out, o, names)
	if err != nil {
		return finish(StatusFailed, friendlyError(err))
	}
	r.BytesOut = int64(len(out.Data))
	return finish(StatusDone, "")
}

// Output is the result of compressing an image in memory.
type Output struct {
	Data     []byte
	Format   codec.Format
	Width    int
	Height   int
	Warnings []string
	Err      error
}

// Compress applies the options to an already decoded image. It touches no
// files, so the TUI's preview can use it directly.
func Compress(dec *codec.Decoded, o Options) Output {
	m := dec.Image
	if o.MaxDim > 0 {
		m = codec.Resize(m, dec.Opaque, o.MaxDim)
	}
	out := Output{Width: m.Rect.Dx(), Height: m.Rect.Dy()}
	level := o.Level
	if level == 0 {
		level = codec.LevelBalanced
	}
	bg := o.Background
	if bg.A == 0 {
		bg = color.NRGBA{255, 255, 255, 255}
	}
	graphic := codec.IsGraphic(m)

	icc := dec.ICC
	if codec.IsSRGB(icc) {
		icc = nil
	}
	var exif []byte
	if o.KeepMetadata {
		exif = dec.Exif
	}

	base := codec.Options{
		Level: level, Quality: o.Quality, Graphic: graphic,
		Background: bg, ICC: icc, Exif: exif,
	}
	for _, c := range candidates(o.Target, dec, graphic) {
		opt := base
		opt.Format, opt.Lossless = c.format, c.lossless
		data, err := codec.Encode(m, dec.Opaque, opt)
		if err != nil {
			out.Err = err
			return out
		}
		if out.Data == nil || len(data) < len(out.Data) {
			out.Data, out.Format = data, c.format
		}
	}
	if out.Format == codec.AVIF && len(icc) > 0 {
		out.Warnings = append(out.Warnings, "color profile dropped, colors may shift")
	}
	if !dec.Opaque && out.Format == codec.JPEG {
		out.Warnings = append(out.Warnings, "transparency flattened")
	}
	if dec.Damaged {
		out.Warnings = append(out.Warnings, "original was damaged, parts of the image may be missing")
	}
	return out
}

type candidate struct {
	format   codec.Format
	lossless bool
}

// outputFormat returns the format used for memory estimates. Auto may pick a
// different one, but never one that costs more than JPEG.
func outputFormat(t Target, in codec.Format) codec.Format {
	switch t {
	case TargetJPEG, TargetAuto:
		return codec.JPEG
	case TargetPNG:
		return codec.PNG
	case TargetWebP:
		return codec.WebP
	case TargetAVIF:
		return codec.AVIF
	}
	return sameFormat(in)
}

// sameFormat maps an input format to one squish can write.
func sameFormat(in codec.Format) codec.Format {
	if in == codec.GIF {
		return codec.PNG
	}
	return in
}

// candidates lists the encodes to try. The smallest result wins.
func candidates(t Target, dec *codec.Decoded, graphic bool) []candidate {
	var f codec.Format
	switch t {
	case TargetAuto:
		if graphic || !dec.Opaque {
			return []candidate{{codec.WebP, true}, {codec.PNG, false}}
		}
		f = codec.JPEG
	case TargetJPEG:
		f = codec.JPEG
	case TargetPNG:
		f = codec.PNG
	case TargetWebP:
		f = codec.WebP
	case TargetAVIF:
		f = codec.AVIF
	default:
		f = sameFormat(dec.Format)
	}
	return []candidate{{f, f == codec.WebP && graphic}}
}

func friendlyError(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "file not found"
	case errors.Is(err, os.ErrPermission):
		return "permission denied"
	case errors.Is(err, codec.ErrUnknownFormat):
		return codec.ErrUnknownFormat.Error()
	case errors.Is(err, codec.ErrDamaged):
		return codec.ErrDamaged.Error()
	case errors.Is(err, codec.ErrTooLarge):
		return codec.ErrTooLarge.Error()
	}
	return err.Error()
}
