package diff

import (
	"slices"
	"testing"
)

func TestSplitFromUnified(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  []SplitRow
	}{
		{
			name:  "context only",
			lines: []string{" a", " b"},
			want:  []SplitRow{{0, 0}, {1, 1}},
		},
		{
			name:  "modified pair on one row",
			lines: []string{" a", "-x := 1", "+x := 2", " b"},
			want:  []SplitRow{{0, 0}, {1, 2}, {3, 3}},
		},
		{
			name:  "unpaired side by side, more deletions",
			lines: []string{"-alpha()", "-beta()", "+gamma := 1"},
			want:  []SplitRow{{0, 2}, {1, -1}},
		},
		{
			name:  "unpaired side by side, more additions",
			lines: []string{"-alpha()", "+one := 1", "+two := 2", "+three := 3"},
			want:  []SplitRow{{0, 1}, {-1, 2}, {-1, 3}},
		},
		{
			name: "unpaired lines between pairs",
			lines: []string{
				"-x := load(path)",
				"-y = transform(x)",
				"-return save(z)",
				"+x := load(cfg.Path)",
				"+cache.Reset()",
				"+metrics.Inc()",
				"+return save(z, opts)",
			},
			// rows: <x >x -y +cache +metrics <return >return
			want: []SplitRow{{0, 1}, {2, 3}, {-1, 4}, {5, 6}},
		},
		{
			name:  "pure delete",
			lines: []string{" a", "-b()", "-c()", " d"},
			want:  []SplitRow{{0, 0}, {1, -1}, {2, -1}, {3, 3}},
		},
		{
			name:  "pure insert",
			lines: []string{" a", "+b()", " c"},
			want:  []SplitRow{{0, 0}, {-1, 1}, {2, 2}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Split(Unified(fileDiff(t, tt.lines...))); !slices.Equal(got, tt.want) {
				t.Errorf("Split = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitHandBuiltRows(t *testing.T) {
	tests := []struct {
		name string
		rows []Row
		want []SplitRow
	}{
		{name: "empty", rows: nil, want: []SplitRow{}},
		{
			name: "ModOld without a partner acts as a deletion",
			rows: []Row{
				{Kind: RowModOld, Pair: -1, Block: 0},
				{Kind: RowAdded, Pair: -1, Block: 0},
				{Kind: RowAdded, Pair: -1, Block: 0},
			},
			want: []SplitRow{{0, 1}, {-1, 2}},
		},
		{
			name: "non-mutual pair is not a pair",
			rows: []Row{
				{Kind: RowModOld, Pair: 1, Block: 0},
				{Kind: RowModNew, Pair: 7, Block: 0},
				{Kind: RowDeleted, Pair: -1, Block: 0},
			},
			want: []SplitRow{{0, 1}, {2, -1}},
		},
		{
			name: "pair listed new-first",
			rows: []Row{
				{Kind: RowModNew, Pair: 1, Block: 0},
				{Kind: RowModOld, Pair: 0, Block: 0},
			},
			want: []SplitRow{{1, 0}},
		},
		{
			name: "adjacent blocks are not mixed",
			rows: []Row{
				{Kind: RowDeleted, Pair: -1, Block: 0},
				{Kind: RowAdded, Pair: -1, Block: 1},
			},
			want: []SplitRow{{0, -1}, {-1, 1}},
		},
		{
			name: "unknown kind is shown on both sides",
			rows: []Row{{Kind: RowKind(42), Pair: -1, Block: -1}},
			want: []SplitRow{{0, 0}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Split(tt.rows); !slices.Equal(got, tt.want) {
				t.Errorf("Split = %v, want %v", got, tt.want)
			}
		})
	}
}
