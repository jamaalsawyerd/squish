package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/jamaalsawyerd/squish/internal/engine"
	"github.com/jamaalsawyerd/squish/internal/units"
)

// runPlain prints one line per file, for scripts and pipelines.
func runPlain(ctx context.Context, inputs []string, o engine.Options) error {
	if len(inputs) == 0 {
		return errors.New("no files given")
	}
	var in, out int64
	var done, skipped, failed, canceled int
	for ev := range engine.Run(ctx, inputs, o) {
		if ev.Type != engine.FileFinished {
			continue
		}
		r := ev.Result
		name := filepath.Base(r.Input)
		switch r.Status {
		case engine.StatusDone:
			done++
			in += r.BytesIn
			out += r.BytesOut
			fmt.Printf("✓ %s  %s → %s  %.0f%% smaller  %s\n", name, units.Bytes(r.BytesIn),
				units.Bytes(r.BytesOut), units.Percent(r.BytesIn, r.BytesOut), r.Output)
		case engine.StatusSkipped:
			skipped++
			fmt.Printf("- %s  skipped: %s\n", name, r.Reason)
		case engine.StatusFailed:
			failed++
			fmt.Printf("✗ %s  failed: %s\n", name, r.Reason)
		case engine.StatusCanceled:
			canceled++
		}
		for _, w := range r.Warnings {
			fmt.Printf("  ! %s\n", w)
		}
	}
	fmt.Printf("\nSaved %s across %d files", units.Bytes(in-out), done)
	if done > 0 {
		fmt.Printf(" (%.0f%% smaller, or %s)", units.Percent(in, out), units.Fun(in-out))
	}
	fmt.Println()
	if skipped > 0 {
		fmt.Printf("%d skipped\n", skipped)
	}
	if canceled > 0 {
		fmt.Printf("%d canceled\n", canceled)
		return context.Canceled
	}
	if failed > 0 {
		return fmt.Errorf("%d files failed", failed)
	}
	return nil
}
