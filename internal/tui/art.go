package tui

import (
	"image"
	"image/color"
	"math"

	"github.com/jamaalsawyerd/squish/internal/gfx"
	"github.com/jamaalsawyerd/squish/internal/tui/theme"
)

// wordmark is drawn on a pixel grid so it can be squished like everything else.
var wordmark = buildWordmark()

// glyphs are chunky so they stay readable when squashed.
var glyphs = map[rune][]string{
	's': {"########", "########", "###     ", "########", "########", "     ###", "########", "########"},
	'q': {" ###### ", "########", "###  ###", "###  ###", "###  ###", "### ####", "########", " #### ##"},
	'u': {"###  ###", "###  ###", "###  ###", "###  ###", "###  ###", "###  ###", "########", " ###### "},
	'i': {"###", "###", "###", "###", "###", "###", "###", "###"},
	'h': {"###  ###", "###  ###", "###  ###", "########", "########", "###  ###", "###  ###", "###  ###"},
}

func buildWordmark() []string {
	rows := make([]string, len(glyphs['s']))
	for i, r := range "squish" {
		for y := range rows {
			if i > 0 {
				rows[y] += "  "
			}
			rows[y] += glyphs[r][y]
		}
	}
	return rows
}

// logoRows is the fixed height of the wordmark in terminal rows.
const logoRows = 4

// drawLogo renders the wordmark squashed to a fraction of its height, widening
// a little as it does, the way a soft thing would.
func drawLogo(squash float64) string {
	w := len(wordmark[0])
	h := len(wordmark)
	boxW, boxH := w+8, logoRows*2
	c := gfx.New(boxW, boxH)
	drawH := max(int(math.Round(float64(boxH)*squash)), 2)
	stretch := 1 + (1-squash)*0.12
	drawW := int(float64(w) * stretch)
	x0 := (boxW - drawW) / 2
	y0 := boxH - drawH
	stops := theme.Gradient(boxW)
	for y := range drawH {
		sy := y * h / drawH
		row := wordmark[sy]
		for x := range drawW {
			if row[x*w/drawW] != '#' {
				continue
			}
			gfx.Set(c, x0+x, y0+y, nrgba(stops[min(x0+x, boxW-1)]))
		}
	}
	return gfx.Render(c)
}

func nrgba(c color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

// mood is the mascot's expression.
type mood int

const (
	moodHappy mood = iota
	moodStrain
	moodDizzy
	moodBlink
)

var (
	mascotBody  = color.NRGBA{255, 111, 163, 255}
	mascotShade = color.NRGBA{214, 64, 122, 255}
	mascotInk   = color.NRGBA{43, 33, 64, 255}
	mascotBlush = color.NRGBA{255, 176, 200, 255}
)

// drawMascot draws Squish, a soft blob, with its feet at (cx, bottom). Its
// volume stays about the same, so it gets wider as it gets flatter.
func drawMascot(c *image.NRGBA, cx, bottom, rx, ry float64, m mood) {
	gfx.FillEllipse(c, cx, bottom-ry, rx, ry, func(t float64) color.NRGBA {
		if t > 0.82 {
			return mascotShade
		}
		return mascotBody
	})
	eyeY := int(bottom - ry*1.08)
	eyeGap := max(int(rx*0.42), 2)
	x0, x1 := int(cx)-eyeGap, int(cx)+eyeGap
	if ry >= 4 {
		cheek := int(rx * 0.72)
		for _, x := range []int{int(cx) - cheek, int(cx) + cheek} {
			gfx.FillRect(c, image.Rect(x-1, eyeY+2, x+2, eyeY+3), mascotBlush)
		}
	}
	switch m {
	case moodHappy:
		for _, x := range []int{x0, x1} {
			gfx.FillRect(c, image.Rect(x-1, eyeY-1, x+1, eyeY+1), mascotInk)
		}
		mouth := eyeY + 2
		x := int(cx)
		gfx.Set(c, x-2, mouth, mascotInk)
		gfx.FillRect(c, image.Rect(x-1, mouth+1, x+1, mouth+2), mascotInk)
		gfx.Set(c, x+1, mouth, mascotInk)
	case moodBlink:
		for _, x := range []int{x0, x1} {
			gfx.FillRect(c, image.Rect(x-1, eyeY, x+1, eyeY+1), mascotInk)
		}
	case moodStrain:
		// Eyes squeezed shut, like >_<.
		gfx.Set(c, x0-1, eyeY-1, mascotInk)
		gfx.Set(c, x0, eyeY, mascotInk)
		gfx.Set(c, x0-1, eyeY+1, mascotInk)
		gfx.Set(c, x1+1, eyeY-1, mascotInk)
		gfx.Set(c, x1, eyeY, mascotInk)
		gfx.Set(c, x1+1, eyeY+1, mascotInk)
	case moodDizzy:
		for _, x := range []int{x0, x1} {
			gfx.Set(c, x-1, eyeY-1, mascotInk)
			gfx.Set(c, x+1, eyeY-1, mascotInk)
			gfx.Set(c, x, eyeY, mascotInk)
			gfx.Set(c, x-1, eyeY+1, mascotInk)
			gfx.Set(c, x+1, eyeY+1, mascotInk)
		}
	}
}

// mascotPortrait renders Squish alone, for panels with nothing else to show.
func mascotPortrait(cols, rows int, m mood) string {
	c := gfx.New(cols, rows*2)
	rx := float64(min(cols, rows*2)) * 0.36
	drawMascot(c, float64(cols)/2, float64(rows*2)-1, rx, rx*0.88, m)
	return gfx.Render(c)
}

type particle struct {
	x, y, vx, vy, g float64
	life            int
	c               color.NRGBA
}

const (
	crumbGravity    = 0.09
	confettiGravity = 0.008
)

func stepParticles(ps []particle) []particle {
	out := ps[:0]
	for _, p := range ps {
		p.x += p.vx
		p.y += p.vy
		p.vy += p.g
		p.life--
		if p.life > 0 {
			out = append(out, p)
		}
	}
	return out
}

func drawParticles(c *image.NRGBA, ps []particle) {
	for _, p := range ps {
		gfx.Set(c, int(p.x), int(p.y), p.c)
	}
}

// press is the animated hydraulic press. Its plate pumps up and down, flattening
// Squish and the current image between it and the base.
type press struct {
	plate   spring // 0 is fully raised, 1 is fully pressed.
	down    bool
	timer   int
	running bool
	thumb   *image.NRGBA
	next    *image.NRGBA
	parts   []particle
	seed    uint32
	hit     bool
}

const (
	pressDownFrames = 12
	pressUpFrames   = 14
	puffCount       = 16
	puffLife        = 24
)

func newPress() press {
	return press{plate: newSpring(7, 0.42, 0), seed: 1}
}

func (p *press) rand() float64 {
	p.seed = p.seed*1664525 + 1013904223
	return float64(p.seed>>16&0xFFFF) / 0xFFFF
}

func (p *press) step(w, h int) {
	target := 0.0
	if p.running {
		p.timer--
		if p.timer <= 0 {
			p.down = !p.down
			if p.down {
				p.timer = pressDownFrames
				p.hit = false
				if p.next != nil {
					p.thumb, p.next = p.next, nil
				}
			} else {
				p.timer = pressUpFrames
			}
		}
		if p.down {
			target = 1
		}
	}
	p.plate.to(target)
	if p.down && !p.hit && p.plate.pos > 0.9 {
		p.hit = true
		p.puff(w, h)
	}
	p.parts = stepParticles(p.parts)
}

// puff sprays out colored crumbs, which are the bytes leaving the file.
func (p *press) puff(w, h int) {
	stops := theme.Gradient(puffCount)
	for i := range puffCount {
		dir := 1.0
		x := float64(w) * 0.72
		if i%2 == 0 {
			dir, x = -1, float64(w)*0.28
		}
		p.parts = append(p.parts, particle{
			x: x, y: float64(h) * 0.82,
			vx: dir * (0.5 + p.rand()*1.3), vy: -0.5 - p.rand()*1.2, g: crumbGravity,
			life: puffLife, c: nrgba(stops[i]),
		})
	}
}

var (
	steelLight = color.NRGBA{150, 146, 170, 255}
	steelMid   = color.NRGBA{108, 104, 128, 255}
	steelDark  = color.NRGBA{74, 70, 92, 255}
)

// view draws the press into a pixel area.
func (p *press) view(w, h int) string {
	c := gfx.New(w, h)
	const (
		pillar = 3
		beam   = 4
		base   = 3
		plateT = 3
	)
	// Frame.
	gfx.FillRect(c, image.Rect(1, 0, 1+pillar, h), steelDark)
	gfx.FillRect(c, image.Rect(w-1-pillar, 0, w-1, h), steelDark)
	gfx.FillRect(c, image.Rect(0, 0, w, beam), steelMid)
	gfx.FillRect(c, image.Rect(0, 0, w, 1), steelLight)
	gfx.FillRect(c, image.Rect(0, h-base, w, h), steelMid)
	gfx.FillRect(c, image.Rect(0, h-base, w, h-base+1), steelLight)

	// Plate travel.
	inner := h - beam - base
	restGap := inner - 3 - plateT
	const pressedShare = 0.42
	gap := float64(restGap) * (1 - (1-pressedShare)*clamp01(p.plate.pos))
	floor := h - base
	plateBottom := floor - int(gap)
	plateTop := plateBottom - plateT

	rodW := max(w/6, 4)
	rod := image.Rect(w/2-rodW/2, beam, w/2-rodW/2+rodW, plateTop)
	gfx.FillRect(c, rod, steelLight)
	gfx.FillRect(c, image.Rect(rod.Min.X, beam, rod.Min.X+1, plateTop), steelMid)
	plate := image.Rect(1+pillar+1, plateTop, w-2-pillar, plateBottom)
	gfx.FillRect(c, plate, theme.Pink)
	gfx.FillRect(c, image.Rect(plate.Min.X, plateTop, plate.Max.X, plateTop+1), theme.Coral)
	gfx.FillRect(c, image.Rect(plate.Min.X, plateBottom-1, plate.Max.X, plateBottom), gfx.Shade(theme.Pink, 0.7))

	// Contents share the gap, and flatten together once it gets too small.
	innerW := float64(w - 2*(pillar+3))
	restMascot := float64(restGap) * 0.42
	restThumb := float64(restGap) * 0.50
	squash := math.Min(1, gap/(restMascot+restThumb))
	spread := 1 / math.Sqrt(squash)
	thumbH := restThumb * squash
	thumbW := math.Min(innerW, restThumb*1.5*spread)
	cx := float64(w) / 2
	r := image.Rect(int(cx-thumbW/2), floor-int(thumbH), int(cx+thumbW/2), floor)
	if p.thumb != nil {
		gfx.Blit(c, p.thumb, r)
	} else {
		gfx.FillRect(c, r, steelDark)
	}
	m := moodHappy
	if squash < 0.8 {
		m = moodStrain
	}
	mh := restMascot * squash / 2
	rx := math.Min(innerW/2, restMascot*0.55*spread)
	drawMascot(c, cx, float64(floor)-thumbH, rx, mh, m)
	drawParticles(c, p.parts)
	return gfx.Render(c)
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}

// confetti is a burst of falling pixels for the results screen.
type confetti struct {
	parts []particle
	seed  uint32
}

const (
	confettiCount = 140
	confettiLife  = 110
	confettiDrop  = 24 // How far above the screen pieces start, so they stagger.
)

func (f *confetti) rand() float64 {
	f.seed = f.seed*1664525 + 1013904223
	return float64(f.seed>>16&0xFFFF) / 0xFFFF
}

func (f *confetti) burst(w int) {
	f.seed = 7
	stops := []color.NRGBA{theme.Pink, theme.Coral, theme.Yellow, theme.Mint, theme.Violet}
	f.parts = make([]particle, 0, confettiCount)
	for i := range confettiCount {
		f.parts = append(f.parts, particle{
			x: f.rand() * float64(w), y: -f.rand() * confettiDrop,
			vx: (f.rand() - 0.5) * 0.5, vy: 0.15 + f.rand()*0.3, g: confettiGravity,
			life: confettiLife/2 + int(f.rand()*confettiLife/2),
			c:    stops[i%len(stops)],
		})
	}
}

func (f *confetti) step() {
	f.parts = stepParticles(f.parts)
}

func (f *confetti) active() bool { return len(f.parts) > 0 }
