package ui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

const (
	iconGit    = "" // nf-dev-git
	iconBranch = "" // powerline branch
	brandLabel = " revue "
)

// tab is one rendered repo tab and where it sits in the tab bar.
type tab struct {
	repo int
	text string
	x, w int
}

// tabs lays out the repo tabs like LazyVim's bufferline. When they do not
// fit, the other tabs lose their branch, then the current tab's branch is
// shortened, then the "revue" label goes, and finally the tabs farthest from
// the current one are dropped.
func (m *Model) tabs() []tab {
	cur := m.currentRepo()
	avail := m.width - ansi.StringWidth(brandLabel)
	build := func(compact bool, branchMax int) []tab {
		ts := make([]tab, len(m.repos))
		for i := range m.repos {
			ts[i] = tab{repo: i, text: m.renderTab(i, i == cur, compact && i != cur, branchMax)}
			ts[i].w = ansi.StringWidth(ts[i].text)
		}
		return ts
	}
	total := func(ts []tab) int {
		n := 0
		for _, t := range ts {
			n += t.w
		}
		return n
	}

	ts := build(false, 0)
	if total(ts) > avail {
		ts = build(true, 0)
	}
	if total(ts) > avail {
		ts = build(true, max(10, m.width/5))
	}
	if total(ts) > avail {
		avail = m.width
	}
	for len(ts) > 1 && total(ts) > avail {
		if cur-ts[0].repo > ts[len(ts)-1].repo-cur {
			ts = ts[1:]
		} else {
			ts = ts[:len(ts)-1]
		}
	}
	x := 0
	for i := range ts {
		ts[i].x = x
		x += ts[i].w
	}
	return ts
}

// renderTab draws repo i's tab; compact leaves out the branch and
// branchMax > 0 shortens it.
func (m *Model) renderTab(i int, active, compact bool, branchMax int) string {
	r := m.repos[i]
	th := m.th
	bg, fg, icon := th.BarBG, th.Muted, th.Muted
	lead := paint(" ", "", bg, false)
	if active {
		bg, fg, icon = th.SelectedBG, th.Accent, th.Accent
		lead = paint("▎", th.Accent, bg, false)
	}

	var b strings.Builder
	b.WriteString(lead)
	b.WriteString(paint(" "+iconGit+" ", icon, bg, false))
	b.WriteString(paint(r.Repo.Name, fg, bg, active))
	if !compact {
		if branch := branchLabel(r.Status.Branch, r.Status.Head); r.Loaded && branch != "" {
			if branchMax > 0 {
				branch = ansi.Truncate(branch, branchMax, "…")
			}
			branchFG := th.Muted
			if active {
				branchFG = th.FG
			}
			b.WriteString(paint("  "+iconBranch+" "+branch, branchFG, bg, false))
			if ab := aheadBehind(r.Status.Ahead, r.Status.Behind); ab != "" {
				b.WriteString(paint(" "+ab, th.Cyan, bg, false))
			}
		}
	}
	b.WriteString(paint("  ", "", bg, false))
	switch {
	case !r.Loaded:
		b.WriteString(paint("…", th.Muted, bg, false))
	case r.Err != nil:
		b.WriteString(paint("!", th.Red, bg, true))
	case len(r.Status.Files) == 0:
		b.WriteString(paint("✓", th.Green, bg, false))
	default:
		b.WriteString(paint("●"+strconv.Itoa(len(r.Status.Files)), th.Yellow, bg, false))
	}
	b.WriteString(paint(" ", "", bg, false))
	return b.String()
}

func branchLabel(branch, head string) string {
	if branch != "" {
		return branch
	}
	return head // detached
}

func aheadBehind(ahead, behind int) string {
	var s string
	if ahead > 0 {
		s += "↑" + strconv.Itoa(ahead)
	}
	if behind > 0 {
		s += "↓" + strconv.Itoa(behind)
	}
	return s
}

func (m *Model) renderTabBar() string {
	var b strings.Builder
	used := 0
	for _, t := range m.tabs() {
		b.WriteString(t.text)
		used += t.w
	}
	if rest := m.width - used; rest > 0 {
		label := brandLabel
		if ansi.StringWidth(label) > rest {
			label = ""
		}
		b.WriteString(m.st.bar.Render(strings.Repeat(" ", rest-ansi.StringWidth(label))))
		b.WriteString(paint(label, m.th.Muted, m.th.BarBG, false))
	}
	return fit(b.String(), m.width)
}
