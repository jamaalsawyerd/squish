package tui

import (
	"slices"
	"testing"
)

func TestParsePaths(t *testing.T) {
	t.Setenv("WSL_DISTRO_NAME", "Ubuntu")
	t.Setenv("HOME", "/home/me")
	tests := []struct {
		name, in string
		want     []string
	}{
		{"plain", "/a/b.jpg", []string{"/a/b.jpg"}},
		{"two", "/a/b.jpg /c/d.png", []string{"/a/b.jpg", "/c/d.png"}},
		{"escaped space", `/a/my\ photo.jpg`, []string{"/a/my photo.jpg"}},
		{"quoted", `'/a/my photo.jpg' "/b/c d.png"`, []string{"/a/my photo.jpg", "/b/c d.png"}},
		{"lines", "/a.jpg\n/b.jpg\r\n", []string{"/a.jpg", "/b.jpg"}},
		{"windows", `C:\Users\Sam\Pictures\a.jpg`, []string{"/mnt/c/Users/Sam/Pictures/a.jpg"}},
		{"quoted windows", `"D:\My Photos\a.jpg"`, []string{"/mnt/d/My Photos/a.jpg"}},
		{"home", "~/Pictures", []string{"/home/me/Pictures"}},
		{"empty", "  ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parsePaths(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("parsePaths(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWindowsPathsOnlyMapOnWSL(t *testing.T) {
	t.Setenv("WSL_DISTRO_NAME", "")
	in := `C:\a.jpg`
	if got := parsePaths(in); len(got) != 1 || got[0] != `C:\a.jpg` {
		t.Errorf("got %q", got)
	}
}
