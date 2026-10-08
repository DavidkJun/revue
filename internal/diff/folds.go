package diff

import "sort"

// minFold is the smallest number of lines worth hiding: a marker that saves
// one or two lines only gets in the way.
const minFold = 4

// Folds hides runs of unchanged lines farther than Context lines from any
// change. It works on any sequence of lines (unified rows or split rows)
// given which of them are changes, so both views share it.
type Folds struct {
	isChange []bool
	context  int
	regions  []foldRegion

	// Prefix sums over the regions, kept current on every state change so
	// that ItemOfLine is a binary search.
	hidden []int // hidden[k]: lines hidden by closed regions before region k
	closed []int // closed[k]: closed regions before region k

	// Built lazily and only after a state change, because the UI asks for
	// it on every frame.
	items      []Item
	itemsValid bool
}

type foldRegion struct {
	start, end int // hidden line range [start, end)
	open       bool
}

// NewFolds computes the foldable regions for the given context size; all of
// them start closed. Runs shorter than a small minimum are never folded.
func NewFolds(isChange []bool, context int) *Folds {
	f := &Folds{isChange: append([]bool(nil), isChange...)}
	f.SetContext(context)
	return f
}

// Items returns the visible entries in order. The slice is shared until the
// fold state next changes; callers must not modify it.
func (f *Folds) Items() []Item {
	if !f.itemsValid {
		f.buildItems()
	}
	return f.items
}

// ItemOfLine returns the index in Items() of the entry showing line, which is
// the closed fold when the line is hidden. Lines out of range are clamped.
func (f *Folds) ItemOfLine(line int) int {
	if len(f.isChange) == 0 {
		return 0
	}
	line = min(max(line, 0), len(f.isChange)-1)
	k := sort.Search(len(f.regions), func(k int) bool { return f.regions[k].start > line })
	if k > 0 && line < f.regions[k-1].end && !f.regions[k-1].open {
		r := k - 1
		return f.regions[r].start - f.hidden[r] + f.closed[r]
	}
	return line - f.hidden[k] + f.closed[k]
}

// FoldAt returns the fold region index whose range contains line, or -1.
func (f *Folds) FoldAt(line int) int {
	k := sort.Search(len(f.regions), func(k int) bool { return f.regions[k].start > line })
	if k > 0 && line < f.regions[k-1].end {
		return k - 1
	}
	return -1
}

// Open opens fold region i.
func (f *Folds) Open(i int) { f.set(i, true) }

// Close closes fold region i.
func (f *Folds) Close(i int) { f.set(i, false) }

// IsOpen reports whether fold region i is open.
func (f *Folds) IsOpen(i int) bool {
	return i >= 0 && i < len(f.regions) && f.regions[i].open
}

// OpenAll opens every fold (the whole file is visible).
func (f *Folds) OpenAll() { f.setAll(true) }

// CloseAll closes every fold.
func (f *Folds) CloseAll() { f.setAll(false) }

// SetContext recomputes the regions for a new context size (all closed).
// Negative sizes count as 0.
func (f *Folds) SetContext(n int) {
	f.context = max(n, 0)
	f.regions = foldRegions(f.isChange, f.context)
	f.changed()
}

// Context returns the current context size.
func (f *Folds) Context() int { return f.context }

func (f *Folds) set(i int, open bool) {
	if i < 0 || i >= len(f.regions) || f.regions[i].open == open {
		return
	}
	f.regions[i].open = open
	f.changed()
}

func (f *Folds) setAll(open bool) {
	dirty := false
	for i := range f.regions {
		if f.regions[i].open != open {
			f.regions[i].open = open
			dirty = true
		}
	}
	if dirty {
		f.changed()
	}
}

// changed refreshes the prefix sums after a state change and marks the
// items stale.
func (f *Folds) changed() {
	f.hidden = make([]int, len(f.regions)+1)
	f.closed = make([]int, len(f.regions)+1)
	for k, r := range f.regions {
		f.hidden[k+1], f.closed[k+1] = f.hidden[k], f.closed[k]
		if !r.open {
			f.hidden[k+1] += r.end - r.start
			f.closed[k+1]++
		}
	}
	f.itemsValid = false
}

func (f *Folds) buildItems() {
	size := len(f.isChange)
	if nr := len(f.hidden) - 1; nr >= 0 {
		size += f.closed[nr] - f.hidden[nr]
	}
	items := make([]Item, 0, size)
	line := 0
	for k, r := range f.regions {
		if r.open {
			continue
		}
		for ; line < r.start; line++ {
			items = append(items, Item{Line: line, Fold: -1})
		}
		items = append(items, Item{Line: -1, Fold: k, Start: r.start, End: r.end})
		line = r.end
	}
	for ; line < len(f.isChange); line++ {
		items = append(items, Item{Line: line, Fold: -1})
	}
	f.items = items
	f.itemsValid = true
}

// foldRegions finds, for every unchanged run, the part farther than context
// lines from a change. A file without changes has nothing to fold around, so
// it is shown whole.
func foldRegions(isChange []bool, context int) []foldRegion {
	n := len(isChange)
	context = min(context, n)
	var regions []foldRegion
	changed := false
	for i := 0; i < n; {
		if isChange[i] {
			changed = true
			i++
			continue
		}
		start := i
		for i < n && !isChange[i] {
			i++
		}
		lo, hi := start+context, i-context
		if start == 0 {
			lo = 0
		}
		if i == n {
			hi = n
		}
		if hi-lo >= minFold {
			regions = append(regions, foldRegion{start: lo, end: hi})
		}
	}
	if !changed {
		return nil
	}
	return regions
}
