package stats

import (
	"os"
	"path/filepath"
	"testing"
)

func ids(as []Achievement) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}

func TestAchievementsUnlockOnce(t *testing.T) {
	var s Stats
	got := s.Add(Batch{Files: 1, Saved: 100})
	if len(got) != 1 || got[0].ID != "first" {
		t.Fatalf("got %v", ids(got))
	}
	if got := s.Add(Batch{Files: 1, Saved: 100}); len(got) != 0 {
		t.Errorf("unlocked again: %v", ids(got))
	}
}

func TestLifetimeAchievementsAccumulate(t *testing.T) {
	var s Stats
	s.Add(Batch{Files: 60, Saved: 600_000_000, Problems: 1})
	got := s.Add(Batch{Files: 60, Saved: 600_000_000, Problems: 1})
	found := map[string]bool{}
	for _, a := range got {
		found[a.ID] = true
	}
	if !found["century"] || !found["gig"] {
		t.Errorf("got %v", ids(got))
	}
}

func TestSpotlessNeedsNoProblems(t *testing.T) {
	var s Stats
	for _, a := range s.Add(Batch{Files: 20, Problems: 1}) {
		if a.ID == "clean" {
			t.Error("batch with a problem should not count")
		}
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deep", "stats.json")
	s := Stats{BytesSaved: 42, Files: 3, Unlocked: []string{"first"}}
	if err := s.Save(path); err != nil {
		t.Fatal(err)
	}
	got := Load(path)
	if got.BytesSaved != 42 || got.Files != 3 || len(got.Unlocked) != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestDamagedFileStartsFresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.json")
	if err := os.WriteFile(path, []byte("{nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(path); got.Files != 0 {
		t.Errorf("got %+v", got)
	}
}
