package tui

import "charm.land/bubbles/v2/key"

func binding(help, desc string, keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(help, desc))
}

var keys = struct {
	Up, Down, Left, Right                   key.Binding
	Open, Parent, Toggle, All, Next, Path   key.Binding
	Hidden, Quit, Back, Start, Zoom         key.Binding
	Prev, NextFile, Wipe, PanUp, PanDown    key.Binding
	PanLeft, PanRight, Compare, Retune, New key.Binding
	Yes, No                                 key.Binding
}{
	Up:       binding("↑", "up", "up", "k"),
	Down:     binding("↓", "down", "down", "j"),
	Left:     binding("←", "less", "left", "h"),
	Right:    binding("→", "more", "right", "l"),
	Open:     binding("enter", "open", "enter"),
	Parent:   binding("backspace", "up a folder", "backspace", "left", "h"),
	Toggle:   binding("space", "select", "space"),
	All:      binding("a", "select all", "a"),
	Next:     binding("tab", "next step", "tab"),
	Path:     binding("g", "go to path", "g"),
	Hidden:   binding(".", "hidden files", "."),
	Quit:     binding("q", "quit", "q", "ctrl+c"),
	Back:     binding("esc", "back", "esc"),
	Start:    binding("enter", "squish!", "enter"),
	Zoom:     binding("z", "zoom", "z"),
	Prev:     binding("[", "previous", "["),
	NextFile: binding("]", "next file", "]"),
	Wipe:     binding("tab", "wipe", "tab"),
	PanUp:    binding("w", "pan", "w"),
	PanDown:  binding("s", "pan", "s"),
	PanLeft:  binding("a", "pan", "a"),
	PanRight: binding("d", "pan", "d"),
	Compare:  binding("enter", "compare", "enter"),
	Retune:   binding("r", "new settings", "r"),
	New:      binding("n", "new batch", "n"),
	Yes:      binding("y", "yes", "y"),
	No:       binding("n", "no", "n", "esc"),
}
