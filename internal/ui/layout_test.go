package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

func TestFrameSizeAtOddSizesInEveryState(t *testing.T) {
	sizes := [][2]int{
		{79, 23}, {200, 60}, {30, 10}, {81, 25}, {101, 31}, {47, 13}, {33, 9}, {21, 7}, {9, 3}, {7, 5}, {250, 70},
	}
	states := map[string]func(h *harness){
		"split":    func(h *harness) { h.send(keyPress("ctrl+shift+d")) },
		"visual":   func(h *harness) { h.keys("ctrl+l", "V", "j", "j") },
		"prompt":   func(h *harness) { h.keys("a") },
		"chatfull": func(h *harness) { h.keys("space", "a", "f") },
		"pending":  func(h *harness) { h.keys("1", "2", "g") },
		"notice":   func(h *harness) { h.send(uimsg.Notify{Text: strings.Repeat("a long notice ", 20), Error: true}) },
		"helpdown": func(h *harness) { h.keys("?", "G") },
		"nomatch":  func(h *harness) { h.keys("ctrl+l", "/", "z", "z", "z") },
		"clean":    func(h *harness) { h.keys("L", "L") },
	}
	for name, setup := range states {
		h := newHarness(t, 120, 40)
		setup(h)
		for _, s := range sizes {
			h.send(tea.WindowSizeMsg{Width: s[0], Height: s[1]})
			checkFrame(t, h.m.View().Content, s[0], s[1])
		}
		if t.Failed() {
			t.Fatalf("state %q", name)
		}
	}
}

func TestPromptStaysOnScreenWhenNarrow(t *testing.T) {
	for _, w := range []int{75, 70, 60, 45, 30} {
		h := newHarness(t, w, 24)
		h.keys("a")
		if !h.m.chat.Prompting() {
			t.Fatalf("width %d: a did not open the prompt", w)
		}
		if !h.m.layout().has(paneChat) || h.m.focused() != paneChat {
			var panes []pane
			for _, b := range h.m.layout().boxes {
				panes = append(panes, b.pane)
			}
			t.Errorf("width %d: typing into a prompt that is not on screen (panes %v, focus %v)", w, panes, h.m.focused())
		}
		h.frame()
	}
}

func TestChatShownWhenNarrowIsReachable(t *testing.T) {
	h := newHarness(t, 70, 24)
	h.keys("space", "a")
	if !h.m.layout().has(paneChat) || h.m.focused() != paneChat {
		t.Fatalf("<space>a at 70 columns: chat visible %v, focus %v", h.m.layout().has(paneChat), h.m.focused())
	}
	h.keys("ctrl+h")
	if h.m.focused() != paneDiff {
		t.Fatalf("ctrl+h from the chat: focus %v", h.m.focused())
	}
	h.keys("ctrl+h")
	if h.m.focused() != paneSidebar {
		t.Fatalf("ctrl+h from the diff: focus %v, want the tree", h.m.focused())
	}
	h.keys("ctrl+l", "ctrl+l")
	if h.m.focused() != paneChat || len(h.git.navigations()) != 0 {
		t.Errorf("ctrl+l back to the chat: focus %v, WezTerm navigations %v", h.m.focused(), h.git.navigations())
	}
}

func TestLayoutKeepsAFocusedChat(t *testing.T) {
	order := func(l layout) []pane {
		var out []pane
		for _, b := range l.boxes {
			out = append(out, b.pane)
		}
		return out
	}
	cases := []struct {
		w    int
		want []pane
	}{
		{170, []pane{paneSidebar, paneDiff, paneChat}},
		{70, []pane{paneDiff, paneChat}},
		{30, []pane{paneChat}},
	}
	for _, c := range cases {
		l := computeLayout(c.w, 30, true, true, false, true)
		total := len(l.boxes) - 1
		for _, b := range l.boxes {
			total += b.w
		}
		if got := order(l); !slices.Equal(got, c.want) || total != c.w {
			t.Errorf("width %d, chat focused: panes %v (total %d), want %v", c.w, got, total, c.want)
		}
	}
}
