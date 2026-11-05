package main

import "testing"

func TestPickVersion(t *testing.T) {
	tests := []struct {
		name, set, recorded, want string
	}{
		{"go install", "dev", "v0.1.0", "0.1.0"},
		{"goreleaser wins", "0.2.0", "v0.1.0", "0.2.0"},
		{"local build with no vcs data", "dev", "(devel)", "dev"},
		{"no build info", "dev", "", "dev"},
		{"between tags", "dev", "v0.1.1-0.20261105063000-89807bc6195d", "0.1.1-0.20261105063000-89807bc6195d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickVersion(tt.set, tt.recorded); got != tt.want {
				t.Errorf("pickVersion(%q, %q) = %q, want %q", tt.set, tt.recorded, got, tt.want)
			}
		})
	}
}
