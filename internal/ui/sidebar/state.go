package sidebar

import (
	"slices"
	"strconv"

	"github.com/DavidkJun/revue/internal/git"
)

// Node identities survive SetRepos: a repo is known by its path and name
// (not its index), a node by its repo, kind and repo-relative path. They key
// the cursor, expansion state and marks.

func repoKey(r Repo) string { return r.Repo.Path + "\x00" + r.Repo.Name }

func (m *Model) id(n *node) string { return m.nodeID(n.repo, n.kind, n.path) }

func (m *Model) fileID(repo int, path string) string { return m.nodeID(repo, fileNode, path) }

func (m *Model) nodeID(repo int, kind nodeKind, path string) string {
	return m.keys[repo] + "\x00" + strconv.Itoa(int(kind)) + path
}

// rowRef remembers a visible row across a rebuild.
type rowRef struct {
	repo string
	id   string
}

func (m *Model) setRepos(repos []Repo) {
	old := make([]rowRef, len(m.rows))
	for i, n := range m.rows {
		old[i] = rowRef{repo: m.keys[n.repo], id: n.id}
	}
	oldCursor, oldOffset := m.cursor, m.offset
	h := m.bodyHeight()
	cursorShown := h > 0 && oldCursor >= oldOffset && oldCursor < oldOffset+h

	m.loaded = true
	m.repos = make([]Repo, len(repos))
	m.keys = make([]string, len(repos))
	m.roots = make([]*node, len(repos))
	m.byID = make(map[string]*node)
	m.files = m.files[:0]
	seen := make(map[string]int)
	for i, r := range repos {
		// The tree points into the files, so it gets its own copy.
		r.Status.Files = slices.Clone(r.Status.Files)
		m.repos[i] = r
		key := repoKey(r)
		if n := seen[key]; n > 0 {
			key += "\x00" + strconv.Itoa(n)
		}
		seen[repoKey(r)]++
		m.keys[i] = key
		m.roots[i] = buildRepo(i, m.repos[i].Status.Files)
		walk(m.roots[i], func(n *node) {
			n.id = m.id(n)
			m.byID[n.id] = n
			for _, p := range n.merged {
				alias := m.nodeID(n.repo, dirNode, p)
				n.aliases = append(n.aliases, alias)
				m.byID[alias] = n
			}
			if n.kind == fileNode {
				m.files = append(m.files, n)
			}
		})
	}
	m.carryRenamedMarks()
	for id := range m.marks {
		if n := m.byID[id]; n == nil || n.kind != fileNode {
			delete(m.marks, id)
		}
	}
	m.flatten()

	m.cursor = m.restoreRow(old, oldCursor)
	if !cursorShown {
		// The wheel moved the view away from the cursor; a refresh must not
		// pull it back, so the row at the top stays at the top.
		m.offset = m.restoreRow(old, oldOffset)
		m.clampOffset()
		return
	}
	// Rows appearing above the cursor should not make the view jump.
	m.offset = oldOffset + m.cursor - oldCursor
	m.ensureVisible()
}

// restoreRow finds where old row at is after a rebuild: the same node, else
// its nearest visible ancestor, else the closest surviving row after it in
// the same repo, else the closest one before it.
func (m *Model) restoreRow(old []rowRef, at int) int {
	if len(m.rows) == 0 || len(old) == 0 {
		return 0
	}
	at = clamp(at, 0, len(old)-1)
	cur := old[at]
	for n := m.byID[cur.id]; n != nil; n = n.parent {
		if i, ok := m.rowOf[n]; ok {
			return i
		}
	}
	visible := func(id string) (int, bool) {
		n := m.byID[id]
		if n == nil {
			return 0, false
		}
		i, ok := m.rowOf[n]
		return i, ok
	}
	for i := at + 1; i < len(old) && old[i].repo == cur.repo; i++ {
		if j, ok := visible(old[i].id); ok {
			return j
		}
	}
	for i := at - 1; i >= 0; i-- {
		if j, ok := visible(old[i].id); ok {
			return j
		}
	}
	return clamp(at, 0, len(m.rows)-1)
}

// flatten lists the visible rows: every repo, and the children of every
// expanded node.
func (m *Model) flatten() {
	m.rows = m.rows[:0]
	m.rowOf = make(map[*node]int, len(m.rowOf))
	var add func(n *node)
	add = func(n *node) {
		m.rowOf[n] = len(m.rows)
		m.rows = append(m.rows, n)
		if n.kind != fileNode && m.isExpanded(n) {
			for _, c := range n.children {
				add(c)
			}
		}
	}
	for _, r := range m.roots {
		add(r)
	}
}

// carryRenamedMarks moves the mark of a file that was renamed (git mv) to
// its new name, so it stays in the Claude context.
func (m *Model) carryRenamedMarks() {
	for _, n := range m.files {
		if n.file.Kind != git.Renamed || n.file.OldPath == "" {
			continue
		}
		old := m.fileID(n.repo, n.file.OldPath)
		if m.marks[old] && m.byID[old] == nil {
			m.marks[n.id] = true
		}
	}
}

// isExpanded reports the node's expansion: what the user chose (on this
// node or on a directory since merged into it), else open for directories
// and for repos that have changes.
func (m *Model) isExpanded(n *node) bool {
	if v, ok := m.expanded[n.id]; ok {
		return v
	}
	for _, a := range n.aliases {
		if v, ok := m.expanded[a]; ok {
			return v
		}
	}
	return m.defaultExpanded(n)
}

func (m *Model) defaultExpanded(n *node) bool {
	return n.kind == dirNode || len(n.children) > 0
}

// setExpanded records a choice. Choices equal to the default are forgotten,
// so a repo collapsed and reopened by hand still closes once it goes clean.
func (m *Model) setExpanded(n *node, open bool) {
	if m.expanded == nil {
		m.expanded = make(map[string]bool)
	}
	for _, a := range n.aliases {
		delete(m.expanded, a)
	}
	if open == m.defaultExpanded(n) {
		delete(m.expanded, n.id)
		return
	}
	m.expanded[n.id] = open
}

// toggle flips a repo or directory and keeps the cursor on it. Scrolling is
// left to the caller: a click must not move the row under the pointer.
func (m *Model) toggle(n *node) {
	m.setExpanded(n, !m.isExpanded(n))
	m.flatten()
	m.cursor = m.rowOf[n]
}

func (m *Model) marked(n *node) bool { return m.marks[n.id] }

func (m *Model) setMarked(n *node, on bool) {
	if m.marks == nil {
		m.marks = make(map[string]bool)
	}
	if on {
		m.marks[n.id] = true
	} else {
		delete(m.marks, n.id)
	}
}

func (m *Model) refs(nodes []*node, keep func(*node) bool) []FileRef {
	var out []FileRef
	for _, n := range nodes {
		if keep == nil || keep(n) {
			out = append(out, FileRef{Repo: n.repo, Path: n.path})
		}
	}
	return out
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
