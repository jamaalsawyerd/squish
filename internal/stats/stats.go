// Package stats keeps lifetime totals and achievements between runs.
package stats

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

// Achievement is a small reward for squishing things.
type Achievement struct {
	ID    string
	Name  string
	Blurb string
}

// Batch summarizes one finished run.
type Batch struct {
	Saved       int64
	Files       int
	Problems    int // Files that failed, were skipped or were canceled.
	BestPercent float64
}

// Stats is what is saved to disk.
type Stats struct {
	BytesSaved  int64    `json:"bytes_saved"`
	Files       int      `json:"files"`
	Runs        int      `json:"runs"`
	BestPercent float64  `json:"best_percent"`
	Unlocked    []string `json:"unlocked"`
}

const (
	gigabyte     = 1_000_000_000
	floppy       = 1_440_000
	ninety       = 90.0
	centuryFiles = 100
	bigBatch     = 50
	cleanBatch   = 10
	floppyStack  = 100
)

var achievements = []struct {
	Achievement
	earned func(Stats, Batch) bool
}{
	{Achievement{"first", "First squish", "You squished something!"}, func(_ Stats, b Batch) bool {
		return b.Files > 0
	}},
	{Achievement{"ninety", "The 90 club", "A file came out over 90% smaller"}, func(_ Stats, b Batch) bool {
		return b.BestPercent >= ninety
	}},
	{Achievement{"clean", "Spotless", "10 or more files with nothing skipped"}, func(_ Stats, b Batch) bool {
		return b.Files >= cleanBatch && b.Problems == 0
	}},
	{Achievement{"bulk", "Bulk discount", "50 files in a single batch"}, func(_ Stats, b Batch) bool {
		return b.Files >= bigBatch
	}},
	{Achievement{"stack", "Floppy stack", "Saved 100 floppy disks in one go"}, func(_ Stats, b Batch) bool {
		return b.Saved >= floppyStack*floppy
	}},
	{Achievement{"century", "Century", "100 files squished, all time"}, func(s Stats, _ Batch) bool {
		return s.Files >= centuryFiles
	}},
	{Achievement{"gig", "Gigabyte gobbler", "1 GB saved, all time"}, func(s Stats, _ Batch) bool {
		return s.BytesSaved >= gigabyte
	}},
}

// Path returns where stats live, or an empty string if there is nowhere to
// keep them.
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "squish", "stats.json")
}

// Load reads saved stats. Missing or damaged files just start from zero:
// nobody's images should depend on this file.
func Load(path string) Stats {
	var s Stats
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if json.Unmarshal(b, &s) != nil {
		return Stats{}
	}
	return s
}

// Save writes stats atomically.
func (s Stats) Save(path string) error {
	if path == "" {
		return errors.New("no location for stats")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Add folds in a batch and returns any newly earned achievements.
func (s *Stats) Add(b Batch) []Achievement {
	s.BytesSaved += b.Saved
	s.Files += b.Files
	s.Runs++
	s.BestPercent = max(s.BestPercent, b.BestPercent)
	var earned []Achievement
	for _, a := range achievements {
		if !slices.Contains(s.Unlocked, a.ID) && a.earned(*s, b) {
			s.Unlocked = append(s.Unlocked, a.ID)
			earned = append(earned, a.Achievement)
		}
	}
	return earned
}
