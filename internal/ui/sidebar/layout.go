package sidebar

import (
	"image/color"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

// seg is a run of text in one style. An empty fg keeps the terminal's
// default foreground.
type seg struct {
	text string
	fg   string
	bold bool
}

// piece is a part of a row that is shortened or dropped as a unit when the
// row does not fit.
type piece struct {
	segs    []seg
	flex    int  // index of the seg that may be shortened with "…", -1 for none
	leftCut bool // shorten from the left, keeping the end ("…app/core")
	dropped bool
}

// squeeze is one step of fitting a row: shorten a piece's flex seg down to
// min cells, or drop the piece entirely when min is negative.
type squeeze struct {
	piece int
	min   int
}

// line is a row laid out as left-aligned pieces, then right-aligned ones
// (pieces[right:]), with a plan saying what gives way first when space runs
// out. Anything still too wide after the plan is cut hard at the edge.
type line struct {
	pieces []piece
	right  int
	plan   []squeeze
}

func fixed(segs ...seg) piece { return piece{segs: segs, flex: -1} }

// flexible makes a piece whose last seg may be shortened.
func flexible(segs ...seg) piece { return piece{segs: segs, flex: len(segs) - 1} }

func (l *line) widths() (left, right int) {
	for i := range l.pieces {
		p := &l.pieces[i]
		if p.dropped {
			continue
		}
		w := 0
		for _, s := range p.segs {
			w += ansi.StringWidth(s.text)
		}
		if i < l.right {
			left += w
		} else {
			right += w
		}
	}
	return left, right
}

// need is the width the row takes now, with one cell of air between the
// two sides.
func (l *line) need() int {
	left, right := l.widths()
	if left > 0 && right > 0 {
		return left + 1 + right
	}
	return left + right
}

// render fits the row into exactly width cells. bg, when set, paints the
// whole row (the cursor row); otherwise the row stays transparent.
func (l *line) render(width int, bg string) string {
	if width <= 0 {
		return ""
	}
	for _, s := range l.plan {
		over := l.need() - width
		if over <= 0 {
			break
		}
		p := &l.pieces[s.piece]
		if p.dropped {
			continue
		}
		if s.min < 0 {
			p.dropped = true
			continue
		}
		if p.flex < 0 {
			continue
		}
		f := &p.segs[p.flex]
		w := ansi.StringWidth(f.text)
		if w <= s.min {
			continue
		}
		f.text = shorten(f.text, max(s.min, w-over), p.leftCut)
	}

	var b strings.Builder
	write := func(from, to int) {
		for i := from; i < to; i++ {
			if l.pieces[i].dropped {
				continue
			}
			for _, s := range l.pieces[i].segs {
				b.WriteString(paint(s.text, s.fg, bg, s.bold))
			}
		}
	}
	left, right := l.widths()
	write(0, l.right)
	if fill := width - left - right; fill > 0 {
		b.WriteString(paint(strings.Repeat(" ", fill), "", bg, false))
	}
	write(l.right, len(l.pieces))

	out := b.String()
	if left+right > width {
		out = ansi.Truncate(out, width, "") + ansi.ResetStyle
	}
	if w := ansi.StringWidth(out); w < width {
		out += paint(strings.Repeat(" ", width-w), "", bg, false)
	}
	return out
}

// shorten fits s into w cells with an ellipsis on the cut side.
func shorten(s string, w int, fromLeft bool) string {
	sw := ansi.StringWidth(s)
	if sw <= w {
		return s
	}
	if w <= 0 {
		return ""
	}
	if !fromLeft {
		return ansi.Truncate(s, w, "…")
	}
	// A wide rune straddling the cut survives TruncateLeft, so cut until it fits.
	for cut := sw - w + 1; cut <= sw; cut++ {
		if out := ansi.TruncateLeft(s, cut, "…"); ansi.StringWidth(out) <= w {
			return out
		}
	}
	return "…"
}

// paint wraps text in one SGR style. Empty colours keep the terminal
// defaults, so unstyled text stays transparent over the user's background.
func paint(text, fg, bg string, bold bool) string {
	if text == "" {
		return ""
	}
	var st ansi.Style
	if c, ok := parseHex(fg); ok {
		st = st.ForegroundColor(c)
	}
	if c, ok := parseHex(bg); ok {
		st = st.BackgroundColor(c)
	}
	if bold {
		st = st.Bold()
	}
	return st.Styled(text)
}

// parseHex reads "#rrggbb". It runs for every styled cell run of every frame,
// so it avoids the fmt-based parser of go-colorful.
func parseHex(s string) (color.RGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, false
	}
	var rgb [3]uint8
	for i := range rgb {
		hi, ok1 := hexDigit(s[1+2*i])
		lo, ok2 := hexDigit(s[2+2*i])
		if !ok1 || !ok2 {
			return color.RGBA{}, false
		}
		rgb[i] = hi<<4 | lo
	}
	return color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 0xff}, true
}

func hexDigit(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// dim mixes colour a toward b, for secondary text that keeps a hint of its
// hue (line counts) without painting a background.
func dim(a, b string, t float64) string {
	ca, errA := colorful.Hex(a)
	cb, errB := colorful.Hex(b)
	if errA != nil || errB != nil {
		return a
	}
	return ca.BlendLab(cb, t).Clamped().Hex()
}
