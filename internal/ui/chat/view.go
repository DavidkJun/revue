package chat

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

const (
	margin = " "   // left and right edge of the panel
	indent = "   " // margin plus the width of the "❯ " marker

	icon       = "󰚩"
	toolIcon   = ""
	chipIcon   = "◆"
	errorIcon  = "✗"
	maxChips   = 6 // per turn in the conversation
	inputChips = 3 // above the prompt input
)

var spinner = [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m *Model) view(focused bool) string {
	if m.height <= 0 {
		return ""
	}
	out := make([]string, 0, m.height)
	out = append(out, m.header(focused))
	if m.height > 1 {
		input := m.inputView()
		out = append(out, m.bodyView(m.height-1-len(input))...)
		out = append(out, input...)
	}
	return strings.Join(out, "\n")
}

func (m *Model) header(focused bool) string {
	w := m.width
	title := fg(m.th.Muted)
	if focused {
		title = fg(m.th.Accent).Bold(true)
	}
	model := m.model
	if model == "" {
		model = "default"
	}
	left := margin + title.Render(icon+" Claude") + fg(m.th.Subtle).Render(" · ") + fg(m.th.Muted).Render(model)

	var right string
	switch {
	case m.active != nil:
		phase := "thinking…"
		if m.active.state == stateAnswering {
			phase = "answering…"
		}
		right = fg(m.th.Accent).Render(spinner[m.frame%len(spinner)]) + fg(m.th.Muted).Render(" "+phase)
	case len(m.turns) == 1:
		right = fg(m.th.Muted).Render("1 turn")
	case len(m.turns) > 1:
		right = fg(m.th.Muted).Render(fmt.Sprintf("%d turns", len(m.turns)))
	}
	if right != "" {
		right += margin
		if gap := w - ansi.StringWidth(left) - ansi.StringWidth(right); gap >= 1 {
			return fit(left+strings.Repeat(" ", gap)+right, w)
		}
	}
	return fit(left, w)
}

// bodyHeight is how many conversation lines fit between the header and the
// input.
func (m *Model) bodyHeight() int {
	return max(m.height-1-len(m.inputView()), 0)
}

func (m *Model) maxOffset() int {
	return max(len(m.body())-m.bodyHeight(), 0)
}

func (m *Model) scrollTo(off int) {
	maxOff := m.maxOffset()
	m.offset = min(max(off, 0), maxOff)
	m.follow = m.offset >= maxOff
}

// jumpTo shows line off at the top (gg, N G) and keeps it there as the
// answer grows. Scrolling onto the last line follows again, a jump does not:
// with everything on screen, the top line is also the last one.
func (m *Model) jumpTo(off int) {
	m.scrollTo(off)
	m.follow = false
}

func (m *Model) bodyView(h int) []string {
	if h <= 0 {
		return nil
	}
	if len(m.turns) == 0 {
		return m.emptyView(h)
	}
	lines := m.body()
	maxOff := max(len(lines)-h, 0)
	if m.follow {
		m.offset = maxOff
	}
	m.offset = min(max(m.offset, 0), maxOff)
	out := make([]string, 0, h)
	out = append(out, lines[m.offset:min(m.offset+h, len(lines))]...)
	for len(out) < h {
		out = append(out, fit("", m.width))
	}
	return out
}

func (m *Model) emptyView(h int) []string {
	w := m.width
	hint, keyWords := "Select lines with v and press a to ask Claude", []string{"v", "a"}
	if m.prompting {
		hint, keyWords = "Claude can read the repos for more context", nil
	}
	muted, key := fg(m.th.Muted), fg(m.th.Accent).Bold(true)
	content := []string{muted.Render(center(icon, w)), ""}
	for _, l := range wrapText(hint, max(w-4, 1)) {
		var b, run strings.Builder
		flush := func() {
			b.WriteString(muted.Render(run.String()))
			run.Reset()
		}
		for i, word := range strings.Split(l, " ") {
			if i > 0 {
				run.WriteByte(' ')
			}
			if slices.Contains(keyWords, word) {
				flush()
				b.WriteString(key.Render(word))
			} else {
				run.WriteString(word)
			}
		}
		flush()
		content = append(content, strings.Repeat(" ", max((w-ansi.StringWidth(l))/2, 0))+b.String())
	}
	// Short panels lose the icon first; half a sentence reads worse than
	// none, so a hint that still does not fit goes too.
	if len(content) > h {
		content = content[2:]
	}
	if len(content) > h {
		content = nil
	}
	out := make([]string, 0, h)
	for range max((h-len(content))/2, 0) {
		out = append(out, fit("", w))
	}
	for _, l := range content {
		out = append(out, fit(l, w))
	}
	for len(out) < h {
		out = append(out, fit("", w))
	}
	return out
}

// body returns the whole conversation laid out at the panel width.
func (m *Model) body() []string {
	if !m.dirty && m.linesWidth == m.width {
		return m.lines
	}
	w := m.width
	blank := fit("", w)
	lines := []string{blank}
	starts := make([]int, len(m.turns))
	for i, t := range m.turns {
		starts[i] = len(lines)
		if i > 0 {
			lines = append(lines, blank, m.separator(), blank)
		}
		lines = append(lines, m.turnLines(t)...)
	}
	lines = append(lines, blank)
	if m.linesWidth != w && !m.follow {
		m.offset = reanchor(m.offset, m.starts, len(m.lines), starts, len(lines))
	}
	m.lines, m.starts, m.linesWidth, m.dirty = lines, starts, w, false
	return lines
}

// reanchor maps a scroll offset from one layout of the turns to another (a
// new width reflows every line), keeping it in the same turn at the same
// relative position, so a reader stays where they were.
func reanchor(off int, oldStarts []int, oldLen int, newStarts []int, newLen int) int {
	k := -1
	for i, s := range oldStarts {
		if s <= off {
			k = i
		}
	}
	if k < 0 || k >= len(newStarts) {
		return off
	}
	span := func(starts []int, total int) (int, int) {
		if k+1 < len(starts) {
			return starts[k], starts[k+1]
		}
		return starts[k], total
	}
	from, to := span(oldStarts, oldLen)
	nfrom, nto := span(newStarts, newLen)
	frac := float64(off-from) / float64(max(to-from, 1))
	return nfrom + int(math.Round(frac*float64(nto-nfrom)))
}

func (m *Model) separator() string {
	return fit(margin+fg(m.th.Subtle).Render(strings.Repeat("─", max(m.width-2, 0))), m.width)
}

// textWidth is the width of answer text, which sits under the question
// text, right of the "❯ " marker.
func (m *Model) textWidth() int { return max(m.width-2*len(margin)-2, 1) }

func (m *Model) turnLines(t *turn) []string {
	w := m.width
	if t.lines != nil && t.linesWidth == w {
		return t.lines
	}
	tw := m.textWidth()
	blank := fit("", w)
	var lines []string
	add := func(s string) { lines = append(lines, fit(s, w)) }

	q := fg(m.th.Accent).Bold(true)
	for i, l := range wrapText(t.question, tw) {
		if i == 0 {
			add(margin + q.Render(promptMarker+l))
		} else {
			add(indent + q.Render(l))
		}
	}
	lines = append(lines, m.chipLines(t.chips, maxChips)...)

	prev := segKind(255)
	for _, s := range t.segs {
		switch s.kind {
		case segTool:
			if prev != segTool {
				lines = append(lines, blank)
			}
			add(indent + fg(m.th.Muted).Render(toolIcon+" "+toolText(s.text, tw-2)))
		case segText:
			r := m.segmentLines(t, s)
			if len(r) == 0 {
				continue
			}
			lines = append(lines, blank)
			lines = append(lines, r...)
		}
		prev = s.kind
	}

	switch t.state {
	case stateDone:
		lines = append(lines, blank)
		add(indent + fg(m.th.Subtle).Render(footer(t)))
	case stateCancelled:
		lines = append(lines, blank)
		add(indent + fg(m.th.Muted).Render("cancelled"))
	case stateError:
		lines = append(lines, blank)
		red := fg(m.th.Red)
		for i, l := range wrapText(errorText(t.err), max(tw-2, 1)) {
			if i == 0 {
				add(indent + red.Render(errorIcon+" "+l))
			} else {
				add(indent + "  " + red.Render(l))
			}
		}
	default:
		if t.state != stateAnswering || prev != segText {
			lines = append(lines, blank)
			add(indent + fg(m.th.Accent).Render(spinner[m.frame%len(spinner)]) + fg(m.th.Muted).Render(" thinking…"))
		}
	}
	t.lines, t.linesWidth = lines, w
	return lines
}

// segmentLines returns the rendered markdown of s. While its turn streams,
// a stale rendering is kept until the next tick re-renders it.
func (m *Model) segmentLines(t *turn, s *segment) []string {
	fresh := s.renderedWidth == m.width && s.renderedLen == len(s.text)
	stale := s.renderedWidth == m.width && t == m.active
	if !fresh && !stale {
		m.renderSegment(s)
	}
	return s.rendered
}

// renderSegment renders s at the text width and frames each line with the
// panel margins, so laying out the conversation only concatenates.
func (m *Model) renderSegment(s *segment) {
	lines, blocks := m.md.render(s.text, m.textWidth(), s.blocks)
	for i, l := range lines {
		lines[i] = fit(indent+l, m.width)
	}
	s.rendered, s.blocks = lines, blocks
	s.renderedWidth, s.renderedLen = m.width, len(s.text)
}

func (m *Model) chipLines(chips []string, limit int) []string {
	if len(chips) == 0 {
		return nil
	}
	tw := m.textWidth()
	st := fg(m.th.Cyan).Faint(true)
	shown := chips
	if len(chips) > limit {
		shown = chips[:limit-1]
	}
	var lines []string
	for _, c := range shown {
		lines = append(lines, fit(indent+st.Render(chipIcon+" "+truncateLeft(c, tw-2)), m.width))
	}
	if len(shown) < len(chips) {
		lines = append(lines, fit(indent+st.Render(fmt.Sprintf("%s +%d more", chipIcon, len(chips)-len(shown))), m.width))
	}
	return lines
}

func footer(t *turn) string {
	model := t.model
	if model == "" {
		model = "default"
	}
	if t.duration <= 0 {
		return model
	}
	return model + " · " + formatDuration(t.duration)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	d = d.Round(time.Second)
	return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
}

// errorText drops the "claude: " prefix of the claude package's errors;
// the panel already says who failed.
func errorText(err error) string {
	if err == nil {
		return "failed"
	}
	return strings.TrimSpace(strings.TrimPrefix(err.Error(), "claude: "))
}

// toolText keeps the start of the tool name and the end of its argument,
// which for paths is the file name.
func toolText(s string, w int) string {
	name, arg, ok := strings.Cut(s, " ")
	if !ok || ansi.StringWidth(name)+2 > w {
		return truncate(s, w)
	}
	return name + " " + truncateLeft(arg, w-ansi.StringWidth(name)-1)
}
