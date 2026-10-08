package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/theme"
)

// styles are the chrome styles, built once from the theme.
type styles struct {
	bar       lipgloss.Style // statusline / tab bar filler
	sep       lipgloss.Style // pane separator
	sepActive lipgloss.Style // separator next to the focused pane
}

func newStyles(th theme.Theme) styles {
	return styles{
		bar:       lipgloss.NewStyle().Background(lipgloss.Color(th.BarBG)),
		sep:       lipgloss.NewStyle().Foreground(lipgloss.Color(th.Subtle)),
		sepActive: lipgloss.NewStyle().Foreground(lipgloss.Color(th.Accent)),
	}
}

// paint renders text with an optional foreground/background.
func paint(text, fg, bg string, bold bool) string {
	if text == "" {
		return ""
	}
	s := lipgloss.NewStyle().Bold(bold)
	if fg != "" {
		s = s.Foreground(lipgloss.Color(fg))
	}
	if bg != "" {
		s = s.Background(lipgloss.Color(bg))
	}
	return s.Render(text)
}

// fit makes line exactly w cells wide, cutting or padding with plain
// (transparent) spaces.
func fit(line string, w int) string {
	if w <= 0 {
		return ""
	}
	n := ansi.StringWidth(line)
	switch {
	case n == w:
		return line
	case n > w:
		line = ansi.Truncate(line, w, "")
		return line + strings.Repeat(" ", w-ansi.StringWidth(line))
	default:
		return line + strings.Repeat(" ", w-n)
	}
}

// lines splits a component view into exactly h lines of exactly w cells.
func lines(view string, w, h int) []string {
	out := strings.Split(view, "\n")
	if len(out) > h {
		out = out[:h]
	}
	for len(out) < h {
		out = append(out, "")
	}
	for i := range out {
		out[i] = fit(out[i], w)
	}
	return out
}

// truncLeft keeps the end of s (a file path reads best by its name),
// marking the cut with "…".
func truncLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return "…" + ansi.TruncateLeft(s, ansi.StringWidth(s)-(w-1), "")
}

// render draws the whole frame: exactly m.height lines of m.width cells.
func (m *Model) render() string {
	if m.width <= 0 || m.height <= 0 {
		return strings.Repeat("\n", max(m.height-1, 0))
	}
	l := m.layout()
	rows := make([]string, 0, m.height)
	if l.tabBar {
		rows = append(rows, m.renderTabBar())
	}
	body := m.renderBody(l)
	if m.helpShown() {
		body = m.overlayHelp(body)
	}
	rows = append(rows, body...)
	if l.statusBar {
		rows = append(rows, m.renderStatus())
	}
	return strings.Join(rows, "\n")
}

func (m *Model) renderBody(l layout) []string {
	if l.bodyH <= 0 {
		return nil
	}
	cur := m.focused()
	cols := make([][]string, len(l.boxes))
	for i, b := range l.boxes {
		focused := b.pane == cur
		var v string
		switch b.pane {
		case paneSidebar:
			v = m.sidebar.View(focused)
		case paneDiff:
			v = m.diff.View(focused)
		case paneChat:
			v = m.chat.View(focused)
		}
		cols[i] = lines(v, b.w, l.bodyH)
	}
	seps := make([]string, max(len(l.boxes)-1, 0))
	for i := range seps {
		st := m.st.sep
		if l.boxes[i].pane == cur || l.boxes[i+1].pane == cur {
			st = m.st.sepActive
		}
		seps[i] = st.Render("│")
	}

	out := make([]string, l.bodyH)
	var b strings.Builder
	for y := range out {
		b.Reset()
		for i := range cols {
			if i > 0 {
				b.WriteString(seps[i-1])
			}
			b.WriteString(cols[i][y])
		}
		out[y] = fit(b.String(), m.width)
	}
	return out
}
