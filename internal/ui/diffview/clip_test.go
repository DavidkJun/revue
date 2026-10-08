package diffview

import (
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/highlight"
)

// renderAll paints row like writeContent does, but with every span and mark
// of the row: the reference for the clipped version.
func renderAll(m *Model, row int, st rowState, w int) string {
	f := m.f
	lk := m.pal.looks[f.rows[row].Kind][st]
	var marks []highlight.Mark
	for _, c := range f.rows[row].Changed {
		marks = append(marks, highlight.Mark{Start: c.Start, End: c.End, BG: lk.word})
	}
	for _, s := range m.lineMatches(row) {
		marks = append(marks, highlight.Mark{Start: int(s.start), End: int(s.end), BG: m.th.SearchBG, FG: m.th.FG, Bold: true})
	}
	ls := highlight.LineStyle{Width: w, XOffset: m.xoff, BG: lk.bg, FG: m.th.FG}
	spans := append([]highlight.Span(nil), m.legibleSpans(f.spans(row), lk.ref)...)
	return highlight.Render(f.rows[row].Text, spans, marks, ls)
}

// Only the spans and marks that can show in the window are painted; the
// result must be what painting all of them gives.
func TestClippedLineMatchesFullRender(t *testing.T) {
	texts := []string{
		"\tconst a = foo(1, 'x'); /* c */ const b = 'Привіт світ' + `${x}` // a世界b\t\tend",
		strings.Repeat("ab\tc世d", 30),
		strings.Repeat("e\u0301x", 50) + "\x01\x7f" + strings.Repeat("ee", 40),
		"\ufeff" + strings.Repeat("abc ", 60),
	}
	for _, text := range texts {
		fd := fileDiff(t, " "+text, "-"+text+" old", "+"+text+" new", "+"+text)
		m := newModel(t, 120, 8, 50, fd)
		for _, pattern := range []string{"e", "a", "c", "世", "x"} {
			m.SetSearch(pattern)
			for _, w := range []int{1, 2, 5, 17, 60} {
				for x := 0; x < 90; x += 3 {
					m.xoff = x
					for row := range m.f.rows {
						kind := m.f.rows[row].Kind
						lk := m.pal.looks[kind][stCursor]
						var b strings.Builder
						m.writeCode(&b, row, m.lineMatches(row), lk, w)
						if want := renderAll(m, row, stCursor, w); b.String() != want {
							t.Fatalf("pattern %q width %d xoff %d row %d:\n got %q\nwant %q", pattern, w, x, row, b.String(), want)
						}
					}
				}
			}
		}
	}
}

// A long minified line with a search for a common letter has hundreds of
// thousands of matches; only the handful on screen may be painted.
func TestDenseMatchesOnLongLineAreClipped(t *testing.T) {
	line := strings.Repeat("function(e){return e.default=e,e}var a=1;", 20000)
	m := newModel(t, 120, 5, 50, fileDiff(t, "+"+line))
	if n := m.SetSearch("e"); n < 100000 {
		t.Fatalf("setup: %d matches", n)
	}
	lk := m.pal.looks[m.f.rows[0].Kind][stNormal]
	for _, x := range []int{0, 4000, 400000} {
		m.xoff = x
		lo, hi := visibleBytes(m.f.rows[0].Text, m.xoff, 100)
		if marks := m.marks(0, m.lineMatches(0), lk, lo, hi); len(marks) > 100 {
			t.Fatalf("xoff %d: %d marks painted for a 100-cell window", x, len(marks))
		}
	}
}
