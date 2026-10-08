package diffview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/highlight"
)

const (
	viewUnified = 0
	viewSplit   = 1
)

// file is everything derived from one FileDiff. Line numbers index the
// lookup tables 1-based (entry n-1 is line n).
type file struct {
	rows     []diff.Row
	split    []diff.SplitRow
	oldLines []string
	newLines []string
	oldSpans [][]highlight.Span // nil until highlighted (or when no lexer matched)
	newSpans [][]highlight.Span

	newRow  []int32 // new-file line → unified row
	oldRow  []int32 // old-file line → unified row
	near    []int32 // unified row → nearest new-file line, 0 when the new file is empty
	splitOf []int32 // unified row → split row showing it
	widths  []int32 // unified row → display width of its text, -1 until measured

	maxWidth int // widest row, -1 until measured
	numW     int // width of the line-number columns

	added, deleted, modified int
}

func newFile(fd *diff.FileDiff) *file {
	rows := diff.Unified(fd)
	f := &file{
		rows:     rows,
		split:    diff.Split(rows),
		oldLines: fd.OldLines(),
		newLines: fd.NewLines(),
		maxWidth: -1,
	}
	f.newRow = make([]int32, len(f.newLines))
	f.oldRow = make([]int32, len(f.oldLines))
	f.near = make([]int32, len(rows))
	f.splitOf = make([]int32, len(rows))
	f.widths = make([]int32, len(rows))
	for i, r := range rows {
		f.widths[i] = -1
		if r.NewNo > 0 && r.NewNo <= len(f.newRow) {
			f.newRow[r.NewNo-1] = int32(i)
		}
		if r.OldNo > 0 && r.OldNo <= len(f.oldRow) {
			f.oldRow[r.OldNo-1] = int32(i)
		}
		switch r.Kind {
		case diff.RowAdded:
			f.added++
		case diff.RowDeleted:
			f.deleted++
		case diff.RowModNew:
			f.modified++
		}
	}
	// A deleted row sits where the next new-file line now is; at the end of
	// the file only the previous one is left.
	next := int32(0)
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].NewNo > 0 {
			next = int32(rows[i].NewNo)
		}
		f.near[i] = next
	}
	prev := int32(0)
	for i, r := range rows {
		if r.NewNo > 0 {
			prev = int32(r.NewNo)
		}
		if f.near[i] == 0 {
			f.near[i] = prev
		}
	}
	for s, sr := range f.split {
		if sr.Left >= 0 {
			f.splitOf[sr.Left] = int32(s)
		}
		if sr.Right >= 0 {
			f.splitOf[sr.Right] = int32(s)
		}
	}
	f.numW = max(digits(max(len(f.oldLines), len(f.newLines))), minNumWidth)
	return f
}

// minNumWidth keeps the gutter of small files as wide as Neovim's default
// number column, so switching files does not shift the code sideways.
const minNumWidth = 3

func digits(n int) int {
	d := 1
	for n >= 10 {
		n /= 10
		d++
	}
	return d
}

// isChange reports per line of view v whether it is part of a change.
func (f *file) isChange(v int) []bool {
	if v == viewUnified {
		out := make([]bool, len(f.rows))
		for i, r := range f.rows {
			out[i] = r.IsChange()
		}
		return out
	}
	out := make([]bool, len(f.split))
	for s, sr := range f.split {
		out[s] = sr.Left != sr.Right
	}
	return out
}

// width returns the display width of row i's text.
func (f *file) width(i int) int {
	if f.widths[i] < 0 {
		f.widths[i] = int32(highlight.Width(f.rows[i].Text, 0))
	}
	return int(f.widths[i])
}

func (f *file) widest() int {
	if f.maxWidth < 0 {
		f.maxWidth = 0
		for i := range f.rows {
			f.maxWidth = max(f.maxWidth, f.width(i))
		}
	}
	return f.maxWidth
}

// spans returns the syntax spans of row i: old-file colours for rows that
// only exist in the old file, new-file colours otherwise.
func (f *file) spans(i int) []highlight.Span {
	r := &f.rows[i]
	if r.Kind == diff.RowDeleted || r.Kind == diff.RowModOld {
		if r.OldNo > 0 && r.OldNo <= len(f.oldSpans) {
			return f.oldSpans[r.OldNo-1]
		}
		return nil
	}
	if r.NewNo > 0 && r.NewNo <= len(f.newSpans) {
		return f.newSpans[r.NewNo-1]
	}
	return nil
}

func (m *Model) active() bool { return m.f != nil && m.message == "" && m.hasFile }

func (m *Model) setFile(repo, path string, fd *diff.FileDiff, keepPosition bool) tea.Cmd {
	keep := keepPosition && m.f != nil && repo == m.repo && path == m.path
	var snap snapshot
	if keep {
		snap = m.snapshot()
	}
	prev := m.f
	m.gen++
	m.repo, m.path, m.hasFile = repo, path, true
	m.message = ""
	m.search.valid = false

	switch {
	case fd == nil:
		m.message = "nothing to show"
	case fd.Binary:
		m.message = "binary file"
	case fd.TooLarge:
		m.message = "file too large to show"
	case len(fd.Lines) == 0:
		m.message = "empty file"
	}
	if m.message != "" {
		m.f = nil
		m.visual = false
		return nil
	}

	m.f = newFile(fd)
	for v := range m.panes {
		isChange := m.f.isChange(v)
		m.panes[v] = pane{folds: diff.NewFolds(isChange, m.context), starts: diff.BlockStarts(isChange)}
	}
	if keep {
		m.restore(snap)
		m.f.oldSpans = carrySpans(prev.oldLines, prev.oldSpans, m.f.oldLines)
		m.f.newSpans = carrySpans(prev.newLines, prev.newSpans, m.f.newLines)
	} else {
		m.visual = false
		m.xoff = 0
		m.home()
	}
	return m.highlightCmd()
}

// carrySpans reuses the colours of unchanged lines from the previous version
// of a file, so a reload does not flash plain text until the new highlight
// arrives.
func carrySpans(prevLines []string, prevSpans [][]highlight.Span, lines []string) [][]highlight.Span {
	if prevSpans == nil || len(prevLines) != len(prevSpans) {
		return nil
	}
	same := len(prevLines) == len(lines)
	for i := 0; same && i < len(lines); i++ {
		same = prevLines[i] == lines[i]
	}
	if same {
		return prevSpans
	}
	byText := make(map[string][]highlight.Span, len(prevLines))
	for i, l := range prevLines {
		byText[l] = prevSpans[i]
	}
	out := make([][]highlight.Span, len(lines))
	for i, l := range lines {
		out[i] = byText[l]
	}
	return out
}
