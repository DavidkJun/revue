// Package diffview is the diff pane: unified and split views of one file
// with syntax colours, folds, a vim-style cursor, visual selection and search.
package diffview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/highlight"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// CONTRACT — the implementing agent replaces the stub bodies (and may split
// them into other files of this package) but must keep every exported name and
// signature. Adding exported helpers or struct fields is fine.

// Location is where the cursor is.
type Location struct {
	Repo    string // repo display name
	Path    string // repo-relative, forward slashes
	NewLine int    // new-file line under the cursor; for a deleted line, the nearest new-file line
	OldLine int    // old-file line under the cursor, 0 if none
}

// Selection is the visual selection.
type Selection struct {
	Repo      string
	Path      string
	StartLine int    // first new-file line covered (nearest one for deleted-only selections)
	EndLine   int    // last new-file line covered
	Text      string // selected lines like a unified diff: "-" old, "+" new, " " context, one per line
	Lines     int    // number of selected rows
}

// Model is the diff pane. It is used through a pointer and mutated in place.
type Model struct {
	th  theme.Theme
	hl  *highlight.Highlighter
	pal palette

	width, height int
	context       int

	repo, path string
	hasFile    bool   // a file is selected, so the header names it
	message    string // shown instead of the diff when not ""
	gen        int    // bumped on every content change; stale highlights carry an old one

	// f is the file's diff. SetMessage keeps it so that a keepPosition reload
	// of the same file right after a message can still restore the position.
	f     *file
	panes [2]pane // indexed by viewUnified / viewSplit
	split bool
	xoff  int  // horizontal scroll in cells, shared by both views
	fresh bool // the position is the initial one, so a resize may redo it

	visual bool
	anchor [2]int // line range [start, end) of the visual anchor in the current view

	search search

	spanBuf []highlight.Span // scratch for legibleSpans
}

// New creates an empty pane. context is the initial number of unchanged lines
// shown around changes.
func New(th theme.Theme, hl *highlight.Highlighter, context int) *Model {
	return &Model{th: th, hl: hl, pal: newPalette(th), context: max(context, 0)}
}

// SetSize sets the pane size in cells, header line included.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	if !m.active() {
		return
	}
	if m.fresh {
		m.home()
		return
	}
	m.follow(m.cur())
}

// SetFile shows fd, the diff of path in repo. With keepPosition (used when the
// same file is reloaded after an edit) the cursor stays on the same new-file
// line, open folds stay open where possible and visual mode is kept. The
// returned command highlights the file in the background.
func (m *Model) SetFile(repo, path string, fd *diff.FileDiff, keepPosition bool) tea.Cmd {
	return m.setFile(repo, path, fd, keepPosition)
}

// SetMessage replaces the content with a centred message (no file, clean
// repo, loading, error).
func (m *Model) SetMessage(msg string) {
	m.gen++
	m.hasFile = false
	m.message = msg
	m.visual = false
}

// Update handles the pane's own messages (background highlight results).
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	if hm, ok := msg.(highlightMsg); ok {
		m.applyHighlight(hm)
	}
	return nil
}

// HandleAction applies a movement (keys.Nav), diff (keys.Diff) or visual
// (keys.Visual) action. count is the typed count, 0 when none. Yank actions
// return a command producing uimsg.Yank.
func (m *Model) HandleAction(a keys.Action, count int) tea.Cmd { return m.handle(a, count) }

// View renders exactly width x height cells; the first line is the header
// (file path, status, counts). focused changes the header accent.
func (m *Model) View(focused bool) string { return m.render(focused) }

// Split reports whether the side-by-side view is active.
func (m *Model) Split() bool { return m.split }

// SetSplit switches between unified and split view, keeping the cursor on the
// same line.
func (m *Model) SetSplit(on bool) { m.setSplit(on) }

// Context returns the current number of context lines.
func (m *Model) Context() int { return m.context }

// Visual reports whether a visual selection is active.
func (m *Model) Visual() bool { return m.visual && m.active() }

// Selection returns the visual selection, if any.
func (m *Model) Selection() (Selection, bool) { return m.selection() }

// Cursor returns the cursor location; false when no file is shown.
func (m *Model) Cursor() (Location, bool) {
	if !m.hasFile {
		return Location{}, false
	}
	loc := Location{Repo: m.repo, Path: m.path}
	if m.active() {
		loc.NewLine, loc.OldLine = m.lineNumbers(m.cursorLine())
	}
	return loc, true
}

// SetSearch highlights matches of pattern (literal, smart-case) and moves to
// the first match at or after the cursor. It returns the number of matches;
// "" clears the search.
func (m *Model) SetSearch(pattern string) int { return m.setSearch(pattern) }

// Stats counts added, deleted and modified lines of the shown file: rows
// shown green, rows shown red, and modified pairs (yellow).
func (m *Model) Stats() (added, deleted, modified int) {
	if !m.active() {
		return 0, 0, 0
	}
	return m.f.added, m.f.deleted, m.f.modified
}

// Position returns the cursor's new-file line and the new file's length,
// for the statusline.
func (m *Model) Position() (line, total int) {
	if !m.active() {
		return 0, 0
	}
	line, _ = m.lineNumbers(m.cursorLine())
	return line, len(m.f.newLines)
}

// Scroll moves the viewport by n lines (mouse wheel), keeping the cursor
// inside it.
func (m *Model) Scroll(n int) {
	if !m.active() {
		return
	}
	p := m.cur()
	p.offset += n
	m.pull(p)
	m.fresh = false
}
