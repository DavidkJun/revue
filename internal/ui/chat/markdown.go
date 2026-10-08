package chat

import (
	"strconv"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"

	"github.com/DavidkJun/revue/internal/theme"
)

// minMarkdownWidth keeps glamour away from degenerate wrap widths; narrower
// panels get the result cut to size.
const minMarkdownWidth = 12

// maxRenderers bounds the per-width renderer cache across many resizes.
const maxRenderers = 8

// markdown renders answers with glamour. Building a renderer parses the
// whole style config, so one is kept per wrap width.
type markdown struct {
	th        theme.Theme
	style     ansi.StyleConfig
	renderers map[int]*glamour.TermRenderer
	renders   int // number of renders, for tests of the streaming throttle
}

func newMarkdown(th theme.Theme) *markdown {
	base, ok := styles.DefaultStyles[th.MarkdownStyle]
	if !ok {
		base = styles.DefaultStyles[styles.DarkStyle]
	}
	// A shallow copy is enough: only top-level fields are replaced, the
	// shared pointers inside are never written through.
	cfg := *base
	var zero uint
	cfg.Document.Margin = &zero
	cfg.Document.BlockPrefix = ""
	cfg.Document.BlockSuffix = ""
	// Top-level fences are rendered by renderCode; the ones glamour still
	// sees (nested in lists) get the diff pane's chroma style as well.
	// Glamour's own chroma config would also be registered process-wide
	// under a fixed name by whoever renders first.
	cfg.CodeBlock.Chroma = nil
	cfg.CodeBlock.Theme = th.SyntaxStyle
	return &markdown{th: th, style: cfg, renderers: map[int]*glamour.TermRenderer{}}
}

// renderedBlock caches one block of a previous render.
type renderedBlock struct {
	mdBlock
	width int
	lines []string
}

// render returns src as lines exactly width cells wide, without leading or
// trailing blank lines and with runs of blank lines collapsed. Blocks equal
// to those in cache are reused: while an answer streams only its last block
// changes. The returned cache is for the next render of the same text.
func (md *markdown) render(src string, width int, cache []renderedBlock) ([]string, []renderedBlock) {
	if width <= 0 || strings.TrimSpace(src) == "" {
		return nil, nil
	}
	md.renders++
	var lines []string
	var blocks []renderedBlock
	for i, b := range splitFences(strings.ReplaceAll(src, "\r\n", "\n")) {
		var part []string
		switch {
		case i < len(cache) && cache[i].mdBlock == b && cache[i].width == width:
			part = cache[i].lines
		case b.code:
			part = md.renderCode(b.text, b.lang, width, b.indent)
		default:
			part = md.renderText(b.text, width)
		}
		blocks = append(blocks, renderedBlock{mdBlock: b, width: width, lines: part})
		if len(part) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, fit("", width))
		}
		lines = append(lines, part...)
	}
	return lines, blocks
}

func (md *markdown) renderText(src string, width int) []string {
	out, err := md.glamour(src, width)
	// Glamour can wrap punctuation that follows styled text ("`code`,")
	// one cell past its width, and fit would cut that punctuation off. A
	// slightly narrower wrap keeps every character.
	for w := width - 1; err == nil && !fits(out, width) && w >= max(width-3, minMarkdownWidth); w-- {
		out, err = md.glamour(src, w)
	}
	if err != nil {
		out = strings.Join(wrapText(src, width), "\n")
	}
	var lines []string
	blank := true // drops leading blank lines
	for _, l := range strings.Split(stripBackground(out), "\n") {
		l = trimRight(l)
		if l == "" {
			if !blank {
				lines = append(lines, "")
			}
			blank = true
			continue
		}
		blank = false
		lines = append(lines, fit(l, width))
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, l := range lines {
		if l == "" {
			lines[i] = strings.Repeat(" ", width)
		}
	}
	return lines
}

func (md *markdown) glamour(src string, width int) (string, error) {
	width = max(width, minMarkdownWidth)
	r, ok := md.renderers[width]
	if !ok {
		var err error
		r, err = glamour.NewTermRenderer(
			glamour.WithStyles(md.style),
			glamour.WithWordWrap(width),
			glamour.WithChromaFormatter("terminal16m"),
		)
		if err != nil {
			return "", err
		}
		if len(md.renderers) >= maxRenderers {
			clear(md.renderers)
		}
		md.renderers[width] = r
	}
	return r.Render(src)
}

// stripBackground removes background colours (and reverse video, which
// paints one) from every SGR sequence. Styles and chroma themes may set
// them, but the panel must stay transparent over the user's blurred
// terminal background.
func stripBackground(s string) string {
	if !strings.Contains(s, "\x1b[") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for {
		i := strings.Index(s, "\x1b[")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		rest := s[i+2:]
		j := 0
		for j < len(rest) && (rest[j] < 0x40 || rest[j] > 0x7e) {
			j++
		}
		if j == len(rest) {
			b.WriteString(s[i:])
			return b.String()
		}
		if rest[j] != 'm' {
			b.WriteString(s[i : i+2+j+1])
		} else if p, keep := filterSGR(rest[:j]); keep {
			b.WriteString("\x1b[" + p + "m")
		}
		s = rest[j+1:]
	}
}

// filterSGR drops the background parameters of one SGR sequence. keep is
// false when nothing but background was set, so the sequence can go.
func filterSGR(params string) (string, bool) {
	if params == "" {
		return "", true
	}
	parts := strings.Split(params, ";")
	out := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if strings.Contains(p, ":") {
			if !strings.HasPrefix(p, "48:") {
				out = append(out, p)
			}
			continue
		}
		n, _ := strconv.Atoi(p)
		switch {
		case n == 38 || n == 48 || n == 58:
			args := 0
			if i+1 < len(parts) {
				switch parts[i+1] {
				case "5":
					args = 2
				case "2":
					args = 4
				}
			}
			args = min(args, len(parts)-1-i)
			if n != 48 {
				out = append(out, parts[i:i+args+1]...)
			}
			i += args
		case n >= 40 && n <= 47, n >= 100 && n <= 107, n == 49, n == 7:
		default:
			out = append(out, p)
		}
	}
	return strings.Join(out, ";"), len(out) > 0
}
