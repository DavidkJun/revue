package diff

import "testing"

// fileDiff builds a FileDiff from lines prefixed like a unified diff:
// ' ' context, '-' deleted, '+' added. Line numbers are filled in.
func fileDiff(t testing.TB, lines ...string) *FileDiff {
	t.Helper()
	fd := &FileDiff{OldPath: "a.go", NewPath: "a.go"}
	oldNo, newNo := 0, 0
	for _, s := range lines {
		if s == "" {
			t.Fatalf("line without a prefix")
		}
		text := s[1:]
		switch s[0] {
		case ' ':
			oldNo++
			newNo++
			fd.Lines = append(fd.Lines, Line{Kind: Context, OldNo: oldNo, NewNo: newNo, Text: text})
		case '-':
			oldNo++
			fd.Lines = append(fd.Lines, Line{Kind: Deleted, OldNo: oldNo, Text: text})
		case '+':
			newNo++
			fd.Lines = append(fd.Lines, Line{Kind: Added, NewNo: newNo, Text: text})
		default:
			t.Fatalf("bad prefix in %q", s)
		}
	}
	return fd
}

var rowPrefix = map[RowKind]string{RowContext: " ", RowAdded: "+", RowDeleted: "-", RowModOld: "<", RowModNew: ">"}

// render shows rows in the fileDiff notation, with '<' and '>' for the old
// and new halves of a modified pair.
func render(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = rowPrefix[r.Kind] + r.Text
	}
	return out
}

// checkRows verifies the invariants every Unified result must satisfy,
// independent of how lines were paired.
func checkRows(t *testing.T, fd *FileDiff, rows []Row) {
	t.Helper()
	if len(rows) != len(fd.Lines) {
		t.Fatalf("got %d rows for %d lines", len(rows), len(fd.Lines))
	}
	runs := 0
	oldNo, newNo := 0, 0
	for i, r := range rows {
		switch r.Kind {
		case RowContext:
			if r.Pair != -1 || r.Block != -1 || r.Changed != nil {
				t.Errorf("row %d: context row %+v", i, r)
			}
		case RowAdded, RowDeleted:
			if r.Pair != -1 || r.Changed != nil {
				t.Errorf("row %d: unpaired row %+v", i, r)
			}
		case RowModOld:
			if r.Pair != i+1 || i+1 >= len(rows) || rows[i+1].Kind != RowModNew || rows[i+1].Pair != i {
				t.Errorf("row %d: ModOld not followed by its ModNew: %+v", i, r)
			}
		case RowModNew:
			if r.Pair != i-1 || i == 0 || rows[i-1].Kind != RowModOld {
				t.Errorf("row %d: ModNew not preceded by its ModOld: %+v", i, r)
			}
		}
		if r.IsChange() {
			if i == 0 || !rows[i-1].IsChange() {
				runs++
			}
			if r.Block != runs-1 {
				t.Errorf("row %d: block %d, want %d", i, r.Block, runs-1)
			}
		}
		hasOld := r.Kind == RowContext || r.Kind == RowDeleted || r.Kind == RowModOld
		hasNew := r.Kind == RowContext || r.Kind == RowAdded || r.Kind == RowModNew
		if hasOld != (r.OldNo > 0) || hasNew != (r.NewNo > 0) {
			t.Errorf("row %d: line numbers %d/%d do not fit kind %d", i, r.OldNo, r.NewNo, r.Kind)
		}
		if hasOld {
			oldNo++
			if r.OldNo != oldNo {
				t.Errorf("row %d: old line %d, want %d", i, r.OldNo, oldNo)
			}
		}
		if hasNew {
			newNo++
			if r.NewNo != newNo {
				t.Errorf("row %d: new line %d, want %d", i, r.NewNo, newNo)
			}
		}
		checkRanges(t, r.Text, r.Changed)
	}
}

// checkRanges verifies ranges are sorted, disjoint, non-touching, non-empty
// and inside s.
func checkRanges(t *testing.T, s string, rs []Range) {
	t.Helper()
	prev := -1
	for _, r := range rs {
		if r.Start < 0 || r.End > len(s) || r.Start >= r.End || r.Start <= prev {
			t.Errorf("bad range %v in %q (all %v)", r, s, rs)
		}
		prev = r.End
	}
}
