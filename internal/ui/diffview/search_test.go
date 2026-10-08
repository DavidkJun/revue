package diffview

import (
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

func searchSample(t *testing.T) *Model {
	t.Helper()
	fd := fileDiff(t, join(
		lines(" Foo bar", " nothing", " foo baz foo"),
		ctx(4, 40),
		lines("-FOO old", "+FOO new", " xfoox"),
	)...)
	return newModel(t, 80, 20, 100, fd)
}

func notifyOf(t *testing.T, msg any) uimsg.Notify {
	t.Helper()
	n, ok := msg.(uimsg.Notify)
	if !ok {
		t.Fatalf("got %#v, want uimsg.Notify", msg)
	}
	return n
}

func TestSearchSmartCase(t *testing.T) {
	m := searchSample(t)
	act(m, keys.Top, 0)
	if n := m.SetSearch("foo"); n != 6 {
		t.Fatalf("foo: %d matches, want 6", n)
	}
	if m.cursorLine() != 0 {
		t.Fatalf("foo: cursor line %d, want the match under the cursor (0)", m.cursorLine())
	}
	if n := m.SetSearch("Foo"); n != 1 {
		t.Fatalf("Foo: %d matches, want 1", n)
	}
	if n := m.SetSearch("FOO"); n != 2 {
		t.Fatalf("FOO: %d matches, want 2", n)
	}
	if n := m.SetSearch("zzz"); n != 0 {
		t.Fatalf("zzz: %d matches", n)
	}
	if n := m.SetSearch(""); n != 0 || m.lineMatches(0) != nil {
		t.Fatalf("clearing left matches")
	}
}

func TestSearchFromCursor(t *testing.T) {
	m := searchSample(t)
	act(m, keys.Top, 2)
	m.SetSearch("foo")
	if m.cursorLine() != 2 {
		t.Fatalf("cursor line %d, want 2", m.cursorLine())
	}
	act(m, keys.Top, 4)
	m.SetSearch("foo")
	if m.cursorLine() != 43 {
		t.Fatalf("cursor line %d, want the next match (43)", m.cursorLine())
	}
}

func TestSearchNextPrev(t *testing.T) {
	m := searchSample(t)
	act(m, keys.Top, 0)
	m.SetSearch("foo") // lines 0, 2, 43 (FOO old), 44 (FOO new), 45
	if msg := act(m, keys.SearchNext, 0); msg != nil || m.cursorLine() != 2 {
		t.Fatalf("n: line %d msg %v", m.cursorLine(), msg)
	}
	act(m, keys.SearchNext, 2)
	if m.cursorLine() != 44 {
		t.Fatalf("2n: line %d, want 44", m.cursorLine())
	}
	act(m, keys.SearchNext, 0)
	n := notifyOf(t, act(m, keys.SearchNext, 0))
	if n.Text != "search wrapped" || n.Error || m.cursorLine() != 0 {
		t.Fatalf("wrap: %+v line %d", n, m.cursorLine())
	}
	n = notifyOf(t, act(m, keys.SearchPrev, 0))
	if n.Text != "search wrapped" || m.cursorLine() != 45 {
		t.Fatalf("N wrap: %+v line %d", n, m.cursorLine())
	}
	if msg := act(m, keys.SearchPrev, 3); msg != nil || m.cursorLine() != 2 {
		t.Fatalf("3N: line %d msg %v", m.cursorLine(), msg)
	}
}

func TestSearchErrors(t *testing.T) {
	m := searchSample(t)
	if n := notifyOf(t, act(m, keys.SearchNext, 0)); !n.Error {
		t.Fatalf("n without a pattern: %+v", n)
	}
	m.SetSearch("zzz")
	n := notifyOf(t, act(m, keys.SearchNext, 0))
	if !n.Error || !strings.Contains(n.Text, "zzz") {
		t.Fatalf("n without matches: %+v", n)
	}
}

func TestSearchOpensFold(t *testing.T) {
	m := newModel(t, 80, 20, 3, sample(t))
	act(m, keys.Top, 0)
	if n := m.SetSearch("c50 "); n != 1 {
		t.Fatalf("%d matches", n)
	}
	if pos(m) != 53 {
		t.Fatalf("search landed on line %d, want 53", pos(m))
	}
	if it := m.cur().folds.Items()[m.cur().cursor]; it.Fold >= 0 {
		t.Fatal("the fold with the match is still closed")
	}
}

func TestSearchSplit(t *testing.T) {
	m := searchSample(t)
	m.SetSplit(true)
	act(m, keys.Top, 0)
	// Context lines count once although both halves show them.
	if n := m.SetSearch("foo"); n != 6 {
		t.Fatalf("split: %d matches, want 6", n)
	}
	act(m, keys.SearchNext, 2)
	if m.cursorLine() != 43 { // the split row of FOO old / FOO new
		t.Fatalf("split 2n: line %d", m.cursorLine())
	}
	m.SetSplit(false)
	if got := len(m.matches()); got != 6 {
		t.Fatalf("unified after split: %d matches", got)
	}
}

func TestSearchNonASCII(t *testing.T) {
	fd := fileDiff(t, " const x = 'Привіт світ';", "+ПРИВІТ")
	m := newModel(t, 80, 10, 100, fd)
	if n := m.SetSearch("привіт"); n != 2 {
		t.Fatalf("%d matches, want 2", n)
	}
	ms := m.lineMatches(0)
	if len(ms) != 1 || fd.Lines[0].Text[ms[0].start:ms[0].end] != "Привіт" {
		t.Fatalf("match %+v", ms)
	}
	if n := m.SetSearch("Привіт"); n != 1 {
		t.Fatalf("upper-case pattern: %d matches, want 1", n)
	}
}

func TestSearchHighlighted(t *testing.T) {
	m := searchSample(t)
	act(m, keys.Top, 0)
	m.SetSearch("bar")
	if !strings.Contains(m.View(true), "48;2;122;92;18m") {
		t.Fatal("no search background in the view")
	}
	m.SetSearch("")
	if strings.Contains(m.View(true), "48;2;122;92;18m") {
		t.Fatal("search background left after clearing")
	}
}
