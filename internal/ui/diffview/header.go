package diffview

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/ui/icons"
)

// header renders the first line: icon, muted directory, file name, the
// change counts and, on the right, the view and context size.
func (m *Model) header(focused bool) string {
	w := m.width
	if !m.hasFile {
		return strings.Repeat(" ", w)
	}
	glyph, glyphFG := icons.For(m.th, m.path)
	dir, name := "", m.path
	if i := strings.LastIndexByte(m.path, '/'); i >= 0 {
		dir, name = m.path[:i+1], m.path[i+1:]
	}
	nameFG := m.th.FG
	if focused {
		nameFG = m.th.Accent
	}

	var counts []string
	countsW := 0
	if m.active() {
		add := func(n int, prefix, fg string) {
			if n == 0 {
				return
			}
			s := prefix + strconv.Itoa(n)
			counts = append(counts, m.pal.seq(fg, "", false)+s)
			countsW += len(s) + 1
		}
		add(m.f.added, "+", m.th.Green)
		add(m.f.deleted, "-", m.th.Red)
		add(m.f.modified, "~", m.th.Yellow)
		if countsW > 0 {
			countsW++ // two spaces before the first count instead of one
		}
	}
	right := ""
	if m.active() {
		mode := "UNIFIED"
		if m.split {
			mode = "SPLIT"
		}
		right = mode + " · ctx " + strconv.Itoa(m.context) + " "
	}

	// lead space, icon, space, then the path
	fixed := 3 + ansi.StringWidth(name)
	dirW := ansi.StringWidth(dir)
	rightW := ansi.StringWidth(right)
	if fixed+dirW+countsW+1+rightW > w {
		right, rightW = "", 0
	}
	if fixed+dirW+countsW > w {
		counts, countsW = nil, 0
	}
	if fixed+dirW > w && dirW > 0 {
		keep := max(w-fixed-1, 0)
		dir = "…" + ansi.TruncateLeft(dir, dirW-keep, "")
		dirW = ansi.StringWidth(dir)
	}

	var b strings.Builder
	b.WriteString(m.pal.seq(glyphFG, "", false))
	b.WriteByte(' ')
	b.WriteString(glyph)
	b.WriteByte(' ')
	b.WriteString(m.pal.seq(m.th.Muted, "", false))
	b.WriteString(dir)
	b.WriteString(m.pal.seq(nameFG, "", true))
	b.WriteString(name)
	if len(counts) > 0 {
		b.WriteString(" ")
		for _, c := range counts {
			b.WriteByte(' ')
			b.WriteString(c)
		}
	}
	used := fixed + dirW + countsW
	if rightW > 0 {
		b.WriteString(reset)
		writeSpaces(&b, w-used-rightW)
		b.WriteString(m.pal.seq(m.th.Muted, "", false))
		b.WriteString(right)
	}
	b.WriteString(reset)
	return fit(b.String(), w)
}
