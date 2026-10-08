package chat

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/DavidkJun/revue/internal/theme"
)

const (
	maxInputLines = 6
	promptMarker  = "❯ "
	// maxInputRows bounds what can be typed or pasted, in wrapped rows; the
	// textarea otherwise stops accepting input at maxInputLines lines.
	maxInputRows = 10000
)

func newInput(th theme.Theme) textarea.Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.EndOfBufferCharacter = ' '
	ta.CharLimit = 0
	ta.DynamicHeight = true
	ta.MinHeight = 1
	ta.MaxHeight = maxInputLines
	ta.MaxContentHeight = maxInputRows
	ta.SetPromptFunc(lipgloss.Width(promptMarker), func(p textarea.PromptInfo) string {
		if p.LineNumber == 0 {
			return promptMarker
		}
		return strings.Repeat(" ", lipgloss.Width(promptMarker))
	})
	// The panel handles enter (send) itself; a newline needs a modifier.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "alt+enter"))
	// The panel reads the clipboard itself (see paste), so its line ends
	// are fixed like those of a terminal paste.
	ta.KeyMap.Paste.SetEnabled(false)

	text := fg(th.FG)
	state := textarea.StyleState{
		Base:        lipgloss.NewStyle(),
		Text:        text,
		CursorLine:  text,
		Placeholder: fg(th.Muted),
		Prompt:      fg(th.Accent).Bold(true),
		EndOfBuffer: fg(th.Muted),
		Selection:   lipgloss.NewStyle().Background(lipgloss.Color(th.VisualBG)),
	}
	// A static cursor: blinking needs blink messages routed back through the
	// app, and a steady block reads fine.
	ta.SetStyles(textarea.Styles{
		Focused: state,
		Blurred: state,
		Cursor:  textarea.CursorStyle{Color: lipgloss.Color(th.Accent), Shape: tea.CursorBlock},
	})
	ta.SetVirtualCursor(true)
	return ta
}

// updateInput handles a message while prompting.
func (m *Model) updateInput(msg tea.Msg) tea.Cmd {
	switch k := msg.(type) {
	case tea.KeyPressMsg:
		switch k.String() {
		case "enter":
			return m.submit()
		case "esc", "ctrl+c":
			m.closePrompt()
			return nil
		case "ctrl+v":
			return paste(m.readClipboard)
		}
	case tea.PasteMsg:
		// Windows clipboards (and WezTerm's paste there) use CRLF, which
		// the textarea would turn into two line breaks.
		k.Content = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(k.Content)
		msg = k
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd
}

// paste reads the clipboard for ctrl+v. The text comes back as a paste,
// which the app routes here while the prompt is open; an unreadable or
// empty clipboard pastes nothing, as in the textarea.
func paste(read func() (string, error)) tea.Cmd {
	return func() tea.Msg {
		s, err := read()
		if err != nil || s == "" {
			return nil
		}
		return tea.PasteMsg{Content: s}
	}
}

func (m *Model) submit() tea.Cmd {
	q := strings.TrimSpace(m.input.Value())
	if q == "" {
		return nil
	}
	items, chips := m.items, m.chips
	m.closePrompt()
	return m.send(q, items, chips)
}

func (m *Model) closePrompt() {
	m.prompting = false
	m.items, m.chips = nil, nil
	m.input.Reset()
	m.input.Blur()
}

func (m *Model) placeholder(hasItems bool) string {
	switch {
	case hasItems:
		return "Ask Claude about this…"
	case len(m.turns) > 0:
		return "Ask a follow-up…"
	default:
		return "Ask Claude…"
	}
}

// inputView is the prompt area at the bottom: a rule, the context chips,
// the text input and a key hint. Parts are dropped, least useful first,
// when the panel is too short for all of them.
func (m *Model) inputView() []string {
	if !m.prompting || m.height <= 1 {
		return nil
	}
	w := m.width
	avail := m.height - 1
	rule := []string{fit(margin+fg(m.th.Muted).Render(strings.Repeat("─", max(w-2, 0))), w)}
	chips := m.chipLines(m.chips, inputChips)
	var text []string
	for _, l := range strings.Split(strings.TrimRight(m.input.View(), "\n"), "\n") {
		text = append(text, fit(margin+l, w))
	}
	text = text[:min(len(text), max(m.input.Height(), 1))]
	hint := []string{m.hint()}

	// Keep a few conversation lines visible when the panel is tall enough.
	room := avail
	if avail >= 8 {
		room = avail - 3
	}
	parts := [][]string{rule, chips, text, hint}
	for _, drop := range []int{3, 1, 0} {
		if count(parts) <= room {
			break
		}
		parts[drop] = nil
	}
	out := make([]string, 0, avail)
	for _, p := range parts {
		out = append(out, p...)
	}
	if len(out) > avail {
		out = out[len(out)-avail:]
	}
	return out
}

func count(parts [][]string) int {
	n := 0
	for _, p := range parts {
		n += len(p)
	}
	return n
}

// hint picks the longest key hint that fits.
func (m *Model) hint() string {
	variants := [][][2]string{
		{{"enter", "send"}, {"shift+enter", "newline"}, {"esc", "cancel"}},
		{{"enter", "send"}, {"⇧enter", "newline"}, {"esc", "cancel"}},
		{{"enter", "send"}, {"esc", "cancel"}},
		{{"enter", "send"}},
	}
	key := fg(m.th.Muted).Bold(true)
	label := fg(m.th.Muted)
	sep := fg(m.th.Subtle).Render(" · ")
	for _, v := range variants {
		var plain, styled []string
		for _, kv := range v {
			plain = append(plain, kv[0]+" "+kv[1])
			styled = append(styled, key.Render(kv[0])+label.Render(" "+kv[1]))
		}
		if lipgloss.Width(strings.Join(plain, " · ")) <= m.width-len(indent)-len(margin) {
			return fit(indent+strings.Join(styled, sep), m.width)
		}
	}
	return fit("", m.width)
}
