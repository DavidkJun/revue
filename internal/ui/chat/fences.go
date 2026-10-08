package chat

import "strings"

// mdBlock is a run of markdown, or one fenced code block taken out of it.
type mdBlock struct {
	code   bool
	lang   string
	text   string
	indent int // of the fence: the item text column when it sits in a list
}

// splitFences cuts the top-level fenced code blocks out of markdown, so
// they can be wrapped with a hanging indent instead of glamour's wrap,
// which restarts long code lines at column 0. An unclosed fence (a
// streaming answer) runs to the end. Fences indented up to three spaces
// (the text column of "1. " and "- " items) are taken out with their
// indent; deeper ones belong to nested lists and stay with the markdown.
func splitFences(src string) []mdBlock {
	var blocks []mdBlock
	var text, code []string
	var fence string
	var lang string
	var indent int
	flushText := func() {
		if t := strings.Join(text, "\n"); strings.TrimSpace(t) != "" {
			blocks = append(blocks, mdBlock{text: t})
		}
		text = nil
	}
	for _, line := range strings.Split(src, "\n") {
		if fence == "" {
			if f, info, n, ok := openFence(line); ok {
				flushText()
				fence, indent, code = f, n, nil
				lang, _, _ = strings.Cut(info, " ")
				continue
			}
			text = append(text, line)
			continue
		}
		if closesFence(line, fence) {
			blocks = append(blocks, mdBlock{code: true, lang: lang, text: strings.Join(code, "\n"), indent: indent})
			fence = ""
			continue
		}
		code = append(code, dedent(line, indent))
	}
	if fence != "" {
		blocks = append(blocks, mdBlock{code: true, lang: lang, text: strings.Join(code, "\n"), indent: indent})
	}
	flushText()
	return blocks
}

// openFence recognises a CommonMark fence opening: up to three spaces, then
// at least three backticks or tildes. Backtick fences cannot have backticks
// in their info string.
func openFence(line string) (fence, info string, indent int, ok bool) {
	indent = len(line) - len(strings.TrimLeft(line, " "))
	if indent > 3 {
		return "", "", 0, false
	}
	rest := line[indent:]
	if rest == "" || (rest[0] != '`' && rest[0] != '~') {
		return "", "", 0, false
	}
	n := len(rest) - len(strings.TrimLeft(rest, rest[:1]))
	if n < 3 {
		return "", "", 0, false
	}
	info = strings.TrimSpace(rest[n:])
	if rest[0] == '`' && strings.Contains(info, "`") {
		return "", "", 0, false
	}
	return rest[:n], info, indent, true
}

func closesFence(line, fence string) bool {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return false
	}
	n := len(t) - len(strings.TrimLeft(t, fence[:1]))
	return n >= len(fence) && strings.TrimSpace(t[n:]) == ""
}

// dedent removes up to n leading spaces, as CommonMark does for the
// content of an indented fence.
func dedent(line string, n int) string {
	for i := 0; i < n && strings.HasPrefix(line, " "); i++ {
		line = line[1:]
	}
	return line
}
