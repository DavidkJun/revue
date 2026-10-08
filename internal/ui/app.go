// Package ui is the app shell: it lays out the tab bar, the file tree, the
// diff pane, the Claude panel and the statusline, routes keys through the
// vim-style resolver to the focused pane, and keeps the repos' state fresh.
package ui

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/config"
	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/highlight"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/chat"
	"github.com/DavidkJun/revue/internal/ui/diffview"
	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/sidebar"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

// Run shows the reviewer for repos found below root until the user quits.
func Run(root string, repos []git.Repo, cfg config.Config) error {
	return run(newModel(root, repos, cfg, theme.Default, defaultDeps()), os.Stdout)
}

// run drives m on out until it quits or dies.
func run(m *Model, out io.Writer, opts ...tea.ProgramOption) error {
	defer func() {
		// Normal exits already restored these; after a crash the user's
		// shell would otherwise keep receiving win32-input-mode sequences,
		// and a streaming claude would outlive revue. The reset goes first
		// in case the model is too broken to cancel cleanly.
		fmt.Fprint(out, m.terminalModes(false))
		m.chat.Cancel()
		m.cancel()
	}()
	_, err := tea.NewProgram(m, append(opts, tea.WithOutput(out))...).Run()
	return err
}

// fileKey names a file of one of the repos.
type fileKey struct {
	repo int
	path string
	ok   bool
}

func key(repo int, path string) fileKey { return fileKey{repo: repo, path: path, ok: true} }

// Model is the root Bubble Tea model. It is used through a pointer.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	deps   deps
	cfg    config.Config
	th     theme.Theme
	st     styles
	title  string // launch folder name, for the window title

	repos   []sidebar.Repo
	polling []bool // a status load of that repo is in flight

	sidebar *sidebar.Model
	diff    *diffview.Model
	chat    *chat.Model

	width, height int
	sizes         [3][2]int // last size given to each pane, to skip no-op SetSize calls
	focus         pane
	showSidebar   bool
	showChat      bool

	resolver keys.Resolver
	help     helpState
	search   searchState
	notice   notice

	// shown is the file the diff pane displays; want is the file whose diff
	// was requested last. They differ while a load is in flight.
	shown, want fileKey
	gen         int                // generation of the latest diff request; older results are dropped
	stopLoad    context.CancelFunc // cancels the diff load of generation gen
	started     bool               // the initial file was selected
}

func newModel(root string, repos []git.Repo, cfg config.Config, th theme.Theme, d deps) *Model {
	ctx, cancel := context.WithCancel(context.Background())
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	m := &Model{
		ctx:         ctx,
		cancel:      cancel,
		deps:        d,
		cfg:         cfg,
		th:          th,
		st:          newStyles(th),
		title:       filepath.Base(abs),
		polling:     make([]bool, len(repos)),
		sidebar:     sidebar.New(th),
		diff:        diffview.New(th, highlight.New(th.SyntaxStyle), cfg.Context),
		focus:       paneSidebar,
		showSidebar: true,
	}
	for _, r := range repos {
		m.repos = append(m.repos, sidebar.Repo{Repo: r})
	}
	m.chat = chat.New(chat.Options{
		Theme:     th,
		Dir:       abs,
		AddDirs:   outside(abs, repos),
		Model:     cfg.Claude.Model,
		Effort:    cfg.Claude.Effort,
		ExtraArgs: cfg.Claude.ExtraArgs,
	})
	m.sidebar.SetRepos(m.repoSnapshot())
	m.diff.SetMessage("Loading changes…")
	return m
}

// outside lists the repo roots Claude cannot reach from dir, which happens
// when revue is started in a subfolder of a repo.
func outside(dir string, repos []git.Repo) []string {
	var out []string
	for _, r := range repos {
		rel, err := filepath.Rel(dir, r.Path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			out = append(out, r.Path)
		}
	}
	return out
}

func (m *Model) repoSnapshot() []sidebar.Repo {
	return append([]sidebar.Repo(nil), m.repos...)
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.Raw(m.terminalModes(true)), pollTick()}
	for i := range m.repos {
		cmds = append(cmds, m.loadStatus(i))
	}
	return tea.Batch(cmds...)
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.focused()
	cmd := m.update(msg)
	m.relayout()
	if m.focused() != before {
		// A count or sequence typed for one pane must not complete in
		// another one's scope (Vim drops it on a mouse click as well).
		m.resolver.Reset()
	}
	return m, cmd
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.PasteMsg:
		return m.handlePaste(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.FocusMsg:
		return m.refresh()
	case tea.BlurMsg:
		return nil
	case pendingTimeoutMsg:
		if msg.seq == m.resolver.Seq() && m.resolver.Busy() {
			m.resolver.Reset()
		}
		return nil
	case noticeExpiredMsg:
		if msg.id == m.notice.id {
			m.notice = notice{id: m.notice.id}
		}
		return nil
	case pollTickMsg:
		return m.poll()
	case statusMsg:
		return m.applyStatus(msg)
	case diffMsg:
		return m.applyDiff(msg)
	case askMsg:
		return m.applyAsk(msg)
	case editorDoneMsg:
		return m.editorDone(msg)
	case navigateDoneMsg:
		if msg.err != nil {
			return m.notify("wezterm: "+msg.err.Error(), true)
		}
		return nil
	case uimsg.FileSelected:
		// Bubble Tea runs each command in its own goroutine, so the
		// messages of quick moves can arrive out of order: only the file
		// under the cursor counts.
		if r, f, ok := m.sidebar.Current(); !ok || f == nil || key(r, f.Path) != key(msg.Repo, msg.Path) {
			return nil
		}
		return m.selectFile(msg.Repo, msg.Path)
	case uimsg.Yank:
		return m.yank(msg)
	case uimsg.Notify:
		return m.notify(msg.Text, msg.Error)
	}
	// Everything else belongs to a component: background highlighting
	// results for the diff pane, streamed answers and ticks for the chat.
	return tea.Batch(m.diff.Update(msg), m.chat.Update(msg))
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = true
	v.WindowTitle = "revue · " + m.title
	return v
}

func (m *Model) layout() layout {
	return computeLayout(m.width, m.height, m.showSidebar, m.showChat, m.chat.Full(), m.focus == paneChat)
}

// focused is the pane that gets the keys: the chosen one while it is
// visible. A pane dropped because the terminal is too narrow keeps its
// claim, so focus comes back when the terminal grows again.
func (m *Model) focused() pane {
	l := m.layout()
	if l.has(m.focus) || len(l.boxes) == 0 {
		return m.focus
	}
	if l.has(paneDiff) {
		return paneDiff
	}
	return l.boxes[0].pane
}

// relayout sizes the visible panes.
func (m *Model) relayout() {
	l := m.layout()
	for _, b := range l.boxes {
		size := [2]int{b.w, l.bodyH}
		if m.sizes[b.pane] == size {
			continue
		}
		m.sizes[b.pane] = size
		switch b.pane {
		case paneSidebar:
			m.sidebar.SetSize(b.w, l.bodyH)
		case paneDiff:
			m.diff.SetSize(b.w, l.bodyH)
		case paneChat:
			m.chat.SetSize(b.w, l.bodyH)
		}
	}
}
