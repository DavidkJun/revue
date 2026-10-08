// Package diff turns a whole-file diff into what the viewer shows: unified and
// split rows, pairing of modified lines with word-level change ranges, and
// folding of long unchanged runs.
package diff

// LineKind says which side(s) of the diff a line belongs to.
type LineKind uint8

const (
	Context LineKind = iota
	Added
	Deleted
)

// Line is one line of a full-context file diff. OldNo and NewNo are 1-based
// line numbers; 0 means the line does not exist on that side.
type Line struct {
	Kind  LineKind
	OldNo int
	NewNo int
	Text  string // no +/-/space prefix, no trailing "\n" or "\r"
}

// FileDiff is the whole-file diff of one file: Lines holds every line of the
// old and new versions in file order, unchanged lines once as Context.
type FileDiff struct {
	OldPath  string // "" when the file did not exist before (added/untracked)
	NewPath  string // "" when the file was deleted
	Binary   bool   // Lines is empty
	TooLarge bool   // the file exceeded the size limit; Lines is empty
	Lines    []Line
}

// RowKind is how a unified-view row is coloured.
type RowKind uint8

const (
	RowContext RowKind = iota
	RowAdded           // only in the new file (green)
	RowDeleted         // only in the old file (red)
	RowModOld          // old version of a modified line (yellow, dimmer)
	RowModNew          // new version of a modified line (yellow)
)

// Range is a half-open byte range [Start, End) within a row's Text.
type Range struct{ Start, End int }

// Row is one line of the unified view.
type Row struct {
	Kind    RowKind
	OldNo   int // 0 if the row has no old-file line
	NewNo   int // 0 if the row has no new-file line
	Text    string
	Changed []Range // word-level changes; only set on RowModOld / RowModNew
	Pair    int     // index of the other half of a modified pair in the same []Row, -1 otherwise
	Block   int     // index of the change block (0, 1, ...) the row belongs to, -1 for context
}

// IsChange reports whether the row is part of a change block.
func (r Row) IsChange() bool { return r.Kind != RowContext }

// SplitRow is one aligned row of the side-by-side view. Left (old side) and
// Right (new side) index into the []Row the split was built from; -1 is a
// filler cell. A context row has Left == Right.
type SplitRow struct{ Left, Right int }

// Item is one visible entry of a folded view: a line (Fold == -1, Line is its
// index) or a closed fold (Fold >= 0, Line == -1) hiding lines [Start, End).
type Item struct {
	Line  int
	Fold  int
	Start int
	End   int
}

// Hidden is the number of lines a closed fold hides.
func (it Item) Hidden() int { return it.End - it.Start }
