package highlight

import (
	"strings"
	"testing"
)

type layoutCase struct {
	name    string
	text    string
	tab     int
	xoff    int
	width   int
	want    string // plain output
	wantLen int    // Width(text, tab)
}

func runLayoutCases(t *testing.T, cases []layoutCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := checkRender(t, tc.text, nil, nil, LineStyle{Width: tc.width, XOffset: tc.xoff, TabWidth: tc.tab})
			if got := plain(out); got != tc.want {
				t.Errorf("plain output %q, want %q", got, tc.want)
			}
			if got := Width(tc.text, tc.tab); got != tc.wantLen {
				t.Errorf("Width = %d, want %d", got, tc.wantLen)
			}
		})
	}
}

func TestRenderTabs(t *testing.T) {
	runLayoutCases(t, []layoutCase{
		{"after one char", "a\tb", 4, 0, 8, "a   b   ", 5},
		{"at start", "\tb", 4, 0, 6, "    b ", 5},
		{"after two chars", "ab\tc", 4, 0, 6, "ab  c ", 5},
		{"on a stop", "abcd\te", 4, 0, 10, "abcd    e ", 9},
		{"two in a row", "abc\t\tx", 4, 0, 10, "abc     x ", 9},
		{"width 8", "a\tb", 8, 0, 10, "a       b ", 9},
		{"default width", "a\tb", 0, 0, 6, "a   b ", 5},
		{"negative width", "a\tb", -3, 0, 6, "a   b ", 5},
		{"width 1", "a\tb", 1, 0, 3, "a b", 3},
		{"offset inside tab", "\tx", 4, 2, 4, "  x ", 5},
		{"offset at tab end", "\tx", 4, 4, 3, "x  ", 5},
		{"offset one before end", "ab\tx", 4, 3, 3, " x ", 5},
		{"tab cut by right edge", "a\tb", 4, 0, 3, "a  ", 5},
		{"stops count before offset", "ab\tc", 4, 1, 4, "b  c", 5},
		{"after wide char", "中\tx", 4, 0, 6, "中  x ", 5},
		{"after cyrillic", "привет\tx", 4, 0, 10, "привет  x ", 9},
		{"after caret", "\x01\tx", 4, 0, 6, "^A  x ", 5},
		{"after invalid byte", "\xff\tx", 4, 0, 6, "\U0000FFFD   x ", 5},
	})
}

func TestRenderUnicode(t *testing.T) {
	family := "\U0001F468\U0000200D\U0001F469\U0000200D\U0001F467"
	runLayoutCases(t, []layoutCase{
		{"cyrillic", "привет", 4, 0, 8, "привет  ", 6},
		{"cyrillic scrolled", "привет", 4, 2, 3, "иве", 6},
		{"cjk", "中文字", 4, 0, 6, "中文字", 6},
		{"cjk cut both sides", "中文字", 4, 1, 4, " 文 ", 6},
		{"cjk cut at right", "中文", 4, 0, 3, "中 ", 4},
		{"emoji", "a\U0001F600b", 4, 0, 4, "a\U0001F600b", 4},
		{"emoji cut at left", "a\U0001F600b", 4, 2, 3, " b ", 4},
		{"zwj family", family + "b", 4, 0, 4, family + "b ", 3},
		{"zwj family cut", family + "b", 4, 1, 2, " b", 3},
		{"flag", "\U0001F1FA\U0001F1E6x", 4, 0, 3, "\U0001F1FA\U0001F1E6x", 3},
		{"vs16", "\U00002764\U0000FE0Fx", 4, 0, 3, "\U00002764\U0000FE0Fx", 3},
		{"skin tone", "\U0001F44D\U0001F3FDx", 4, 0, 3, "\U0001F44D\U0001F3FDx", 3},
		{"combining mark", "e\U00000301x", 4, 0, 3, "e\U00000301x ", 2},
		{"stacked marks", "x\U00000301\U00000323y", 4, 0, 2, "x\U00000301\U00000323y", 2},
		{"combining after cyrillic", "и\U00000306й", 4, 0, 2, "и\U00000306й", 2},
		{"hangul jamo", "\U00001100\U00001161\U000011A8x", 4, 0, 3, "\U00001100\U00001161\U000011A8x", 3},
		{"wide cut at both edges", "中", 4, 1, 1, " ", 2},
		{"offset beyond text", "abc", 4, 10, 5, "     ", 3},
		{"offset exactly at end", "abc", 4, 3, 2, "  ", 3},
	})
}

func TestRenderControlAndInvalid(t *testing.T) {
	runLayoutCases(t, []layoutCase{
		{"c0 and del", "\x00\x01\x1b\x1f\x7f", 4, 0, 10, "^@^A^[^_^?", 10},
		{"carriage return", "a\rb", 4, 0, 4, "a^Mb", 4},
		{"newline", "a\nb", 4, 0, 4, "a^Jb", 4},
		{"escape sequence is neutralised", "\x1b[31mred", 4, 0, 10, "^[[31mred ", 9},
		{"c1", "\U00000085x", 4, 0, 3, "^Ex", 3},
		{"caret cut at left", "\x01x", 4, 1, 2, "Ax", 3},
		{"caret cut at right", "\x01x", 4, 0, 1, "^", 3},
		{"invalid byte", "a\xffb", 4, 0, 3, "a\U0000FFFDb", 3},
		{"truncated sequence at end", "ab\xe2\x82", 4, 0, 4, "ab\U0000FFFD\U0000FFFD", 4},
		{"truncated after cyrillic", "п\xd0", 4, 0, 2, "п\U0000FFFD", 2},
		{"lone continuation", "\x80\x80", 4, 0, 2, "\U0000FFFD\U0000FFFD", 2},
		{"surrogate", "\xed\xa0\x80", 4, 0, 3, "\U0000FFFD\U0000FFFD\U0000FFFD", 3},
		{"literal replacement char", "\U0000FFFD", 4, 0, 1, "\U0000FFFD", 1},
	})
}

// A cluster next to a stand-in (tab, caret, U+FFFD, padding) must not merge
// with it when the output is measured.
func TestRenderStandInNeighbours(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"\U00000301", 0},             // combining mark with nothing before it
		{"\t\U00000301x", 5},          // after a tab
		{"\x01\U0001F3FB", 4},         // skin tone alone is wide; after a space it would not be
		{"\xff\U0000FE0F", 1},         // VS16 after U+FFFD
		{"\t\U0000200D\U0001F600", 6}, // ZWJ after a tab, then an emoji
		{"\U00000600\t", 4},           // Prepend before a tab
		{"\U00000600", 0},             // Prepend before the padding
		{"\U00000600\x01", 2},         // Prepend before a caret
		{"\U00000600\xff", 1},         // Prepend before an invalid byte
		{"\U00000600\U00000301\t", 4}, // cluster ending in a mark after Prepend
		{"\t\U00000903\t", 8},         // spacing mark after a tab
	}
	for _, tc := range cases {
		if got := Width(tc.text, 4); got != tc.want {
			t.Errorf("Width(%+q) = %d, want %d", tc.text, got, tc.want)
		}
		for _, w := range []int{1, 2, 3, 4, 5, 9} {
			for _, x := range []int{0, 1, 2, 4} {
				checkRender(t, tc.text, nil, nil, LineStyle{Width: w, XOffset: x})
			}
		}
	}
}

func TestRenderEmptyAndZeroWidth(t *testing.T) {
	out := checkRender(t, "", nil, nil, LineStyle{Width: 3})
	if got := plain(out); got != "   " {
		t.Errorf("empty text renders %q, want three spaces", got)
	}
	if Width("", 4) != 0 {
		t.Errorf("Width of empty text is not 0")
	}
	wide := checkRender(t, "x", nil, nil, LineStyle{Width: 150})
	if got := plain(wide); got != "x"+strings.Repeat(" ", 149) {
		t.Errorf("long padding: %q", got)
	}
	for _, w := range []int{0, -1} {
		if got := Render("abc", nil, nil, LineStyle{Width: w}); got != "" {
			t.Errorf("Render with width %d = %q, want empty", w, got)
		}
	}
}

func TestRenderStyles(t *testing.T) {
	const (
		red   = "#ff0000"
		blue  = "#0000ff"
		green = "#00ff00"
		white = "#ffffff"
		grey  = "#aaaaaa"
		dark  = "#111111"
	)
	spans := []Span{{Start: 0, End: 5, FG: red, Bold: true}}
	marks := []Mark{
		{Start: 2, End: 4, BG: blue},
		{Start: 3, End: 6, BG: green, FG: white},
	}
	out := checkRender(t, "abcdefgh", spans, marks, LineStyle{Width: 10, FG: grey, BG: dark})
	want := []cellStyle{
		{fg: red, bg: dark, bold: true},
		{fg: red, bg: dark, bold: true},
		{fg: red, bg: blue, bold: true},
		{fg: white, bg: green, bold: true},
		{fg: white, bg: green, bold: true},
		{fg: white, bg: green},
		{fg: grey, bg: dark},
		{fg: grey, bg: dark},
	}
	cells := decode(t, out)
	if len(cells) != 10 {
		t.Fatalf("got %d cells, want 10", len(cells))
	}
	for i, w := range want {
		if cells[i].style != w {
			t.Errorf("cell %d (%q): style %+v, want %+v", i, cells[i].text, cells[i].style, w)
		}
	}
	for i := 8; i < 10; i++ {
		if c := cells[i]; c.text != " " || c.style.bg != dark || c.style.underline {
			t.Errorf("padding cell %d: %+v, want a space on %s", i, c, dark)
		}
	}
}

func TestRenderAttributes(t *testing.T) {
	spans := []Span{
		{Start: 0, End: 1, Bold: true},
		{Start: 1, End: 2, Italic: true},
		{Start: 2, End: 3, Underline: true},
	}
	marks := []Mark{{Start: 3, End: 4, Bold: true}}
	cells := decode(t, checkRender(t, "abcde", spans, marks, LineStyle{Width: 6}))
	want := []cellStyle{{bold: true}, {italic: true}, {underline: true}, {bold: true}, {}, {}}
	for i, w := range want {
		if cells[i].style != w {
			t.Errorf("cell %d (%q): style %+v, want %+v", i, cells[i].text, cells[i].style, w)
		}
	}
}

func TestRenderOverlappingSpans(t *testing.T) {
	// Not what File produces, but Render must still pick one span per cell:
	// the last one covering it, as with marks.
	spans := []Span{{Start: 0, End: 4, FG: "#ff0000"}, {Start: 2, End: 3, FG: "#00ff00"}}
	cells := decode(t, checkRender(t, "abcd", spans, nil, LineStyle{Width: 4}))
	want := []string{"#ff0000", "#ff0000", "#00ff00", "#ff0000"}
	for i, w := range want {
		if cells[i].style.fg != w {
			t.Errorf("cell %d: fg %q, want %q", i, cells[i].style.fg, w)
		}
	}
}

func TestRenderTerminalDefaults(t *testing.T) {
	spans := []Span{{Start: 0, End: 3, Bold: true}}
	out := checkRender(t, "abc\tdef", spans, nil, LineStyle{Width: 12})
	if strings.Contains(out, "38;2") || strings.Contains(out, "48;2") {
		t.Errorf("no colour was asked for, yet the output paints one: %q", out)
	}

	// A mark on a transparent line paints only the marked cells.
	marks := []Mark{{Start: 1, End: 2, BG: "#123456"}}
	cells := decode(t, checkRender(t, "abc", nil, marks, LineStyle{Width: 5}))
	for i, c := range cells {
		want := ""
		if i == 1 {
			want = "#123456"
		}
		if c.style.bg != want {
			t.Errorf("cell %d: bg %q, want %q", i, c.style.bg, want)
		}
	}

	// A malformed colour means the terminal default, never a guess.
	bad := decode(t, checkRender(t, "ab", []Span{{Start: 0, End: 2, FG: "red"}}, nil, LineStyle{Width: 2, BG: "#12"}))
	for i, c := range bad {
		if c.style.fg != "" || c.style.bg != "" {
			t.Errorf("cell %d: %+v, want terminal defaults", i, c.style)
		}
	}
}

func TestRenderEmitsSGROnlyOnChange(t *testing.T) {
	spans := []Span{{Start: 0, End: 2, FG: "#ff0000"}, {Start: 2, End: 4, FG: "#ff0000"}}
	out := checkRender(t, "abcd", spans, nil, LineStyle{Width: 4})
	if n := strings.Count(out, "\x1b["); n != 2 {
		t.Errorf("got %d escape sequences, want 2 (style once, reset at the end): %q", n, out)
	}
	if !strings.HasPrefix(out, "\x1b[0;38;2;255;0;0m") {
		t.Errorf("first sequence must reset and set the colour: %q", out)
	}
	upper := checkRender(t, "a", []Span{{Start: 0, End: 1, FG: "#FFAA0B"}}, nil, LineStyle{Width: 1})
	if !strings.HasPrefix(upper, "\x1b[0;38;2;255;170;11m") {
		t.Errorf("upper-case hex colour: %q", upper)
	}
}

func TestRenderMarksBeyondText(t *testing.T) {
	// Marks past the end of the text do not reach into the padding.
	marks := []Mark{{Start: 1, End: 100, BG: "#222222"}}
	cells := decode(t, checkRender(t, "ab", nil, marks, LineStyle{Width: 5, BG: "#000000"}))
	want := []string{"#000000", "#222222", "#000000", "#000000", "#000000"}
	for i, w := range want {
		if cells[i].style.bg != w {
			t.Errorf("cell %d: bg %q, want %q", i, cells[i].style.bg, w)
		}
	}
}

func TestRenderTabTakesItsStyle(t *testing.T) {
	marks := []Mark{{Start: 1, End: 2, BG: "#222222"}}
	cells := decode(t, checkRender(t, "a\tb", nil, marks, LineStyle{Width: 5}))
	for i := 1; i <= 3; i++ {
		if cells[i].style.bg != "#222222" {
			t.Errorf("tab cell %d: bg %q, want the mark's", i, cells[i].style.bg)
		}
	}
	if cells[4].style.bg != "" {
		t.Errorf("cell after the tab took the mark: %+v", cells[4])
	}
}
