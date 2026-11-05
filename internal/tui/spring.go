package tui

import (
	"math"
	"time"

	"github.com/charmbracelet/harmonica"

	tea "charm.land/bubbletea/v2"
)

const (
	frameRate = 30
	frameTime = time.Second / frameRate
	settleEps = 0.01
)

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(frameTime, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// spring is an animated value that overshoots and settles.
type spring struct {
	s        harmonica.Spring
	pos, vel float64
}

func newSpring(freq, damping, start float64) spring {
	return spring{s: harmonica.NewSpring(harmonica.FPS(frameRate), freq, damping), pos: start}
}

func (s *spring) to(target float64) {
	s.pos, s.vel = s.s.Update(s.pos, s.vel, target)
}

func (s *spring) settled(target float64) bool {
	return math.Abs(s.pos-target) < settleEps && math.Abs(s.vel) < settleEps
}
