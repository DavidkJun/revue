package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/sidebar"
)

// action runs a resolved key binding.
func (m *Model) action(a keys.Action, count int) tea.Cmd {
	switch a {
	case keys.FocusLeft:
		return m.moveFocus(-1)
	case keys.FocusRight:
		return m.moveFocus(1)
	case keys.PrevRepo:
		return m.jumpRepo(-1)
	case keys.NextRepo:
		return m.jumpRepo(1)
	case keys.PrevFile:
		return m.jumpFile(-1)
	case keys.NextFile:
		return m.jumpFile(1)
	case keys.ToggleSplit:
		m.diff.SetSplit(!m.diff.Split())
	case keys.Ask:
		return m.ask(false)
	case keys.AskRepo:
		return m.ask(true)
	case keys.ToggleChat:
		m.setChat(!m.showChat)
	case keys.ChatHide:
		m.setChat(false)
	case keys.ToggleSidebar:
		m.showSidebar = !m.showSidebar
		switch {
		case m.showSidebar:
			m.focus = paneSidebar
		case m.focus == paneSidebar:
			m.focus = paneDiff
		}
	case keys.OpenEditor:
		return m.openEditor()
	case keys.Refresh:
		return m.refresh()
	case keys.Help:
		m.help = helpState{open: true}
	case keys.Quit:
		if m.showChat && m.layout().has(paneChat) {
			m.setChat(false)
			return nil
		}
		return m.quit()
	case keys.QuitAll:
		return m.quit()
	case keys.Interrupt:
		if m.chat.Busy() {
			m.chat.Cancel()
			return nil
		}
		return m.quit()
	case keys.Escape:
		m.resolver.Reset()
		m.search = searchState{}
		m.diff.SetSearch("")
	case keys.Search:
		m.startSearch()
	default:
		return m.paneAction(a, count)
	}
	return nil
}

// setChat shows the Claude panel and focuses it, or hides it.
func (m *Model) setChat(show bool) {
	m.showChat = show
	switch {
	case show:
		m.focus = paneChat
	case m.focus == paneChat:
		m.focus = paneDiff
	}
}

// paneAction hands a movement or pane-specific action to the focused pane.
func (m *Model) paneAction(a keys.Action, count int) tea.Cmd {
	switch m.focused() {
	case paneSidebar:
		cmd := m.sidebar.HandleAction(a, count)
		if m.sidebar.OpenRequested() {
			m.focus = paneDiff
		}
		m.afterSidebarMove()
		return cmd
	case paneChat:
		return m.chat.HandleAction(a, count)
	}
	return m.diff.HandleAction(a, count)
}

// afterSidebarMove explains the empty diff pane when the cursor reaches a
// clean repo; files announce themselves with uimsg.FileSelected.
func (m *Model) afterSidebarMove() {
	if r, f, ok := m.sidebar.Current(); ok && f == nil {
		m.showRepoNode(r)
	}
}

// moveFocus moves to the next open pane in direction dir that is on screen
// once focused: a narrow terminal shows the focused chat instead of the
// tree, so the chat is reachable although hidden right now. Past the edge
// it moves to the neighbouring WezTerm pane, the way the user's Neovim does.
func (m *Model) moveFocus(dir int) tea.Cmd {
	from, prev := m.focused(), m.focus
	open := map[pane]bool{paneSidebar: m.showSidebar, paneDiff: true, paneChat: m.showChat}
	for p := from + pane(dir); p >= paneSidebar && p <= paneChat; p += pane(dir) {
		if !open[p] {
			continue
		}
		m.focus = p
		if m.layout().has(p) {
			return nil
		}
	}
	m.focus = prev
	if dir < 0 {
		return m.navigateOut("Left")
	}
	return m.navigateOut("Right")
}

// jumpRepo selects the first changed file of the previous/next repo, or its
// node when it is clean.
func (m *Model) jumpRepo(dir int) tea.Cmd {
	n := len(m.repos)
	if n == 0 {
		return nil
	}
	target := ((m.currentRepo()+dir)%n + n) % n
	for _, f := range m.sidebar.Files() {
		if f.Repo == target {
			return m.sidebar.Select(target, f.Path)
		}
	}
	cmd := m.sidebar.Select(target, "")
	m.showRepoNode(target)
	return cmd
}

// jumpFile selects the changed file after (dir 1) or before (dir -1) the
// current one in tree order, continuing into the next/previous repo and
// wrapping at the ends.
func (m *Model) jumpFile(dir int) tea.Cmd {
	files := m.sidebar.Files()
	if len(files) == 0 {
		return nil
	}
	i := m.fileIndex(files, dir)
	f := files[((i+dir)%len(files)+len(files))%len(files)]
	return m.sidebar.Select(f.Repo, f.Path)
}

// fileIndex is where the current file sits in files: the file under the
// sidebar cursor, else the shown file when the cursor is on a directory of
// its repo (between files). Otherwise, on a repo node, it is a virtual
// position just before the repo's first file, so "next" lands on that file
// and "previous" on the one before it.
func (m *Model) fileIndex(files []sidebar.FileRef, dir int) int {
	repo, cur, ok := m.sidebar.Current()
	_, path, onDir := m.sidebar.CurrentDir()
	want := fileKey{}
	switch {
	case ok && cur != nil:
		want = key(repo, cur.Path)
	case m.shown.ok && (!ok || m.shown.repo == repo && onDir && path != ""):
		want, repo = m.shown, m.shown.repo
	}
	if want.ok {
		for i, f := range files {
			if f.Repo == want.repo && f.Path == want.path {
				return i
			}
		}
	}
	for i, f := range files {
		if f.Repo >= repo {
			if dir > 0 {
				return i - 1
			}
			return i
		}
	}
	if dir > 0 {
		return len(files) - 1
	}
	return len(files)
}
