package ui

import (
	tea "charm.land/bubbletea/v2"
)

// wheelStep is how many lines one wheel notch scrolls.
const wheelStep = 3

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	ev := msg.Mouse()
	l := m.layout()
	switch msg.(type) {
	case tea.MouseWheelMsg:
		step := 0
		switch ev.Button {
		case tea.MouseWheelUp:
			step = -wheelStep
		case tea.MouseWheelDown:
			step = wheelStep
		default:
			return nil
		}
		if m.helpShown() {
			m.help.scroll = clamp(m.help.scroll+step, 0, m.helpBox(l.bodyH).maxScroll)
			return nil
		}
		if ev.Y < l.bodyY || ev.Y >= l.bodyY+l.bodyH {
			return nil
		}
		p, ok := l.at(ev.X)
		if !ok {
			return nil
		}
		switch p {
		case paneSidebar:
			m.sidebar.Scroll(step)
		case paneDiff:
			m.diff.Scroll(step)
		case paneChat:
			m.chat.Scroll(step)
		}
	case tea.MouseClickMsg:
		if ev.Button != tea.MouseLeft {
			return nil
		}
		if m.helpShown() {
			m.help = helpState{}
			return nil
		}
		if l.tabBar && ev.Y == 0 {
			return m.clickTab(ev.X)
		}
		if ev.Y < l.bodyY || ev.Y >= l.bodyY+l.bodyH {
			return nil
		}
		p, ok := l.at(ev.X)
		if !ok || m.chat.Prompting() && p != paneChat {
			return nil
		}
		m.focus = p
		if p == paneSidebar {
			cmd := m.sidebar.Click(ev.Y - l.bodyY)
			m.afterSidebarMove()
			return cmd
		}
	}
	return nil
}

// clickTab selects the repo of the clicked tab.
func (m *Model) clickTab(x int) tea.Cmd {
	for _, t := range m.tabs() {
		if x >= t.x && x < t.x+t.w {
			if t.repo == m.currentRepo() {
				return nil
			}
			return m.jumpRepo(t.repo - m.currentRepo())
		}
	}
	return nil
}
