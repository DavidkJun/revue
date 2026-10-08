package diffview

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

func (m *Model) startVisual() {
	p := m.cur()
	s, e := span(p.folds.Items()[p.cursor])
	m.visual = true
	m.anchor = [2]int{s, e}
}

// selectedLines is the line range [lo, hi) of the current view between the
// anchor and the cursor; closed folds inside it count with all their lines.
func (m *Model) selectedLines() (lo, hi int) {
	p := m.cur()
	s, e := span(p.folds.Items()[p.cursor])
	return min(s, m.anchor[0]), max(e, m.anchor[1])
}

// rowsIn returns the unified rows shown on lines [lo, hi) of the current
// view, in file order.
func (m *Model) rowsIn(lo, hi int) []int {
	if !m.split {
		out := make([]int, 0, hi-lo)
		for i := lo; i < hi; i++ {
			out = append(out, i)
		}
		return out
	}
	out := make([]int, 0, 2*(hi-lo))
	for _, sr := range m.f.split[lo:hi] {
		if sr.Left >= 0 {
			out = append(out, sr.Left)
		}
		if sr.Right >= 0 && sr.Right != sr.Left {
			out = append(out, sr.Right)
		}
	}
	slices.Sort(out)
	return out
}

var rowPrefix = [numKinds]byte{
	diff.RowContext: ' ',
	diff.RowAdded:   '+',
	diff.RowDeleted: '-',
	diff.RowModOld:  '-',
	diff.RowModNew:  '+',
}

func (m *Model) selection() (Selection, bool) {
	sel, _, ok := m.collect()
	return sel, ok
}

// collect builds the selection and reports whether it contains changes.
func (m *Model) collect() (sel Selection, changed, ok bool) {
	if !m.Visual() {
		return Selection{}, false, false
	}
	lo, hi := m.selectedLines()
	rows := m.rowsIn(lo, hi)
	sel = Selection{Repo: m.repo, Path: m.path, Lines: hi - lo}
	var b strings.Builder
	for k, i := range rows {
		r := &m.f.rows[i]
		if k > 0 {
			b.WriteByte('\n')
		}
		b.WriteByte(rowPrefix[r.Kind])
		b.WriteString(r.Text)
		changed = changed || r.IsChange()
		if r.NewNo > 0 {
			if sel.StartLine == 0 {
				sel.StartLine = r.NewNo
			}
			sel.EndLine = r.NewNo
		}
	}
	if sel.StartLine == 0 && len(rows) > 0 {
		sel.StartLine = int(m.f.near[rows[0]])
		sel.EndLine = sel.StartLine
	}
	sel.Text = b.String()
	return sel, changed, true
}

// yankSelection yanks the selection and leaves visual mode. A selection of
// unchanged lines is yanked as plain code: the markers only matter when
// they say what changed.
func (m *Model) yankSelection() tea.Cmd {
	sel, changed, ok := m.collect()
	m.visual = false
	if !ok {
		return nil
	}
	text := sel.Text
	if !changed {
		text = stripPrefixes(text)
	}
	return yank(text, plural(sel.Lines, "line"))
}

func stripPrefixes(text string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = l[1:]
		}
	}
	return strings.Join(lines, "\n")
}

// yankLine yanks the text of the cursor line (the new side of a modified
// pair in the split view); on a closed fold it yanks every hidden line, as
// yy does in Vim.
func (m *Model) yankLine() tea.Cmd {
	p := m.cur()
	s, e := span(p.folds.Items()[p.cursor])
	v := m.view()
	texts := make([]string, 0, e-s)
	for l := s; l < e; l++ {
		texts = append(texts, m.f.rows[m.primary(v, l)].Text)
	}
	return yank(strings.Join(texts, "\n"), plural(e-s, "line"))
}

// yankRef yanks "<repo>/<path>:<line>", the selection's line range in visual
// mode, so it can be pasted into Claude Code running in the launch folder.
func (m *Model) yankRef() tea.Cmd {
	ref := m.path
	if m.repo != "" {
		ref = m.repo + "/" + m.path
	}
	if sel, ok := m.selection(); ok {
		ref += ":" + strconv.Itoa(sel.StartLine)
		if sel.EndLine != sel.StartLine {
			ref += "-" + strconv.Itoa(sel.EndLine)
		}
		m.visual = false
	} else if loc, _ := m.Cursor(); loc.NewLine > 0 {
		ref += ":" + strconv.Itoa(loc.NewLine)
	}
	return yank(ref, ref)
}

func yank(text, what string) tea.Cmd {
	return func() tea.Msg { return uimsg.Yank{Text: text, What: what} }
}

func notify(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return uimsg.Notify{Text: text, Error: isErr} }
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
