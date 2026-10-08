package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/git"
)

const (
	// Bubble Tea's VT input on Windows drops Shift from ctrl+letter;
	// win32-input-mode reports real key events, so ctrl+shift+d arrives
	// intact while ctrl+d stays ctrl+d.
	win32InputOn  = "\x1b[?9001h"
	win32InputOff = "\x1b[?9001l"
	// The user's WezTerm smart-nav forwards ctrl+h/l to panes that claim to
	// be Neovim, so revue gets its own pane navigation first.
	isNvimTrue  = "\x1b]1337;SetUserVar=IS_NVIM=dHJ1ZQ==\a"
	isNvimFalse = "\x1b]1337;SetUserVar=IS_NVIM=ZmFsc2U=\a"
)

// terminalModes returns the escape sequences that turn revue's terminal
// modes on (at start and after resuming) or off (before exiting or handing
// the terminal to the editor).
func (m *Model) terminalModes(on bool) string {
	wezterm := os.Getenv("WEZTERM_PANE") != ""
	switch {
	case on && wezterm:
		return win32InputOn + isNvimTrue
	case on:
		return win32InputOn
	case wezterm:
		return win32InputOff + isNvimFalse
	default:
		return win32InputOff
	}
}

func (m *Model) quit() tea.Cmd {
	m.chat.Cancel()
	m.cancel()
	return tea.Sequence(tea.Raw(m.terminalModes(false)), tea.Quit)
}

type editorDoneMsg struct{ err error }

// openEditor suspends the TUI and opens the shown file at the cursor line.
func (m *Model) openEditor() tea.Cmd {
	if !m.shown.ok {
		return m.notify("no file to open", true)
	}
	fc, ok := m.findFile(m.shown.repo, m.shown.path)
	if ok && fc.Kind == git.Deleted {
		return m.notify(m.shown.path+" was deleted", true)
	}
	argv := strings.Fields(m.cfg.Editor)
	if len(argv) == 0 {
		return m.notify("no editor configured", true)
	}
	line := 1
	if loc, ok := m.diff.Cursor(); ok && loc.NewLine > 0 {
		line = loc.NewLine
	}
	repoPath := m.repos[m.shown.repo].Repo.Path
	path := filepath.Join(repoPath, filepath.FromSlash(m.shown.path))
	cmd := exec.Command(argv[0], append(argv[1:], "+"+strconv.Itoa(line), path)...)
	cmd.Dir = repoPath
	return tea.Sequence(
		tea.Raw(m.terminalModes(false)),
		tea.ExecProcess(cmd, func(err error) tea.Msg { return editorDoneMsg{err} }),
	)
}

func (m *Model) editorDone(msg editorDoneMsg) tea.Cmd {
	cmds := []tea.Cmd{tea.Raw(m.terminalModes(true)), m.refresh()}
	if msg.err != nil {
		cmds = append(cmds, m.notify(m.cfg.Editor+": "+msg.err.Error(), true))
	}
	return tea.Batch(cmds...)
}

type navigateDoneMsg struct{ err error }

// navigateOut hands focus to the neighbouring WezTerm pane.
func (m *Model) navigateOut(direction string) tea.Cmd {
	if m.deps.navigate == nil {
		return nil
	}
	ctx, nav := m.ctx, m.deps.navigate
	return func() tea.Msg { return navigateDoneMsg{nav(ctx, direction)} }
}
