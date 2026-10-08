package diffview

import (
	"sort"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/ui/keys"
)

// hscrollStep is how far h and l scroll sideways.
const hscrollStep = 4

func (m *Model) handle(a keys.Action, count int) tea.Cmd {
	if !m.active() {
		if a == keys.YankRef && m.hasFile {
			return m.yankRef()
		}
		return nil
	}
	m.fresh = false
	p := m.cur()
	n := max(count, 1)
	switch a {
	case keys.Down:
		p.cursor += n
		m.follow(p)
	case keys.Up:
		p.cursor -= n
		m.follow(p)
	case keys.Top, keys.Bottom:
		switch {
		case count > 0:
			m.gotoNewLine(count)
		case a == keys.Top:
			p.cursor = 0
			m.follow(p)
		default:
			p.cursor = len(p.folds.Items()) - 1
			m.follow(p)
		}
	case keys.HalfDown:
		m.scrollWithCursor(p, m.half(count))
	case keys.HalfUp:
		m.scrollWithCursor(p, -m.half(count))
	case keys.PageDown:
		m.page(p, n)
	case keys.PageUp:
		m.page(p, -n)
	case keys.ScrollDown:
		p.offset += n
		m.pull(p)
	case keys.ScrollUp:
		p.offset -= n
		m.pull(p)
	case keys.CenterCursor, keys.CursorTop, keys.CursorBottom:
		if count > 0 {
			m.gotoNewLine(count)
		}
		m.place(p, a)
	case keys.NextChange:
		m.jumpChange(p, n, true)
	case keys.PrevChange:
		m.jumpChange(p, n, false)
	case keys.ScrollLeft:
		m.xoff = max(m.xoff-hscrollStep*n, 0)
	case keys.ScrollRight:
		// Past the limit already (it shrinks when the view or pane gets
		// wider), a step right must not jump back left.
		m.xoff = max(min(m.xoff+hscrollStep*n, m.maxXOffset()), m.xoff)
	case keys.ScrollHome:
		m.xoff = 0
	case keys.FoldOpen:
		m.foldOpen(p)
	case keys.FoldClose:
		m.foldClose(p)
	case keys.FoldToggle:
		m.foldToggle(p)
	case keys.FoldOpenAll:
		m.remap(p, p.folds.OpenAll)
	case keys.FoldCloseAll:
		m.remap(p, p.folds.CloseAll)
	case keys.ContextMore:
		m.setContext(m.context + contextStep*n)
	case keys.ContextLess:
		m.setContext(m.context - contextStep*n)
	case keys.VisualStart:
		m.startVisual()
	case keys.VisualEnd:
		m.visual = false
	case keys.Yank:
		return m.yankSelection()
	case keys.YankLine:
		return m.yankLine()
	case keys.YankRef:
		return m.yankRef()
	case keys.SearchNext:
		return m.searchJump(n, true)
	case keys.SearchPrev:
		return m.searchJump(n, false)
	}
	return nil
}

// half is the ctrl-d/ctrl-u distance: half a page, or the count like Vim's
// 'scroll' option.
func (m *Model) half(count int) int {
	if count > 0 {
		return count
	}
	return max(m.bodyHeight()/2, 1)
}

// scrollWithCursor moves the window and the cursor by d lines; at either
// end of the file only the cursor keeps moving, as in Vim.
func (m *Model) scrollWithCursor(p *pane, d int) {
	p.offset += d
	p.cursor = clamp(p.cursor+d, 0, max(len(p.folds.Items())-1, 0))
	m.pull(p)
}

// page scrolls k pages, keeping two lines of the previous page like ctrl-f
// and ctrl-b; at the end of the file the cursor goes to the last line.
func (m *Model) page(p *pane, k int) {
	n, h := len(p.folds.Items()), m.bodyHeight()
	maxOff := max(n-h, 0)
	step := max(h-2, 1)
	switch {
	case k > 0 && p.offset >= maxOff:
		p.cursor = n - 1
		m.follow(p)
		return
	case k < 0 && p.offset == 0:
		p.cursor = 0
		m.follow(p)
		return
	}
	p.offset += k * step
	m.pull(p)
}

// place is zz, zt and zb.
func (m *Model) place(p *pane, a keys.Action) {
	h, so := m.bodyHeight(), m.so()
	switch a {
	case keys.CenterCursor:
		p.offset = p.cursor - (h-1)/2
	case keys.CursorTop:
		p.offset = p.cursor - so
	case keys.CursorBottom:
		p.offset = p.cursor - h + 1 + so
	}
	p.offset = clamp(p.offset, 0, max(len(p.folds.Items())-h, 0))
}

// gotoNewLine moves to new-file line n (clamped), opening its fold. A
// deleted file has only old lines, which then take its place.
func (m *Model) gotoNewLine(n int) {
	f := m.f
	var row int32
	switch {
	case len(f.newRow) > 0:
		row = f.newRow[clamp(n, 1, len(f.newRow))-1]
	case len(f.oldRow) > 0:
		row = f.oldRow[clamp(n, 1, len(f.oldRow))-1]
	}
	m.reveal(m.cur(), m.lineOfRow(m.view(), int(row)))
}

// jumpChange moves to the start of the count-th next (or previous) change
// block. It does not wrap.
func (m *Model) jumpChange(p *pane, count int, forward bool) {
	if len(p.starts) == 0 {
		return
	}
	at := func(k int) int { return p.folds.ItemOfLine(p.starts[k]) }
	cur := p.cursor
	for range count {
		var k int
		if forward {
			k = sort.Search(len(p.starts), func(k int) bool { return at(k) > cur })
		} else {
			k = sort.Search(len(p.starts), func(k int) bool { return at(k) >= cur }) - 1
		}
		if k < 0 || k >= len(p.starts) {
			break
		}
		cur = at(k)
	}
	p.cursor = cur
	m.follow(p)
	m.showBlock(p)
}

// showBlock scrolls down far enough to show the change block starting at
// the cursor, as much of it as fits with the cursor still in the window: a
// reviewer jumps to a change to read it.
func (m *Model) showBlock(p *pane) {
	h, so := m.bodyHeight(), m.so()
	items := p.folds.Items()
	end := p.cursor
	for end+1 < len(items) && items[end+1].Fold < 0 && m.isChangeLine(items[end+1].Line) {
		end++
	}
	if want := end + so - (h - 1); want > p.offset {
		p.offset = clamp(min(want, p.cursor-so), 0, max(len(items)-h, 0))
	}
}

func (m *Model) isChangeLine(l int) bool {
	if m.split {
		sr := m.f.split[l]
		return sr.Left != sr.Right
	}
	return m.f.rows[l].IsChange()
}

// maxXOffset is the scroll at which the widest line ends at the pane edge.
func (m *Model) maxXOffset() int {
	return max(m.f.widest()-m.codeWidth(), 0)
}
