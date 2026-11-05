package engine

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jamaalsawyerd/squish/internal/codec"
)

func photo(w, h int) *image.NRGBA {
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	v := uint32(7)
	for i := 0; i < len(m.Pix); i += 4 {
		v = v*1664525 + 1013904223
		m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = uint8(v>>24), uint8(v>>16), uint8(v>>8), 255
	}
	return m
}

func writeRawPNG(t *testing.T, path string, m image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := png.Encoder{CompressionLevel: png.NoCompression}
	if err := enc.Encode(f, m); err != nil {
		t.Fatal(err)
	}
}

func writeJPEG(t *testing.T, path string, q int) {
	t.Helper()
	data, err := codec.Encode(photo(200, 150), true, codec.Options{Format: codec.JPEG, Quality: q})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
}

func runAll(t *testing.T, ctx context.Context, inputs []string, o Options) []Result {
	t.Helper()
	var out []Result
	for ev := range Run(ctx, inputs, o) {
		if ev.Type == FileFinished {
			out = append(out, ev.Result)
		}
	}
	return out
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	return out
}

func TestWritesBesideOriginalAndLeavesItAlone(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.jpg")
	writeJPEG(t, in, 98)
	before, _ := os.ReadFile(in)

	rs := runAll(t, context.Background(), []string{in}, Options{Level: 8})
	if len(rs) != 1 || rs[0].Status != StatusDone {
		t.Fatalf("results = %+v", rs)
	}
	if want := filepath.Join(dir, "a-squished.jpg"); rs[0].Output != want {
		t.Errorf("output = %s, want %s", rs[0].Output, want)
	}
	after, _ := os.ReadFile(in)
	if string(before) != string(after) {
		t.Error("original was changed")
	}
	if rs[0].Saved() <= 0 {
		t.Errorf("saved = %d", rs[0].Saved())
	}
}

func TestOutputKeepsModeAndDate(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.jpg")
	writeJPEG(t, in, 98)
	rs := runAll(t, context.Background(), []string{in}, Options{Level: 8})
	src, _ := os.Stat(in)
	dst, err := os.Stat(rs[0].Output)
	if err != nil {
		t.Fatal(err)
	}
	if src.Mode() != dst.Mode() {
		t.Errorf("mode = %v, want %v", dst.Mode(), src.Mode())
	}
	if !src.ModTime().Equal(dst.ModTime()) {
		t.Errorf("date = %v, want %v", dst.ModTime(), src.ModTime())
	}
}

func TestNeverOverwritesExistingOutput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.jpg")
	writeJPEG(t, in, 98)
	existing := filepath.Join(dir, "a-squished.jpg")
	if err := os.WriteFile(existing, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs := runAll(t, context.Background(), []string{in}, Options{Level: 8})
	if want := filepath.Join(dir, "a-squished-2.jpg"); rs[0].Output != want {
		t.Errorf("output = %s, want %s", rs[0].Output, want)
	}
	if got, _ := os.ReadFile(existing); string(got) != "precious" {
		t.Error("existing file was overwritten")
	}
}

func TestTwoInputsCannotClaimOneOutput(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.jpg"), filepath.Join(dir, "a.png")
	writeJPEG(t, a, 98)
	writeRawPNG(t, b, photo(200, 150))
	rs := runAll(t, context.Background(), []string{a, b}, Options{Target: TargetWebP, Level: 8})
	if len(rs) != 2 || rs[0].Output == rs[1].Output {
		t.Fatalf("outputs collided: %+v", rs)
	}
}

func TestKeepsOriginalWhenOutputWouldBeLarger(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.jpg")
	writeJPEG(t, in, 10)
	rs := runAll(t, context.Background(), []string{in}, Options{Quality: 100})
	if rs[0].Status != StatusSkipped {
		t.Fatalf("status = %v, want skipped: %+v", rs[0].Status, rs[0])
	}
	if got := listDir(t, dir); !slices.Equal(got, []string{"a.jpg"}) {
		t.Errorf("files = %v, want only the original", got)
	}
}

func TestConversionIsKeptEvenWhenLarger(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.jpg")
	writeJPEG(t, in, 10)
	rs := runAll(t, context.Background(), []string{in}, Options{Target: TargetPNG, Level: 1})
	if rs[0].Status != StatusDone {
		t.Fatalf("result = %+v", rs[0])
	}
	if !slices.Contains(rs[0].Warnings, "larger than the original") {
		t.Errorf("warnings = %v", rs[0].Warnings)
	}
}

func TestInPlaceReplacesOnlyWhenSmaller(t *testing.T) {
	dir := t.TempDir()
	big := filepath.Join(dir, "big.jpg")
	writeJPEG(t, big, 98)
	small := filepath.Join(dir, "small.jpg")
	writeJPEG(t, small, 10)
	smallBefore, _ := os.ReadFile(small)
	st, _ := os.Stat(big)

	rs := runAll(t, context.Background(), []string{big}, Options{InPlace: true, Level: 8})
	if rs[0].Output != big || rs[0].Status != StatusDone {
		t.Fatalf("result = %+v", rs[0])
	}
	if after, _ := os.Stat(big); after.Size() >= st.Size() {
		t.Errorf("size = %d, want under %d", after.Size(), st.Size())
	}
	rs = runAll(t, context.Background(), []string{small}, Options{InPlace: true, Quality: 100})
	if rs[0].Status != StatusSkipped {
		t.Fatalf("result = %+v", rs[0])
	}
	if got, _ := os.ReadFile(small); string(got) != string(smallBefore) {
		t.Error("original was replaced by a larger file")
	}
	if got := listDir(t, dir); len(got) != 2 {
		t.Errorf("leftover files: %v", got)
	}
}

func TestBadFilesFailWithoutStoppingOthers(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.jpg")
	writeJPEG(t, good, 98)
	bad := filepath.Join(dir, "bad.jpg")
	if err := os.WriteFile(bad, []byte("definitely not a jpeg"), 0o644); err != nil {
		t.Fatal(err)
	}
	rs := runAll(t, context.Background(), []string{bad, good, filepath.Join(dir, "gone.jpg")}, Options{Level: 8})
	got := map[string]Status{}
	for _, r := range rs {
		got[filepath.Base(r.Input)] = r.Status
	}
	if got["good.jpg"] != StatusDone || got["bad.jpg"] != StatusFailed || got["gone.jpg"] != StatusFailed {
		t.Errorf("statuses = %v", got)
	}
}

func TestCancelLeavesNoFiles(t *testing.T) {
	dir := t.TempDir()
	var inputs []string
	for _, n := range []string{"a", "b", "c", "d"} {
		p := filepath.Join(dir, n+".jpg")
		writeJPEG(t, p, 98)
		inputs = append(inputs, p)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rs := runAll(t, ctx, inputs, Options{Level: 8})
	if len(rs) != len(inputs) {
		t.Fatalf("got %d results, want one for every input", len(rs))
	}
	for _, r := range rs {
		if r.Status != StatusCanceled {
			t.Errorf("%s: status = %v", r.Input, r.Status)
		}
	}
	if got := listDir(t, dir); len(got) != len(inputs) {
		t.Errorf("files = %v", got)
	}
}

func TestOutDirKeepsNames(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.png")
	writeRawPNG(t, in, photo(64, 64))
	out := filepath.Join(dir, "web", "images")
	rs := runAll(t, context.Background(), []string{in}, Options{Target: TargetWebP, OutDir: out, Level: 8})
	if want := filepath.Join(out, "a.webp"); rs[0].Output != want {
		t.Errorf("output = %s, want %s", rs[0].Output, want)
	}
}

func TestMaxDimResizes(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.jpg")
	writeJPEG(t, in, 98)
	rs := runAll(t, context.Background(), []string{in}, Options{MaxDim: 100, Level: 5})
	if rs[0].Width != 100 || rs[0].Height != 75 {
		t.Errorf("size = %dx%d, want 100x75", rs[0].Width, rs[0].Height)
	}
}

func TestAutoPicksLosslessForGraphics(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "shot.png")
	m := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	for i := 0; i < len(m.Pix); i += 4 {
		m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = 30, 30, 46, 255
	}
	writeRawPNG(t, in, m)
	rs := runAll(t, context.Background(), []string{in}, Options{Target: TargetAuto})
	if rs[0].Status != StatusDone {
		t.Fatalf("result = %+v", rs[0])
	}
	if rs[0].Format == codec.JPEG {
		t.Errorf("format = %v, graphics should not become JPEG", rs[0].Format)
	}
}

func TestExpand(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{
		"a.jpg", "b.PNG", "notes.txt", ".hidden.jpg", "a-squished.jpg", "a-squished-2.jpg",
		"not-squished-yet.jpg", "sub/c.webp",
	} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := func(paths []string) []string {
		var out []string
		for _, p := range paths {
			out = append(out, filepath.Base(p))
		}
		slices.Sort(out)
		return out
	}
	got, err := Expand([]string{dir}, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a.jpg", "b.PNG", "not-squished-yet.jpg"}; !slices.Equal(base(got), want) {
		t.Errorf("flat = %v, want %v", base(got), want)
	}
	got, _ = Expand([]string{dir}, true)
	if !slices.Contains(base(got), "c.webp") {
		t.Errorf("recursive = %v", base(got))
	}
	named := filepath.Join(dir, "a-squished.jpg")
	got, _ = Expand([]string{named, named}, false)
	if len(got) != 1 {
		t.Errorf("named file: %v, want it once", got)
	}
	if !strings.HasSuffix(got[0], "a-squished.jpg") {
		t.Errorf("got %v", got)
	}
}
