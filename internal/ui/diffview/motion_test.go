package diffview

import (
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// sample has two change blocks: a modified line plus two added lines after
// new line 30, and three deleted lines after new line 63.
//
//	unified rows 0-29 context (new 1-30), 30 ModOld, 31 ModNew (new 31),
//	32-33 added (new 32-33), 34-63 context (new 34-63), 64-66 deleted
//	(old 62-64), 67-96 context (new 64-93)
func sample(t testing.TB) *diff.FileDiff {
	return fileDiff(t, join(
		ctx(1, 30),
		lines("-const a = 1;", "+const a = 2;", "+const b = 3;", "+const c = 4;"),
		ctx(31, 30),
		lines("-gone1", "-gone2", "-gone3"),
		ctx(61, 30),
	)...)
}

func TestSampleShape(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t))
	f := m.f
	if len(f.rows) != 97 || len(f.split) != 96 || len(f.newLines) != 93 {
		t.Fatalf("rows %d split %d new %d", len(f.rows), len(f.split), len(f.newLines))
	}
	want := map[int]diff.RowKind{29: diff.RowContext, 30: diff.RowModOld, 31: diff.RowModNew, 32: diff.RowAdded, 33: diff.RowAdded, 34: diff.RowContext, 64: diff.RowDeleted, 66: diff.RowDeleted, 67: diff.RowContext}
	for i, k := range want {
		if f.rows[i].Kind != k {
			t.Errorf("row %d kind %d, want %d", i, f.rows[i].Kind, k)
		}
	}
	if a, d, mod := m.Stats(); a != 2 || d != 3 || mod != 1 {
		t.Errorf("Stats = %d %d %d, want 2 3 1", a, d, mod)
	}
}

func TestStartsOnFirstChange(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t))
	if got := m.cursorLine(); got != 30 {
		t.Fatalf("cursor line %d, want 30", got)
	}
	if l, total := m.Position(); l != 31 || total != 93 {
		t.Fatalf("Position = %d/%d, want 31/93", l, total)
	}
	p := m.cur()
	if p.cursor < p.offset || p.cursor >= p.offset+m.bodyHeight() {
		t.Fatalf("cursor %d outside window at %d", p.cursor, p.offset)
	}
}

func TestVerticalMotions(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t)) // body 19, scrolloff 4
	p := m.cur()

	act(m, keys.Top, 0)
	if pos(m) != 1 || p.offset != 0 {
		t.Fatalf("gg: line %d offset %d", pos(m), p.offset)
	}
	act(m, keys.Down, 5)
	if pos(m) != 6 {
		t.Fatalf("5j: line %d, want 6", pos(m))
	}
	act(m, keys.Up, 2)
	if pos(m) != 4 {
		t.Fatalf("2k: line %d, want 4", pos(m))
	}
	act(m, keys.Up, 100)
	if pos(m) != 1 {
		t.Fatalf("100k: line %d, want 1", pos(m))
	}
	act(m, keys.Top, 40)
	if pos(m) != 40 {
		t.Fatalf("40gg: line %d, want 40", pos(m))
	}
	act(m, keys.Bottom, 10)
	if pos(m) != 10 {
		t.Fatalf("10G: line %d, want 10", pos(m))
	}
	act(m, keys.Bottom, 1000)
	if pos(m) != 93 {
		t.Fatalf("1000G: line %d, want 93", pos(m))
	}
	act(m, keys.Top, 0)
	act(m, keys.Bottom, 0)
	if pos(m) != 93 || p.cursor != 96 || p.offset != 97-19 {
		t.Fatalf("G: line %d cursor %d offset %d", pos(m), p.cursor, p.offset)
	}
}

func TestScrolloff(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t))
	p := m.cur()
	act(m, keys.Top, 0)
	act(m, keys.Down, 15)
	if row := p.cursor - p.offset; row != 19-1-scrollOff {
		t.Fatalf("cursor on screen row %d, want %d", row, 19-1-scrollOff)
	}
	act(m, keys.Down, 1)
	if row := p.cursor - p.offset; row != 19-1-scrollOff {
		t.Fatalf("after j: screen row %d", row)
	}
	act(m, keys.Up, 10)
	if row := p.cursor - p.offset; row != scrollOff {
		t.Fatalf("after 10k: screen row %d, want %d", row, scrollOff)
	}
}

func TestFarJumpCentres(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t))
	p := m.cur()
	act(m, keys.Top, 0)
	act(m, keys.Top, 60)
	if row := p.cursor - p.offset; row != 9 {
		t.Fatalf("60gg: screen row %d, want the middle (9)", row)
	}
}

func TestHalfAndPageMotions(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t))
	p := m.cur()
	act(m, keys.Top, 0)
	act(m, keys.HalfDown, 0)
	if p.offset != 9 || p.cursor != 9+scrollOff {
		t.Fatalf("ctrl-d from the top: offset %d cursor %d", p.offset, p.cursor)
	}
	act(m, keys.Top, 30)
	before, off := p.cursor, p.offset
	act(m, keys.HalfDown, 0)
	if p.cursor != before+9 || p.offset != off+9 {
		t.Fatalf("ctrl-d: cursor %d->%d offset %d->%d", before, p.cursor, off, p.offset)
	}
	act(m, keys.HalfUp, 0)
	if p.cursor != before || p.offset != off {
		t.Fatalf("ctrl-u: cursor %d offset %d, want %d %d", p.cursor, p.offset, before, off)
	}
	act(m, keys.HalfDown, 3)
	if p.cursor != before+3 || p.offset != off+3 {
		t.Fatalf("3 ctrl-d: cursor %d offset %d", p.cursor, p.offset)
	}
	act(m, keys.Bottom, 0)
	act(m, keys.HalfDown, 0)
	if p.cursor != 96 {
		t.Fatalf("ctrl-d at the end: cursor %d", p.cursor)
	}

	act(m, keys.Top, 0)
	act(m, keys.PageDown, 0)
	if p.offset != 17 || p.cursor != 17+scrollOff {
		t.Fatalf("ctrl-f: offset %d cursor %d", p.offset, p.cursor)
	}
	act(m, keys.PageUp, 0)
	if p.offset != 0 {
		t.Fatalf("ctrl-b: offset %d", p.offset)
	}
	act(m, keys.PageDown, 10)
	act(m, keys.PageDown, 0)
	if p.cursor != 96 {
		t.Fatalf("ctrl-f at the end: cursor %d", p.cursor)
	}
	act(m, keys.PageUp, 10)
	act(m, keys.PageUp, 0)
	if p.cursor != 0 {
		t.Fatalf("ctrl-b at the top: cursor %d", p.cursor)
	}
}

func TestLineScroll(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t))
	p := m.cur()
	act(m, keys.Top, 0)
	act(m, keys.ScrollDown, 0)
	if p.offset != 1 || p.cursor != 1+scrollOff {
		t.Fatalf("ctrl-e: offset %d cursor %d", p.offset, p.cursor)
	}
	act(m, keys.ScrollDown, 5)
	if p.offset != 6 {
		t.Fatalf("5 ctrl-e: offset %d", p.offset)
	}
	act(m, keys.ScrollUp, 2)
	if p.offset != 4 {
		t.Fatalf("2 ctrl-y: offset %d", p.offset)
	}
	m.Scroll(1000)
	if p.offset != 97-19 || p.cursor < p.offset+scrollOff {
		t.Fatalf("wheel down: offset %d cursor %d", p.offset, p.cursor)
	}
	m.Scroll(-1000)
	if p.offset != 0 || p.cursor > 19-1-scrollOff {
		t.Fatalf("wheel up: offset %d cursor %d", p.offset, p.cursor)
	}
}

func TestPlaceCursor(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t))
	p := m.cur()
	act(m, keys.Top, 50)
	act(m, keys.CenterCursor, 0)
	if row := p.cursor - p.offset; row != 9 {
		t.Fatalf("zz: row %d", row)
	}
	act(m, keys.CursorTop, 0)
	if row := p.cursor - p.offset; row != scrollOff {
		t.Fatalf("zt: row %d", row)
	}
	act(m, keys.CursorBottom, 0)
	if row := p.cursor - p.offset; row != 19-1-scrollOff {
		t.Fatalf("zb: row %d", row)
	}
	act(m, keys.CursorTop, 45)
	if pos(m) != 45 || p.cursor-p.offset != scrollOff {
		t.Fatalf("45zt: line %d row %d", pos(m), p.cursor-p.offset)
	}
	act(m, keys.Top, 0)
	act(m, keys.CenterCursor, 0)
	if p.offset != 0 {
		t.Fatalf("zz at the top scrolled to %d", p.offset)
	}
}

func TestChangeJumps(t *testing.T) {
	m := newModel(t, 80, 20, 100, sample(t))
	act(m, keys.Top, 0)
	act(m, keys.NextChange, 0)
	if m.cursorLine() != 30 {
		t.Fatalf("]c: line %d, want 30", m.cursorLine())
	}
	act(m, keys.NextChange, 0)
	if m.cursorLine() != 64 {
		t.Fatalf("]c: line %d, want 64", m.cursorLine())
	}
	if loc, _ := m.Cursor(); loc.OldLine != 62 || loc.NewLine != 64 {
		t.Fatalf("deleted row location %+v", loc)
	}
	act(m, keys.NextChange, 0)
	if m.cursorLine() != 64 {
		t.Fatalf("]c at the last change moved to %d", m.cursorLine())
	}
	act(m, keys.PrevChange, 0)
	act(m, keys.PrevChange, 0)
	if m.cursorLine() != 30 {
		t.Fatalf("[c[c: line %d, want 30", m.cursorLine())
	}
	act(m, keys.Bottom, 0)
	act(m, keys.PrevChange, 2)
	if m.cursorLine() != 30 {
		t.Fatalf("2[c: line %d, want 30", m.cursorLine())
	}
	act(m, keys.Top, 0)
	act(m, keys.NextChange, 2)
	if m.cursorLine() != 64 {
		t.Fatalf("2]c: line %d, want 64", m.cursorLine())
	}
	act(m, keys.Top, 33) // inside the first block
	act(m, keys.PrevChange, 0)
	if m.cursorLine() != 30 {
		t.Fatalf("[c inside a block: line %d, want its start", m.cursorLine())
	}
}

func TestChangeJumpShowsBlock(t *testing.T) {
	fd := fileDiff(t, join(ctx(1, 60), lines("+a1", "+a2", "+a3", "+a4", "+a5", "+a6", "+a7", "+a8"), ctx(61, 60))...)
	m := newModel(t, 80, 21, 100, fd) // body 20
	p := m.cur()
	act(m, keys.Top, 0)
	act(m, keys.NextChange, 0)
	end := p.folds.ItemOfLine(67)
	if end+scrollOff > p.offset+m.bodyHeight()-1 {
		t.Fatalf("block end %d not visible in window at %d", end, p.offset)
	}
}

func TestHorizontalScroll(t *testing.T) {
	long := "+x := 1 // " + strings.Repeat("abcdefghij", 20)
	fd := fileDiff(t, " short", long, " tail")
	m := newModel(t, 60, 6, 100, fd)
	act(m, keys.ScrollRight, 0)
	if m.xoff != hscrollStep {
		t.Fatalf("l: xoff %d", m.xoff)
	}
	act(m, keys.ScrollRight, 3)
	if m.xoff != 4*hscrollStep {
		t.Fatalf("3l: xoff %d", m.xoff)
	}
	act(m, keys.ScrollLeft, 2)
	if m.xoff != 2*hscrollStep {
		t.Fatalf("2h: xoff %d", m.xoff)
	}
	act(m, keys.ScrollRight, 1000)
	if m.xoff != m.maxXOffset() || m.xoff == 0 {
		t.Fatalf("1000l: xoff %d, max %d", m.xoff, m.maxXOffset())
	}
	act(m, keys.ScrollHome, 0)
	if m.xoff != 0 {
		t.Fatalf("0: xoff %d", m.xoff)
	}
	act(m, keys.ScrollLeft, 1)
	if m.xoff != 0 {
		t.Fatalf("h at column 0: xoff %d", m.xoff)
	}
}

func TestFirstSizeAfterSetFile(t *testing.T) {
	m := New(themeDefault(), nil, 100)
	m.SetFile("revue", "src/app.ts", fileDiff(t, join(ctx(1, 100), lines("+x"), ctx(101, 50))...), false)
	m.SetSize(80, 21)
	p := m.cur()
	if m.cursorLine() != 100 || p.cursor-p.offset != 20/3 {
		t.Fatalf("cursor line %d on screen row %d, want 100 on row %d", m.cursorLine(), p.cursor-p.offset, 20/3)
	}
	act(m, keys.Up, 1)
	m.SetSize(80, 30)
	if m.cursorLine() != 99 {
		t.Fatalf("a resize after a motion moved the cursor to %d", m.cursorLine())
	}
}

// A key that scrolls right must never move the view left, even when the
// scroll is past what the current view needs (after leaving the narrower
// split view, or after the pane grew).
func TestScrollRightNeverScrollsLeft(t *testing.T) {
	long := " const s = '" + strings.Repeat("x", 200) + "';"
	m := newModel(t, 120, 10, 100, fileDiff(t, long, "+added"))
	m.SetSplit(true)
	act(m, keys.ScrollRight, 1000)
	before := m.xoff
	m.SetSplit(false)
	act(m, keys.ScrollRight, 1)
	if m.xoff < before {
		t.Fatalf("l after leaving the split view scrolled left: %d -> %d", before, m.xoff)
	}

	m.SetSize(60, 10)
	act(m, keys.ScrollRight, 1000)
	before = m.xoff
	m.SetSize(120, 10)
	act(m, keys.ScrollRight, 1)
	if m.xoff < before {
		t.Fatalf("l after the pane grew scrolled left: %d -> %d", before, m.xoff)
	}
}
