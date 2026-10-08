package diffview

import (
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// edited is sample with two more lines added at the top of the new file, as
// if Claude edited it again.
func edited(t *testing.T) *diff.FileDiff {
	return fileDiff(t, join(
		lines("+import a;", "+import b;"),
		ctx(1, 30),
		lines("-const a = 1;", "+const a = 2;", "+const b = 3;", "+const c = 4;"),
		ctx(31, 30),
		lines("-gone1", "-gone2", "-gone3"),
		ctx(61, 30),
	)...)
}

func TestKeepPosition(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.Top, 40)
	act(m, keys.ContextLess, 5) // context 50
	m.SetSplit(true)
	act(m, keys.VisualStart, 0)
	act(m, keys.Up, 2)
	p := m.cur()
	row := p.cursor - p.offset
	run(m, m.SetFile("revue", "src/app.ts", edited(t), true))
	if pos(m) != 38 {
		t.Fatalf("line %d, want 38", pos(m))
	}
	if !m.Split() || m.Context() != 50 || !m.Visual() {
		t.Fatalf("split %v context %d visual %v", m.Split(), m.Context(), m.Visual())
	}
	if q := m.cur(); q.cursor-q.offset != row {
		t.Fatalf("screen row %d, want %d", q.cursor-q.offset, row)
	}
	sel, _ := m.Selection()
	if sel.StartLine != 38 || sel.EndLine != 40 {
		t.Fatalf("selection %d-%d, want 38-40", sel.StartLine, sel.EndLine)
	}
}

func TestKeepPositionDeletedRow(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.NextChange, 1)
	act(m, keys.Down, 1) // old line 63
	run(m, m.SetFile("revue", "src/app.ts", edited(t), true))
	if loc, _ := m.Cursor(); loc.OldLine != 63 {
		t.Fatalf("old line %d, want 63", loc.OldLine)
	}
}

func TestKeepOpenFolds(t *testing.T) {
	m := newModel(t, 100, 30, 3, sample(t))
	act(m, keys.Top, 50) // opens the middle fold
	run(m, m.SetFile("revue", "src/app.ts", sample(t), true))
	p := m.cur()
	if pos(m) != 50 {
		t.Fatalf("line %d", pos(m))
	}
	if it := p.folds.Items()[p.cursor]; it.Fold >= 0 {
		t.Fatal("the open fold closed on reload")
	}
	if k := p.folds.FoldAt(0); p.folds.IsOpen(k) {
		t.Fatal("a closed fold opened on reload")
	}
}

func TestNewFileResets(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.Top, 10)
	act(m, keys.VisualStart, 0)
	m.xoff = 8
	run(m, m.SetFile("revue", "src/other.ts", sample(t), true))
	if m.Visual() || m.xoff != 0 || m.cursorLine() != 30 {
		t.Fatalf("other file: visual %v xoff %d line %d", m.Visual(), m.xoff, m.cursorLine())
	}
	act(m, keys.Top, 10)
	run(m, m.SetFile("revue", "src/other.ts", sample(t), false))
	if m.cursorLine() != 30 {
		t.Fatalf("same file without keepPosition: line %d", m.cursorLine())
	}
}

func TestKeepPositionAfterMessage(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.Top, 12)
	m.SetMessage("loading…")
	if _, ok := m.Cursor(); ok {
		t.Fatal("cursor reported while a message is shown")
	}
	run(m, m.SetFile("revue", "src/app.ts", sample(t), true))
	if pos(m) != 12 {
		t.Fatalf("line %d, want 12", pos(m))
	}
}

func TestStaleHighlightIgnored(t *testing.T) {
	m := newModel(t, 100, 30, 100, nil)
	stale := m.SetFile("revue", "src/app.ts", sample(t), false)
	fresh := m.SetFile("revue", "src/b.ts", sample(t), false)
	if stale == nil || fresh == nil {
		t.Fatal("SetFile returned no highlight command")
	}
	m.Update(stale())
	if m.f.newSpans != nil {
		t.Fatal("a stale highlight was applied")
	}
	m.Update(fresh())
	if m.f.newSpans == nil || m.f.oldSpans == nil {
		t.Fatal("the current highlight was not applied")
	}
}

func TestReloadKeepsColours(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	if m.f.newSpans == nil {
		t.Fatal("setup: no spans")
	}
	m.SetFile("revue", "src/app.ts", edited(t), true)
	if m.f.oldSpans == nil || m.f.newSpans == nil || len(m.f.newSpans) != len(m.f.newLines) {
		t.Fatal("colours dropped until the new highlight arrives")
	}
}

func TestKeepHorizontalScroll(t *testing.T) {
	long := " const s = '" + strings.Repeat("x", 200) + "';"
	m := newModel(t, 80, 10, 100, fileDiff(t, long, "+added"))
	act(m, keys.ScrollRight, 5)
	run(m, m.SetFile("revue", "src/app.ts", fileDiff(t, long, "+added", "+more"), true))
	if m.xoff != 5*hscrollStep {
		t.Fatalf("xoff %d after reload", m.xoff)
	}
}

// Switching views while a message covers the file must still carry the
// cursor over, or the reload that follows restores a stale position.
func TestSplitWhileMessageKeepsPosition(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.Top, 50)
	m.SetMessage("src/app.ts is no longer changed")
	m.SetSplit(true)
	run(m, m.SetFile("revue", "src/app.ts", sample(t), true))
	if !m.Split() || pos(m) != 50 {
		t.Fatalf("split %v, line %d, want 50", m.Split(), pos(m))
	}
	m.SetMessage("loading")
	m.SetSplit(false)
	run(m, m.SetFile("revue", "src/app.ts", sample(t), true))
	if m.Split() || pos(m) != 50 {
		t.Fatalf("split %v, line %d, want 50", m.Split(), pos(m))
	}
}
