package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const (
	sepRight = "" // powerline, pointing right
	sepLeft  = "" // powerline, pointing left
)

const iconClaude = "󰚩" // nf-md-robot, as in the Claude panel

// claudeFrames matches the Claude panel's spinner.
var claudeFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// minPathWidth is what the file path keeps before right-hand items are
// dropped for it.
const minPathWidth = 16

// A notice is cut down to minNoticeWidth before an item of keepPriority or
// more is dropped for it, so a long one does not empty the statusline.
const (
	minNoticeWidth = 12
	keepPriority   = 50
)

// section is where a right-hand statusline item goes, like lualine's x, y
// and z sections.
type section uint8

const (
	secX section = iota // on the bar, text already styled
	secY                // view section, plain text
	secZ                // position section, plain text
)

// segment is one right-hand statusline item; when the line is too narrow
// the items with the lowest priority are dropped first.
type segment struct {
	sec  section
	text string
	prio int
	// shrink renders the item w cells wide; nil for items that cannot be
	// shortened.
	shrink func(w int) string
}

// mode is the badge at the left, like lualine's section a.
func (m *Model) mode() (label, colour string) {
	switch {
	case m.search.active:
		return "SEARCH", m.th.Yellow
	case m.chat.Prompting():
		return "PROMPT", m.th.Green
	case m.diff.Visual():
		return "VISUAL", m.th.Magenta
	}
	return "NORMAL", m.th.Blue
}

func (m *Model) renderStatus() string {
	if m.search.active {
		return m.renderSearch()
	}
	label, colour := m.mode()
	th := m.th
	midBG := th.Subtle

	badge := paint(" "+label+" ", th.BarBG, colour, true)

	// Like lualine, the bar describes what the diff pane shows; the tab bar
	// already says where the tree cursor is.
	repo, branch := "", ""
	cur := m.currentRepo()
	if m.shown.ok {
		cur = m.shown.repo
	}
	if cur < len(m.repos) {
		r := m.repos[cur]
		repo = r.Repo.Name
		// Long feature branches would otherwise push the file path out.
		branch = ansi.Truncate(branchLabel(r.Status.Branch, r.Status.Head), max(12, m.width/5), "…")
	}
	repoSeg := " " + repo + " "
	if branch != "" {
		repoSeg += iconBranch + " " + branch + " "
	}
	path := ""
	if m.shown.ok {
		path = m.shown.path
	}

	left := badge + paint(sepRight, colour, midBG, false) +
		paint(repoSeg, colour, midBG, false) + paint(sepRight, midBG, th.BarBG, false)
	leftW := ansi.StringWidth(left)
	minPath := 0
	if path != "" {
		minPath = min(ansi.StringWidth(path), minPathWidth) + 2
	}
	right := m.rightSegments(colour)
	for {
		rightText := joinRight(right, colour, midBG, th.BarBG)
		avail := m.width - ansi.StringWidth(rightText)
		need := leftW + minPath + 1
		if need <= avail || len(right) == 0 {
			var p string
			if pathW := avail - leftW - 3; pathW > 0 && path != "" {
				p = m.renderPath(truncLeft(path, pathW))
			}
			gap := max(avail-leftW-ansi.StringWidth(p), 0)
			line := left + p + m.st.bar.Render(strings.Repeat(" ", gap)) + rightText
			return fitBar(line, m.width, m.st.bar.Render)
		}
		low := lowest(right)
		if right[low].prio >= keepPriority && shrinkBy(right, need-avail) {
			continue
		}
		right = append(right[:low:low], right[low+1:]...)
	}
}

// shrinkBy shortens the shrinkable item by over cells if it stays at least
// minNoticeWidth wide.
func shrinkBy(segs []segment, over int) bool {
	for i, s := range segs {
		if s.shrink == nil {
			continue
		}
		w := ansi.StringWidth(s.text) - over
		if w < minNoticeWidth {
			return false
		}
		segs[i].text = s.shrink(w)
		return true
	}
	return false
}

// renderPath shows a repo-relative path on the bar with its directory
// dimmed, so the file name stands out.
func (m *Model) renderPath(path string) string {
	dir, base := "", path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		dir, base = path[:i+1], path[i+1:]
	}
	return paint(" "+dir, m.th.Muted, m.th.BarBG, false) + paint(base+" ", m.th.FG, m.th.BarBG, false)
}

// rightSegments are, in display order: pending keys, notification, change
// counts, marks, Claude, then the view and position sections.
func (m *Model) rightSegments(colour string) []segment {
	th := m.th
	bar := th.BarBG
	var segs []segment
	// Pending keys outrank the notice: they answer what is being typed.
	if p := m.resolver.Pending(); p != "" {
		segs = append(segs, segment{sec: secX, text: paint(p, th.Accent, bar, true), prio: 100})
	}
	if m.notice.text != "" {
		fg := th.FG
		if m.notice.err {
			fg = th.Red
		}
		render := func(w int) string { return paint(ansi.Truncate(m.notice.text, w, "…"), fg, bar, false) }
		segs = append(segs, segment{sec: secX, text: render(max(m.width/2, minNoticeWidth)), prio: 90, shrink: render})
	}
	if m.shown.ok {
		a, d, mod := m.diff.Stats()
		var parts []string
		if a > 0 {
			parts = append(parts, paint("+"+strconv.Itoa(a), th.Green, bar, false))
		}
		if d > 0 {
			parts = append(parts, paint("-"+strconv.Itoa(d), th.Red, bar, false))
		}
		if mod > 0 {
			parts = append(parts, paint("~"+strconv.Itoa(mod), th.Yellow, bar, false))
		}
		if len(parts) > 0 {
			segs = append(segs, segment{sec: secX, text: strings.Join(parts, paint(" ", "", bar, false)), prio: 50})
		}
	}
	if n := len(m.sidebar.Marked()); n > 0 {
		segs = append(segs, segment{sec: secX, text: paint("● "+strconv.Itoa(n), th.Magenta, bar, false), prio: 40})
	}
	if m.chat.Busy() {
		frame := claudeFrames[int(time.Now().UnixMilli()/80)%len(claudeFrames)]
		segs = append(segs, segment{sec: secX, text: paint(frame+" "+m.chat.ModelName(), th.Magenta, bar, false), prio: 45})
	} else if name := m.chat.ModelName(); name != "" {
		segs = append(segs, segment{sec: secX, text: paint(iconClaude+" "+name, th.Muted, bar, false), prio: 10})
	}
	view := "UNIFIED"
	if m.diff.Split() {
		view = "SPLIT"
	}
	segs = append(segs,
		segment{sec: secY, text: view, prio: 60},
		segment{sec: secY, text: "ctx " + strconv.Itoa(m.diff.Context()), prio: 20},
	)
	if line, total := m.diff.Position(); total > 0 {
		segs = append(segs, segment{sec: secZ, text: fmt.Sprintf("%d/%d", line, total), prio: 70})
	}
	return segs
}

// joinRight renders the right half: the x items on the bar, then the y and
// z sections behind powerline separators.
func joinRight(segs []segment, colour, midBG, bar string) string {
	var x, y, z []string
	for _, s := range segs {
		switch s.sec {
		case secY:
			y = append(y, s.text)
		case secZ:
			z = append(z, s.text)
		default:
			x = append(x, s.text)
		}
	}
	var b strings.Builder
	spacer := paint("  ", "", bar, false)
	for _, t := range x {
		b.WriteString(t)
		b.WriteString(spacer)
	}
	prev := bar
	if len(y) > 0 {
		b.WriteString(paint(sepLeft, midBG, prev, false))
		b.WriteString(paint(" "+strings.Join(y, " · ")+" ", colour, midBG, false))
		prev = midBG
	}
	if len(z) > 0 {
		b.WriteString(paint(sepLeft, colour, prev, false))
		b.WriteString(paint(" "+strings.Join(z, " ")+" ", bar, colour, true))
	}
	return b.String()
}

// lowest is the index of the item to drop first.
func lowest(segs []segment) int {
	low := 0
	for i, s := range segs {
		if s.prio < segs[low].prio {
			low = i
		}
	}
	return low
}

// fitBar is fit for bars: padding keeps the bar background.
func fitBar(line string, w int, pad func(...string) string) string {
	n := ansi.StringWidth(line)
	if n > w {
		return fit(line, w)
	}
	if n < w {
		line += pad(strings.Repeat(" ", w-n))
	}
	return line
}

func (m *Model) renderSearch() string {
	th := m.th
	label, colour := m.mode()
	badge := paint(" "+label+" ", th.BarBG, colour, true) + paint(sepRight, colour, th.BarBG, false)

	var info string
	switch {
	case m.search.input == "":
	case m.search.matches == 0:
		info = paint(" no matches ", th.Red, th.BarBG, false)
	case m.search.matches == 1:
		info = paint(" 1 match ", th.Muted, th.BarBG, false)
	default:
		info = paint(" "+strconv.Itoa(m.search.matches)+" matches ", th.Muted, th.BarBG, false)
	}
	cursor := paint(" ", th.BarBG, th.FG, false)
	avail := m.width - ansi.StringWidth(badge) - ansi.StringWidth(info) - 3
	input := paint(" /"+truncLeft(m.search.input, max(avail, 0)), th.FG, th.BarBG, false)
	line := badge + input + cursor
	gap := max(m.width-ansi.StringWidth(line)-ansi.StringWidth(info), 0)
	return fitBar(line+m.st.bar.Render(strings.Repeat(" ", gap))+info, m.width, m.st.bar.Render)
}
