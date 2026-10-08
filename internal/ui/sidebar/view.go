package sidebar

import (
	"strconv"
	"strings"

	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/ui/icons"
)

// rightMargin is the air kept at the right edge, so the line counts do not
// touch the pane border. Very narrow panes give it up.
func (m *Model) rightMargin() int {
	if m.width >= 16 {
		return 1
	}
	return 0
}

func (m *Model) view(focused bool) string {
	if m.height <= 0 {
		return ""
	}
	out := make([]string, 0, m.height)
	out = append(out, m.finish(m.headerLine(focused), ""))
	for i := 0; i < m.bodyHeight(); i++ {
		idx := m.offset + i
		switch {
		case idx < len(m.rows):
			bg := ""
			if idx == m.cursor {
				bg = m.th.SelectedBG
			}
			out = append(out, m.finish(m.rowLine(m.rows[idx]), bg))
		case i == 0 && m.loaded && len(m.repos) == 0:
			out = append(out, m.finish(&line{pieces: []piece{
				flexible(seg{text: "  No git repositories", fg: m.th.Muted}),
			}, right: 1, plan: []squeeze{{0, 1}}}, ""))
		default:
			out = append(out, strings.Repeat(" ", max(m.width, 0)))
		}
	}
	return strings.Join(out, "\n")
}

// finish renders a line into the pane width, right margin included.
func (m *Model) finish(l *line, bg string) string {
	margin := m.rightMargin()
	return l.render(m.width-margin, bg) + paint(strings.Repeat(" ", margin), "", bg, false)
}

func (m *Model) headerLine(focused bool) *line {
	title := m.th.Muted
	if focused {
		title = m.th.Accent
	}
	total := len(m.files)
	l := &line{
		pieces: []piece{
			flexible(seg{text: " " + iconTree + " ", fg: title, bold: focused}, seg{text: "Files", fg: title, bold: focused}),
			fixed(),
		},
		right: 1,
		plan:  []squeeze{{1, -1}, {0, 1}},
	}
	if m.loaded && total > 0 {
		l.pieces[1] = fixed(seg{text: strconv.Itoa(total), fg: m.th.Muted})
	}
	return l
}

func (m *Model) rowLine(n *node) *line {
	switch n.kind {
	case repoNode:
		return m.repoLine(n)
	case dirNode:
		return m.dirLine(n)
	}
	return m.fileLine(n)
}

func (m *Model) chevron(n *node) seg {
	if m.isExpanded(n) {
		return seg{text: chevronOpen, fg: m.th.Muted}
	}
	return seg{text: chevronClosed, fg: m.th.Muted}
}

// indent is the left part of a row: a one-cell gutter where marks go, then
// two cells per level with a faint guide under every directory, like
// neo-tree's indent markers.
func (m *Model) indent(n *node, gutter seg) []seg {
	out := []seg{gutter}
	if n.depth > 0 {
		out = append(out, seg{text: "  "})
	}
	if n.depth > 1 {
		out = append(out, seg{text: strings.Repeat("│ ", n.depth-1), fg: m.th.Subtle})
	}
	return out
}

// repoLine: "▾ 󰊢 name  branch ↑1 ↓2" with a right-aligned file count; the
// branch gives way first, the name last.
func (m *Model) repoLine(n *node) *line {
	r := m.repos[n.repo]
	th := m.th
	const (
		pLead = iota
		pName
		pState
		pMsg
		pBranch
		pSync
		pCount
	)
	l := &line{
		pieces: []piece{
			pLead:   fixed(append(m.indent(n, seg{text: " "}), m.chevron(n), seg{text: " "}, seg{text: iconGit, fg: th.Magenta}, seg{text: " "})...),
			pName:   flexible(seg{text: printable(r.Repo.Name), fg: th.FG, bold: true}),
			pState:  fixed(),
			pMsg:    fixed(),
			pBranch: fixed(),
			pSync:   fixed(),
			pCount:  fixed(),
		},
		right: pCount,
		plan: []squeeze{
			{pBranch, 8}, {pMsg, 6}, {pCount, -1}, {pBranch, -1}, {pSync, -1}, {pMsg, -1},
			{pName, 6}, {pState, -1}, {pName, 1},
		},
	}
	switch {
	case !r.Loaded:
		l.pieces[pState] = fixed(seg{text: " …", fg: th.Muted})
		return l
	case r.Err != nil:
		l.pieces[pState] = fixed(seg{text: " !", fg: th.Red, bold: true})
		l.pieces[pMsg] = flexible(seg{text: " "}, seg{text: shortError(r.Err), fg: th.Muted})
		return l
	case len(r.Status.Files) == 0:
		l.pieces[pState] = fixed(seg{text: " ✓", fg: th.Green})
	}

	st := r.Status
	switch {
	case st.Branch != "":
		l.pieces[pBranch] = flexible(seg{text: "  "}, seg{text: iconBranch + " ", fg: th.Muted}, seg{text: printable(st.Branch), fg: th.Muted})
	case st.Head != "":
		l.pieces[pBranch] = flexible(seg{text: "  "}, seg{text: iconCommit + " ", fg: th.Muted}, seg{text: st.Head, fg: th.Muted})
	}
	var sync []string
	if st.Ahead > 0 {
		sync = append(sync, "↑"+strconv.Itoa(st.Ahead))
	}
	if st.Behind > 0 {
		sync = append(sync, "↓"+strconv.Itoa(st.Behind))
	}
	if len(sync) > 0 {
		l.pieces[pSync] = fixed(seg{text: " " + strings.Join(sync, " "), fg: th.Cyan})
	}
	if c := len(st.Files); c > 0 && !m.isExpanded(n) {
		l.pieces[pCount] = fixed(seg{text: strconv.Itoa(c), fg: th.Muted})
	}
	return l
}

func (m *Model) dirLine(n *node) *line {
	icon := iconFolder
	if m.isExpanded(n) {
		icon = iconFolderOpen
	}
	l := &line{
		pieces: []piece{
			fixed(append(m.indent(n, seg{text: " "}), m.chevron(n), seg{text: " "}, seg{text: icon, fg: m.th.Blue}, seg{text: " "})...),
			flexible(seg{text: printable(n.label), fg: m.th.FG}),
		},
		right: 2,
		plan:  []squeeze{{1, 1}},
	}
	l.pieces[1].leftCut = true
	return l
}

// fileLine: mark gutter, status letter, icon, name, right-aligned "+12 -3".
// When space is short the rename source goes first, then the name shrinks
// to a few cells, then the counts go.
func (m *Model) fileLine(n *node) *line {
	f := n.file
	th := m.th
	const (
		pLead = iota
		pFrom
		pName
		pCounts
	)
	mark := seg{text: " "}
	if m.marked(n) {
		mark = seg{text: markGlyph, fg: th.Magenta}
	}
	icon, iconColor := icons.For(th, f.Path)
	letterColor, letterBold := m.kindStyle(f.Kind)
	l := &line{
		pieces: []piece{
			pLead: fixed(append(m.indent(n, mark), seg{text: f.Kind.Letter(), fg: letterColor, bold: letterBold},
				seg{text: " "}, seg{text: icon, fg: iconColor}, seg{text: " "})...),
			pFrom:   fixed(),
			pName:   flexible(seg{text: printable(n.label), fg: th.FG}),
			pCounts: fixed(m.counts(f)...),
		},
		right: pCounts,
		plan:  []squeeze{{pFrom, -1}, {pName, 6}, {pCounts, -1}, {pName, 1}},
	}
	if f.OldPath != "" && f.OldPath != f.Path {
		from := f.OldPath
		newDir, _ := splitPath(f.Path)
		if oldDir, oldBase := splitPath(f.OldPath); oldDir == newDir {
			from = oldBase
		}
		l.pieces[pFrom] = fixed(seg{text: printable(from), fg: th.Muted}, seg{text: " → ", fg: th.Muted})
	}
	return l
}

func (m *Model) kindStyle(k git.Kind) (color string, bold bool) {
	th := m.th
	switch k {
	case git.Added:
		return th.Green, false
	case git.Deleted:
		return th.Red, false
	case git.Renamed, git.Copied:
		return th.Blue, false
	case git.Untracked:
		return th.Cyan, false
	case git.TypeChanged:
		return th.Magenta, false
	case git.Conflicted:
		return th.Red, true
	}
	return th.Yellow, false
}

// counts renders "+12 -3" in toned-down green and red: they are secondary to
// the name and should not compete with the status letters.
func (m *Model) counts(f *git.FileChange) []seg {
	th := m.th
	if f.Binary {
		return []seg{{text: "bin", fg: th.Muted}}
	}
	var out []seg
	if f.Added > 0 {
		out = append(out, seg{text: "+" + strconv.Itoa(f.Added), fg: m.addFG})
	}
	if f.Deleted > 0 {
		if len(out) > 0 {
			out = append(out, seg{text: " "})
		}
		out = append(out, seg{text: "-" + strconv.Itoa(f.Deleted), fg: m.delFG})
	}
	return out
}

// shortError keeps what fits on the repo row: the first line, without the
// "git status: fatal: " lead-in, which would fill a narrow pane before the
// part that says what went wrong.
func shortError(err error) string {
	s := err.Error()
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if cmd, ok := strings.CutPrefix(s, "git "); ok {
		if _, msg, ok := strings.Cut(cmd, ": "); ok {
			s = msg
		}
	}
	for _, lead := range []string{"fatal: ", "error: "} {
		s = strings.TrimPrefix(s, lead)
	}
	return printable(s)
}

// printable replaces control characters, which would break the row width
// and could inject terminal sequences, with a visible placeholder. Invalid
// UTF-8 goes too: the terminal gives each bad byte a cell while
// ansi.StringWidth counts none, so the row would overflow.
func printable(s string) string {
	s = strings.ToValidUTF8(s, "?")
	clean := true
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			clean = false
			break
		}
	}
	if clean {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			return '?'
		}
		return r
	}, s)
}
