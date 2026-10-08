package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// cellOf finds text in the plain frame and returns its column and row.
func cellOf(t *testing.T, frame, text string) (x, y int) {
	t.Helper()
	for y, line := range strings.Split(frame, "\n") {
		if i := strings.Index(line, text); i >= 0 {
			return len([]rune(line[:i])), y
		}
	}
	t.Fatalf("%q is not on screen:\n%s", text, frame)
	return 0, 0
}

func TestClickLandsOnTheRowUnderThePointer(t *testing.T) {
	for _, size := range [][2]int{{140, 40}, {100, 30}} {
		h := newHarness(t, size[0], size[1])
		x, y := cellOf(t, h.plain(), "app.module")
		h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		if _, p := h.current(); p != "src/app.module.ts" {
			t.Errorf("%v: click on app.module.ts at %d,%d selected %q", size, x, y, p)
		}
		if h.m.shown != key(0, "src/app.module.ts") {
			t.Errorf("%v: diff shows %+v", size, h.m.shown)
		}

		x, y = cellOf(t, h.plain(), "atlas-front-end")
		if y != 0 {
			t.Fatalf("front-end tab on row %d", y)
		}
		h.send(tea.MouseClickMsg{X: x, Y: 0, Button: tea.MouseLeft})
		if r, _ := h.current(); r != 1 {
			t.Errorf("%v: click on the front-end tab selected repo %d", size, r)
		}
	}
}

func TestClickOnSeparatorOrBarsKeepsFocus(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("space", "a", "ctrl+h") // diff focused, chat open
	l := h.m.layout()
	d, _ := l.find(paneDiff)
	for _, c := range [][2]int{
		{d.x - 1, 10},                // separator left of the diff
		{d.x + d.w, 10},              // separator right of the diff
		{d.x + 2, 39},                // statusline
		{d.x + 2, l.bodyY + l.bodyH}, // below the body
	} {
		h.send(tea.MouseClickMsg{X: c[0], Y: c[1], Button: tea.MouseLeft})
		if h.m.focused() != paneDiff {
			t.Errorf("click at %d,%d moved the focus to %v", c[0], c[1], h.m.focused())
			h.keys("ctrl+l", "ctrl+h")
		}
	}
	c, _ := l.find(paneChat)
	h.send(tea.MouseClickMsg{X: c.x, Y: 10, Button: tea.MouseLeft})
	if h.m.focused() != paneChat {
		t.Errorf("click on the chat's first column: focus %v", h.m.focused())
	}
}
