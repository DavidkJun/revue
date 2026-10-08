package highlight

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// oCell is one cell (or one zero-width cluster) of a line as the oracle lays
// it out: start is the byte of the text it is drawn from, -1 for padding.
type oCell struct {
	text  string
	width int
	start int
}

// oracleLayout lays text out without any of layout.go: every maximal run of
// valid, non-control text is segmented and measured on its own by ansi, and
// every stand-in has its fixed width. It returns the cells and the width.
func oracleLayout(text string, tabWidth int) ([]oCell, int) {
	if tabWidth <= 0 {
		tabWidth = 4
	}
	fffd := ansi.StringWidth(replacement)
	var cells []oCell
	col := 0
	segStart := -1
	flush := func(end int) {
		if segStart < 0 {
			return
		}
		seg := text[segStart:end]
		for j := 0; j < len(seg); {
			c, w := ansi.FirstGraphemeCluster(seg[j:], ansi.GraphemeWidth)
			cells = append(cells, oCell{text: c, width: w, start: segStart + j})
			col += w
			j += len(c)
		}
		segStart = -1
	}
	for i := 0; i < len(text); {
		r, n := utf8.DecodeRuneInString(text[i:])
		switch {
		case r == utf8.RuneError && n == 1:
			flush(i)
			cells = append(cells, oCell{text: replacement, width: fffd, start: i})
			col += fffd
		case r == '\t':
			flush(i)
			for k := tabWidth - col%tabWidth; k > 0; k-- {
				cells = append(cells, oCell{text: " ", width: 1, start: i})
				col++
			}
		case r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0:
			flush(i)
			glyph := string(rune(r ^ 0x40))
			if r >= 0x80 {
				glyph = string(rune(r - 0x40))
			}
			cells = append(cells, oCell{text: "^", width: 1, start: i}, oCell{text: glyph, width: 1, start: i})
			col += 2
		default:
			if segStart < 0 {
				segStart = i
			}
		}
		i += n
	}
	flush(len(text))
	return cells, col
}

// oracleWindow cuts columns [left, left+width) out of the oracle layout and
// pads it: a wide cell cut by an edge becomes spaces, and a zero-width
// cluster shows when it sits inside the window.
func oracleWindow(cells []oCell, left, width int) []oCell {
	left = max(left, 0)
	right := left + width
	var out []oCell
	col := 0
	for _, c := range cells {
		if col >= right {
			break
		}
		switch {
		case c.width == 0:
			if col >= left {
				out = append(out, c)
			}
		case col >= left && col+c.width <= right:
			out = append(out, c)
		case col+c.width > left:
			for k := max(col, left); k < min(col+c.width, right); k++ {
				out = append(out, oCell{text: " ", width: 1, start: c.start})
			}
		}
		col += c.width
	}
	for k := max(col, left); k < right; k++ {
		out = append(out, oCell{text: " ", width: 1, start: -1})
	}
	return out
}

// normColour is what a colour shows as: malformed means terminal default.
func normColour(c string) string {
	if _, _, _, ok := parseHex(c); !ok {
		return ""
	}
	return strings.ToLower(c)
}

// oracleStyle is the style of the cell drawn from byte off, straight from the
// rules: the last covering span, then the last covering mark.
func oracleStyle(off int, spans []Span, marks []Mark, st LineStyle) cellStyle {
	s := cellStyle{fg: st.FG, bg: st.BG}
	var cover *Span
	for k := range spans {
		if spans[k].Start <= off && off < spans[k].End {
			cover = &spans[k]
		}
	}
	if cover != nil {
		if cover.FG != "" {
			s.fg = cover.FG
		}
		s.bold, s.italic, s.underline = cover.Bold, cover.Italic, cover.Underline
	}
	var win *Mark
	for k := range marks {
		if marks[k].Start <= off && off < marks[k].End {
			win = &marks[k]
		}
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
	s.fg, s.bg = normColour(s.fg), normColour(s.bg)
	return s
}

// checkAgainstOracle renders one case and compares everything visible with
// the oracle: total width (raw and stripped), Width, the characters shown and
// the style of every cell.
func checkAgainstOracle(t *testing.T, text string, spans []Span, marks []Mark, st LineStyle) {
	t.Helper()
	cells, total := oracleLayout(text, st.TabWidth)
	if got := Width(text, st.TabWidth); got != total {
		t.Fatalf("Width(%+q, %d) = %d, oracle %d", text, st.TabWidth, got, total)
	}
	out := checkRender(t, text, spans, marks, st)
	if st.Width <= 0 {
		return
	}
	if !strings.HasPrefix(out, "\x1b[0") {
		t.Fatalf("Render(%+q, %+v) does not start with a reset: %q", text, st, out)
	}
	want := oracleWindow(cells, st.XOffset, st.Width)
	var wantText strings.Builder
	for _, c := range want {
		wantText.WriteString(c.text)
	}
	if got, w := plain(out), strings.ReplaceAll(wantText.String(), zwsp, ""); got != w {
		t.Fatalf("Render(%+q, %+v):\n got %+q\nwant %+q", text, st, got, w)
	}

	var got []testCell
	for _, c := range decode(t, out) {
		if c.width > 0 {
			got = append(got, c)
		}
	}
	k := 0
	for _, w := range want {
		if w.width == 0 {
			continue
		}
		if k >= len(got) {
			t.Fatalf("Render(%+q, %+v): ran out of cells at %d: %q", text, st, k, out)
		}
		g := got[k]
		k++
		if g.text != w.text {
			t.Fatalf("Render(%+q, %+v): cell %d is %+q, want %+q: %q", text, st, k-1, g.text, w.text, out)
		}
		if w.start < 0 {
			if g.style.bg != normColour(st.BG) || g.style.underline {
				t.Fatalf("Render(%+q, %+v): padding cell %d has %+v: %q", text, st, k-1, g.style, out)
			}
			continue
		}
		if ws := oracleStyle(w.start, spans, marks, st); g.style != ws {
			t.Fatalf("Render(%+q, spans %+v, marks %+v, %+v): cell %d (%+q from byte %d) has %+v, want %+v\n%q",
				text, spans, marks, st, k-1, g.text, w.start, g.style, ws, out)
		}
	}
	if k != len(got) {
		t.Fatalf("Render(%+q, %+v): %d cells, oracle %d: %q", text, st, len(got), k, out)
	}
}

// oraclePieces is what random lines are made of: ASCII, every grapheme
// class that joins or splits clusters, wide and ambiguous characters,
// default-ignorables, controls and malformed UTF-8.
var oraclePieces = []string{
	"a", "Z", "0", " ", "{", "~", "\t", "\t", "\t",
	"\x00", "\x01", "\x1b", "\x1f", "\x7f", "\r", "\n",
	"\U00000080", "\U00000085", "\U0000009B", "\U0000009F",
	"\xff", "\xfe", "\x80", "\xbf", "\xc0\xaf", "\xc3", "\xe2\x82", "\xf0\x9f\x98", "\xed\xa0\x80", "\xf4\x90\x80\x80",
	"п", "ё", "Ї", "中", "文", "\U0000FF71", "\U0000FF21", "\U000000A0", "\U000000B7", "\U00002026", "\U000000E9",
	"\U0000FFFD", "\U0000FFFE", "\U0000FFFF", "\U0010FFFF", "\U000E0001", "\U0000E000", "\U00000378",
	"\U0001F600", "\U0001F468\U0000200D\U0001F469\U0000200D\U0001F467", "\U0001F1FA\U0001F1E6", "\U0001F1FA", "\U0001F1E6",
	"\U00002764\U0000FE0F", "\U00002764", "\U0001F44D\U0001F3FD", "1\U0000FE0F\U000020E3", "\U0001F3F4\U000E0067\U000E0062\U000E0065\U000E006E\U000E0067\U000E007F",
	"\U00000301", "\U00000323", "\U0000FE0F", "\U0000FE0E", "\U0001F3FB", "\U0000200D", "\U0000200C", "\U00000903", "\U0000093F",
	"\U00000600", "\U00000605", "\U000006DD", "\U000110BD", "\U00000D4E",
	"\U0000200B", "\U000000AD", "\U0000FEFF", "\U00002028", "\U00002029", "\U0000200E", "\U0000202E", "\U00002060", "\U0000061C",
	"\U00001100", "\U00001161", "\U000011A8", "\U0000AC00", "\U0000AC01",
	"क", "\U0000094D", "ष", "त", "\U0000093C",
	"e\U00000301", "\U00000E33", "\U00000E40",
}

func randomOracleText(r *rand.Rand) string {
	var b strings.Builder
	for n := r.IntN(14); n >= 0; n-- {
		switch r.IntN(12) {
		case 0:
			// Any scalar value at all; surrogates encode as U+FFFD.
			b.WriteRune(rune(r.IntN(utf8.MaxRune + 1)))
		case 1:
			b.WriteByte(byte(r.IntN(256)))
		default:
			b.WriteString(oraclePieces[r.IntN(len(oraclePieces))])
		}
	}
	return b.String()
}

var oracleColours = []string{"", "#ff0000", "#00FF00", "#0000ff", "#c0caf5", "nope", "#12", "#gg0000", "#1234567"}

func randomOracleSpans(r *rand.Rand, n int) []Span {
	var spans []Span
	if r.IntN(2) == 0 {
		// Sorted and disjoint, like File's.
		for at := r.IntN(3); at < n && len(spans) < 30; {
			l := r.IntN(5)
			spans = append(spans, Span{Start: at, End: at + l, FG: oracleColours[r.IntN(len(oracleColours))],
				Bold: r.IntN(2) == 0, Italic: r.IntN(3) == 0, Underline: r.IntN(5) == 0})
			at += l + r.IntN(3)
		}
		return spans
	}
	for range r.IntN(6) {
		s, e := randomRange(r, n)
		spans = append(spans, Span{Start: s, End: e, FG: oracleColours[r.IntN(len(oracleColours))],
			Bold: r.IntN(2) == 0, Italic: r.IntN(3) == 0, Underline: r.IntN(5) == 0})
	}
	return spans
}

func randomOracleMarks(r *rand.Rand, n int) []Mark {
	var marks []Mark
	for range r.IntN(5) {
		s, e := randomRange(r, n)
		marks = append(marks, Mark{Start: s, End: e, BG: oracleColours[r.IntN(len(oracleColours))],
			FG: oracleColours[r.IntN(len(oracleColours))], Bold: r.IntN(3) == 0})
	}
	return marks
}

// randomRange returns a byte range that is often reversed, empty, negative
// or far outside a line of n bytes.
func randomRange(r *rand.Rand, n int) (int, int) {
	pick := func() int {
		switch r.IntN(8) {
		case 0:
			return -1 - r.IntN(5)
		case 1:
			return n + r.IntN(5)
		case 2:
			return math.MaxInt - r.IntN(2)
		case 3:
			return math.MinInt + r.IntN(2)
		}
		return r.IntN(n + 1)
	}
	return pick(), pick()
}

func TestRenderAgainstOracle(t *testing.T) {
	r := rand.New(rand.NewPCG(2026, 10))
	for range 6000 {
		text := randomOracleText(r)
		_, total := oracleLayout(text, 4)
		st := LineStyle{
			Width:    r.IntN(total+8) - 1,
			XOffset:  r.IntN(total+6) - 2,
			TabWidth: r.IntN(10) - 1,
			FG:       oracleColours[r.IntN(len(oracleColours))],
			BG:       oracleColours[r.IntN(len(oracleColours))],
		}
		if r.IntN(10) == 0 {
			st.Width = 1 + r.IntN(200)
		}
		var spans []Span
		var marks []Mark
		if r.IntN(4) != 0 {
			spans = randomOracleSpans(r, len(text))
			marks = randomOracleMarks(r, len(text))
		}
		checkAgainstOracle(t, text, spans, marks, st)
	}
}

func FuzzRender(f *testing.F) {
	f.Add("const a = 1;", 0, 20, 4, 0, 5, 2, 8)
	f.Add("\tп\U00000301中\U0001F468\U0000200D\U0001F469\x01\xff", 3, 7, 4, 1, 4, 0, 3)
	f.Add("\U00000600\t\U00000903\x7f\U00000085\xe2\x82", 1, 3, 8, 2, 3, 1, 2)
	f.Add("e\U00000301\U0001F1FA\U0001F1E6\U0001F1FA\U0000FE0F\U0000200B", 2, 5, 2, 0, 9, 3, 4)
	f.Fuzz(func(t *testing.T, text string, xoff, width, tab, s0, e0, m0, m1 int) {
		if width > 500 || len(text) > 400 {
			return
		}
		spans := []Span{{Start: s0, End: e0, FG: "#ff0000", Bold: true}, {Start: e0, End: s0 + 3, Underline: true}}
		marks := []Mark{{Start: m0, End: m1, BG: "#00ff00"}, {Start: m1, End: m0, BG: "#0000ff", FG: "#ffffff"}}
		checkAgainstOracle(t, text, spans, marks, LineStyle{Width: width, XOffset: xoff, TabWidth: tab % 17, BG: "#111111"})
	})
}
