package sidebar

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

// maxScrolloff is Vim's 'scrolloff': rows kept visible around the cursor.
const maxScrolloff = 4

func (m *Model) handleAction(a keys.Action, count int) tea.Cmd {
	n := max(count, 1)
	switch a {
	case keys.Down:
		return m.moveTo(m.cursor + n)
	case keys.Up:
		return m.moveTo(m.cursor - n)
	case keys.Top:
		return m.moveTo(max(count-1, 0))
	case keys.Bottom:
		if count > 0 {
			return m.moveTo(count - 1)
		}
		return m.moveTo(len(m.rows) - 1)
	case keys.HalfDown:
		return m.page(m.halfPage(count))
	case keys.HalfUp:
		return m.page(-m.halfPage(count))
	case keys.PageDown:
		return m.page(m.fullPage() * n)
	case keys.PageUp:
		return m.page(-m.fullPage() * n)
	case keys.ScrollDown:
		return m.scrollLines(n)
	case keys.ScrollUp:
		return m.scrollLines(-n)
	case keys.CenterCursor:
		m.offset = m.cursor - m.bodyHeight()/2
		m.clampOffset()
	case keys.CursorTop:
		m.offset = m.cursor - m.scrolloff()
		m.clampOffset()
	case keys.CursorBottom:
		m.offset = m.cursor - m.bodyHeight() + 1 + m.scrolloff()
		m.clampOffset()
	case keys.Expand:
		return m.expand()
	case keys.Collapse:
		m.collapse()
	case keys.Mark:
		return m.mark()
	case keys.ClearMarks:
		if len(m.marks) == 0 {
			return nil
		}
		clear(m.marks)
		return notify("Marks cleared")
	}
	return nil
}

func (m *Model) current() *node {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor]
}

// moveTo puts the cursor on row i (clamped) and reports a newly reached file.
func (m *Model) moveTo(i int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	prev := m.current()
	m.cursor = clamp(i, 0, len(m.rows)-1)
	m.ensureVisible()
	return m.landed(prev)
}

// landed returns the FileSelected command when the cursor left prev for a file.
func (m *Model) landed(prev *node) tea.Cmd {
	n := m.current()
	if n == nil || n == prev || n.kind != fileNode {
		return nil
	}
	return fileSelected(n)
}

// page moves the view and the cursor together, like Vim's Ctrl-D/Ctrl-F: at
// the ends of the list the cursor keeps going on its own, and scrolloff is
// kept by moving the cursor rather than the view.
func (m *Model) page(d int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	prev := m.current()
	m.offset += d
	m.clampOffset()
	m.cursor = clamp(m.cursor+d, 0, len(m.rows)-1)
	m.cursorIntoView()
	return m.landed(prev)
}

// scrollLines moves the view only (Ctrl-E/Ctrl-Y), dragging the cursor
// along when it would leave the scrolloff zone.
func (m *Model) scrollLines(d int) tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	prev := m.current()
	m.offset += d
	m.clampOffset()
	m.cursorIntoView()
	return m.landed(prev)
}

func (m *Model) expand() tea.Cmd {
	n := m.current()
	switch {
	case n == nil:
		return nil
	case n.kind == fileNode:
		m.open = true
		return fileSelected(n)
	case !m.isExpanded(n):
		m.toggle(n)
		m.ensureVisible()
		return nil
	case len(n.children) > 0:
		return m.moveTo(m.cursor + 1)
	}
	return nil
}

func (m *Model) collapse() {
	n := m.current()
	switch {
	case n == nil:
	case n.kind != fileNode && m.isExpanded(n):
		m.toggle(n)
		m.ensureVisible()
	case n.parent != nil:
		m.cursor = m.rowOf[n.parent]
		m.ensureVisible()
	}
}

// mark toggles the file under the cursor, or every file below a repo or
// directory: all marked → unmark them, otherwise mark them all.
func (m *Model) mark() tea.Cmd {
	n := m.current()
	if n == nil {
		return nil
	}
	files := filesUnder(n)
	if len(files) == 0 {
		return nil
	}
	all := true
	for _, f := range files {
		all = all && m.marked(f)
	}
	for _, f := range files {
		m.setMarked(f, !all)
	}
	return notify(markedText(len(m.marks)))
}

func markedText(n int) string {
	switch n {
	case 0:
		return "No files marked for Claude"
	case 1:
		return "1 file marked for Claude"
	}
	return strconv.Itoa(n) + " files marked for Claude"
}

func (m *Model) selectNode(repo int, path string) tea.Cmd {
	if repo < 0 || repo >= len(m.roots) {
		return nil
	}
	target := m.roots[repo]
	if path != "" {
		target = m.byID[m.fileID(repo, path)]
		if target == nil {
			return nil
		}
	}
	reopened := false
	for p := target.parent; p != nil; p = p.parent {
		if !m.isExpanded(p) {
			m.setExpanded(p, true)
			reopened = true
		}
	}
	if reopened {
		m.flatten()
	}
	m.cursor = m.rowOf[target]
	m.ensureVisible()
	if target.kind == fileNode {
		return fileSelected(target)
	}
	return nil
}

func (m *Model) click(y int) tea.Cmd {
	if y < 1 || y > m.bodyHeight() {
		return nil
	}
	i := m.offset + y - 1
	if i >= len(m.rows) {
		return nil
	}
	// The clicked row is on screen already. Scrolling to keep scrolloff
	// would move it away from the pointer, and a second click would then
	// land on another row.
	n := m.rows[i]
	m.cursor = i
	if n.kind == fileNode {
		return fileSelected(n)
	}
	m.toggle(n)
	m.clampOffset()
	return nil
}

func (m *Model) bodyHeight() int { return max(m.height-1, 0) }

func (m *Model) scrolloff() int { return clamp((m.bodyHeight()-1)/2, 0, maxScrolloff) }

func (m *Model) halfPage(count int) int {
	if count > 0 {
		return count
	}
	return max(m.bodyHeight()/2, 1)
}

// fullPage keeps two rows of overlap, as Vim's Ctrl-F does.
func (m *Model) fullPage() int { return max(m.bodyHeight()-2, 1) }

func (m *Model) maxOffset() int { return max(len(m.rows)-m.bodyHeight(), 0) }

func (m *Model) clampOffset() { m.offset = clamp(m.offset, 0, m.maxOffset()) }

// ensureVisible scrolls so the cursor sits inside the scrolloff zone.
func (m *Model) ensureVisible() {
	m.cursor = clamp(m.cursor, 0, len(m.rows)-1)
	h := m.bodyHeight()
	if h == 0 {
		m.offset = 0
		return
	}
	so := m.scrolloff()
	if m.cursor-so < m.offset {
		m.offset = m.cursor - so
	}
	if m.cursor+so > m.offset+h-1 {
		m.offset = m.cursor + so - h + 1
	}
	m.clampOffset()
}

// cursorIntoView is ensureVisible the other way round: the view stays and
// the cursor moves.
func (m *Model) cursorIntoView() {
	h := m.bodyHeight()
	if h == 0 || len(m.rows) == 0 {
		return
	}
	so := m.scrolloff()
	lo, hi := m.offset+so, m.offset+h-1-so
	if m.offset == 0 {
		lo = 0
	}
	if m.offset >= m.maxOffset() {
		hi = len(m.rows) - 1
	}
	m.cursor = clamp(m.cursor, lo, min(hi, len(m.rows)-1))
}

func fileSelected(n *node) tea.Cmd {
	msg := uimsg.FileSelected{Repo: n.repo, Path: n.path}
	return func() tea.Msg { return msg }
}

func notify(text string) tea.Cmd {
	return func() tea.Msg { return uimsg.Notify{Text: text} }
}
