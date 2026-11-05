package engine

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamaalsawyerd/squish/internal/codec"
)

// TestEstimateAccuracy compares estimates with real encodes on a folder of
// real photos. Set SQUISH_SAMPLES to run it: the images are too big to commit.
func TestEstimateAccuracy(t *testing.T) {
	dir := os.Getenv("SQUISH_SAMPLES")
	if dir == "" {
		t.Skip("SQUISH_SAMPLES is not set")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	var sum, worst float64
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		dec, err := codec.Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		o := Options{Level: codec.LevelBalanced}
		est, real := Estimate(dec, o), Compress(dec, o)
		errPct := 100 * (float64(len(est.Data))/float64(len(real.Data)) - 1)
		t.Logf("%-40s real %7d est %7d  %+.0f%%", filepath.Base(p), len(real.Data), len(est.Data), errPct)
		sum += math.Abs(errPct)
		worst = max(worst, math.Abs(errPct))
	}
	t.Logf("mean error %.1f%%, worst %.1f%%, across %d files", sum/float64(len(paths)), worst, len(paths))
}
