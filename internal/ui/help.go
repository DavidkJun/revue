package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/ui/keys"
)

type helpState struct {
	open   bool
	scroll int
}

// helpEntry is one action of the help overlay with all its keys.
type helpEntry struct {
	keys string // "j / down"
	help string
}

type helpGroup struct {
	title   string
	entries []helpEntry
}

// helpGroups builds the overlay from the keymap itself, merging aliases
// (same scope, action and help text) into one entry.
func helpGroups() []helpGroup {
	scopes := []keys.Scope{keys.Global, keys.Nav, keys.Sidebar, keys.Diff, keys.Visual, keys.Chat}
	groups := make([]helpGroup, 0, len(scopes))
	for _, sc := range scopes {
		g := helpGroup{title: sc.String()}
		index := map[string]int{}
		for _, b := range keys.Bindings {
			if b.Scope != sc {
				continue
			}
			id := string(b.Action) + "\x00" + b.Help
			if i, ok := index[id]; ok {
				g.entries[i].keys += " / " + keyLabel(b.Keys)
				continue
			}
			index[id] = len(g.entries)
			g.entries = append(g.entries, helpEntry{keys: keyLabel(b.Keys), help: b.Help})
		}
		if len(g.entries) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}

// keyLabel writes a binding the way Vim users read it: "g g" → "gg",
// "space a" → "<space>a".
func keyLabel(seq string) string {
	parts := strings.Fields(seq)
	if len(parts) == 1 {
		return parts[0]
	}
	for i, p := range parts {
		if p == "space" {
			parts[i] = "<space>"
		}
	}
	return strings.Join(parts, "")
}

const (
	helpColGap = 4
	helpIndent = 2
)

// helpColumn is a run of groups rendered side by side with other columns.
type helpColumn struct {
	groups      []helpGroup
	keyW, helpW int
}

func (c helpColumn) width() int { return helpIndent + c.keyW + 2 + c.helpW }

func (c helpColumn) height() int {
	n := 0
	for i, g := range c.groups {
		if i > 0 {
			n++ // blank line between groups
		}
		n += 1 + len(g.entries)
	}
	return n
}

// splitColumns distributes groups over n columns in order, aiming for equal
// heights.
func splitColumns(groups []helpGroup, n int) []helpColumn {
	total := 0
	for _, g := range groups {
		total += len(g.entries) + 2
	}
	target := (total + n - 1) / n
	cols := make([]helpColumn, 0, n)
	var cur helpColumn
	h := 0
	for i, g := range groups {
		size := len(g.entries) + 2
		left := len(groups) - i
		if len(cur.groups) > 0 && (h+size/2 > target || left < n-len(cols)) && len(cols) < n-1 {
			cols = append(cols, cur)
			cur, h = helpColumn{}, 0
		}
		cur.groups = append(cur.groups, g)
		h += size
	}
	cols = append(cols, cur)
	for i := range cols {
		for _, g := range cols[i].groups {
			for _, e := range g.entries {
				cols[i].keyW = max(cols[i].keyW, ansi.StringWidth(e.keys))
				cols[i].helpW = max(cols[i].helpW, ansi.StringWidth(e.help))
			}
			cols[i].helpW = max(cols[i].helpW, ansi.StringWidth(g.title)-helpIndent-cols[i].keyW-2)
		}
	}
	return cols
}

// helpContent lays the groups out in as many columns as fit in width and
// returns the styled lines and their width.
func (m *Model) helpContent(width int) ([]string, int) {
	groups := helpGroups()
	var cols []helpColumn
	for n := 3; n >= 1; n-- {
		cols = splitColumns(groups, n)
		w := (len(cols) - 1) * helpColGap
		for _, c := range cols {
			w += c.width()
		}
		if w <= width || n == 1 {
			break
		}
	}
	if len(cols) == 1 && cols[0].width() > width {
		cols[0].helpW = max(width-helpIndent-cols[0].keyW-2, 1)
	}

	th := m.th
	rendered := make([][]string, len(cols))
	height, totalW := 0, (len(cols)-1)*helpColGap
	for i, c := range cols {
		w := c.width()
		totalW += w
		var out []string
		for gi, g := range c.groups {
			if gi > 0 {
				out = append(out, strings.Repeat(" ", w))
			}
			out = append(out, fit(paint(g.title, th.Magenta, "", true), w))
			for _, e := range g.entries {
				k := paint(fit(e.keys, c.keyW), th.Cyan, "", false)
				h := paint(ansi.Truncate(e.help, c.helpW, "…"), th.FG, "", false)
				out = append(out, fit(strings.Repeat(" ", helpIndent)+k+"  "+h, w))
			}
		}
		rendered[i] = out
		height = max(height, len(out))
	}

	gap := strings.Repeat(" ", helpColGap)
	out := make([]string, height)
	for y := range out {
		parts := make([]string, len(cols))
		for i, c := range cols {
			if y < len(rendered[i]) {
				parts[i] = rendered[i][y]
			} else {
				parts[i] = strings.Repeat(" ", c.width())
			}
		}
		out[y] = strings.Join(parts, gap)
	}
	return out, totalW
}

// helpBox is the overlay's geometry within the body.
type helpBox struct {
	lines      []string // content, all of it
	x, y, w, h int      // box including its border
	maxScroll  int
}

func (m *Model) helpBox(bodyH int) helpBox {
	content, cw := m.helpContent(max(m.width-6, 1))
	b := helpBox{lines: content}
	b.w = min(cw+4, m.width)
	b.h = min(len(content)+2, bodyH)
	b.x = (m.width - b.w) / 2
	b.y = (bodyH - b.h) / 2
	b.maxScroll = max(len(content)-(b.h-2), 0)
	return b
}

// helpShown reports whether the help overlay is open and has room to be
// drawn; a help nobody can see must not take the keys.
func (m *Model) helpShown() bool {
	return m.help.open && m.layout().bodyH >= 3 && m.width >= 8
}

// overlayHelp draws the help box centred over the body lines.
func (m *Model) overlayHelp(body []string) []string {
	b := m.helpBox(len(body))
	scroll := min(m.help.scroll, b.maxScroll)
	th := m.th
	border := func(s string) string { return paint(s, th.Accent, "", false) }
	inner := b.w - 2

	top := border("╭─") + paint(" Keys ", th.Accent, "", true)
	top += border(strings.Repeat("─", max(inner-7, 0)) + "╮")
	hint := " j/k scroll · ? close "
	if b.maxScroll == 0 {
		hint = " ? close "
	}
	hintW := ansi.StringWidth(hint)
	bottom := border("╰" + strings.Repeat("─", max(inner-hintW-1, 0)))
	bottom += paint(hint, th.Muted, "", false) + border("─╯")

	box := []string{fit(top, b.w)}
	for i := range b.h - 2 {
		line := ""
		if j := scroll + i; j < len(b.lines) {
			line = b.lines[j]
		}
		box = append(box, border("│")+" "+fit(line, inner-2)+" "+border("│"))
	}
	box = append(box, fit(bottom, b.w))

	// The panes behind the box fade to plain dim text, so the box reads as
	// a modal without painting a background.
	out := make([]string, len(body))
	for i, line := range body {
		out[i] = paint(ansi.Strip(line), th.Subtle, "", false)
	}
	for i, line := range box {
		y := b.y + i
		if y < 0 || y >= len(out) {
			continue
		}
		left := ansi.Truncate(out[y], b.x, "")
		right := ansi.TruncateLeft(out[y], b.x+b.w, "")
		out[y] = fit(fit(left, b.x)+line+right, m.width)
	}
	return out
}
