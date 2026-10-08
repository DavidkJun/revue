package diffview

import (
	"strconv"
	"strings"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/theme"
)

// rowState is how a row relates to the cursor and the visual selection; the
// palette has one look per kind of row and state.
type rowState uint8

const (
	stNormal rowState = iota
	stCursor
	stVisual
	stVisualCursor
	numStates
)

func stateOf(cursor, selected bool) rowState {
	switch {
	case selected && cursor:
		return stVisualCursor
	case selected:
		return stVisual
	case cursor:
		return stCursor
	}
	return stNormal
}

func (s rowState) cursor() bool { return s == stCursor || s == stVisualCursor }

// look is the background of a row's content and of its changed words; ""
// leaves the terminal background showing through. ref is the brighter of
// the two, which syntax colours on the row must stay legible against.
type look struct {
	bg, word, ref string
}

// sign is the marker in the sign column of a row kind.
type sign struct {
	text, fg string
}

const numKinds = int(diff.RowModNew) + 1

// palette resolves the theme into what rows are painted with, blending the
// cursor and selection variants once instead of on every frame.
type palette struct {
	th theme.Theme

	looks  [numKinds][numStates]look
	signs  [numKinds]sign
	fold   [numStates]string    // fold marker row background
	foldFG [numStates][2]string // fold marker text and its "(zo)" hint
	filler [numStates]string    // split-view filler background
	hatch  string               // filler glyphs, quieter than any text

	numFG    [numStates]string // line numbers
	gutterBG [numStates]string // number columns: only the selection paints them
	numBold  [numStates]bool

	seqs     map[seqKey]string
	readable map[[2]string]string // syntax colour on a background → legible colour
}

type seqKey struct {
	fg, bg string
	bold   bool
}

const (
	// cursorLift is how much lighter (CIE L*, 0-1) a coloured row under the
	// cursor gets. Lightening in HCL keeps the hue, so the row reads as the
	// same diff colour; blending toward white greyed red and yellow rows.
	cursorLift = 0.09
	// visualMix tints coloured rows toward the selection colour; context
	// rows take the selection colour itself.
	visualMix = 0.35
)

func newPalette(th theme.Theme) palette {
	p := palette{th: th, seqs: map[seqKey]string{}, readable: map[[2]string]string{}}
	base := [numKinds]look{
		diff.RowContext: {},
		diff.RowAdded:   {bg: th.AddBG, word: th.AddWordBG},
		diff.RowDeleted: {bg: th.DelBG, word: th.DelWordBG},
		diff.RowModOld:  {bg: th.ModOldBG, word: th.ModWordBG},
		diff.RowModNew:  {bg: th.ModBG, word: th.ModWordBG},
	}
	for k, l := range base {
		p.looks[k][stNormal] = l
		if l.bg == "" {
			p.looks[k][stCursor] = look{bg: th.CursorLineBG}
			p.looks[k][stVisual] = look{bg: th.VisualBG}
			p.looks[k][stVisualCursor] = look{bg: lift(th.VisualBG, cursorLift/2)}
			continue
		}
		p.looks[k][stCursor] = look{bg: lift(l.bg, cursorLift), word: lift(l.word, cursorLift/2)}
		vis := look{bg: blend(l.bg, th.VisualBG, visualMix), word: blend(l.word, th.VisualBG, visualMix)}
		p.looks[k][stVisual] = vis
		p.looks[k][stVisualCursor] = look{bg: lift(vis.bg, cursorLift), word: lift(vis.word, cursorLift/2)}
	}
	p.signs = [numKinds]sign{
		diff.RowContext: {" ", ""},
		diff.RowAdded:   {"+", th.Green},
		diff.RowDeleted: {"-", th.Red},
		diff.RowModOld:  {"~", blend(th.Yellow, th.ModOldBG, 0.35)},
		diff.RowModNew:  {"~", th.Yellow},
	}
	p.fold = [numStates]string{th.FoldBG, th.CursorLineBG, th.VisualBG, lift(th.VisualBG, cursorLift/2)}
	// The marker is meant to be quiet, but on a row the cursor or the
	// selection paints, its text would all but vanish.
	p.foldFG[stNormal] = [2]string{th.Muted, th.Subtle}
	for s := stCursor; s < numStates; s++ {
		p.foldFG[s] = [2]string{p.legible(th.Muted, p.fold[s]), p.legible(th.Subtle, p.fold[s])}
	}
	p.filler = [numStates]string{"", th.CursorLineBG, th.VisualBG, lift(th.VisualBG, cursorLift/2)}
	p.hatch = blend(th.Subtle, th.FoldBG, 0.3)
	selNum := blend(th.FG, th.VisualBG, 0.45)
	p.numFG = [numStates]string{th.Subtle, th.Yellow, selNum, th.Yellow}
	p.numBold = [numStates]bool{false, true, false, true}
	p.gutterBG = [numStates]string{"", "", th.VisualBG, th.VisualBG}
	for k := range p.looks {
		for s := range p.looks[k] {
			l := &p.looks[k][s]
			l.ref = l.bg
			if l.word != "" && luminance(l.word) > luminance(l.bg) {
				l.ref = l.word
			}
		}
	}
	return p
}

// seq returns the SGR sequence that resets the style and applies fg, bg and
// bold ("" leaves a colour at the terminal default).
func (p *palette) seq(fg, bg string, bold bool) string {
	k := seqKey{fg, bg, bold}
	if s, ok := p.seqs[k]; ok {
		return s
	}
	var b strings.Builder
	b.WriteString("\x1b[0")
	if bold {
		b.WriteString(";1")
	}
	writeColor(&b, "38", fg)
	writeColor(&b, "48", bg)
	b.WriteByte('m')
	s := b.String()
	p.seqs[k] = s
	return s
}

func writeColor(b *strings.Builder, kind, hex string) {
	c, err := colorful.Hex(hex)
	if hex == "" || err != nil {
		return
	}
	r, g, bl := c.RGB255()
	b.WriteByte(';')
	b.WriteString(kind)
	b.WriteString(";2;")
	b.WriteString(strconv.Itoa(int(r)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(g)))
	b.WriteByte(';')
	b.WriteString(strconv.Itoa(int(bl)))
}

// blend mixes a toward b by t in RGB; an unparsable colour is returned as is.
func blend(a, b string, t float64) string {
	ca, err := colorful.Hex(a)
	if err != nil {
		return a
	}
	cb, err := colorful.Hex(b)
	if err != nil {
		return a
	}
	return ca.BlendRgb(cb, t).Clamped().Hex()
}

const reset = "\x1b[0m"

// minContrast keeps syntax colours on coloured rows about as legible as on
// the plain background: TokyoNight's comment colour has a contrast of ~2.7
// there but nearly vanishes on a modified line under the cursor.
const minContrast = 2.6

// legible returns fg, brightened toward the theme's text colour until it has
// minContrast against bg.
func (p *palette) legible(fg, bg string) string {
	k := [2]string{fg, bg}
	if c, ok := p.readable[k]; ok {
		return c
	}
	out := fg
	for t := 0.1; contrast(out, bg) < minContrast && t < 1.05; t += 0.1 {
		out = blend(fg, p.th.FG, t)
	}
	p.readable[k] = out
	return out
}

// contrast is the WCAG contrast ratio of two colours.
func contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func luminance(hex string) float64 {
	c, err := colorful.Hex(hex)
	if err != nil {
		return 0
	}
	r, g, b := c.LinearRgb()
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// lift makes a colour lighter by dl in CIE L* without changing its hue.
func lift(hex string, dl float64) string {
	c, err := colorful.Hex(hex)
	if err != nil {
		return hex
	}
	h, ch, l := c.Hcl()
	return colorful.Hcl(h, ch, l+dl).Clamped().Hex()
}
