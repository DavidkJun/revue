package chat

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/highlight"
)

const (
	codeGutter = "▏ " // marks a code block without painting a background
	codeHang   = "  " // extra indent of a wrapped code line's continuation
	tabWidth   = 4
)

// renderCode highlights a fenced code block with the diff pane's chroma
// style and wraps long lines with a hanging indent. indent shifts the block
// right, under the text of the list item it belongs to.
func (md *markdown) renderCode(code, lang string, width, indent int) []string {
	indent = min(indent, width/4)
	gutter := strings.Repeat(" ", indent) + fg(md.th.Subtle).Render(codeGutter)
	codeW := max(width-indent-ansi.StringWidth(codeGutter), 1)

	var out []string
	for _, l := range md.highlight(code, lang) {
		// Continuations line up past the line's own indentation, like
		// Vim's breakindent, so they never read as the next statement.
		lead := min(len(l.plain)-len(strings.TrimLeft(l.plain, " ")), codeW/2)
		hang := strings.Repeat(" ", lead) + codeHang
		for i, r := range breakRows(l.plain, codeW, max(codeW-len(hang), 1)) {
			prefix := gutter
			if i > 0 {
				prefix += hang
			}
			out = append(out, fit(prefix+ansi.Cut(l.styled, r[0], r[1]), width))
		}
	}
	return out
}

type codeLine struct{ plain, styled string }

func (md *markdown) highlight(code, lang string) []codeLine {
	code = expandTabs(strings.TrimRight(code, "\n"))
	style := chromastyles.Get(md.th.SyntaxStyle)
	lexer := lexers.Fallback
	if lang != "" {
		if l := lexers.Get(lang); l != nil {
			lexer = l
		}
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, code+"\n")
	if err != nil {
		it = chroma.Literator(chroma.Token{Type: chroma.Text, Value: code})
	}
	// Same refinements as the diff pane, so code reads alike in both.
	it = chroma.Literator(highlight.Refine(lexer.Config().Name, it.Tokens())...)

	sgr := map[chroma.TokenType]string{}
	var lines []codeLine
	var plain, styled strings.Builder
	endLine := func() {
		lines = append(lines, codeLine{plain: plain.String(), styled: styled.String()})
		plain.Reset()
		styled.Reset()
	}
	for tok := it(); tok != chroma.EOF; tok = it() {
		seq, ok := sgr[tok.Type]
		if !ok {
			seq = entrySGR(style.Get(tok.Type))
			sgr[tok.Type] = seq
		}
		for i, part := range strings.Split(tok.Value, "\n") {
			if i > 0 {
				endLine()
			}
			if part == "" {
				continue
			}
			plain.WriteString(part)
			if seq == "" {
				styled.WriteString(part)
			} else {
				styled.WriteString(seq + part + "\x1b[m")
			}
		}
	}
	if plain.Len() > 0 {
		endLine()
	}
	return lines
}

// entrySGR turns a chroma style entry into an SGR sequence, foreground and
// attributes only.
func entrySGR(e chroma.StyleEntry) string {
	var p []string
	if e.Bold == chroma.Yes {
		p = append(p, "1")
	}
	if e.Italic == chroma.Yes {
		p = append(p, "3")
	}
	if e.Underline == chroma.Yes {
		p = append(p, "4")
	}
	if e.Colour.IsSet() {
		p = append(p, fmt.Sprintf("38;2;%d;%d;%d", e.Colour.Red(), e.Colour.Green(), e.Colour.Blue()))
	}
	if len(p) == 0 {
		return ""
	}
	return "\x1b[" + strings.Join(p, ";") + "m"
}

// breakRows splits a line into cell ranges no wider than first (then rest
// for the continuations), breaking after a space when one is in the last
// two thirds of the row.
func breakRows(s string, first, rest int) [][2]int {
	var rows [][2]int
	start, pos, limit, space := 0, 0, first, -1
	for s != "" {
		g, gw := ansi.FirstGraphemeCluster(s, ansi.GraphemeWidth)
		s = s[len(g):]
		if pos+gw-start > limit && pos > start {
			end := pos
			if space > start+limit/3 && pos+gw-space <= rest {
				end = space
			}
			rows = append(rows, [2]int{start, end})
			start, limit, space = end, rest, -1
		}
		pos += gw
		if g == " " {
			space = pos
		}
	}
	return append(rows, [2]int{start, pos})
}

func expandTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch r {
		case '\t':
			n := tabWidth - col%tabWidth
			b.WriteString(strings.Repeat(" ", n))
			col += n
		case '\n':
			b.WriteRune(r)
			col = 0
		default:
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}
