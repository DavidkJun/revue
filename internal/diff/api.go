package diff

// OldLines reconstructs the old file's lines (Context + Deleted, in order).
func (fd *FileDiff) OldLines() []string { return fd.side(Deleted) }

// NewLines reconstructs the new file's lines (Context + Added, in order).
func (fd *FileDiff) NewLines() []string { return fd.side(Added) }

func (fd *FileDiff) side(kind LineKind) []string {
	if fd == nil {
		return nil
	}
	var out []string
	for _, l := range fd.Lines {
		if l.Kind == Context || l.Kind == kind {
			out = append(out, l.Text)
		}
	}
	return out
}

// Stats counts added and deleted lines.
func (fd *FileDiff) Stats() (added, deleted int) {
	if fd == nil {
		return 0, 0
	}
	for _, l := range fd.Lines {
		switch l.Kind {
		case Added:
			added++
		case Deleted:
			deleted++
		}
	}
	return added, deleted
}

// Unified builds the unified-view rows. Within each change block, deleted
// lines are paired with the added lines that replaced them by similarity
// (order preserving); a pair is emitted as adjacent RowModOld, RowModNew rows
// with word-level Changed ranges. Unpaired deletions/additions stay
// RowDeleted/RowAdded and keep their relative order.
func Unified(fd *FileDiff) []Row { return unified(fd) }

// Split aligns unified rows side by side: context rows on both sides, a
// modified pair on one row, unpaired deletions/additions next to a filler.
func Split(rows []Row) []SplitRow { return split(rows) }

// WordDiff compares two versions of a line and returns the changed byte
// ranges in each plus a similarity in [0, 1] (1 = identical).
//
// Tokens are runs of letters/digits/underscores, runs of whitespace and
// single other characters. Similarity counts only non-whitespace bytes, so a
// whitespace-only change scores 1 while its ranges still show where it is.
func WordDiff(a, b string) (aChanged, bChanged []Range, similarity float64) { return wordDiff(a, b) }

// BlockStarts returns the indices where a run of change lines begins.
func BlockStarts(isChange []bool) []int {
	var out []int
	for i, c := range isChange {
		if c && (i == 0 || !isChange[i-1]) {
			out = append(out, i)
		}
	}
	return out
}
