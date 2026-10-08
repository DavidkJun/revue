package sidebar

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

// fc builds a FileChange; the kind comes from the first letter of spec
// ("M src/a.ts", "? new.txt").
func fc(spec string, added, deleted int) git.FileChange {
	kinds := map[byte]git.Kind{
		'M': git.Modified, 'A': git.Added, 'D': git.Deleted, 'R': git.Renamed,
		'C': git.Copied, '?': git.Untracked, 'T': git.TypeChanged, 'U': git.Conflicted,
	}
	return git.FileChange{Path: spec[2:], Kind: kinds[spec[0]], Added: added, Deleted: deleted}
}

func paths(specs ...string) []git.FileChange {
	out := make([]git.FileChange, len(specs))
	for i, s := range specs {
		out[i] = fc(s, 1, 0)
	}
	return out
}

func repo(name string, files ...git.FileChange) Repo {
	return Repo{
		Repo:   git.Repo{Name: name, Path: `C:\work\` + name},
		Status: git.Status{Branch: "main", Head: "abc1234", Files: files},
		Loaded: true,
	}
}

func errRepo(name string) Repo {
	r := repo(name)
	// The shape internal/git gives its errors.
	r.Err = errors.New("git status: fatal: bad object HEAD (exit status 128)")
	return r
}

func newModel(w, h int, repos ...Repo) *Model {
	m := New(theme.Default)
	m.SetSize(w, h)
	m.SetRepos(repos)
	return m
}

// rows renders the visible tree as plain text, one string per row: the
// row's label prefixed by its kind ("r:", "d:", "f:") and indented by depth.
func rowsOf(m *Model) []string {
	out := make([]string, len(m.rows))
	for i, n := range m.rows {
		out[i] = strings.Repeat("  ", n.depth) + string("rdf"[n.kind]) + ":" + n.label
	}
	return out
}

func curLabel(m *Model) string {
	n := m.current()
	if n == nil {
		return ""
	}
	return string("rdf"[n.kind]) + ":" + n.path
}

func act(m *Model, a keys.Action, count int) tea.Msg {
	cmd := m.HandleAction(a, count)
	if cmd == nil {
		return nil
	}
	return cmd()
}

func wantSelected(t *testing.T, msg tea.Msg, repo int, path string) {
	t.Helper()
	got, ok := msg.(uimsg.FileSelected)
	if !ok {
		t.Fatalf("want FileSelected{%d %q}, got %#v", repo, path, msg)
	}
	if got.Repo != repo || got.Path != path {
		t.Fatalf("want FileSelected{%d %q}, got %+v", repo, path, got)
	}
}

func wantNoMsg(t *testing.T, msg tea.Msg) {
	t.Helper()
	if msg != nil {
		t.Fatalf("want no message, got %#v", msg)
	}
}

func equal(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func plainLines(view string) []string {
	return strings.Split(ansi.Strip(view), "\n")
}

func statusOf(files []git.FileChange) git.Status { return git.Status{Files: files} }
