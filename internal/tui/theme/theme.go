// Package theme holds squish's colors and text styles.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// The hot end of the palette is used for big files and the cool end for small
// ones, so colors fade from hot to cool as images get squished.
var (
	Pink   = color.NRGBA{255, 79, 139, 255}
	Coral  = color.NRGBA{255, 138, 91, 255}
	Yellow = color.NRGBA{255, 209, 102, 255}
	Mint   = color.NRGBA{94, 234, 212, 255}
	Violet = color.NRGBA{167, 139, 250, 255}
)

// Theme is a set of styles for one background.
type Theme struct {
	Text, Muted, Faint      lipgloss.Style
	Accent, Good, Warn, Bad lipgloss.Style
	Bold, Key               lipgloss.Style
	Panel, PanelActive      lipgloss.Style
	PanelTitle              lipgloss.Style
	Selected                lipgloss.Style
	TrackColor              color.Color
}

// New builds the theme for a dark or light terminal.
func New(dark bool) Theme {
	c := lipgloss.LightDark(dark)
	text := c(lipgloss.Color("#2B2140"), lipgloss.Color("#F4F0FF"))
	muted := c(lipgloss.Color("#6B6280"), lipgloss.Color("#A8A0C0"))
	faint := c(lipgloss.Color("#9A93AD"), lipgloss.Color("#6A6480"))
	border := c(lipgloss.Color("#D5CFE3"), lipgloss.Color("#4A4460"))
	pink := c(lipgloss.Color("#D81B60"), Pink)
	good := c(lipgloss.Color("#00897B"), Mint)
	warn := c(lipgloss.Color("#B26A00"), Yellow)
	bad := c(lipgloss.Color("#C62828"), lipgloss.Color("#FF6B6B"))
	base := lipgloss.NewStyle()
	panel := base.Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1)
	return Theme{
		Text:        base.Foreground(text),
		Muted:       base.Foreground(muted),
		Faint:       base.Foreground(faint),
		Accent:      base.Foreground(pink),
		Good:        base.Foreground(good),
		Warn:        base.Foreground(warn),
		Bad:         base.Foreground(bad),
		Bold:        base.Foreground(text).Bold(true),
		Key:         base.Foreground(pink).Bold(true),
		Panel:       panel,
		PanelActive: panel.BorderForeground(pink),
		PanelTitle:  base.Foreground(muted).Bold(true),
		Selected:    base.Foreground(pink).Bold(true),
		TrackColor:  border,
	}
}

// Gradient returns n colors from hot pink through to mint.
func Gradient(n int) []color.Color {
	return lipgloss.Blend1D(max(n, 2), Pink, Coral, Yellow, Mint)
}
