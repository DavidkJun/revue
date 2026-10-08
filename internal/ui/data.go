package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/ui/sidebar"
)

const (
	pollInterval  = 2 * time.Second
	statusTimeout = 30 * time.Second
	diffTimeout   = 30 * time.Second
)

type pollTickMsg struct{}

type statusMsg struct {
	repo   int
	status git.Status
	err    error
}

type diffMsg struct {
	gen  int
	file fileKey
	fd   *diff.FileDiff
	err  error
	keep bool
}

func pollTick() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg { return pollTickMsg{} })
}

// loadStatus reads repo i's status in the background, unless a load is
// already running (its result will be just as fresh).
func (m *Model) loadStatus(i int) tea.Cmd {
	if m.polling[i] {
		return nil
	}
	m.polling[i] = true
	ctx, fn, path := m.ctx, m.deps.status, m.repos[i].Repo.Path
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(ctx, statusTimeout)
		defer cancel()
		st, err := fn(ctx, path)
		return statusMsg{repo: i, status: st, err: err}
	}
}

func (m *Model) poll() tea.Cmd {
	return tea.Batch(m.refresh(), pollTick())
}

// refresh reloads every repo's status now.
func (m *Model) refresh() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.repos))
	for i := range m.repos {
		cmds = append(cmds, m.loadStatus(i))
	}
	return tea.Batch(cmds...)
}

func (m *Model) applyStatus(msg statusMsg) tea.Cmd {
	i := msg.repo
	m.polling[i] = false
	r := &m.repos[i]

	if msg.err != nil {
		if r.Loaded && r.Err != nil && r.Err.Error() == msg.err.Error() {
			return nil
		}
		// Keep the last good status so a transient failure (a git lock,
		// a timeout) does not empty the tree.
		r.Err, r.Loaded = msg.err, true
		m.sidebar.SetRepos(m.repoSnapshot())
		return m.initialSelect()
	}
	if r.Loaded && r.Err == nil && r.Status.Fingerprint == msg.status.Fingerprint {
		return nil
	}

	before := m.sidebar.Files()
	curRepo, cur, _ := m.sidebar.Current()
	cursorOnWant := cur != nil && m.want == key(curRepo, cur.Path)

	r.Status, r.Err, r.Loaded = msg.status, nil, true
	m.sidebar.SetRepos(m.repoSnapshot())

	if !m.started {
		return m.initialSelect()
	}
	if !m.want.ok {
		return m.idleRepoChanged(i)
	}
	if m.want.repo != i {
		return nil
	}
	if _, ok := m.findFile(i, m.want.path); ok {
		return m.loadFile(m.want, m.want == m.shown)
	}
	if to, ok := m.renamedTo(i, m.want.path); ok {
		// git mv: the file lives on under its new name.
		if cursorOnWant {
			m.sidebar.Select(i, to)
		}
		return m.loadFile(key(i, to), false)
	}
	return m.fileGone(before, cursorOnWant)
}

// renamedTo finds the new name of a file renamed from path in repo i.
func (m *Model) renamedTo(i int, path string) (string, bool) {
	for _, fc := range m.repos[i].Status.Files {
		if fc.Kind == git.Renamed && fc.OldPath == path {
			return fc.Path, true
		}
	}
	return "", false
}

// initialSelect previews the first changed file of the first repo with
// changes, as soon as every repo before it has reported.
func (m *Model) initialSelect() tea.Cmd {
	if m.started {
		return nil
	}
	if m.want.ok {
		m.started = true // the user picked a file before the data arrived
		return nil
	}
	for i, r := range m.repos {
		if !r.Loaded {
			return nil
		}
		if r.Err != nil || len(r.Status.Files) == 0 {
			continue
		}
		for _, f := range m.sidebar.Files() {
			if f.Repo == i {
				m.started = true
				return m.sidebar.Select(i, f.Path)
			}
		}
	}
	m.started = true
	if len(m.repos) > 0 {
		m.sidebar.Select(0, "")
		m.showRepoNode(0)
	}
	return nil
}

// fileGone handles the wanted file dropping out of the changes (committed,
// reverted): the cursor moves to the nearest changed file of the same repo.
func (m *Model) fileGone(before []sidebar.FileRef, cursorOnIt bool) tea.Cmd {
	gone := m.want
	note := m.notify(gone.path+" is no longer changed", false)
	m.dropLoad()
	m.want = fileKey{}
	if !cursorOnIt {
		m.shown = fileKey{}
		m.diff.SetMessage(gone.path + " is no longer changed")
		return note
	}
	if next, ok := nearestFile(before, m.sidebar.Files(), gone); ok {
		return tea.Batch(note, m.sidebar.Select(next.Repo, next.Path))
	}
	m.sidebar.Select(gone.repo, "")
	m.showRepoNode(gone.repo)
	return note
}

// nearestFile picks the file of gone's repo that now sits where gone was:
// the first surviving file after it in the old order, else the last one
// before it, else any file of the repo.
func nearestFile(before, after []sidebar.FileRef, gone fileKey) (sidebar.FileRef, bool) {
	exists := make(map[sidebar.FileRef]bool, len(after))
	var first *sidebar.FileRef
	for i, f := range after {
		exists[f] = true
		if f.Repo == gone.repo && first == nil {
			first = &after[i]
		}
	}
	idx := -1
	for i, f := range before {
		if f.Repo == gone.repo && f.Path == gone.path {
			idx = i
			break
		}
	}
	if idx >= 0 {
		for _, f := range before[idx+1:] {
			if f.Repo == gone.repo && exists[f] {
				return f, true
			}
		}
		for i := idx - 1; i >= 0; i-- {
			if f := before[i]; f.Repo == gone.repo && exists[f] {
				return f, true
			}
		}
	}
	if first != nil {
		return *first, true
	}
	return sidebar.FileRef{}, false
}

// idleRepoChanged updates the diff pane when no file is shown and the
// cursor sits on repo i: a repo that just got changes previews its first
// file (e.g. Claude edited a file while everything was clean).
func (m *Model) idleRepoChanged(i int) tea.Cmd {
	r, f, ok := m.sidebar.Current()
	if !ok || r != i {
		return nil
	}
	if f != nil {
		return m.loadFile(key(i, f.Path), false)
	}
	for _, ref := range m.sidebar.Files() {
		if ref.Repo == i {
			return m.sidebar.Select(i, ref.Path)
		}
	}
	m.showRepoNode(i)
	return nil
}

// showRepoNode explains an empty diff pane when the cursor sits on a repo
// without changes.
func (m *Model) showRepoNode(i int) {
	r := m.repos[i]
	if !r.Loaded || len(r.Status.Files) > 0 {
		return
	}
	m.dropLoad()
	m.shown, m.want = fileKey{}, fileKey{}
	if r.Err != nil {
		m.diff.SetMessage(r.Repo.Name + ": " + r.Err.Error())
		return
	}
	m.diff.SetMessage("✓ " + r.Repo.Name + " has no uncommitted changes")
}

// selectFile reacts to the sidebar landing on a file.
func (m *Model) selectFile(repo int, path string) tea.Cmd {
	if repo < 0 || repo >= len(m.repos) {
		return nil
	}
	k := key(repo, path)
	if k == m.shown {
		// Back on the file already shown: drop a load still in flight.
		m.dropLoad()
		m.want = k
		return nil
	}
	if k == m.want {
		return nil
	}
	return m.loadFile(k, false)
}

// loadFile reads k's diff in the background. The result is applied only if
// no newer request was made in the meantime.
func (m *Model) loadFile(k fileKey, keep bool) tea.Cmd {
	fc, ok := m.findFile(k.repo, k.path)
	if !ok {
		return nil
	}
	m.dropLoad()
	m.want = k
	ctx, cancel := context.WithTimeout(m.ctx, diffTimeout)
	m.stopLoad = cancel
	gen, fn, repoPath := m.gen, m.deps.fileDiff, m.repos[k.repo].Repo.Path
	return func() tea.Msg {
		defer cancel()
		fd, err := fn(ctx, repoPath, fc)
		return diffMsg{gen: gen, file: k, fd: fd, err: err, keep: keep}
	}
}

// dropLoad makes the diff load in flight stale and stops its git process:
// while Claude keeps editing the shown file every poll starts a new load,
// and a slow git would otherwise pile them up.
func (m *Model) dropLoad() {
	m.gen++
	if m.stopLoad != nil {
		m.stopLoad()
		m.stopLoad = nil
	}
}

func (m *Model) applyDiff(msg diffMsg) tea.Cmd {
	if msg.gen != m.gen {
		return nil
	}
	keep := msg.keep && msg.file == m.shown
	m.shown = msg.file
	if msg.err != nil {
		m.diff.SetMessage("Could not read " + msg.file.path + ": " + msg.err.Error())
		return nil
	}
	return m.diff.SetFile(m.repos[msg.file.repo].Repo.Name, msg.file.path, msg.fd, keep)
}

func (m *Model) findFile(repo int, path string) (git.FileChange, bool) {
	if repo < 0 || repo >= len(m.repos) {
		return git.FileChange{}, false
	}
	for _, fc := range m.repos[repo].Status.Files {
		if fc.Path == path {
			return fc, true
		}
	}
	return git.FileChange{}, false
}

// currentRepo is the repo under the sidebar cursor.
func (m *Model) currentRepo() int {
	if r, _, ok := m.sidebar.Current(); ok && r >= 0 && r < len(m.repos) {
		return r
	}
	if m.shown.ok {
		return m.shown.repo
	}
	return 0
}
