package highlight

import (
	"math"
	"strings"
)

// cellStyle is everything Render varies per cell.
type cellStyle struct {
	fg, bg                  string
	bold, italic, underline bool
}

// lineStyler resolves the style of the cell drawn from a byte offset.
// Offsets must not decrease between calls. A style holds for the run of
// bytes up to the next span or mark edge, so it is resolved once per run.
type lineStyler struct {
	spans  spanCursor
	marks  []Mark
	fg, bg string

	cur    cellStyle
	until  int // cur holds for offsets below until
	primed bool
}

// at returns the style at off and whether a new run starts there.
func (l *lineStyler) at(off int) (cellStyle, bool) {
	if l.primed && off < l.until {
		return l.cur, false
	}
	s := cellStyle{fg: l.fg, bg: l.bg}
	sp, until := l.spans.at(off)
	if sp != nil {
		if sp.FG != "" {
			s.fg = sp.FG
		}
		s.bold, s.italic, s.underline = sp.Bold, sp.Italic, sp.Underline
	}
	var win *Mark
	for k := range l.marks {
		m := &l.marks[k]
		if m.Start <= off && off < m.End {
			win = m
		}
		until = min(until, nextEdge(m.Start, m.End, off))
	}
	if win != nil {
		if win.BG != "" {
			s.bg = win.BG
		}
		if win.FG != "" {
			s.fg = win.FG
		}
		s.bold = s.bold || win.Bold
	}
	l.cur, l.until, l.primed = s, until, true
	return s, true
}

// nextEdge returns the first of start and end that lies beyond off.
func nextEdge(start, end, off int) int {
	switch {
	case start > off:
		return start
	case end > off:
		return end
	}
	return math.MaxInt
}

// spanCursor finds the span covering an offset. Spans from File are sorted
// and disjoint, which allows a forward-only walk; anything else falls back to
// a scan where the last covering span wins, as with marks.
type spanCursor struct {
	spans    []Span
	next     int
	disjoint bool
}

func newSpanCursor(spans []Span) spanCursor {
	disjoint := true
	end := 0
	for _, sp := range spans {
		if sp.Start < end || sp.Start > sp.End {
			disjoint = false
			break
		}
		end = sp.End
	}
	return spanCursor{spans: spans, disjoint: disjoint}
}

// at returns the span covering off (nil if none) and the next offset at
// which that can change.
func (c *spanCursor) at(off int) (*Span, int) {
	if c.disjoint {
		for c.next < len(c.spans) && c.spans[c.next].End <= off {
			c.next++
		}
		if c.next == len(c.spans) {
			return nil, math.MaxInt
		}
		if sp := &c.spans[c.next]; sp.Start <= off {
			return sp, sp.End
		}
		return nil, c.spans[c.next].Start
	}
	var cover *Span
	until := math.MaxInt
	for k := range c.spans {
		sp := &c.spans[k]
		if sp.Start <= off && off < sp.End {
			cover = sp
		}
		until = min(until, nextEdge(sp.Start, sp.End, off))
	}
	return cover, until
}

// painter writes cells, emitting SGR only where the style changes.
type painter struct {
	b       strings.Builder
	cur     cellStyle
	started bool
}

func (p *painter) style(s cellStyle) {
	if p.started && s == p.cur {
		return
	}
	writeSGR(&p.b, p.cur, s, !p.started)
	p.cur, p.started = s, true
}

const spaces = "                                                                "

func (p *painter) spaces(n int) {
	for n > len(spaces) {
		p.b.WriteString(spaces)
		n -= len(spaces)
	}
	p.b.WriteString(spaces[:n])
}

func render(text string, spans []Span, marks []Mark, st LineStyle) string {
	if st.Width <= 0 {
		return ""
	}
	tabWidth := tabStop(st.TabWidth)
	left := max(st.XOffset, 0)
	right := left + st.Width
	styler := lineStyler{spans: newSpanCursor(spans), marks: marks, fg: st.FG, bg: st.BG}

	var p painter
	p.b.Grow(st.Width + min(len(text), 4*st.Width) + 24*(min(len(spans), st.Width)+2*len(marks)+2))

	col := 0
	afterStandIn := true
	for i := 0; i < len(text) && col < right; {
		u := nextUnit(text, i, col, tabWidth, afterStandIn)
		if col+u.width > left || (u.width == 0 && col >= left) {
			if s, fresh := styler.at(i); fresh {
				p.style(s)
			}
			p.unit(text[i:u.end], u, col, left, right)
		}
		col += u.width
		afterStandIn = u.kind != unitText
		i = u.end
	}

	if pad := right - max(col, left); pad > 0 {
		// Only the background and underline of a space show, so keep the rest
		// of the current style rather than switch it for nothing.
		pst := cellStyle{fg: st.FG, bg: st.BG}
		if p.started {
			pst = cellStyle{fg: p.cur.fg, bg: st.BG, bold: p.cur.bold, italic: p.cur.italic}
		}
		p.style(pst)
		p.spaces(pad)
	}
	p.b.WriteString("\x1b[0m")
	return p.b.String()
}

// unit writes the visible part of u, which occupies columns
// [col, col+u.width) of the line; the window is [left, right).
func (p *painter) unit(src string, u unit, col, left, right int) {
	switch u.kind {
	case unitTab:
		p.spaces(min(col+u.width, right) - max(col, left))
		return
	case unitCaret:
		if col >= left {
			p.b.WriteByte('^')
		}
		if col+1 < right {
			p.b.WriteByte(u.glyph)
		}
		return
	}
	if col < left || col+u.width > right {
		p.spaces(min(col+u.width, right) - max(col, left))
		return
	}
	if u.kind == unitInvalid {
		p.b.WriteString(replacement)
		return
	}
	if u.fenceBefore {
		p.b.WriteString(zwsp)
	}
	p.b.WriteString(src)
	if u.fenceAfter {
		p.b.WriteString(zwsp)
	}
}
