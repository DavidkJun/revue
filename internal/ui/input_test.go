package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPendingKeysDoNotFollowTheFocus(t *testing.T) {
	// "]" waits for "c" in the diff; after a click on the tree the next key
	// belongs to the tree, not to "] j".
	h := newHarness(t, 140, 40)
	h.keys("ctrl+l", "]")
	if !h.m.resolver.Busy() {
		t.Fatal("] is not pending in the diff")
	}
	s, _ := h.m.layout().find(paneSidebar)
	h.send(tea.MouseClickMsg{X: s.x + 2, Y: h.m.layout().bodyY, Button: tea.MouseLeft}) // header row
	if h.m.focused() != paneSidebar {
		t.Fatalf("focus = %v after clicking the tree", h.m.focused())
	}
	before := h.m.shown
	h.keys("j")
	if r, p := h.current(); h.m.shown == before || p == "" {
		t.Errorf("j after the click did not move the tree: repo %d %q, shown %+v", r, p, h.m.shown)
	}

	// The same for a count: "5" typed in the tree does not reach the chat.
	h = newHarness(t, 140, 40)
	h.keys("space", "a", "ctrl+h", "y")
	c, _ := h.m.layout().find(paneChat)
	h.send(tea.MouseClickMsg{X: c.x + 2, Y: 5, Button: tea.MouseLeft})
	h.keys("y")
	if !strings.Contains(h.m.notice.text, "No answer to yank yet") {
		t.Errorf("y in the chat after a pending y in the diff: notice %q", h.m.notice.text)
	}
}

func TestLoneModifierKeysDoNotResetPendingKeys(t *testing.T) {
	// In win32-input-mode a Shift press arrives on its own before the
	// shifted key: "1 0 <shift> G" is still 10G, "z <shift> R" still zR.
	shift := tea.KeyPressMsg{Code: tea.KeyLeftShift, Mod: tea.ModShift}
	h := newHarness(t, 140, 40)
	h.keys("ctrl+l", "1", "0")
	h.send(shift)
	h.keys("G")
	if line, _ := h.m.diff.Position(); line != 10 {
		t.Errorf("10<shift>G went to line %d", line)
	}
	h.keys("z")
	h.send(shift)
	if !h.m.resolver.Busy() {
		t.Error("<shift> dropped the pending z")
	}
}

func TestPasteGoesToThePrompt(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("a")
	if !h.m.chat.Prompting() {
		t.Fatal("a did not open the prompt")
	}
	// "why is this\r\nasync?" as it arrives through conhost in
	// win32-input-mode: CR and LF each become an Enter, each Enter a "\n".
	h.send(tea.PasteMsg{Content: "why is this\n\nasync?"})
	rows := strings.Split(h.plain(), "\n")
	for i, r := range rows {
		if strings.Contains(r, "why is this") {
			if i+1 == len(rows) || !strings.Contains(rows[i+1], "async?") {
				t.Errorf("the pasted line break doubled:\n%s", strings.Join(rows[i:min(i+3, len(rows))], "\n"))
			}
			return
		}
	}
	t.Errorf("pasted text is not in the prompt:\n%s", h.plain())
}

func TestUndoubleNewlines(t *testing.T) {
	cases := map[string]string{
		"a\n\nb":          "a\nb",
		"a\n\n\n\nb\n\nc": "a\n\nb\nc",
		"a\nb\n\nc":       "a\nb\n\nc", // a single break: nothing was doubled
		"a\r\n\r\nb":      "a\r\n\r\nb",
		"no newline":      "no newline",
		"trailing\n\n":    "trailing\n",
	}
	for in, want := range cases {
		if got := undoubleNewlines(in); got != want {
			t.Errorf("undoubleNewlines(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHelpTooSmallToDrawDoesNotEatKeys(t *testing.T) {
	h := newHarness(t, 60, 4) // two body rows: no room for the box
	h.keys("?")
	before := h.m.shown
	h.keys("j")
	if h.m.shown == before {
		t.Errorf("j was swallowed by a help overlay nobody can see")
	}
}
