package ui

import (
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/ui/keys"
)

// pendingTimeout is how long a started key sequence ("g", "<space>") waits
// for its next key, like Vim's timeoutlen.
const pendingTimeout = time.Second

type pendingTimeoutMsg struct{ seq int }

// searchState is the "/" input in the statusline.
type searchState struct {
	active  bool
	input   string
	matches int
	prev    string // pattern to restore on esc
	line    int    // cursor line to return to on esc
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	switch {
	case m.helpShown():
		return m.helpKey(k)
	case m.search.active:
		return m.searchKey(k)
	case m.chat.Prompting():
		return m.chat.Update(k)
	case modifierOnly(k):
		return nil
	}
	res := m.resolver.Feed(keys.Name(k), m.scopes()...)
	if res.Pending {
		seq := m.resolver.Seq()
		return tea.Tick(pendingTimeout, func(time.Time) tea.Msg { return pendingTimeoutMsg{seq} })
	}
	if res.Action == "" {
		return nil
	}
	return m.action(res.Action, res.Count)
}

// modifierOnly reports a press of Shift, Ctrl, Alt, Super or a lock key on
// its own. win32-input-mode reports those before the key they modify; fed to
// the resolver they would drop a pending count or sequence ("10G", "zR").
func modifierOnly(k tea.KeyPressMsg) bool {
	switch k.Code {
	case tea.KeyLeftShift, tea.KeyRightShift, tea.KeyLeftCtrl, tea.KeyRightCtrl,
		tea.KeyLeftAlt, tea.KeyRightAlt, tea.KeyLeftSuper, tea.KeyRightSuper,
		tea.KeyLeftHyper, tea.KeyRightHyper, tea.KeyLeftMeta, tea.KeyRightMeta,
		tea.KeyIsoLevel3Shift, tea.KeyIsoLevel5Shift,
		tea.KeyCapsLock, tea.KeyNumLock, tea.KeyScrollLock:
		return true
	}
	return false
}

func (m *Model) handlePaste(p tea.PasteMsg) tea.Cmd {
	if runtime.GOOS == "windows" {
		p.Content = undoubleNewlines(p.Content)
	}
	switch {
	case m.search.active:
		// The pattern is one line, like Vim's.
		line, _, _ := strings.Cut(p.Content, "\n")
		m.setSearchInput(m.search.input + strings.TrimRight(line, "\r"))
	case m.chat.Prompting():
		return m.chat.Update(p)
	}
	return nil
}

// scopes is the binding scope stack of the focused pane, most specific
// first.
func (m *Model) scopes() []keys.Scope {
	switch m.focused() {
	case paneSidebar:
		return []keys.Scope{keys.Sidebar, keys.Nav}
	case paneChat:
		return []keys.Scope{keys.Chat, keys.Nav}
	}
	if m.diff.Visual() {
		return []keys.Scope{keys.Visual, keys.Diff, keys.Nav}
	}
	return []keys.Scope{keys.Diff, keys.Nav}
}

func (m *Model) startSearch() {
	line := 0
	if loc, ok := m.diff.Cursor(); ok {
		line = loc.NewLine
	}
	m.search = searchState{active: true, prev: m.search.prev, line: line}
}

func (m *Model) searchKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "enter":
		m.search.active = false
		m.search.prev = m.search.input
		switch {
		case m.search.input == "":
			m.diff.SetSearch("")
		case m.search.matches == 0:
			return m.notify("pattern not found: "+m.search.input, true)
		}
	case "esc", "ctrl+c":
		m.search.active = false
		m.diff.SetSearch(m.search.prev)
		if m.search.line > 0 {
			m.diff.HandleAction(keys.Top, m.search.line)
		}
	case "backspace", "ctrl+h":
		if m.search.input == "" {
			m.search.active = false
			m.diff.SetSearch(m.search.prev)
			return nil
		}
		_, size := utf8.DecodeLastRuneInString(m.search.input)
		m.setSearchInput(m.search.input[:len(m.search.input)-size])
	case "ctrl+w":
		m.setSearchInput(dropLastWord(m.search.input))
	case "ctrl+u":
		m.setSearchInput("")
	default:
		// Text, not the key name: a Cyrillic pattern must stay Cyrillic.
		// Control combinations carry no text.
		if k.Text != "" {
			m.setSearchInput(m.search.input + k.Text)
		}
	}
	return nil
}

// setSearchInput searches as the pattern is typed. Like Vim's incsearch,
// every edit searches again from where the search started, so backspacing
// goes back to the earlier match.
func (m *Model) setSearchInput(s string) {
	m.search.input = s
	if m.search.line > 0 {
		m.diff.HandleAction(keys.Top, m.search.line)
	}
	m.search.matches = m.diff.SetSearch(s)
}

// undoubleNewlines repairs a paste that came through conhost in
// win32-input-mode: the CR and the LF of every CRLF arrive as two Enter
// presses, which the input decoder turns into two "\n". With bracketed paste
// WezTerm passes the clipboard's CRLF through unchanged, so every line break
// doubles. Such a paste has only even runs of "\n"; a single break means the
// text was not doubled and is left alone.
func undoubleNewlines(s string) string {
	if strings.Contains(s, "\r") || !strings.Contains(s, "\n\n") {
		return s
	}
	var b strings.Builder
	for rest := s; len(rest) > 0; {
		i := strings.IndexByte(rest, '\n')
		if i < 0 {
			b.WriteString(rest)
			break
		}
		n := len(rest[i:]) - len(strings.TrimLeft(rest[i:], "\n"))
		if n%2 == 1 {
			return s
		}
		b.WriteString(rest[:i])
		b.WriteString(strings.Repeat("\n", n/2))
		rest = rest[i+n:]
	}
	return b.String()
}

func dropLastWord(s string) string {
	r := []rune(s)
	i := len(r)
	for i > 0 && unicode.IsSpace(r[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(r[i-1]) {
		i--
	}
	return string(r[:i])
}

func (m *Model) helpKey(k tea.KeyPressMsg) tea.Cmd {
	switch keys.Name(k) {
	case "?", "q", "esc", "ctrl+c":
		m.help = helpState{}
	case "j", "down", "ctrl+e":
		m.help.scroll++
	case "k", "up", "ctrl+y":
		m.help.scroll--
	case "ctrl+d", "ctrl+f", "pgdown", "space":
		m.help.scroll += max(1, m.layout().bodyH/2)
	case "ctrl+u", "ctrl+b", "pgup":
		m.help.scroll -= max(1, m.layout().bodyH/2)
	case "g", "home":
		m.help.scroll = 0
	case "G", "end":
		m.help.scroll = 1 << 30
	}
	m.help.scroll = clamp(m.help.scroll, 0, m.helpBox(m.layout().bodyH).maxScroll)
	return nil
}
