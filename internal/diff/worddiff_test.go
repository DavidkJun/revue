package diff

import (
	"math"
	"slices"
	"strings"
	"testing"
)

func TestWordDiff(t *testing.T) {
	tests := []struct {
		name   string
		a, b   string
		wantA  []Range
		wantB  []Range
		wantSi float64
	}{
		{name: "identical", a: "x := 1", b: "x := 1", wantSi: 1},
		{name: "both empty", a: "", b: "", wantSi: 1},
		{name: "from empty", a: "", b: "abc", wantB: []Range{{0, 3}}, wantSi: 0},
		{name: "to empty", a: "abc", b: "", wantA: []Range{{0, 3}}, wantSi: 0},
		{
			name: "one word", a: "const a = 1;", b: "const b = 1;",
			wantA: []Range{{6, 7}}, wantB: []Range{{6, 7}}, wantSi: 16.0 / 18,
		},
		{
			name: "operator", a: "x+y", b: "x-y",
			wantA: []Range{{1, 2}}, wantB: []Range{{1, 2}}, wantSi: 4.0 / 6,
		},
		{
			name: "insertion inside a call", a: "save(z)", b: "save(z, opts)",
			wantB: []Range{{6, 12}}, wantSi: 14.0 / 19,
		},
		{
			name: "changes across matched whitespace merge", a: "a x b", b: "a y c",
			wantA: []Range{{2, 5}}, wantB: []Range{{2, 5}}, wantSi: 2.0 / 6,
		},
		{
			name: "changes across punctuation stay apart", a: "f(a, b)", b: "f(x, y)",
			wantA: []Range{{2, 3}, {5, 6}}, wantB: []Range{{2, 3}, {5, 6}}, wantSi: 8.0 / 12,
		},
		{
			name: "phrase replaced", a: "foo bar baz", b: "foo qux quux baz",
			wantA: []Range{{4, 7}}, wantB: []Range{{4, 12}}, wantSi: 12.0 / 22,
		},
		{
			name: "inner whitespace only", a: "if x {", b: "if  x {",
			wantA: []Range{{2, 3}}, wantB: []Range{{2, 4}}, wantSi: 1,
		},
		{
			name: "indentation only", a: "\tfoo()", b: "    foo()",
			wantA: []Range{{0, 1}}, wantB: []Range{{0, 4}}, wantSi: 1,
		},
		{
			name: "trailing whitespace only", a: "x := 1", b: "x := 1  ",
			wantB: []Range{{6, 8}}, wantSi: 1,
		},
		{
			name: "whitespace-only lines", a: "", b: "   ",
			wantB: []Range{{0, 3}}, wantSi: 1,
		},
		{
			name: "threshold exactly one half", a: "a := 1", b: "b := 2",
			wantA: []Range{{0, 1}, {5, 6}}, wantB: []Range{{0, 1}, {5, 6}}, wantSi: 0.5,
		},
		{
			name: "cyrillic word replaced", a: "привіт світ", b: "привіт мир",
			wantA: []Range{{13, 21}}, wantB: []Range{{13, 19}}, wantSi: 24.0 / 38,
		},
		{
			// Words are atomic: one changed letter marks the whole word.
			name: "cyrillic letter changed", a: "let значення = 1", b: "let значенне = 1",
			wantA: []Range{{4, 20}}, wantB: []Range{{4, 20}}, wantSi: 10.0 / 42,
		},
		{
			name: "emoji", a: "ok 👍", b: "ok 👎",
			wantA: []Range{{3, 7}}, wantB: []Range{{3, 7}}, wantSi: 4.0 / 12,
		},
		{
			// The skin-tone modifier belongs to the emoji's grapheme, so the
			// range covers the whole visible character.
			name: "emoji modifier", a: "hi 👍🏽!", b: "hi 👍🏿!",
			wantA: []Range{{3, 11}}, wantB: []Range{{3, 11}}, wantSi: 6.0 / 22,
		},
		{
			name: "emoji zwj sequence", a: "👨‍👩‍👧", b: "👨‍👩‍👦",
			wantA: []Range{{0, 18}}, wantB: []Range{{0, 18}}, wantSi: 0,
		},
		{
			name: "combining accent stays in its word", a: "café ok", b: "cafe ok",
			wantA: []Range{{0, 6}}, wantB: []Range{{0, 4}}, wantSi: 4.0 / 14,
		},
		{
			// A standalone accent is written on a space or NBSP; the cluster
			// is a visible character, so removing it is not a whitespace edit.
			name: "mark on a space is not whitespace", a: "a ́", b: "a",
			wantA: []Range{{1, 4}}, wantSi: 2.0 / 5,
		},
		{
			name: "mark on a no-break space is not whitespace", a: " ́", b: " ",
			wantA: []Range{{0, 4}}, wantB: []Range{{0, 2}}, wantSi: 0,
		},
		{
			name: "invalid utf-8 does not panic", a: "a\xffb", b: "a\xfeb",
			wantA: []Range{{1, 2}}, wantB: []Range{{1, 2}}, wantSi: 4.0 / 6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotA, gotB, si := WordDiff(tt.a, tt.b)
			if !slices.Equal(gotA, tt.wantA) || !slices.Equal(gotB, tt.wantB) {
				t.Errorf("ranges %v / %v, want %v / %v", gotA, gotB, tt.wantA, tt.wantB)
			}
			if math.Abs(si-tt.wantSi) > 1e-9 {
				t.Errorf("similarity %v, want %v", si, tt.wantSi)
			}
			checkRanges(t, tt.a, gotA)
			checkRanges(t, tt.b, gotB)
		})
	}
}

func TestTokenize(t *testing.T) {
	s := "foo_1(bar,  привіт)👍🏽\t"
	var got []string
	for _, tok := range tokenize(s) {
		got = append(got, s[tok.start:tok.end])
	}
	want := []string{"foo_1", "(", "bar", ",", "  ", "привіт", ")", "👍🏽", "\t"}
	if !slices.Equal(got, want) {
		t.Errorf("tokens %q, want %q", got, want)
	}
}

// Past the LCS budget a line pair degrades to prefix/suffix comparison: the
// whole differing middle is one change instead of a slow exact diff.
func TestWordDiffFallsBackOnHugeLines(t *testing.T) {
	a := "A " + strings.Repeat("w ", 3000) + "Z"
	b := "B " + strings.Repeat("w ", 3000) + "Y"
	gotA, gotB, si := WordDiff(a, b)
	if !slices.Equal(gotA, []Range{{0, len(a)}}) || !slices.Equal(gotB, []Range{{0, len(b)}}) {
		t.Errorf("ranges %v / %v", gotA, gotB)
	}
	if si != 0 {
		t.Errorf("similarity %v, want 0", si)
	}

	// A local edit in an equally long line is still found exactly.
	c := strings.Repeat("w ", 3000) + "Z"
	d := strings.Repeat("w ", 1500) + "x " + strings.Repeat("w ", 1500) + "Z"
	_, gotD, si := WordDiff(c, d)
	if !slices.Equal(gotD, []Range{{3000, 3002}}) {
		t.Errorf("local edit ranges %v", gotD)
	}
	if si < 0.99 {
		t.Errorf("local edit similarity %v", si)
	}
}
