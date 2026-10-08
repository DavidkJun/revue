package highlight

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// testCell is one grapheme cluster of rendered output with the style the
// SGR sequences before it left in effect.
type testCell struct {
	text  string
	width int
	style cellStyle
}

// decode splits Render output the way ansi.StringWidth does (a cluster never
// spans an escape sequence) and replays the SGR state. It fails the test on
// anything but SGR sequences and printable UTF-8.
func decode(t *testing.T, out string) []testCell {
	t.Helper()
	var cells []testCell
	var cur cellStyle
	for i := 0; i < len(out); {
		if out[i] == 0x1b {
			j := i + 2
			if j > len(out) || out[i+1] != '[' {
				t.Fatalf("escape that is not CSI at byte %d of %q", i, out)
			}
			for j < len(out) && (out[j] >= '0' && out[j] <= '9' || out[j] == ';') {
				j++
			}
			if j >= len(out) || out[j] != 'm' {
				t.Fatalf("non-SGR sequence at byte %d of %q", i, out)
			}
			cur = applySGR(t, cur, out[i+2:j])
			i = j + 1
			continue
		}
		if out[i] < 0x20 || out[i] == 0x7f {
			t.Fatalf("raw control byte %#x at %d of %q", out[i], i, out)
		}
		c, w := ansi.FirstGraphemeCluster(out[i:], ansi.GraphemeWidth)
		cells = append(cells, testCell{text: c, width: w, style: cur})
		i += len(c)
	}
	return cells
}

func applySGR(t *testing.T, s cellStyle, params string) cellStyle {
	t.Helper()
	ps := strings.Split(params, ";")
	for k := 0; k < len(ps); k++ {
		switch ps[k] {
		case "0", "":
			s = cellStyle{}
		case "1":
			s.bold = true
		case "22":
			s.bold = false
		case "3":
			s.italic = true
		case "23":
			s.italic = false
		case "4":
			s.underline = true
		case "24":
			s.underline = false
		case "39":
			s.fg = ""
		case "49":
			s.bg = ""
		case "38", "48":
			if k+4 >= len(ps) || ps[k+1] != "2" {
				t.Fatalf("colour that is not 24-bit in %q", params)
			}
			var rgb [3]int
			for c := range rgb {
				v, err := strconv.Atoi(ps[k+2+c])
				if err != nil || v < 0 || v > 255 {
					t.Fatalf("bad colour component in %q", params)
				}
				rgb[c] = v
			}
			hex := fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2])
			if ps[k] == "38" {
				s.fg = hex
			} else {
				s.bg = hex
			}
			k += 4
		default:
			t.Fatalf("unexpected SGR parameter %q in %q", ps[k], params)
		}
	}
	return s
}

// plain is what the output shows: escapes and the invisible fences removed.
func plain(out string) string {
	return strings.ReplaceAll(ansi.Strip(out), zwsp, "")
}

// checkRender renders and verifies the invariants every line must keep.
func checkRender(t *testing.T, text string, spans []Span, marks []Mark, st LineStyle) string {
	t.Helper()
	out := Render(text, spans, marks, st)
	if st.Width <= 0 {
		if out != "" {
			t.Fatalf("Render(%q, width %d) = %q, want empty", text, st.Width, out)
		}
		return out
	}
	if got := ansi.StringWidth(out); got != st.Width {
		t.Fatalf("Render(%q, %+v): width %d, want %d; out %q", text, st, got, st.Width, out)
	}
	if got := ansi.StringWidth(ansi.Strip(out)); got != st.Width {
		t.Fatalf("Render(%q, %+v): stripped width %d, want %d; out %q", text, st, got, st.Width, out)
	}
	if !utf8.ValidString(out) {
		t.Fatalf("Render(%q, %+v) is not valid UTF-8: %q", text, st, out)
	}
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Fatalf("Render(%q, %+v) does not end with a reset: %q", text, st, out)
	}
	sum := 0
	for _, c := range decode(t, out) {
		sum += c.width
	}
	if sum != st.Width {
		t.Fatalf("Render(%q, %+v): cells add up to %d, want %d", text, st, sum, st.Width)
	}
	return out
}
