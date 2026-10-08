package diff

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// changes turns a pattern like "..C.." into isChange flags ('C' = change).
func changes(pattern string) []bool {
	out := make([]bool, len(pattern))
	for i := range pattern {
		out[i] = pattern[i] == 'C'
	}
	return out
}

func regionSpans(f *Folds) [][2]int {
	var out [][2]int
	for _, r := range f.regions {
		out = append(out, [2]int{r.start, r.end})
	}
	return out
}

func TestFoldRegions(t *testing.T) {
	dots := func(n int) string { return strings.Repeat(".", n) }
	tests := []struct {
		name    string
		pattern string
		context int
		want    [][2]int
	}{
		{name: "empty", pattern: "", context: 3},
		{name: "no changes is never folded", pattern: dots(40), context: 3},
		{name: "all changes", pattern: "CCCC", context: 0},
		{name: "file start", pattern: dots(12) + "C", context: 3, want: [][2]int{{0, 9}}},
		{name: "file end", pattern: "C" + dots(12), context: 3, want: [][2]int{{4, 13}}},
		{name: "between changes", pattern: "C" + dots(10) + "C", context: 3, want: [][2]int{{4, 8}}},
		{name: "between changes, 3 hidden is too small", pattern: "C" + dots(9) + "C", context: 3},
		{name: "file start, minimum size", pattern: dots(7) + "C", context: 3, want: [][2]int{{0, 4}}},
		{name: "file start, too small", pattern: dots(6) + "C", context: 3},
		{name: "file end, too small", pattern: "C" + dots(6), context: 3},
		{name: "context 0", pattern: "....C.....", context: 0, want: [][2]int{{0, 4}, {5, 10}}},
		{name: "context 0, short runs", pattern: "..C...C..", context: 0},
		{name: "negative context counts as 0", pattern: "....C.....", context: -5, want: [][2]int{{0, 4}, {5, 10}}},
		{name: "huge context", pattern: "C" + dots(20) + "C", context: 1 << 62},
		{
			name:    "several runs",
			pattern: dots(10) + "C" + dots(2) + "CC" + dots(12) + "C" + dots(10),
			context: 2,
			want:    [][2]int{{0, 8}, {17, 25}, {30, 38}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewFolds(changes(tt.pattern), tt.context)
			if got := regionSpans(f); !slices.Equal(got, tt.want) {
				t.Errorf("regions %v, want %v", got, tt.want)
			}
			checkItems(t, f, len(tt.pattern))
		})
	}
}

func TestFoldsState(t *testing.T) {
	// Regions with context 2: [3, 9) between the changes and [14, 22) at the end.
	f := NewFolds(changes("C.........."+"C.........."), 2)
	if got, want := regionSpans(f), [][2]int{{3, 9}, {14, 22}}; !slices.Equal(got, want) {
		t.Fatalf("regions %v, want %v", got, want)
	}

	wantItems := []Item{
		{Line: 0, Fold: -1}, {Line: 1, Fold: -1}, {Line: 2, Fold: -1},
		{Line: -1, Fold: 0, Start: 3, End: 9},
		{Line: 9, Fold: -1}, {Line: 10, Fold: -1}, {Line: 11, Fold: -1}, {Line: 12, Fold: -1}, {Line: 13, Fold: -1},
		{Line: -1, Fold: 1, Start: 14, End: 22},
	}
	if got := f.Items(); !slices.Equal(got, wantItems) {
		t.Fatalf("items\n got %v\nwant %v", got, wantItems)
	}
	if h := f.Items()[3].Hidden(); h != 6 {
		t.Errorf("Hidden = %d, want 6", h)
	}

	itemOf := map[int]int{-5: 0, 0: 0, 2: 2, 3: 3, 5: 3, 8: 3, 9: 4, 13: 8, 14: 9, 21: 9, 100: 9}
	for line, want := range itemOf {
		if got := f.ItemOfLine(line); got != want {
			t.Errorf("ItemOfLine(%d) = %d, want %d", line, got, want)
		}
	}
	foldAt := map[int]int{-1: -1, 0: -1, 2: -1, 3: 0, 8: 0, 9: -1, 13: -1, 14: 1, 21: 1, 22: -1}
	for line, want := range foldAt {
		if got := f.FoldAt(line); got != want {
			t.Errorf("FoldAt(%d) = %d, want %d", line, got, want)
		}
	}

	f.Open(0)
	if !f.IsOpen(0) || f.IsOpen(1) {
		t.Errorf("IsOpen after Open(0): %v %v", f.IsOpen(0), f.IsOpen(1))
	}
	if n := len(f.Items()); n != 15 {
		t.Errorf("items after Open(0): %d, want 15", n)
	}
	if got := f.ItemOfLine(5); got != 5 {
		t.Errorf("ItemOfLine(5) in an open fold = %d, want 5", got)
	}
	if got := f.ItemOfLine(20); got != 14 {
		t.Errorf("ItemOfLine(20) = %d, want 14", got)
	}
	if got := f.FoldAt(5); got != 0 {
		t.Errorf("FoldAt(5) in an open fold = %d, want 0", got)
	}
	checkItems(t, f, 22)

	f.Close(0)
	if f.IsOpen(0) || len(f.Items()) != 10 {
		t.Errorf("Close(0): open=%v items=%d", f.IsOpen(0), len(f.Items()))
	}

	f.OpenAll()
	if n := len(f.Items()); n != 22 {
		t.Errorf("items after OpenAll: %d, want 22", n)
	}
	for i, it := range f.Items() {
		if it.Line != i || it.Fold != -1 {
			t.Fatalf("OpenAll item %d = %+v", i, it)
		}
	}
	f.CloseAll()
	if got := f.Items(); !slices.Equal(got, wantItems) {
		t.Errorf("items after CloseAll\n got %v\nwant %v", got, wantItems)
	}

	// Out-of-range indices are ignored.
	f.Open(-1)
	f.Open(2)
	f.Close(99)
	if f.IsOpen(-1) || f.IsOpen(2) {
		t.Error("out-of-range fold reported open")
	}

	f.Open(1)
	f.SetContext(4)
	if f.Context() != 4 {
		t.Errorf("Context = %d, want 4", f.Context())
	}
	if got, want := regionSpans(f), [][2]int{{16, 22}}; !slices.Equal(got, want) {
		t.Errorf("regions after SetContext(4) %v, want %v", got, want)
	}
	if f.IsOpen(0) {
		t.Error("SetContext must close every fold")
	}
	if n := len(f.Items()); n != 17 {
		t.Errorf("items after SetContext(4): %d, want 17", n)
	}
	checkItems(t, f, 22)

	f.SetContext(-3)
	if f.Context() != 0 {
		t.Errorf("Context after SetContext(-3) = %d, want 0", f.Context())
	}
}

func TestFoldsItemsCached(t *testing.T) {
	f := NewFolds(changes("C"+strings.Repeat(".", 30)), 3)
	a, b := f.Items(), f.Items()
	if &a[0] != &b[0] {
		t.Error("Items rebuilt without a state change")
	}
	f.Close(0) // already closed: no state change
	if c := f.Items(); &c[0] != &a[0] {
		t.Error("Items rebuilt after a no-op Close")
	}
	f.Open(0)
	if c := f.Items(); len(c) == len(a) {
		t.Error("Items not rebuilt after Open")
	}
}

func TestFoldsZeroAndCopy(t *testing.T) {
	var zero Folds
	if len(zero.Items()) != 0 || zero.ItemOfLine(3) != 0 || zero.FoldAt(0) != -1 || zero.IsOpen(0) {
		t.Error("zero Folds misbehaves")
	}
	zero.OpenAll()
	zero.SetContext(5)

	flags := changes("C" + strings.Repeat(".", 10))
	f := NewFolds(flags, 1)
	flags[5] = true
	f.SetContext(1)
	if got, want := regionSpans(f), [][2]int{{2, 11}}; !slices.Equal(got, want) {
		t.Errorf("Folds aliases the caller's slice: regions %v, want %v", got, want)
	}
}

// ItemOfLine is computed arithmetically; check it against the items list for
// many random layouts and fold states.
func TestFoldsItemOfLineMatchesItems(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for range 500 {
		n := rng.IntN(80)
		flags := make([]bool, n)
		for i := range flags {
			flags[i] = rng.IntN(12) == 0
		}
		f := NewFolds(flags, rng.IntN(6))
		for k := range f.regions {
			if rng.IntN(2) == 0 {
				f.Open(k)
			}
		}
		checkItems(t, f, n)
	}
}

// checkItems verifies Items covers every line once, in order, and that
// ItemOfLine and FoldAt agree with it.
func checkItems(t *testing.T, f *Folds, n int) {
	t.Helper()
	next := 0
	for idx, it := range f.Items() {
		if it.Fold < 0 {
			if it.Line != next {
				t.Fatalf("item %d shows line %d, want %d", idx, it.Line, next)
			}
			if f.ItemOfLine(next) != idx {
				t.Fatalf("ItemOfLine(%d) = %d, want %d", next, f.ItemOfLine(next), idx)
			}
			next++
			continue
		}
		if it.Line != -1 || it.Start != next || it.End <= it.Start || f.IsOpen(it.Fold) {
			t.Fatalf("bad fold item %d: %+v (next line %d)", idx, it, next)
		}
		for l := it.Start; l < it.End; l++ {
			if f.ItemOfLine(l) != idx || f.FoldAt(l) != it.Fold {
				t.Fatalf("hidden line %d: ItemOfLine %d FoldAt %d, want %d %d", l, f.ItemOfLine(l), f.FoldAt(l), idx, it.Fold)
			}
		}
		next = it.End
	}
	if next != n {
		t.Fatalf("items cover %d lines, want %d", next, n)
	}
}
