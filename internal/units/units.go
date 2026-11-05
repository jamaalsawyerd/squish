// Package units formats sizes, and turns them into sillier units.
package units

import (
	"fmt"
	"math"
)

// Bytes formats a size for people.
func Bytes(n int64) string {
	const k = 1000
	abs := math.Abs(float64(n))
	switch {
	case abs < k:
		return fmt.Sprintf("%d B", n)
	case abs < k*k:
		return fmt.Sprintf("%.0f KB", float64(n)/k)
	case abs < k*k*k:
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	}
	return fmt.Sprintf("%.2f GB", float64(n)/(k*k*k))
}

// Percent returns how much smaller after is than before.
func Percent(before, after int64) float64 {
	if before <= 0 {
		return 0
	}
	return 100 * (1 - float64(after)/float64(before))
}

// PercentLabel formats a saving without ever claiming a file vanished, which
// is what rounding would do at the top of the scale.
func PercentLabel(p float64) string {
	if p > maxShownPercent {
		return fmt.Sprintf(">%d%%", maxShownPercent)
	}
	return fmt.Sprintf("%.0f%%", p)
}

const maxShownPercent = 99

type funUnit struct {
	bytes          float64
	single, plural string
}

// Largest first, so that the biggest unit that reads naturally wins.
var funUnits = []funUnit{
	{4.7e9, "single-layer DVD", "single-layer DVDs"},
	{700e6, "CD-ROM", "CD-ROMs"},
	{100e6, "Zip disk", "Zip disks"},
	{1.44e6, "floppy disk", "floppy disks"},
	{420e3, "minute of 56k dial-up", "minutes of 56k dial-up"},
}

// Fun describes a size in units nobody uses any more.
func Fun(n int64) string {
	v := float64(n)
	for _, u := range funUnits {
		if c := v / u.bytes; c >= 1 {
			if c < 1.5 {
				return "1 " + u.single
			}
			return fmt.Sprintf("%.0f %s", c, u.plural)
		}
	}
	return "less than a minute of 56k dial-up"
}
