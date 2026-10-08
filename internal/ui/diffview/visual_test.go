package diffview

import (
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/ui/keys"
)

func TestSplitKeepsLine(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.Top, 40)
	p := m.cur()
	row := p.cursor - p.offset
	m.SetSplit(true)
	if !m.Split() || pos(m) != 40 {
		t.Fatalf("split: line %d", pos(m))
	}
	if q := m.cur(); q.cursor-q.offset != row {
		t.Fatalf("split: screen row %d, want %d", q.cursor-q.offset, row)
	}
	m.SetSplit(false)
	if m.Split() || pos(m) != 40 {
		t.Fatalf("unified again: line %d", pos(m))
	}

	// A deleted row keeps its old-file line.
	act(m, keys.Top, 0)
	act(m, keys.NextChange, 2)
	act(m, keys.Down, 1)
	if loc, _ := m.Cursor(); loc.OldLine != 63 {
		t.Fatalf("setup: old line %d", loc.OldLine)
	}
	m.SetSplit(true)
	if loc, _ := m.Cursor(); loc.OldLine != 63 {
		t.Fatalf("split: old line %d, want 63", loc.OldLine)
	}
	m.SetSplit(false)
	if loc, _ := m.Cursor(); loc.OldLine != 63 {
		t.Fatalf("unified: old line %d, want 63", loc.OldLine)
	}
}

func TestSplitToggleTwiceKeepsModOld(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	if m.cursorLine() != 30 {
		t.Fatalf("setup: line %d", m.cursorLine())
	}
	m.SetSplit(true)
	m.SetSplit(false)
	if m.cursorLine() != 30 {
		t.Fatalf("toggled twice: line %d, want the old half (30)", m.cursorLine())
	}
}

func TestSplitLocation(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	m.SetSplit(true)
	loc, ok := m.Cursor()
	if !ok || loc.NewLine != 31 || loc.OldLine != 31 || loc.Repo != "revue" || loc.Path != "src/app.ts" {
		t.Fatalf("pair row location %+v", loc)
	}
}

func TestVisualUnified(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.VisualStart, 0)
	act(m, keys.Down, 3)
	sel, ok := m.Selection()
	if !ok || !m.Visual() {
		t.Fatal("no selection")
	}
	want := "-const a = 1;\n+const a = 2;\n+const b = 3;\n+const c = 4;"
	if sel.Text != want || sel.StartLine != 31 || sel.EndLine != 33 || sel.Lines != 4 {
		t.Fatalf("selection %+v", sel)
	}
	if sel.Repo != "revue" || sel.Path != "src/app.ts" {
		t.Fatalf("selection names %q %q", sel.Repo, sel.Path)
	}

	// Moving above the anchor extends the other way.
	act(m, keys.Up, 5)
	sel, _ = m.Selection()
	want = " const c29 = 29;\n const c30 = 30;\n-const a = 1;"
	if sel.Text != want || sel.StartLine != 29 || sel.EndLine != 30 || sel.Lines != 3 {
		t.Fatalf("upward selection %+v", sel)
	}
	act(m, keys.VisualEnd, 0)
	if _, ok := m.Selection(); ok || m.Visual() {
		t.Fatal("visual mode still on")
	}
}

func TestVisualDeletedOnly(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.NextChange, 1)
	act(m, keys.VisualStart, 0)
	act(m, keys.Down, 2)
	sel, _ := m.Selection()
	if sel.Text != "-gone1\n-gone2\n-gone3" || sel.StartLine != 64 || sel.EndLine != 64 {
		t.Fatalf("deleted-only selection %+v", sel)
	}
}

func TestVisualSplit(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	m.SetSplit(true)
	act(m, keys.VisualStart, 0)
	act(m, keys.Down, 2)
	sel, _ := m.Selection()
	want := "-const a = 1;\n+const a = 2;\n+const b = 3;\n+const c = 4;"
	if sel.Text != want || sel.StartLine != 31 || sel.EndLine != 33 || sel.Lines != 3 {
		t.Fatalf("split selection %+v", sel)
	}
	m.SetSplit(false)
	sel, ok := m.Selection()
	if !ok || sel.Text != want || sel.Lines != 4 {
		t.Fatalf("selection after switching to unified %+v", sel)
	}
	m.SetSplit(true)
	if sel, _ := m.Selection(); sel.Text != want || sel.Lines != 3 {
		t.Fatalf("selection after switching back %+v", sel)
	}
}

func TestVisualOverFold(t *testing.T) {
	m := newModel(t, 100, 30, 3, sample(t))
	act(m, keys.Top, 0)
	act(m, keys.VisualStart, 0)
	act(m, keys.Down, 1)
	sel, _ := m.Selection()
	if sel.Lines != 28 || sel.StartLine != 1 || sel.EndLine != 28 {
		t.Fatalf("selection over a fold %+v", sel)
	}
}

func TestYank(t *testing.T) {
	m := newModel(t, 100, 30, 100, sample(t))
	act(m, keys.VisualStart, 0)
	act(m, keys.Down, 1)
	y := yankOf(t, act(m, keys.Yank, 0))
	if y.Text != "-const a = 1;\n+const a = 2;" || y.What != "2 lines" || m.Visual() {
		t.Fatalf("visual yank %+v, visual %v", y, m.Visual())
	}

	act(m, keys.Top, 5)
	act(m, keys.VisualStart, 0)
	act(m, keys.Down, 1)
	y = yankOf(t, act(m, keys.Yank, 0))
	if y.Text != "const c5 = 5;\nconst c6 = 6;" {
		t.Fatalf("context-only yank keeps markers: %q", y.Text)
	}

	act(m, keys.Top, 5)
	y = yankOf(t, act(m, keys.YankLine, 0))
	if y.Text != "const c5 = 5;" || y.What != "1 line" {
		t.Fatalf("yy %+v", y)
	}
	y = yankOf(t, act(m, keys.YankRef, 0))
	if y.Text != "revue/src/app.ts:5" || y.What != y.Text {
		t.Fatalf("yp %+v", y)
	}
	act(m, keys.Top, 31)
	act(m, keys.VisualStart, 0)
	act(m, keys.Down, 2)
	y = yankOf(t, act(m, keys.YankRef, 0))
	if y.Text != "revue/src/app.ts:31-33" || m.Visual() {
		t.Fatalf("yp in visual mode %+v", y)
	}

	m.SetSplit(true)
	act(m, keys.Top, 31)
	if y = yankOf(t, act(m, keys.YankLine, 0)); y.Text != "const a = 2;" {
		t.Fatalf("yy on a split pair yanked %q", y.Text)
	}
}

func TestYankFold(t *testing.T) {
	m := newModel(t, 100, 30, 3, sample(t))
	act(m, keys.Top, 0)
	y := yankOf(t, act(m, keys.YankLine, 0))
	if y.What != "27 lines" || !strings.HasPrefix(y.Text, "const c1 = 1;\nconst c2 = 2;") || strings.Count(y.Text, "\n") != 26 {
		t.Fatalf("yy on a fold %+v", y)
	}
}

func TestYankRefWithoutRepo(t *testing.T) {
	m := newModel(t, 100, 30, 100, nil)
	m.SetFile("", "main.go", fileDiff(t, " a", "+b"), false)
	act(m, keys.Top, 2)
	if y := yankOf(t, act(m, keys.YankRef, 0)); y.Text != "main.go:2" {
		t.Fatalf("yp %q", y.Text)
	}
}
