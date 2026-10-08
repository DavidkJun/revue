package diffview

import (
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/highlight"
)

const (
	barGlyph   = "▎"
	moreGlyph  = "›"
	fillGlyph  = "╱"
	splitGlyph = "│"
)

func (m *Model) render(focused bool) string {
	w, h := m.width, m.height
	if h <= 0 {
		return ""
	}
	if w <= 0 {
		return strings.Repeat("\n", h-1)
	}
	var b strings.Builder
	b.Grow(w * h * 3)
	b.WriteString(m.header(focused))
	body := h - 1
	if !m.active() {
		m.writeMessage(&b, body)
		return b.String()
	}
	p := m.cur()
	items := p.folds.Items()
	off := clamp(p.offset, 0, max(len(items)-1, 0))
	selLo, selHi := 0, 0
	if m.visual {
		selLo, selHi = m.selectedLines()
	}
	minW := m.minWidth()
	var narrow strings.Builder
	for r := range body {
		b.WriteByte('\n')
		i := off + r
		if i >= len(items) {
			writeSpaces(&b, w)
			continue
		}
		it := items[i]
		s, e := span(it)
		st := stateOf(i == p.cursor, s < selHi && e > selLo)
		if w >= minW {
			m.writeItem(&b, it, st, focused, w)
			continue
		}
		// Too narrow for the gutter: lay the line out at the smallest width
		// that works and cut it.
		narrow.Reset()
		m.writeItem(&narrow, it, st, focused, minW)
		b.WriteString(fit(narrow.String(), w))
	}
	return b.String()
}

func (m *Model) writeItem(b *strings.Builder, it diff.Item, st rowState, focused bool, w int) {
	switch {
	case it.Fold >= 0:
		m.writeFold(b, it, st, focused, w)
	case m.split:
		m.writeSplit(b, it.Line, st, focused, w)
	default:
		m.writeUnified(b, it.Line, st, focused, w)
	}
}

// Gutters: unified "▎old new " and split "▎num " per half, then the content
// "s code" where s is the sign column.
func (m *Model) unifiedGutter() int { return 3 + 2*m.f.numW }
func (m *Model) splitGutter() int   { return 2 + m.f.numW }

// minWidth is the narrowest pane whose lines still have a code column.
func (m *Model) minWidth() int {
	if m.split {
		return 2*(m.splitGutter()+3) + 1
	}
	return m.unifiedGutter() + 3
}

// codeWidth is the number of code cells per line (the narrower half in the
// split view).
func (m *Model) codeWidth() int {
	w := max(m.width, m.minWidth())
	if m.split {
		return (w-1)/2 - m.splitGutter() - 2
	}
	return w - m.unifiedGutter() - 2
}

func (m *Model) writeUnified(b *strings.Builder, row int, st rowState, focused bool, w int) {
	r := &m.f.rows[row]
	m.writeBar(b, st, focused, m.pal.gutterBG[st])
	m.writeNum(b, r.OldNo, st)
	m.writeNum(b, r.NewNo, st)
	m.writeContent(b, row, r.Kind, m.lineMatches(row), st, w-m.unifiedGutter())
}

func (m *Model) writeSplit(b *strings.Builder, line int, st rowState, focused bool, w int) {
	sr := m.f.split[line]
	lw := (w - 1) / 2
	rw := w - 1 - lw
	ms := m.lineMatches(line)
	left, right := ms, ms
	if sr.Left != sr.Right {
		left, right = sideMatches(ms, 0), sideMatches(ms, 1)
	}
	m.writeBar(b, st, focused, m.pal.gutterBG[st])
	m.writeHalf(b, sr.Left, true, left, st, lw-1)
	b.WriteString(m.pal.seq(m.th.Subtle, m.pal.gutterBG[st], false))
	b.WriteString(splitGlyph)
	b.WriteString(m.pal.seq("", m.pal.gutterBG[st], false))
	b.WriteByte(' ')
	m.writeHalf(b, sr.Right, false, right, st, rw-1)
}

// writeHalf writes one side of a split row after its first column: the
// line number and the content, or hatching where the side has no line.
func (m *Model) writeHalf(b *strings.Builder, row int, old bool, ms []match, st rowState, w int) {
	if row < 0 {
		nw := m.f.numW + 1
		b.WriteString(m.pal.seq("", m.pal.gutterBG[st], false))
		writeSpaces(b, nw)
		b.WriteString(m.pal.seq(m.pal.hatch, m.pal.filler[st], false))
		for range w - nw {
			b.WriteString(fillGlyph)
		}
		b.WriteString(reset)
		return
	}
	r := &m.f.rows[row]
	n := r.NewNo
	if old {
		n = r.OldNo
	}
	m.writeNum(b, n, st)
	m.writeContent(b, row, r.Kind, ms, st, w-m.f.numW-1)
}

func sideMatches(ms []match, side uint8) []match {
	k := sort.Search(len(ms), func(k int) bool { return ms[k].side >= side })
	e := sort.Search(len(ms), func(k int) bool { return ms[k].side > side })
	return ms[k:e]
}

// writeBar writes the first column: the accent bar on the focused cursor row.
func (m *Model) writeBar(b *strings.Builder, st rowState, focused bool, bg string) {
	if st.cursor() && focused {
		b.WriteString(m.pal.seq(m.th.Accent, bg, false))
		b.WriteString(barGlyph)
		return
	}
	b.WriteString(m.pal.seq("", bg, false))
	b.WriteByte(' ')
}

// writeNum writes a right-aligned line number (blank for 0) and a space.
func (m *Model) writeNum(b *strings.Builder, n int, st rowState) {
	b.WriteString(m.pal.seq(m.pal.numFG[st], m.pal.gutterBG[st], m.pal.numBold[st]))
	var buf [20]byte
	digits := buf[:0]
	if n > 0 {
		digits = strconv.AppendInt(digits, int64(n), 10)
	}
	writeSpaces(b, m.f.numW-len(digits))
	b.Write(digits)
	b.WriteByte(' ')
}

// writeContent writes the sign column, a space and the code of row into w
// cells, with a "›" in the last cell when the line goes on past the edge.
func (m *Model) writeContent(b *strings.Builder, row int, kind diff.RowKind, ms []match, st rowState, w int) {
	if w <= 0 {
		return
	}
	lk := m.pal.looks[kind][st]
	sg := m.pal.signs[kind]
	b.WriteString(m.pal.seq(sg.fg, lk.bg, true))
	b.WriteString(sg.text)
	if w == 1 {
		b.WriteString(reset)
		return
	}
	b.WriteByte(' ')
	codeW := w - 2
	if codeW == 0 {
		b.WriteString(reset)
		return
	}
	f := m.f
	more := f.width(row)-m.xoff > codeW
	if more {
		codeW--
	}
	if codeW > 0 {
		m.writeCode(b, row, ms, lk, codeW)
	}
	if more {
		b.WriteString(m.pal.seq(m.th.Muted, lk.bg, false))
		b.WriteString(moreGlyph)
		b.WriteString(reset)
	}
}

// writeCode paints the code of row into w cells at the horizontal scroll.
// Only the spans and marks that can reach the window go to Render, which
// looks at every mark it gets: a minified line searched for a common letter
// has hundreds of thousands of matches.
func (m *Model) writeCode(b *strings.Builder, row int, ms []match, lk look, w int) {
	text := m.f.rows[row].Text
	lo, hi := visibleBytes(text, m.xoff, w)
	spans := m.legibleSpans(clipSpans(m.f.spans(row), lo, hi), lk.ref)
	st := highlight.LineStyle{Width: w, XOffset: m.xoff, BG: lk.bg, FG: m.th.FG}
	b.WriteString(highlight.Render(text, spans, m.marks(row, ms, lk, lo, hi), st))
}

// marks paints changed words, then search matches over them, of the bytes
// [lo, hi) of row.
func (m *Model) marks(row int, ms []match, lk look, lo, hi int) []highlight.Mark {
	changed := m.f.rows[row].Changed
	ms = clipMatches(ms, lo, hi)
	if len(changed) == 0 && len(ms) == 0 {
		return nil
	}
	out := make([]highlight.Mark, 0, len(changed)+len(ms))
	for _, c := range changed {
		if c.End > lo && c.Start < hi {
			out = append(out, highlight.Mark{Start: c.Start, End: c.End, BG: lk.word})
		}
	}
	for _, s := range ms {
		out = append(out, highlight.Mark{Start: int(s.start), End: int(s.end), BG: m.th.SearchBG, FG: m.th.FG, Bold: true})
	}
	return out
}

// writeFold writes a closed fold as one muted full-width line.
func (m *Model) writeFold(b *strings.Builder, it diff.Item, st rowState, focused bool, w int) {
	bg := m.pal.fold[st]
	m.writeBar(b, st, focused, bg)
	g := m.unifiedGutter()
	if m.split {
		g = m.splitGutter()
	}
	b.WriteString(m.pal.seq("", bg, false))
	writeSpaces(b, g-1)
	fg := m.pal.foldFG[st]
	parts := [...]struct{ text, fg string }{
		{"⋯ " + plural(it.Hidden(), "unchanged line"), fg[0]},
		{"  (zo)", fg[1]},
		{"  ⋯", fg[0]},
	}
	rest := w - g
	for k, pt := range parts {
		pw := ansi.StringWidth(pt.text)
		if pw > rest {
			if k == 0 {
				b.WriteString(m.pal.seq(pt.fg, bg, false))
				b.WriteString(fit(pt.text, rest))
				rest = 0
			}
			break
		}
		b.WriteString(m.pal.seq(pt.fg, bg, false))
		b.WriteString(pt.text)
		rest -= pw
	}
	b.WriteString(m.pal.seq("", bg, false))
	writeSpaces(b, rest)
	b.WriteString(reset)
}

func (m *Model) writeMessage(b *strings.Builder, body int) {
	w := m.width
	var lines []string
	if m.message != "" {
		// A narrow pane wraps the message instead of cutting it: the end
		// of "x is no longer changed" is the part that matters.
		limit := max(w-2, 1)
		for _, l := range strings.Split(printable(m.message), "\n") {
			for _, wl := range strings.Split(ansi.Wrap(l, limit, "/"), "\n") {
				lines = append(lines, strings.TrimSpace(wl))
			}
		}
	}
	top := max((body-len(lines))/2, 0)
	for r := range body {
		b.WriteByte('\n')
		k := r - top
		if k < 0 || k >= len(lines) {
			writeSpaces(b, w)
			continue
		}
		text := ansi.Truncate(lines[k], w, "…")
		tw := ansi.StringWidth(text)
		left := (w - tw) / 2
		writeSpaces(b, left)
		b.WriteString(m.pal.seq(m.th.Muted, "", false))
		b.WriteString(text)
		b.WriteString(reset)
		writeSpaces(b, w-left-tw)
	}
}

// printable drops the control characters of s except newlines, and turns
// tabs into spaces: the terminal moves the cursor for them although
// ansi.StringWidth counts them as no cells.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r == '\n':
			return r
		case r < 0x20, r >= 0x7f && r < 0xa0:
			return -1
		}
		return r
	}, s)
}

const spaces = "                                                                              "

func writeSpaces(b *strings.Builder, n int) {
	for n > len(spaces) {
		b.WriteString(spaces)
		n -= len(spaces)
	}
	if n > 0 {
		b.WriteString(spaces[:n])
	}
}

// fit cuts or pads s to exactly w cells.
func fit(s string, w int) string {
	sw := ansi.StringWidth(s)
	if sw > w {
		s = ansi.Truncate(s, w, "") + reset
		sw = ansi.StringWidth(s)
	}
	if sw < w {
		s += strings.Repeat(" ", w-sw)
	}
	return s
}

// legibleSpans returns spans with colours that would be hard to read on bg
// brightened. The result shares a buffer that the next call reuses.
func (m *Model) legibleSpans(spans []highlight.Span, bg string) []highlight.Span {
	if bg == "" {
		return spans
	}
	out := spans
	for k, sp := range spans {
		if sp.FG == "" {
			continue
		}
		fg := m.pal.legible(sp.FG, bg)
		if fg == sp.FG {
			continue
		}
		if &out[0] == &spans[0] {
			m.spanBuf = append(m.spanBuf[:0], spans...)
			out = m.spanBuf
		}
		out[k].FG = fg
	}
	return out
}
