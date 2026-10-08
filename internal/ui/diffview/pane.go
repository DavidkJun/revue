package diffview

import "github.com/DavidkJun/revue/internal/diff"

// scrollOff is how many lines stay visible above and below the cursor, as
// with LazyVim's scrolloff.
const scrollOff = 4

// pane is the cursor, scroll and fold state of one view. Its lines are
// unified rows in the unified view and split rows in the split view; cursor
// and offset index the folded items.
type pane struct {
	folds  *diff.Folds
	starts []int // first line of every change block
	cursor int
	offset int // item on the first body line
}

func (m *Model) view() int {
	if m.split {
		return viewSplit
	}
	return viewUnified
}

func (m *Model) cur() *pane { return &m.panes[m.view()] }

func (m *Model) bodyHeight() int { return max(m.height-1, 0) }

// span is the range of lines an item shows: one line, or all a fold hides.
func span(it diff.Item) (start, end int) {
	if it.Fold >= 0 {
		return it.Start, it.End
	}
	return it.Line, it.Line + 1
}

// cursorLine is the line under the cursor, the first hidden one on a fold.
func (m *Model) cursorLine() int {
	p := m.cur()
	items := p.folds.Items()
	if len(items) == 0 {
		return 0
	}
	s, _ := span(items[clamp(p.cursor, 0, len(items)-1)])
	return s
}

// primary is the unified row that stands for line l of view v: in the split
// view the new side, or the old side next to a filler.
func (m *Model) primary(v, l int) int {
	if v == viewUnified {
		return l
	}
	sr := m.f.split[l]
	if sr.Right >= 0 {
		return sr.Right
	}
	return sr.Left
}

// rowRange returns the first and last unified row shown on line l of view v.
func (m *Model) rowRange(v, l int) (lo, hi int) {
	if v == viewUnified {
		return l, l
	}
	sr := m.f.split[l]
	lo, hi = sr.Left, sr.Right
	if lo < 0 {
		lo = hi
	}
	if hi < 0 {
		hi = lo
	}
	return min(lo, hi), max(lo, hi)
}

func (m *Model) lineOfRow(v, row int) int {
	if v == viewUnified {
		return row
	}
	return int(m.f.splitOf[row])
}

// lineNumbers returns the new-file line (nearest one for deleted rows) and
// the old-file line (0 if none) shown on line l of the current view.
func (m *Model) lineNumbers(l int) (newLine, oldLine int) {
	f := m.f
	if len(f.rows) == 0 {
		return 0, 0
	}
	v := m.view()
	row := m.primary(v, l)
	newLine = int(f.near[row])
	if v == viewSplit {
		if left := f.split[l].Left; left >= 0 {
			return newLine, f.rows[left].OldNo
		}
		return newLine, 0
	}
	return newLine, f.rows[row].OldNo
}

func clamp(x, lo, hi int) int {
	if x > hi {
		x = hi
	}
	if x < lo {
		x = lo
	}
	return x
}

func (m *Model) so() int { return clamp(scrollOff, 0, (m.bodyHeight()-1)/2) }

// follow scrolls so the cursor is inside the window with scrolloff lines
// around it, the way Vim scrolls after a cursor motion: by as little as
// needed, or to the middle when the cursor jumped more than half a page away.
func (m *Model) follow(p *pane) {
	n, h := len(p.folds.Items()), m.bodyHeight()
	p.cursor = clamp(p.cursor, 0, max(n-1, 0))
	if h == 0 {
		p.offset = p.cursor
		return
	}
	so := m.so()
	above := p.offset + so - p.cursor
	below := p.cursor + so - (p.offset + h - 1)
	switch {
	case above > h/2 || below > h/2:
		p.offset = p.cursor - (h-1)/2
	case above > 0:
		p.offset -= above
	case below > 0:
		p.offset += below
	}
	p.offset = clamp(p.offset, 0, max(n-h, 0))
}

// pull keeps the cursor inside the window after the window moved, as Vim
// does for ctrl-e and the mouse wheel.
func (m *Model) pull(p *pane) {
	n, h := len(p.folds.Items()), m.bodyHeight()
	p.offset = clamp(p.offset, 0, max(n-h, 0))
	if n == 0 || h == 0 {
		p.cursor = clamp(p.cursor, 0, max(n-1, 0))
		return
	}
	so := m.so()
	lo, hi := p.offset+so, p.offset+h-1-so
	if p.offset == 0 {
		lo = 0
	}
	if p.offset+h >= n {
		hi = n - 1
	}
	p.cursor = clamp(p.cursor, lo, max(min(hi, n-1), lo))
}

// remap runs a change of the fold state and puts the cursor back on the line
// it was on, at the same screen row.
func (m *Model) remap(p *pane, change func()) {
	line := m.cursorLine()
	row := p.cursor - p.offset
	change()
	p.cursor = p.folds.ItemOfLine(line)
	p.offset = p.cursor - row
	m.follow(p)
}

// reveal moves the cursor to line l, opening the fold that hides it.
func (m *Model) reveal(p *pane, l int) {
	if k := p.folds.FoldAt(l); k >= 0 && !p.folds.IsOpen(k) {
		p.folds.Open(k)
	}
	p.cursor = p.folds.ItemOfLine(l)
	m.follow(p)
}

// home puts the cursor on the first change, with some context above it when
// the change is not on the first page.
func (m *Model) home() {
	p := m.cur()
	p.cursor, p.offset = 0, 0
	m.fresh = true
	if len(p.starts) == 0 {
		m.follow(p)
		return
	}
	p.cursor = p.folds.ItemOfLine(p.starts[0])
	if h := m.bodyHeight(); p.cursor > h-1-m.so() {
		p.offset = p.cursor - h/3
	}
	m.follow(p)
	m.showBlock(p)
}
