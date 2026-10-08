package ui

// pane identifies one of the three body panes, in their left-to-right order.
type pane int

const (
	paneSidebar pane = iota
	paneDiff
	paneChat
)

// Below this the diff pane is unusable, so side panes are dropped instead:
// the chat first, then the sidebar. A focused chat stays, since its prompt
// may be taking the keys; the sidebar goes first, then the diff.
const minDiffWidth = 20

// box is a visible pane and the columns it occupies.
type box struct {
	pane pane
	x, w int
}

// layout is where everything goes for one frame.
type layout struct {
	tabBar    bool // row 0 is the tab bar
	statusBar bool // the last row is the statusline
	bodyY     int
	bodyH     int
	boxes     []box // left to right, one separator column between neighbours
}

func computeLayout(width, height int, sidebar, chat, chatFull, chatFocused bool) layout {
	var l layout
	switch {
	case height >= 3:
		l.tabBar, l.statusBar = true, true
		l.bodyY, l.bodyH = 1, height-2
	case height == 2:
		l.statusBar = true
		l.bodyH = 1
	case height == 1:
		l.statusBar = true
	}
	if width <= 0 {
		return l
	}

	alone := []box{{pane: paneChat, w: width}}
	if chat && chatFull {
		l.boxes = alone
		return l
	}

	sw := clamp(width*28/100, 24, 44)
	cw := width * 2 / 5
	diffWidth := func() int {
		w := width
		if sidebar {
			w -= sw + 1
		}
		if chat {
			w -= cw + 1
		}
		return w
	}
	if chat && !chatFocused && diffWidth() < minDiffWidth {
		chat = false
	}
	if sidebar && diffWidth() < minDiffWidth {
		sidebar = false
	}
	if chat && diffWidth() < minDiffWidth {
		l.boxes = alone
		return l
	}

	x := 0
	if sidebar {
		l.boxes = append(l.boxes, box{pane: paneSidebar, x: x, w: sw})
		x += sw + 1
	}
	l.boxes = append(l.boxes, box{pane: paneDiff, x: x, w: diffWidth()})
	x += diffWidth() + 1
	if chat {
		l.boxes = append(l.boxes, box{pane: paneChat, x: x, w: cw})
	}
	return l
}

// has reports whether p is visible.
func (l layout) has(p pane) bool {
	_, ok := l.find(p)
	return ok
}

func (l layout) find(p pane) (box, bool) {
	for _, b := range l.boxes {
		if b.pane == p {
			return b, true
		}
	}
	return box{}, false
}

// at returns the pane under column x of the body; separators belong to no
// pane.
func (l layout) at(x int) (pane, bool) {
	for _, b := range l.boxes {
		if x >= b.x && x < b.x+b.w {
			return b.pane, true
		}
	}
	return 0, false
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
