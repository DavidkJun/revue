package ui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/claude"
	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// Patch budgets: enough for Claude to see the change in full in the common
// case, small enough to send quickly.
const (
	filePatchBytes = 60 << 10
	repoPatchBytes = 150 << 10
	patchTimeout   = 30 * time.Second
)

// askTarget is one context item before its patch is read.
type askTarget struct {
	item  claude.ContextItem
	files []git.FileChange // what the patch covers
	limit int
	chip  string
}

type askMsg struct {
	items []claude.ContextItem
	chips []string
	err   error
}

// ask opens the prompt with context picked by priority: the visual
// selection, else the marked files, else the whole repo or directory when
// the tree cursor is on its node, else the shown file at the cursor line. repoWide asks
// about every change of the current repo instead.
func (m *Model) ask(repoWide bool) tea.Cmd {
	return m.askAbout(m.askTargets(repoWide))
}

func (m *Model) askTargets(repoWide bool) []askTarget {
	if repoWide {
		return m.repoTarget(m.currentRepo())
	}
	if sel, ok := m.diff.Selection(); ok && m.shown.ok {
		m.diff.HandleAction(keys.VisualEnd, 0)
		t := m.fileTarget(m.shown.repo, m.shown.path)
		t.item.StartLine, t.item.EndLine = sel.StartLine, sel.EndLine
		t.item.Selection = sel.Text
		t.chip += lineRange(sel.StartLine, sel.EndLine)
		return []askTarget{t}
	}
	if marked := m.sidebar.Marked(); len(marked) > 0 {
		targets := make([]askTarget, 0, len(marked))
		for _, f := range marked {
			targets = append(targets, m.fileTarget(f.Repo, f.Path))
		}
		return targets
	}
	if r, dir, ok := m.sidebar.CurrentDir(); ok && (m.focused() == paneSidebar || !m.shown.ok) {
		if dir == "" {
			return m.repoTarget(r)
		}
		return m.dirTarget(r, dir)
	}
	if !m.shown.ok {
		return nil
	}
	t := m.fileTarget(m.shown.repo, m.shown.path)
	if loc, ok := m.diff.Cursor(); ok && loc.NewLine > 0 {
		t.item.StartLine, t.item.EndLine = loc.NewLine, loc.NewLine
		t.chip += lineRange(loc.NewLine, loc.NewLine)
	}
	return []askTarget{t}
}

func (m *Model) fileTarget(repo int, path string) askTarget {
	r := m.repos[repo].Repo
	t := askTarget{
		item:  claude.ContextItem{Repo: r.Name, RepoPath: r.Path, Path: path},
		limit: filePatchBytes,
		chip:  r.Name + "/" + path,
	}
	if fc, ok := m.findFile(repo, path); ok {
		t.files = []git.FileChange{fc}
	}
	return t
}

func (m *Model) repoTarget(repo int) []askTarget {
	if repo < 0 || repo >= len(m.repos) {
		return nil
	}
	r := m.repos[repo]
	// A clean repo is still worth asking about (Claude can read its code), it
	// just has no changes to attach.
	chip := r.Repo.Name + " (no changes)"
	if len(r.Status.Files) > 0 {
		chip = r.Repo.Name + " " + fileCount(len(r.Status.Files))
	}
	return []askTarget{{
		item:  claude.ContextItem{Repo: r.Repo.Name, RepoPath: r.Repo.Path},
		files: append([]git.FileChange(nil), r.Status.Files...),
		limit: repoPatchBytes,
		chip:  chip,
	}}
}

// dirTarget covers the changes below dir, which claude.BuildPrompt tells
// from a file by its trailing slash.
func (m *Model) dirTarget(repo int, dir string) []askTarget {
	if repo < 0 || repo >= len(m.repos) {
		return nil
	}
	r := m.repos[repo]
	var files []git.FileChange
	for _, fc := range r.Status.Files {
		if strings.HasPrefix(fc.Path, dir+"/") {
			files = append(files, fc)
		}
	}
	return []askTarget{{
		item:  claude.ContextItem{Repo: r.Repo.Name, RepoPath: r.Repo.Path, Path: dir + "/"},
		files: files,
		limit: repoPatchBytes,
		chip:  r.Repo.Name + "/" + dir + "/ " + fileCount(len(files)),
	}}
}

func fileCount(n int) string {
	if n == 1 {
		return "(1 file)"
	}
	return fmt.Sprintf("(%d files)", n)
}

func lineRange(start, end int) string {
	if end <= start {
		return ":" + strconv.Itoa(start)
	}
	return ":" + strconv.Itoa(start) + "-" + strconv.Itoa(end)
}

// askAbout reads the patches in the background; the prompt opens when they
// are ready.
func (m *Model) askAbout(targets []askTarget) tea.Cmd {
	ctx, patch := m.ctx, m.deps.patch
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, patchTimeout)
		defer cancel()
		var msg askMsg
		var errs []error
		for _, t := range targets {
			if len(t.files) > 0 {
				p, err := patch(ctx, t.item.RepoPath, t.files, t.limit)
				if err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", t.chip, err))
				}
				t.item.Patch = p
			}
			msg.items = append(msg.items, t.item)
			msg.chips = append(msg.chips, t.chip)
		}
		msg.err = errors.Join(errs...)
		return msg
	}
}

func (m *Model) applyAsk(msg askMsg) tea.Cmd {
	m.setChat(true)
	cmds := []tea.Cmd{m.chat.StartPrompt(msg.items, msg.chips)}
	if msg.err != nil {
		cmds = append(cmds, m.notify(msg.err.Error(), true))
	}
	return tea.Batch(cmds...)
}
