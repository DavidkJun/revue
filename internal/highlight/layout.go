package highlight

import (
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

const (
	defaultTabWidth = 4

	// zwsp is a grapheme-cluster Control of zero width. Writing it between two
	// pieces of output guarantees ansi.StringWidth measures them separately.
	zwsp = "\U0000200B"

	replacement = "\U0000FFFD"
)

// replacementWidth is 1, or 2 when RUNEWIDTH_EASTASIAN makes ambiguous
// characters wide; ansi decides that, so ask it.
var replacementWidth = ansi.StringWidth(replacement)

type unitKind uint8

const (
	unitText    unitKind = iota // the text's own bytes: ASCII or one grapheme cluster
	unitTab                     // width spaces up to the next tab stop
	unitCaret                   // '^' followed by glyph, vim's notation for a control character
	unitInvalid                 // U+FFFD standing in for one byte that is not UTF-8
)

// unit is one display element of a line, starting at a byte offset the
// caller tracks and ending at end.
//
// Tabs, carets and U+FFFD are stand-ins: output that is not the text's own
// bytes. A grapheme cluster next to a stand-in can merge with it when
// measured (a combining mark after a tab joins the last space; a Prepend
// character before a tab swallows the first space), which would change its
// width. fenceBefore and fenceAfter mark the clusters that need a zwsp on
// that side to stay a cluster of their own.
type unit struct {
	end         int
	width       int
	kind        unitKind
	glyph       byte
	fenceBefore bool
	fenceAfter  bool
}

func tabStop(tabWidth int) int {
	if tabWidth <= 0 {
		return defaultTabWidth
	}
	return tabWidth
}

func width(text string, tabWidth int) int {
	tabWidth = tabStop(tabWidth)
	col := 0
	afterStandIn := true
	for i := 0; i < len(text); {
		u := nextUnit(text, i, col, tabWidth, afterStandIn)
		col += u.width
		afterStandIn = u.kind != unitText
		i = u.end
	}
	return col
}

// nextUnit lays out the unit starting at byte i of s, which begins at
// column col. afterStandIn reports whether the previous unit was a stand-in
// (or i is the start of the line).
func nextUnit(s string, i, col, tabWidth int, afterStandIn bool) unit {
	b := s[i]
	switch {
	case b >= 0x20 && b < 0x7f:
		// Printable ASCII is a cluster of its own unless a combining mark or
		// similar follows, which only a non-ASCII byte can start.
		if i+1 == len(s) || s[i+1] < utf8.RuneSelf {
			return unit{end: i + 1, width: 1}
		}
	case b == '\t':
		return unit{end: i + 1, width: tabWidth - col%tabWidth, kind: unitTab}
	case b < 0x20:
		return unit{end: i + 1, width: 2, kind: unitCaret, glyph: b + 0x40}
	case b == 0x7f:
		return unit{end: i + 1, width: 2, kind: unitCaret, glyph: '?'}
	default:
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			return unit{end: i + 1, width: replacementWidth, kind: unitInvalid}
		}
		if r < 0xa0 {
			// C1 controls, shown with the letters of their C0 counterparts.
			return unit{end: i + n, width: 2, kind: unitCaret, glyph: byte(r - 0x40)}
		}
	}
	return clusterUnit(s, i, afterStandIn)
}

// clusterUnit measures the grapheme cluster at byte i with the segmenter and
// widths ansi.StringWidth uses, so the painted line measures exactly as laid
// out.
func clusterUnit(s string, i int, afterStandIn bool) unit {
	c, w := ansi.FirstGraphemeCluster(s[i:], ansi.GraphemeWidth)
	if !utf8.ValidString(c) {
		// The segmenter lets a Prepend character absorb invalid bytes and
		// keeps an incomplete sequence at the end of the text attached to the
		// cluster before it. Those bytes become U+FFFD units of their own.
		c = validPrefix(c)
		w = ansi.StringWidth(c)
	}
	u := unit{end: i + len(c), width: w}
	if afterStandIn {
		if r, _ := utf8.DecodeRuneInString(c); r >= utf8.RuneSelf && joins(' ', r) {
			u.fenceBefore = true
		}
	}
	if r, _ := utf8.DecodeLastRuneInString(c); r >= utf8.RuneSelf && standInAt(s, u.end) && joins(r, ' ') {
		u.fenceAfter = true
	}
	return u
}

// standInAt reports whether the output following byte j of s is not the
// text's own bytes: a stand-in unit, or the padding after the line.
func standInAt(s string, j int) bool {
	if j >= len(s) {
		return true
	}
	b := s[j]
	if b < 0x20 || b == 0x7f {
		return true
	}
	if b < utf8.RuneSelf {
		return false
	}
	r, n := utf8.DecodeRuneInString(s[j:])
	return (r == utf8.RuneError && n == 1) || r < 0xa0
}

// joins reports whether a followed by b forms a single grapheme cluster.
// Every stand-in is ASCII or U+FFFD, and against those only the pairwise
// rules (Extend, ZWJ, SpacingMark after anything; anything after Prepend)
// can join, so testing with a space is exact.
func joins(a, b rune) bool {
	var buf [2 * utf8.UTFMax]byte
	n := utf8.EncodeRune(buf[:], a)
	m := utf8.EncodeRune(buf[n:], b)
	c, _ := ansi.FirstGraphemeCluster(buf[:n+m], ansi.GraphemeWidth)
	return len(c) > n
}

// validPrefix returns the longest prefix of s that is valid UTF-8.
func validPrefix(s string) string {
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 {
			return s[:i]
		}
		i += n
	}
	return s
}
