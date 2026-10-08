package diffview

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/highlight"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

// fileDiff builds a FileDiff from lines prefixed like a unified diff:
// ' ' context, '-' deleted, '+' added. Line numbers are filled in.
func fileDiff(t testing.TB, lines ...string) *diff.FileDiff {
	t.Helper()
	fd := &diff.FileDiff{OldPath: "src/app.ts", NewPath: "src/app.ts"}
	oldNo, newNo := 0, 0
	for _, s := range lines {
		if s == "" {
			t.Fatalf("line without a prefix")
		}
		text := s[1:]
		switch s[0] {
		case ' ':
			oldNo++
			newNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Context, OldNo: oldNo, NewNo: newNo, Text: text})
		case '-':
			oldNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Deleted, OldNo: oldNo, Text: text})
		case '+':
			newNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Added, NewNo: newNo, Text: text})
		default:
			t.Fatalf("bad prefix in %q", s)
		}
	}
	return fd
}

// ctx returns n context lines whose text names their position, starting at
// from.
func ctx(from, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(" const c%d = %d;", from+i, from+i)
	}
	return out
}

func join(parts ...[]string) []string {
	var out []string
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func lines(s ...string) []string { return s }

// newModel returns a pane of the given size showing fd with the given
// context, highlighted synchronously.
func newModel(t testing.TB, w, h, context int, fd *diff.FileDiff) *Model {
	t.Helper()
	m := New(theme.Default, highlight.New(theme.Default.SyntaxStyle), context)
	m.SetSize(w, h)
	if fd != nil {
		run(m, m.SetFile("revue", "src/app.ts", fd, false))
	}
	return m
}

// run executes cmd and feeds its message back, the way Bubble Tea would.
func run(m *Model, cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	m.Update(msg)
	return msg
}

func act(m *Model, a keys.Action, count int) tea.Msg {
	cmd := m.HandleAction(a, count)
	if cmd == nil {
		return nil
	}
	return cmd()
}

// plainLines renders the view and strips the styling.
func plainLines(t testing.TB, m *Model, focused bool) []string {
	t.Helper()
	out := m.View(focused)
	checkSize(t, out, m.width, m.height)
	return strings.Split(ansi.Strip(out), "\n")
}

func checkSize(t testing.TB, out string, w, h int) {
	t.Helper()
	if h == 0 {
		if out != "" {
			t.Fatalf("height 0: got %q", out)
		}
		return
	}
	ls := strings.Split(out, "\n")
	if len(ls) != h {
		t.Fatalf("%dx%d: got %d lines", w, h, len(ls))
	}
	for i, l := range ls {
		if got := ansi.StringWidth(l); got != w {
			t.Fatalf("%dx%d: line %d is %d cells wide: %q", w, h, i, got, l)
		}
	}
}

// pos returns the cursor's new-file line.
func pos(m *Model) int {
	l, _ := m.Position()
	return l
}

func yankOf(t testing.TB, msg tea.Msg) uimsg.Yank {
	t.Helper()
	y, ok := msg.(uimsg.Yank)
	if !ok {
		t.Fatalf("got %#v, want uimsg.Yank", msg)
	}
	return y
}

func themeDefault() theme.Theme { return theme.Default }
