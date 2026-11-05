package units

import "testing"

func TestBytes(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{999, "999 B"},
		{1500, "2 KB"},
		{640_000, "640 KB"},
		{8_100_000, "8.1 MB"},
		{2_500_000_000, "2.50 GB"},
		{-2_000_000, "-2.0 MB"},
	}
	for _, tt := range tests {
		if got := Bytes(tt.in); got != tt.want {
			t.Errorf("Bytes(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPercent(t *testing.T) {
	if got := Percent(100, 25); got != 75 {
		t.Errorf("got %v", got)
	}
	if got := Percent(0, 25); got != 0 {
		t.Errorf("empty input: got %v", got)
	}
}

func TestPercentLabel(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{{0, "0%"}, {82.4, "82%"}, {99, "99%"}, {99.7, ">99%"}}
	for _, tt := range tests {
		if got := PercentLabel(tt.in); got != tt.want {
			t.Errorf("PercentLabel(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFun(t *testing.T) {
	tests := []struct {
		in   int64
		want string
	}{
		{100, "less than a minute of 56k dial-up"},
		{420_000, "1 minute of 56k dial-up"},
		{148_000_000, "1 Zip disk"},
		{2_000_000, "1 floppy disk"},
		{14_400_000, "10 floppy disks"},
		{9_400_000_000, "2 single-layer DVDs"},
	}
	for _, tt := range tests {
		if got := Fun(tt.in); got != tt.want {
			t.Errorf("Fun(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
