package diff

import (
	"fmt"
	"slices"
	"testing"
)

func TestUnifiedPairing(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{
			name:  "one-line edit",
			lines: []string{" a", "-x := 1", "+x := 2", " b"},
			want:  []string{" a", "<x := 1", ">x := 2", " b"},
		},
		{
			name:  "value edit inside one token",
			lines: []string{"-x := 1234", "+x := 1235", " timeout: 3000,", "-retries: 3,", "+retries: 5,"},
			want:  []string{"<x := 1234", ">x := 1235", " timeout: 3000,", "<retries: 3,", ">retries: 5,"},
		},
		{
			name:  "short unrelated lines stay red and green",
			lines: []string{"-const b = 2;", "-if (ok) {", "+let c = 3;", "+} else {"},
			want:  []string{"-const b = 2;", "-if (ok) {", "+let c = 3;", "+} else {"},
		},
		{
			name:  "renamed variable",
			lines: []string{"-count := len(items)", "-total += count", "+n := len(items)", "+total += n"},
			want:  []string{"<count := len(items)", ">n := len(items)", "<total += count", ">total += n"},
		},
		{
			name:  "pure insert",
			lines: []string{" a", "+b := 1", "+c := 2", " d"},
			want:  []string{" a", "+b := 1", "+c := 2", " d"},
		},
		{
			name:  "pure delete",
			lines: []string{" a", "-b := 1", "-c := 2", " d"},
			want:  []string{" a", "-b := 1", "-c := 2", " d"},
		},
		{
			name: "unrelated rewrite stays red and green",
			lines: []string{
				`-return fmt.Errorf("read: %w", err)`,
				`-}`,
				"+for _, item := range items {",
				"+\tsum += item.Size",
			},
			want: []string{
				`-return fmt.Errorf("read: %w", err)`,
				`-}`,
				"+for _, item := range items {",
				"+\tsum += item.Size",
			},
		},
		{
			name:  "moved line across context is never paired",
			lines: []string{" a", "-moved()", " b", " c", "+moved()", " d"},
			want:  []string{" a", "-moved()", " b", " c", "+moved()", " d"},
		},
		{
			// Order preserving: only one of two swapped lines can pair, and
			// equal totals prefer the earlier deletion.
			name:  "swapped lines in one block",
			lines: []string{"-import \"fmt\"", "-func main() {}", "+func main() {}", "+import \"fmt\""},
			want:  []string{"+func main() {}", "<import \"fmt\"", ">import \"fmt\"", "-func main() {}"},
		},
		{
			name: "several edits in one block",
			lines: []string{
				"-x := load(path)",
				"-y = transform(x)",
				"-return save(z)",
				"+x := load(cfg.Path)",
				"+cache.Reset()",
				"+metrics.Inc()",
				"+return save(z, opts)",
			},
			want: []string{
				"<x := load(path)",
				">x := load(cfg.Path)",
				"-y = transform(x)",
				"+cache.Reset()",
				"+metrics.Inc()",
				"<return save(z)",
				">return save(z, opts)",
			},
		},
		{
			name:  "interleaved input is collected per side",
			lines: []string{"-a := 1", "+a := 2", "-b := 1", "+b := 2"},
			want:  []string{"<a := 1", ">a := 2", "<b := 1", ">b := 2"},
		},
		{
			name:  "indent change",
			lines: []string{"-\tfoo(bar)", "+    foo(bar)"},
			want:  []string{"<\tfoo(bar)", ">    foo(bar)"},
		},
		{
			name:  "blank lines never anchor a pair",
			lines: []string{"-", "-x := 1", "+", "+x := 2"},
			want:  []string{"-", "+", "<x := 1", ">x := 2"},
		},
		{
			name:  "blank against blank",
			lines: []string{" a", "-", "+   ", " b"},
			want:  []string{" a", "-", "+   ", " b"},
		},
		{
			name: "more deletions than additions",
			lines: []string{
				"-a := compute(1)",
				"-debug(a)",
				"-b := compute(2)",
				"+a := compute(10)",
				"+b := compute(20)",
			},
			want: []string{
				"<a := compute(1)",
				">a := compute(10)",
				"-debug(a)",
				"<b := compute(2)",
				">b := compute(20)",
			},
		},
		{
			name: "more additions than deletions",
			lines: []string{
				"-a := compute(1)",
				"-b := compute(2)",
				"+a := compute(10)",
				"+log(a)",
				"+b := compute(20)",
				"+return a + b",
			},
			want: []string{
				"<a := compute(1)",
				">a := compute(10)",
				"+log(a)",
				"<b := compute(2)",
				">b := compute(20)",
				"+return a + b",
			},
		},
		{
			name:  "best match wins over position",
			lines: []string{"-total := price * qty", "+// compute the total", "+total := price * quantity"},
			want:  []string{"+// compute the total", "<total := price * qty", ">total := price * quantity"},
		},
		{
			name:  "two blocks",
			lines: []string{"-a := 1", "+a := 2", " mid", "-b := 1", "+c()"},
			want:  []string{"<a := 1", ">a := 2", " mid", "-b := 1", "+c()"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fd := fileDiff(t, tt.lines...)
			rows := Unified(fd)
			if got := render(rows); !slices.Equal(got, tt.want) {
				t.Errorf("rows\n got %q\nwant %q", got, tt.want)
			}
			checkRows(t, fd, rows)
		})
	}
}

func TestUnifiedRowFields(t *testing.T) {
	fd := fileDiff(t, " a", "-x := 1", "+x := 2", "+y()", " b", "-z")
	rows := Unified(fd)
	want := []Row{
		{Kind: RowContext, OldNo: 1, NewNo: 1, Text: "a", Pair: -1, Block: -1},
		{Kind: RowModOld, OldNo: 2, Text: "x := 1", Changed: []Range{{5, 6}}, Pair: 2, Block: 0},
		{Kind: RowModNew, NewNo: 2, Text: "x := 2", Changed: []Range{{5, 6}}, Pair: 1, Block: 0},
		{Kind: RowAdded, NewNo: 3, Text: "y()", Pair: -1, Block: 0},
		{Kind: RowContext, OldNo: 3, NewNo: 4, Text: "b", Pair: -1, Block: -1},
		{Kind: RowDeleted, OldNo: 4, Text: "z", Pair: -1, Block: 1},
	}
	if len(rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(want), rows)
	}
	for i := range want {
		if !rowsEqual(rows[i], want[i]) {
			t.Errorf("row %d\n got %+v\nwant %+v", i, rows[i], want[i])
		}
	}
}

func TestUnifiedIndentChangeRanges(t *testing.T) {
	rows := Unified(fileDiff(t, "-\tfoo(bar)", "+    foo(bar)"))
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	if !slices.Equal(rows[0].Changed, []Range{{0, 1}}) || !slices.Equal(rows[1].Changed, []Range{{0, 4}}) {
		t.Errorf("changed ranges %v / %v", rows[0].Changed, rows[1].Changed)
	}
}

func TestUnifiedEdgeCases(t *testing.T) {
	if rows := Unified(nil); len(rows) != 0 {
		t.Errorf("nil diff: %v", rows)
	}
	if rows := Unified(&FileDiff{Binary: true}); len(rows) != 0 {
		t.Errorf("binary diff: %v", rows)
	}
	// Unknown kinds degrade to context rather than failing.
	rows := Unified(&FileDiff{Lines: []Line{{Kind: LineKind(9), OldNo: 1, NewNo: 1, Text: "?"}}})
	if len(rows) != 1 || rows[0].Kind != RowContext {
		t.Errorf("unknown kind: %+v", rows)
	}
}

// Above maxDensePairs only a band of pairs is scored; with few deletions the
// band is wide enough to still find the replacement anywhere in the block.
func TestUnifiedSkewedLargeBlock(t *testing.T) {
	var adds []string
	for j := range 3000 {
		adds = append(adds, fmt.Sprintf("+item%d,", j))
	}
	adds[1700] = "+value := compute(43)"
	fd := fileDiff(t, append([]string{" a", "-value := compute(42)"}, append(adds, " b")...)...)
	rows := Unified(fd)
	checkRows(t, fd, rows)
	pairs := 0
	for i, r := range rows {
		if r.Kind == RowModOld {
			pairs++
			if r.Text != "value := compute(42)" || rows[i+1].Text != "value := compute(43)" {
				t.Errorf("paired %q with %q", r.Text, rows[i+1].Text)
			}
		}
	}
	if pairs != 1 {
		t.Errorf("%d pairs, want 1", pairs)
	}
}

func rowsEqual(a, b Row) bool {
	return a.Kind == b.Kind && a.OldNo == b.OldNo && a.NewNo == b.NewNo && a.Text == b.Text &&
		slices.Equal(a.Changed, b.Changed) && a.Pair == b.Pair && a.Block == b.Block
}

func TestFileDiffLines(t *testing.T) {
	fd := fileDiff(t, " a", "-b", "+c", "+d", " e", "-f")
	if got, want := fd.OldLines(), []string{"a", "b", "e", "f"}; !slices.Equal(got, want) {
		t.Errorf("OldLines %q, want %q", got, want)
	}
	if got, want := fd.NewLines(), []string{"a", "c", "d", "e"}; !slices.Equal(got, want) {
		t.Errorf("NewLines %q, want %q", got, want)
	}
	if added, deleted := fd.Stats(); added != 2 || deleted != 2 {
		t.Errorf("Stats %d/%d, want 2/2", added, deleted)
	}
	var none *FileDiff
	if none.OldLines() != nil || none.NewLines() != nil {
		t.Error("nil FileDiff has lines")
	}
	if a, d := none.Stats(); a != 0 || d != 0 {
		t.Error("nil FileDiff has stats")
	}
}

func TestBlockStarts(t *testing.T) {
	tests := []struct {
		pattern string
		want    []int
	}{
		{"", nil},
		{"....", nil},
		{"CC..", []int{0}},
		{".C.CC..C", []int{1, 3, 7}},
		{"CCCC", []int{0}},
	}
	for _, tt := range tests {
		if got := BlockStarts(changes(tt.pattern)); !slices.Equal(got, tt.want) {
			t.Errorf("BlockStarts(%q) = %v, want %v", tt.pattern, got, tt.want)
		}
	}
}
