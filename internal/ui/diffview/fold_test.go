package diffview

import (
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/ui/keys"
)

// With context 3 the sample folds into 22 items:
//
//	fold [0,27), 27..36, fold [37,61), 61..69, fold [70,97)
func foldedSample(t *testing.T) *Model {
	t.Helper()
	m := newModel(t, 80, 30, 3, sample(t))
	if n := len(m.cur().folds.Items()); n != 22 {
		t.Fatalf("%d items, want 22", n)
	}
	return m
}

func TestFoldMarker(t *testing.T) {
	m := foldedSample(t)
	act(m, keys.Top, 0)
	got := plainLines(t, m, true)
	if !strings.Contains(got[1], "⋯ 27 unchanged lines  (zo)  ⋯") {
		t.Fatalf("first body line %q is not the fold marker", got[1])
	}
	if !strings.Contains(m.View(true), m.pal.seq("", m.th.FoldBG, false)) &&
		!strings.Contains(m.View(true), m.pal.seq("", m.pal.fold[stCursor], false)) {
		t.Fatalf("fold marker is not painted with the fold background")
	}
}

func TestFoldOpenClose(t *testing.T) {
	m := foldedSample(t)
	p := m.cur()
	act(m, keys.Top, 0)
	if it := p.folds.Items()[p.cursor]; it.Fold < 0 {
		t.Fatalf("gg is not on the first fold: %+v", it)
	}
	act(m, keys.FoldOpen, 0)
	if n := len(p.folds.Items()); n != 22+26 || m.cursorLine() != 0 || pos(m) != 1 {
		t.Fatalf("zo: %d items, cursor line %d", n, m.cursorLine())
	}
	act(m, keys.Down, 10)
	act(m, keys.FoldClose, 0)
	if it := p.folds.Items()[p.cursor]; it.Fold < 0 || len(p.folds.Items()) != 22 {
		t.Fatalf("zc: cursor item %+v, %d items", it, len(p.folds.Items()))
	}
	act(m, keys.FoldToggle, 0)
	if len(p.folds.Items()) != 48 {
		t.Fatalf("za on a fold did not open it")
	}
	act(m, keys.FoldToggle, 0)
	if len(p.folds.Items()) != 22 {
		t.Fatalf("za inside an open fold did not close it")
	}
	// A change line is in no fold, so zc there does nothing.
	act(m, keys.Top, 31)
	act(m, keys.FoldClose, 0)
	if len(p.folds.Items()) != 22 || pos(m) != 31 {
		t.Fatalf("zc on a change line changed the folds")
	}
}

func TestFoldAll(t *testing.T) {
	m := foldedSample(t)
	p := m.cur()
	act(m, keys.Top, 31)
	row := p.cursor - p.offset
	act(m, keys.FoldOpenAll, 0)
	if len(p.folds.Items()) != 97 || pos(m) != 31 || p.cursor-p.offset != row {
		t.Fatalf("zR: %d items, line %d, row %d (was %d)", len(p.folds.Items()), pos(m), p.cursor-p.offset, row)
	}
	act(m, keys.Top, 50)
	act(m, keys.FoldCloseAll, 0)
	if it := p.folds.Items()[p.cursor]; len(p.folds.Items()) != 22 || it.Fold != 1 {
		t.Fatalf("zM: %d items, cursor on %+v", len(p.folds.Items()), it)
	}
}

func TestGotoLineOpensFold(t *testing.T) {
	m := foldedSample(t)
	act(m, keys.Top, 50)
	if pos(m) != 50 {
		t.Fatalf("50gg: line %d", pos(m))
	}
	if it := m.cur().folds.Items()[m.cur().cursor]; it.Fold >= 0 {
		t.Fatalf("cursor on a fold after 50gg")
	}
}

func TestContextChange(t *testing.T) {
	m := foldedSample(t)
	p := m.cur()
	act(m, keys.Top, 31)
	row := p.cursor - p.offset
	act(m, keys.ContextMore, 0)
	if m.Context() != 13 || pos(m) != 31 || p.cursor-p.offset != row {
		t.Fatalf("+: context %d line %d row %d", m.Context(), pos(m), p.cursor-p.offset)
	}
	if n := len(p.folds.Items()); n != 97-(17+4+17)+3 {
		t.Fatalf("+: %d items", n)
	}
	act(m, keys.ContextMore, 2)
	if m.Context() != 33 {
		t.Fatalf("2+: context %d", m.Context())
	}
	act(m, keys.ContextLess, 100)
	if m.Context() != 0 || pos(m) != 31 {
		t.Fatalf("100-: context %d line %d", m.Context(), pos(m))
	}
	act(m, keys.ContextLess, 0)
	if m.Context() != 0 {
		t.Fatalf("- at 0: context %d", m.Context())
	}
	// The split view refolds with the same context.
	m.SetSplit(true)
	if m.cur().folds.Context() != 0 || pos(m) != 31 {
		t.Fatalf("split: context %d line %d", m.cur().folds.Context(), pos(m))
	}
}

func TestContextLessHidesCursorLine(t *testing.T) {
	m := newModel(t, 80, 30, 30, sample(t))
	act(m, keys.Top, 5)
	act(m, keys.ContextLess, 1)
	p := m.cur()
	if it := p.folds.Items()[p.cursor]; it.Fold < 0 || it.Start > 4 || it.End <= 4 {
		t.Fatalf("cursor not on the fold hiding its line: %+v", it)
	}
}

// The fold marker is quiet on its own background, but once the cursor or the
// selection paints the row its text must stay readable, the "(zo)" hint
// above all: that is when it is useful.
func TestFoldTextReadableOnCursorAndSelection(t *testing.T) {
	m := New(themeDefault(), nil, 50)
	for st := stCursor; st < numStates; st++ {
		for i, fg := range m.pal.foldFG[st] {
			if c := contrast(fg, m.pal.fold[st]); c < minContrast {
				t.Errorf("state %d part %d: contrast %.2f of %s on %s", st, i, c, fg, m.pal.fold[st])
			}
		}
	}
}
