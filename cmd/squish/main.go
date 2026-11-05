// Command squish is a terminal app for compressing images.
package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"os/signal"
	"runtime/debug"
	"strconv"
	"strings"

	"charm.land/fang/v2"
	"github.com/charmbracelet/x/term"
	"github.com/jamaalsawyerd/squish/internal/codec"
	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/stats"
	"github.com/jamaalsawyerd/squish/internal/tui"
	"github.com/spf13/cobra"
)

// version is set at build time by GoReleaser. Other builds leave it alone and
// use the version the Go toolchain records instead.
var version = "dev"

// buildVersion returns the version to show for this binary.
func buildVersion() string {
	var recorded string
	if info, ok := debug.ReadBuildInfo(); ok {
		recorded = info.Main.Version
	}
	return pickVersion(version, recorded)
}

// pickVersion prefers a version set at build time, then the one the toolchain
// recorded. The leading v is dropped so every build prints versions the same way.
func pickVersion(set, recorded string) string {
	if set != "dev" || recorded == "" || recorded == "(devel)" {
		return set
	}
	return strings.TrimPrefix(recorded, "v")
}

type flags struct {
	format       string
	preset       string
	level        int
	quality      int
	maxSize      int
	out          string
	inPlace      bool
	keepMetadata bool
	recursive    bool
	workers      int
	background   string
	noTUI        bool
	noAnim       bool
}

func main() {
	if err := execute(); err != nil {
		os.Exit(1)
	}
}

func execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return fang.Execute(ctx, newRootCmd(), fang.WithVersion(buildVersion()))
}

func newRootCmd() *cobra.Command {
	var f flags
	cmd := &cobra.Command{
		Use:   "squish [files or folders...]",
		Short: "A fun terminal app for compressing images",
		Long: "Squish your images down to size. Run it with no arguments to pick files " +
			"in the app, or pass files and folders to start with those.\n\n" +
			"Originals are never touched unless you pass --in-place. Output goes beside " +
			"each file as name-squished.ext, or into the folder given with --out.",
		Example: `  # Open the app and pick files
  squish

  # Open the app with a folder already loaded
  squish ~/Pictures/holiday

  # Compress without the app, and resize while you're at it
  squish --no-tui --max-size 2000 --out web/ photos/

  # Convert everything to the best format for its content
  squish --no-tui --format auto *.png`,
		Args:          cobra.ArbitraryArgs, // Fang adds subcommands, which would otherwise reject paths.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd.Context(), args, f)
		},
	}
	fl := cmd.Flags()
	fl.StringVarP(&f.format, "format", "f", "same", "output format: same, auto, jpeg, png, webp or avif")
	fl.StringVarP(&f.preset, "preset", "p", "balanced", "how hard to squeeze: high (best quality), balanced or small")
	fl.IntVarP(&f.level, "level", "l", 0, "squeeze level from 1 (gentle) to 10 (hard), instead of a preset")
	fl.IntVarP(&f.quality, "quality", "q", 0, "exact encoder quality from 1 to 100, instead of a preset")
	fl.IntVarP(&f.maxSize, "max-size", "s", 0, "shrink images so the longest side fits this many pixels")
	fl.StringVarP(&f.out, "out", "o", "", "write output to this folder")
	fl.BoolVar(&f.inPlace, "in-place", false, "replace originals, but only with smaller files")
	fl.BoolVarP(&f.keepMetadata, "keep-metadata", "m", false, "keep EXIF data, including camera details and location")
	fl.BoolVarP(&f.recursive, "recursive", "r", false, "include subfolders")
	fl.IntVarP(&f.workers, "workers", "w", 0, "files to compress at once (default: based on your CPU)")
	fl.StringVar(&f.background, "background", "#ffffff", "color behind transparent areas in formats without transparency")
	fl.BoolVar(&f.noTUI, "no-tui", false, "print progress as text instead of opening the app")
	fl.BoolVar(&f.noAnim, "no-anim", false, "turn off animations in the app")
	return cmd
}

func (f flags) options() (engine.Options, error) {
	var o engine.Options
	var err error
	if o.Target, err = engine.ParseTarget(strings.ToLower(f.format)); err != nil {
		return o, err
	}
	if o.Background, err = parseColor(f.background); err != nil {
		return o, err
	}
	switch strings.ToLower(f.preset) {
	case "high":
		o.Level = codec.LevelHigh
	case "balanced":
		o.Level = codec.LevelBalanced
	case "small":
		o.Level = codec.LevelSmall
	default:
		return o, fmt.Errorf("unknown preset %q (use high, balanced or small)", f.preset)
	}
	if f.level != 0 {
		if f.level < codec.MinLevel || f.level > codec.MaxLevel {
			return o, fmt.Errorf("level must be from %d to %d", codec.MinLevel, codec.MaxLevel)
		}
		o.Level = f.level
	}
	if f.quality < 0 || f.quality > 100 {
		return o, errors.New("quality must be from 1 to 100")
	}
	if f.maxSize < 0 {
		return o, errors.New("max size cannot be negative")
	}
	if f.inPlace && f.out != "" {
		return o, errors.New("--in-place and --out cannot be used together")
	}
	o.Quality, o.MaxDim, o.OutDir = f.quality, f.maxSize, f.out
	o.InPlace, o.KeepMetadata, o.Workers = f.inPlace, f.keepMetadata, f.workers
	return o, nil
}

func parseColor(s string) (color.NRGBA, error) {
	h := strings.TrimPrefix(s, "#")
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil || len(h) != 6 {
		return color.NRGBA{}, fmt.Errorf("background must look like #ffffff, not %q", s)
	}
	return color.NRGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, nil
}

func run(ctx context.Context, args []string, f flags) error {
	o, err := f.options()
	if err != nil {
		return err
	}
	inputs, err := engine.Expand(args, f.recursive)
	if err != nil {
		return err
	}
	if len(args) > 0 && len(inputs) == 0 {
		return errors.New("no images found")
	}
	interactive := term.IsTerminal(os.Stdout.Fd()) && term.IsTerminal(os.Stdin.Fd())
	if f.noTUI || !interactive {
		return runPlain(ctx, inputs, o)
	}
	summary, err := tui.Run(ctx, tui.Config{
		Files: inputs, Options: o, NoAnim: f.noAnim, StatsPath: stats.Path(),
	})
	if err != nil {
		return err
	}
	if s := summary.String(); s != "" {
		fmt.Println(s)
	}
	return nil
}
