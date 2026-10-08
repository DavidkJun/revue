package chat

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// fit makes s exactly w cells wide: cut when longer, padded with unstyled
// spaces when shorter. Styles and hyperlinks are closed before the padding so
// nothing leaks into the cells (or panes) that follow.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	sw := ansi.StringWidth(s)
	if sw > w {
		s = ansi.Truncate(s, w, "")
		sw = ansi.StringWidth(s)
	}
	if strings.IndexByte(s, '\x1b') >= 0 {
		if strings.Contains(s, "\x1b]8;") {
			s += "\x1b]8;;\a"
		}
		s += "\x1b[m"
	}
	if sw < w {
		s += strings.Repeat(" ", w-sw)
	}
	return s
}

// fits reports whether every rendered line is at most width cells wide.
func fits(out string, width int) bool {
	for _, l := range strings.Split(out, "\n") {
		if ansi.StringWidth(trimRight(l)) > width {
			return false
		}
	}
	return true
}

// trimRight drops trailing spaces and the SGR sequences around them. With
// backgrounds stripped, trailing spaces are invisible, and glamour styles
// every padding cell separately, which bloats each line tenfold.
func trimRight(s string) string {
	for {
		switch {
		case strings.HasSuffix(s, " "):
			s = s[:len(s)-1]
		case strings.HasSuffix(s, "m"):
			i := strings.LastIndex(s, "\x1b[")
			if i < 0 || !isSGRParams(s[i+2:len(s)-1]) {
				return s
			}
			s = s[:i]
		default:
			return s
		}
	}
}

func isSGRParams(p string) bool {
	for i := 0; i < len(p); i++ {
		if c := p[i]; (c < '0' || c > '9') && c != ';' && c != ':' {
			return false
		}
	}
	return true
}

// wrapText word-wraps plain text to w cells, breaking words that do not fit
// on a line of their own. Explicit newlines are kept.
func wrapText(s string, w int) []string {
	s = strings.ReplaceAll(s, "\t", "    ")
	if w < 1 {
		w = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(ansi.Wrap(para, w, ""), "\n")...)
	}
	return out
}

// truncateLeft keeps the end of a plain string, which for paths and
// "path:line" chips is the part worth reading.
func truncateLeft(s string, w int) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	if w <= 1 {
		return ansi.Truncate("…", w, "")
	}
	r := []rune(s)
	for i := range r {
		if ansi.StringWidth(string(r[i:])) <= w-1 {
			return "…" + string(r[i:])
		}
	}
	return "…"
}

// truncate cuts a plain string to w cells with an ellipsis.
func truncate(s string, w int) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	if w <= 1 {
		return ansi.Truncate("…", w, "")
	}
	return ansi.Truncate(s, w, "…")
}

// center pads s on the left so it sits in the middle of w cells.
func center(s string, w int) string {
	if gap := (w - ansi.StringWidth(s)) / 2; gap > 0 {
		return strings.Repeat(" ", gap) + s
	}
	return s
}

func fg(c string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(c)) }
