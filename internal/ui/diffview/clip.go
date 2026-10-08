package diffview

import (
	"sort"

	"github.com/DavidkJun/revue/internal/highlight"
)

// maxCellsPerByte is the most cells one byte of a line takes in
// highlight.Render: a tab at the default tab width.
const maxCellsPerByte = 4

// visibleBytes bounds the bytes of text that highlight.Render can draw in w
// cells from cell xoff on: none before lo, none from hi on. It counts each
// byte at the fewest and at the most cells it can take (printable ASCII 1,
// a control 2, a tab 1 to maxCellsPerByte, any other byte 0 to 2), so the
// bounds hold for any text and are tight for code.
func visibleBytes(text string, xoff, w int) (lo, hi int) {
	left, right := xoff, xoff+w
	fewest, most := 0, 0
	for ; hi < len(text) && fewest < right; hi++ {
		switch c := text[hi]; {
		case c >= 0x20 && c < 0x7f:
			fewest, most = fewest+1, most+1
		case c == '\t':
			fewest, most = fewest+1, most+maxCellsPerByte
		case c < 0x20 || c == 0x7f:
			fewest, most = fewest+2, most+2
		default:
			most += 2
		}
		// No unit starting by here reaches the window.
		if most+maxCellsPerByte <= left {
			lo = hi + 1
		}
	}
	return lo, hi
}

// clipSpans returns the spans overlapping [lo, hi). Spans from
// highlight.File follow the text, so they are sorted and disjoint.
func clipSpans(spans []highlight.Span, lo, hi int) []highlight.Span {
	k := sort.Search(len(spans), func(k int) bool { return spans[k].End > lo })
	e := k + sort.Search(len(spans)-k, func(j int) bool { return spans[k+j].Start >= hi })
	return spans[k:e]
}

// clipMatches returns the matches overlapping [lo, hi) of one line side,
// which are sorted and do not overlap.
func clipMatches(ms []match, lo, hi int) []match {
	k := sort.Search(len(ms), func(k int) bool { return int(ms[k].end) > lo })
	e := k + sort.Search(len(ms)-k, func(j int) bool { return int(ms[k+j].start) >= hi })
	return ms[k:e]
}
