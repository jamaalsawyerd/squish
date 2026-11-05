package codec

// Squeeze levels are squish's single quality scale. Each maps to the native
// setting that gives every codec about the same visual quality, because the
// native numbers are not comparable: quality 75 means very different things to
// jpegli and to AVIF. docs/codecs.md has the measurements behind the tables.
const (
	MinLevel = 1
	MaxLevel = 10

	LevelHigh     = 3
	LevelBalanced = 5
	LevelSmall    = 7
)

var (
	// targetSSIM is the score the JPEG column reaches on real camera photos.
	targetSSIM = [MaxLevel]float64{0.985, 0.97, 0.95, 0.935, 0.92, 0.905, 0.89, 0.875, 0.865, 0.85}

	jpegQuality = [MaxLevel]int{96, 92, 87, 82, 76, 70, 63, 58, 52, 45}
	webpQuality = [MaxLevel]int{97, 95, 92, 86, 78, 70, 58, 46, 34, 25}
	avifQuality = [MaxLevel]int{88, 80, 69, 60, 53, 48, 45, 42, 38, 34}
)

func clampLevel(level int) int {
	return max(MinLevel, min(level, MaxLevel))
}

// Quality returns a format's native quality setting for a squeeze level.
func Quality(f Format, level int) int {
	i := clampLevel(level) - 1
	switch f {
	case JPEG:
		return jpegQuality[i]
	case WebP:
		return webpQuality[i]
	case AVIF:
		return avifQuality[i]
	}
	return 100
}

// TargetSSIM returns the quality floor that a squeeze level aims for.
func TargetSSIM(level int) float64 {
	return targetSSIM[clampLevel(level)-1]
}
