// Package sidebar is the file tree: repos → directories → changed files, with
// status letters, icons, line counts and marks for Claude context.
package sidebar

import (
	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// CONTRACT — the implementing agent replaces the stub bodies (and may split
// them into other files of this package) but must keep every exported name and
// signature. Adding exported helpers or struct fields is fine.

// Repo is one repo's data as the sidebar shows it.
type Repo struct {
	Repo   git.Repo
	Status git.Status
	Err    error // last status error, shown on the repo node
	Loaded bool  // false until the first status arrived
}

// FileRef names a changed file.
type FileRef struct {
	Repo int    // index into the repos passed to SetRepos
	Path string // repo-relative, forward slashes
}

// Model is the sidebar. It is used through a pointer and mutated in place.
type Model struct {
	th           theme.Theme
	addFG, delFG string // line counts: green and red toned down toward Muted

	width, height int

	loaded bool // SetRepos has been called
	repos  []Repo
	keys   []string // identity of each repo, stable across SetRepos
	roots  []*node  // one tree per repo
	rows   []*node  // visible rows in display order
	rowOf  map[*node]int
	files  []*node // every file node in display order
	byID   map[string]*node

	cursor int // index into rows
	offset int // first visible row

	expanded map[string]bool // expansion choices that differ from the default, by node id
	marks    map[string]bool // marked files, by node id
	open     bool
}

// New creates an empty sidebar.
func New(th theme.Theme) *Model {
	return &Model{
		th:       th,
		addFG:    dim(th.Green, th.Muted, 0.45),
		delFG:    dim(th.Red, th.Muted, 0.45),
		expanded: map[string]bool{},
		marks:    map[string]bool{},
	}
}

// SetSize sets the size in cells, header line included.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.ensureVisible()
}

// SetRepos replaces the data. The cursor stays on the same node (or the
// nearest one if it disappeared), expansion state is kept, marks on files
// that are no longer changed are dropped. Repos with changes start expanded.
func (m *Model) SetRepos(repos []Repo) { m.setRepos(repos) }

// HandleAction applies a movement (keys.Nav) or sidebar (keys.Sidebar)
// action. When the cursor lands on a file it returns a command producing
// uimsg.FileSelected; Expand on a file additionally reports open=true via
// OpenRequested so the app can focus the diff pane.
func (m *Model) HandleAction(a keys.Action, count int) tea.Cmd { return m.handleAction(a, count) }

// OpenRequested reports (and clears) whether the last action asked to open
// the file under the cursor in the diff pane.
func (m *Model) OpenRequested() bool {
	open := m.open
	m.open = false
	return open
}

// View renders exactly width x height cells; the first line is the header.
func (m *Model) View(focused bool) string { return m.view(focused) }

// Current returns the repo under the cursor and the file, or nil on a repo
// or directory node. ok is false when there are no repos.
func (m *Model) Current() (repo int, file *git.FileChange, ok bool) {
	n := m.current()
	if len(m.repos) == 0 || n == nil {
		return 0, nil, false
	}
	return n.repo, n.file, true
}

// CurrentDir tells repo and directory nodes apart, which Current cannot: ok
// is true when the cursor is on one of them, and dir is the directory's
// repo-relative path ("" for the repo node). For a compacted chain
// ("src/app/core") dir is the deepest directory, which holds all its files.
func (m *Model) CurrentDir() (repo int, dir string, ok bool) {
	n := m.current()
	if len(m.repos) == 0 || n == nil || n.kind == fileNode {
		return 0, "", false
	}
	return n.repo, n.path, true
}

// Select moves the cursor to the file (expanding its parents), or to the repo
// node when path is "". It returns a command producing uimsg.FileSelected
// when it lands on a file.
func (m *Model) Select(repo int, path string) tea.Cmd { return m.selectNode(repo, path) }

// Files lists every changed file in display order, across repos.
func (m *Model) Files() []FileRef { return m.refs(m.files, nil) }

// Marked lists marked files in display order.
func (m *Model) Marked() []FileRef { return m.refs(m.files, m.marked) }

// ClearMarks removes every mark.
func (m *Model) ClearMarks() { clear(m.marks) }

// Click handles a mouse click on row y of the pane (0 = header).
func (m *Model) Click(y int) tea.Cmd { return m.click(y) }

// Scroll moves the viewport by n rows (mouse wheel).
func (m *Model) Scroll(n int) {
	m.offset += n
	m.clampOffset()
}
