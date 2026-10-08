package highlight

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// fragments are the pieces random lines are built from: everything that has
// tripped up terminal width calculations at some point.
var fragments = []string{
	"a", "Z", " ", "{", "\t", "\t", "\x01", "\x1b", "\x7f", "\r", "\U00000085", "\U0000009B",
	"\xff", "\xe2\x82", "\xc3", "\x80",
	"п", "ё", "中", "文", "\U0000FFFD",
	"\U0001F600", "\U0001F468\U0000200D\U0001F469\U0000200D\U0001F467",
	"\U0001F1FA\U0001F1E6", "\U0001F1FA",
	"\U00002764\U0000FE0F", "\U0001F44D\U0001F3FD",
	"\U00000301", "\U0000FE0F", "\U0001F3FB", "\U0000200D", "\U00000903",
	"\U00000600", "\U0000200B", "\U000000AD", "\U0000FEFF",
	"\U00001100", "\U00001161", "\U000011A8", "\U0000AC00",
	"क", "\U0000094D", "ष", "e\U00000301",
}

func randomLine(r *rand.Rand) string {
	var b strings.Builder
	for n := r.IntN(10); n >= 0; n-- {
		b.WriteString(fragments[r.IntN(len(fragments))])
	}
	return b.String()
}

// visibleOnly drops the escapes and zero-width clusters of rendered output.
func visibleOnly(out string) string {
	s := ansi.Strip(out)
	var b strings.Builder
	for i := 0; i < len(s); {
		c, w := ansi.FirstGraphemeCluster(s[i:], ansi.GraphemeWidth)
		if w > 0 {
			b.WriteString(c)
		}
		i += len(c)
	}
	return b.String()
}

// window cuts columns [left, left+width) out of a fully rendered plain line
// the way Render is meant to: a wide cluster cut by an edge becomes spaces.
func window(full string, left, width int) string {
	var b strings.Builder
	right := left + width
	col := 0
	for i := 0; i < len(full); {
		c, w := ansi.FirstGraphemeCluster(full[i:], ansi.GraphemeWidth)
		i += len(c)
		switch {
		case c == zwsp, col >= right:
		case w == 0:
			if col >= left {
				b.WriteString(c)
			}
		case col >= left && col+w <= right:
			b.WriteString(c)
		case col+w > left:
			b.WriteString(strings.Repeat(" ", min(col+w, right)-max(col, left)))
		}
		col += w
	}
	if col < right {
		b.WriteString(strings.Repeat(" ", right-max(col, left)))
	}
	return b.String()
}

func TestRenderRandomLines(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	for range 3000 {
		text := randomLine(r)
		tab := 1 + r.IntN(8)
		w := Width(text, tab)

		full := checkRender(t, text, nil, nil, LineStyle{Width: max(w, 1), TabWidth: tab})
		padded := checkRender(t, text, nil, nil, LineStyle{Width: w + 5, TabWidth: tab})
		// A zero-width cluster exactly at the right edge is outside the
		// window, so compare what shows.
		if w > 0 && visibleOnly(padded) != visibleOnly(full)+"     " {
			t.Fatalf("%+q: Width %d does not hold the whole line:\n%+q\n%+q", text, w, ansi.Strip(full), ansi.Strip(padded))
		}

		ref := ansi.Strip(padded)
		for range 6 {
			x := r.IntN(w + 3)
			n := 1 + r.IntN(w+3)
			out := checkRender(t, text, nil, nil, LineStyle{Width: n, XOffset: x, TabWidth: tab})
			if got, want := plain(out), strings.ReplaceAll(window(ref, x, n), zwsp, ""); got != want {
				t.Fatalf("%+q at offset %d width %d:\n got %+q\nwant %+q", text, x, n, got, want)
			}
		}
	}
}

func TestRenderRandomStyles(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 5))
	colours := []string{"", "#ff0000", "#00ff00", "#0000ff", "#c0caf5", "nope"}
	for range 1000 {
		text := randomLine(r)
		var spans []Span
		for at := 0; at < len(text); {
			n := 1 + r.IntN(4)
			spans = append(spans, Span{
				Start: at, End: at + n, FG: colours[r.IntN(len(colours))],
				Bold: r.IntN(2) == 0, Italic: r.IntN(3) == 0, Underline: r.IntN(5) == 0,
			})
			at += n + r.IntN(2)
		}
		var marks []Mark
		for range r.IntN(4) {
			s := r.IntN(len(text) + 1)
			marks = append(marks, Mark{Start: s, End: s + r.IntN(6), BG: colours[r.IntN(len(colours))], Bold: r.IntN(2) == 0})
		}
		st := LineStyle{
			Width:    1 + r.IntN(30),
			XOffset:  r.IntN(8),
			TabWidth: r.IntN(9),
			FG:       colours[r.IntN(len(colours))],
			BG:       colours[r.IntN(len(colours))],
		}
		checkRender(t, text, spans, marks, st)
	}
}
