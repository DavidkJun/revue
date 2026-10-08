package sidebar

import (
	"strconv"
	"testing"

	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

// sampleTree: r:api, d:src, f:src/a.ts, f:src/b.ts, f:z.ts, r:web, f:x.ts
func sampleTree(h int) *Model {
	return newModel(40, h,
		repo("api", paths("M src/a.ts", "M src/b.ts", "M z.ts")...),
		repo("web", paths("M x.ts")...),
	)
}

func TestMoveDownUpWithCounts(t *testing.T) {
	m := sampleTree(20)
	wantNoMsg(t, act(m, keys.Down, 0)) // src
	wantSelected(t, act(m, keys.Down, 0), 0, "src/a.ts")
	wantSelected(t, act(m, keys.Down, 4), 1, "x.ts") // z.ts, web, x.ts
	wantNoMsg(t, act(m, keys.Down, 5))               // already at the bottom
	wantSelected(t, act(m, keys.Up, 2), 0, "z.ts")
	wantNoMsg(t, act(m, keys.Up, 100))
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
}

func TestTopBottomWithCounts(t *testing.T) {
	m := sampleTree(20)
	wantSelected(t, act(m, keys.Bottom, 0), 1, "x.ts")
	wantNoMsg(t, act(m, keys.Top, 0))
	if m.cursor != 0 {
		t.Fatalf("gg: cursor = %d", m.cursor)
	}
	wantSelected(t, act(m, keys.Bottom, 3), 0, "src/a.ts") // 3G: row 3
	wantSelected(t, act(m, keys.Top, 4), 0, "src/b.ts")    // 4gg: row 4
	act(m, keys.Bottom, 99)
	if m.cursor != len(m.rows)-1 {
		t.Fatalf("99G: cursor = %d", m.cursor)
	}
}

func longTree(n, h int) *Model {
	var files []git.FileChange
	for i := 0; i < n; i++ {
		files = append(files, fc("M f"+pad3(i)+".ts", 1, 1))
	}
	return newModel(40, h, repo("api", files...))
}

func pad3(i int) string {
	s := strconv.Itoa(i)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

func TestScrolloffKeepsFourRows(t *testing.T) {
	m := longTree(100, 21) // 20 body rows
	act(m, keys.Down, 15)
	if m.offset != 0 {
		t.Fatalf("offset = %d after reaching row 15 of 20", m.offset)
	}
	act(m, keys.Down, 1) // row 16: 3 rows below would remain
	if m.offset != 1 {
		t.Fatalf("offset = %d, want 1", m.offset)
	}
	act(m, keys.Bottom, 0)
	if m.offset != m.maxOffset() || m.cursor != 100 {
		t.Fatalf("G: offset %d cursor %d", m.offset, m.cursor)
	}
	act(m, keys.Up, 16)
	if m.cursor-m.offset != 4 {
		t.Fatalf("scrolling up keeps 4 rows above: cursor %d offset %d", m.cursor, m.offset)
	}
}

func TestScrolloffShrinksInTinyPanes(t *testing.T) {
	m := longTree(30, 4) // 3 body rows: scrolloff 1
	act(m, keys.Down, 2)
	if m.offset != 1 {
		t.Fatalf("offset = %d, want 1", m.offset)
	}
	m.SetSize(40, 2) // one body row
	act(m, keys.Down, 5)
	if m.offset != m.cursor {
		t.Fatalf("one-row pane must show the cursor row: offset %d cursor %d", m.offset, m.cursor)
	}
}

func TestHalfAndFullPages(t *testing.T) {
	m := longTree(100, 21)
	act(m, keys.HalfDown, 0)
	// The view moves half a page; scrolloff then pushes the cursor down.
	if m.cursor != 14 || m.offset != 10 {
		t.Fatalf("ctrl-d: cursor %d offset %d", m.cursor, m.offset)
	}
	act(m, keys.HalfDown, 0)
	if m.cursor != 24 || m.offset != 20 {
		t.Fatalf("ctrl-d twice: cursor %d offset %d", m.cursor, m.offset)
	}
	act(m, keys.HalfUp, 0)
	if m.cursor != 14 || m.offset != 10 {
		t.Fatalf("ctrl-u: cursor %d offset %d", m.cursor, m.offset)
	}
	act(m, keys.PageDown, 0)
	if m.offset != 28 || m.cursor != 32 {
		t.Fatalf("ctrl-f: cursor %d offset %d", m.cursor, m.offset)
	}
	act(m, keys.PageDown, 10)
	if m.cursor != 100 || m.offset != m.maxOffset() {
		t.Fatalf("10 ctrl-f: cursor %d offset %d", m.cursor, m.offset)
	}
	act(m, keys.PageUp, 0)
	act(m, keys.PageUp, 0)
	act(m, keys.PageUp, 0)
	act(m, keys.PageUp, 0)
	act(m, keys.PageUp, 0)
	act(m, keys.PageUp, 0)
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("ctrl-b to the top: cursor %d offset %d", m.cursor, m.offset)
	}
}

func TestScrollLinesDragsCursor(t *testing.T) {
	m := longTree(100, 21)
	msg := act(m, keys.ScrollDown, 6)
	if m.offset != 6 || m.cursor != 10 {
		t.Fatalf("ctrl-e: offset %d cursor %d", m.offset, m.cursor)
	}
	wantSelected(t, msg, 0, "f009.ts")
	act(m, keys.ScrollUp, 2)
	if m.offset != 4 || m.cursor != 10 {
		t.Fatalf("ctrl-y keeps the cursor when it stays in view: offset %d cursor %d", m.offset, m.cursor)
	}
}

func TestCenterTopBottom(t *testing.T) {
	m := longTree(100, 21)
	act(m, keys.Down, 50)
	act(m, keys.CenterCursor, 0)
	if m.offset != 40 {
		t.Fatalf("zz: offset %d", m.offset)
	}
	act(m, keys.CursorTop, 0)
	if m.offset != 46 {
		t.Fatalf("zt: offset %d", m.offset)
	}
	act(m, keys.CursorBottom, 0)
	if m.offset != 35 {
		t.Fatalf("zb: offset %d", m.offset)
	}
}

func TestWheelScrollsViewOnly(t *testing.T) {
	m := longTree(100, 21)
	m.Scroll(30)
	if m.offset != 30 || m.cursor != 0 {
		t.Fatalf("wheel: offset %d cursor %d", m.offset, m.cursor)
	}
	m.Scroll(1000)
	if m.offset != m.maxOffset() {
		t.Fatalf("wheel past the end: offset %d", m.offset)
	}
	m.Scroll(-1000)
	if m.offset != 0 {
		t.Fatalf("wheel past the top: offset %d", m.offset)
	}
}

func TestExpandAndCollapseOnEveryNodeKind(t *testing.T) {
	m := sampleTree(20)
	// Expanded repo: Expand goes to its first child.
	wantNoMsg(t, act(m, keys.Expand, 0))
	if curLabel(m) != "d:src" {
		t.Fatalf("expand on open repo → %s", curLabel(m))
	}
	// Expanded dir: Expand goes to its first child, a file.
	wantSelected(t, act(m, keys.Expand, 0), 0, "src/a.ts")
	// File: Expand asks to open it.
	wantSelected(t, act(m, keys.Expand, 0), 0, "src/a.ts")
	if !m.OpenRequested() || m.OpenRequested() {
		t.Fatal("OpenRequested must report once, then clear")
	}
	// File: Collapse goes to the parent.
	wantNoMsg(t, act(m, keys.Collapse, 0))
	if curLabel(m) != "d:src" {
		t.Fatalf("collapse on file → %s", curLabel(m))
	}
	// Expanded dir: Collapse collapses it.
	act(m, keys.Collapse, 0)
	equal(t, rowsOf(m), []string{"r:", "  d:src", "  f:z.ts", "r:", "  f:x.ts"})
	// Collapsed dir: Collapse goes to the parent.
	act(m, keys.Collapse, 0)
	if curLabel(m) != "r:" {
		t.Fatalf("collapse on closed dir → %s", curLabel(m))
	}
	// Expanded repo: Collapse collapses it; collapsed repo: nothing happens.
	act(m, keys.Collapse, 0)
	act(m, keys.Collapse, 0)
	equal(t, rowsOf(m), []string{"r:", "r:", "  f:x.ts"})
	if m.cursor != 0 {
		t.Fatalf("cursor %d", m.cursor)
	}
	// Collapsed repo: Expand expands and stays; collapsed dir remembered.
	act(m, keys.Expand, 0)
	equal(t, rowsOf(m), []string{"r:", "  d:src", "  f:z.ts", "r:", "  f:x.ts"})
	if m.cursor != 0 {
		t.Fatalf("expand moved the cursor to %d", m.cursor)
	}
	// Collapsed dir: Expand expands and stays.
	act(m, keys.Down, 0)
	act(m, keys.Expand, 0)
	if curLabel(m) != "d:src" || len(m.rows) != 7 {
		t.Fatalf("expand on closed dir: on %s with %d rows", curLabel(m), len(m.rows))
	}
	if m.OpenRequested() {
		t.Fatal("only files request opening")
	}
}

func TestExpandOnCleanRepo(t *testing.T) {
	m := newModel(40, 10, repo("clean"))
	wantNoMsg(t, act(m, keys.Expand, 0))
	if !m.isExpanded(m.roots[0]) {
		t.Fatal("expand should open a clean repo")
	}
	wantNoMsg(t, act(m, keys.Expand, 0)) // nothing to move into
	act(m, keys.Collapse, 0)
	if m.isExpanded(m.roots[0]) {
		t.Fatal("collapse should close it again")
	}
}

func TestMarkFileDirAndRepo(t *testing.T) {
	m := sampleTree(20)
	m.Select(0, "src/b.ts")
	msg := act(m, keys.Mark, 0)
	if n, ok := msg.(uimsg.Notify); !ok || n.Text != "1 file marked for Claude" {
		t.Fatalf("mark file: %#v", msg)
	}
	act(m, keys.Up, 2) // src
	if msg := act(m, keys.Mark, 0); msg.(uimsg.Notify).Text != "2 files marked for Claude" {
		t.Fatalf("mark dir with one marked file should mark both: %#v", msg)
	}
	if msg := act(m, keys.Mark, 0); msg.(uimsg.Notify).Text != "No files marked for Claude" {
		t.Fatalf("mark dir with all marked should unmark: %#v", msg)
	}
	act(m, keys.Up, 0) // repo
	act(m, keys.Mark, 0)
	m.Select(1, "x.ts")
	act(m, keys.Mark, 0)
	got := m.Marked()
	want := []FileRef{{0, "src/a.ts"}, {0, "src/b.ts"}, {0, "z.ts"}, {1, "x.ts"}}
	if len(got) != len(want) {
		t.Fatalf("Marked() = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Marked() = %v, want %v", got, want)
		}
	}
	if msg := act(m, keys.ClearMarks, 0); msg.(uimsg.Notify).Text != "Marks cleared" {
		t.Fatalf("clear: %#v", msg)
	}
	if len(m.Marked()) != 0 {
		t.Fatal("marks left after clearing")
	}
	wantNoMsg(t, act(m, keys.ClearMarks, 0))
	act(m, keys.Mark, 0)
	m.ClearMarks()
	if len(m.Marked()) != 0 {
		t.Fatal("ClearMarks left marks")
	}
}

func TestMarkCollapsedDirMarksHiddenFiles(t *testing.T) {
	m := sampleTree(20)
	act(m, keys.Down, 0)
	act(m, keys.Collapse, 0)
	act(m, keys.Mark, 0)
	if len(m.Marked()) != 2 {
		t.Fatalf("Marked() = %v", m.Marked())
	}
}

func TestMarkOnCleanRepoDoesNothing(t *testing.T) {
	m := newModel(40, 10, repo("clean"))
	wantNoMsg(t, act(m, keys.Mark, 0))
}

func TestSelectExpandsCollapsedParents(t *testing.T) {
	m := sampleTree(20)
	act(m, keys.Down, 0)
	act(m, keys.Collapse, 0) // src
	act(m, keys.Collapse, 0) // to api
	act(m, keys.Collapse, 0) // collapse api
	wantSelected(t, m.Select(0, "src/b.ts")(), 0, "src/b.ts")
	if curLabel(m) != "f:src/b.ts" {
		t.Fatalf("cursor on %s", curLabel(m))
	}
	equal(t, rowsOf(m), []string{"r:", "  d:src", "    f:a.ts", "    f:b.ts", "  f:z.ts", "r:", "  f:x.ts"})

	if cmd := m.Select(1, ""); cmd != nil {
		t.Fatal("selecting a repo node must not emit FileSelected")
	}
	if curLabel(m) != "r:" || m.current().repo != 1 {
		t.Fatalf("cursor on %s", curLabel(m))
	}
	if m.Select(0, "nope.ts") != nil || m.Select(5, "") != nil || m.Select(-1, "") != nil {
		t.Fatal("unknown targets must be ignored")
	}
	if m.current().repo != 1 {
		t.Fatal("a failed Select must not move the cursor")
	}
}

func TestSelectScrollsIntoView(t *testing.T) {
	m := longTree(100, 11)
	m.Select(0, "f080.ts")
	if m.cursor < m.offset || m.cursor >= m.offset+m.bodyHeight() {
		t.Fatalf("cursor %d not visible at offset %d", m.cursor, m.offset)
	}
}

func TestClick(t *testing.T) {
	m := sampleTree(20)
	wantSelected(t, m.Click(3)(), 0, "src/a.ts") // header, api, src, a.ts
	if curLabel(m) != "f:src/a.ts" {
		t.Fatalf("cursor on %s", curLabel(m))
	}
	if m.Click(2) != nil {
		t.Fatal("clicking a directory must not emit FileSelected")
	}
	equal(t, rowsOf(m), []string{"r:", "  d:src", "  f:z.ts", "r:", "  f:x.ts"})
	m.Click(2)
	if len(m.rows) != 7 || curLabel(m) != "d:src" {
		t.Fatal("second click should reopen the directory")
	}
	m.Click(1)
	equal(t, rowsOf(m), []string{"r:", "r:", "  f:x.ts"})
	for _, y := range []int{0, -1, 4, 19, 20, 100} {
		before := m.cursor
		if m.Click(y) != nil || m.cursor != before {
			t.Fatalf("click on row %d should do nothing", y)
		}
	}
}

func TestClickUsesScrollOffset(t *testing.T) {
	m := longTree(100, 11)
	m.Scroll(50)
	wantSelected(t, m.Click(1)(), 0, "f049.ts") // row 50 is the 50th file (row 0 is the repo)
}

func TestCurrent(t *testing.T) {
	m := New(theme.Default)
	if _, _, ok := m.Current(); ok {
		t.Fatal("no repos: ok must be false")
	}
	m = sampleTree(20)
	if r, f, ok := m.Current(); !ok || r != 0 || f != nil {
		t.Fatalf("on repo: %d %v %v", r, f, ok)
	}
	m.Select(1, "x.ts")
	if r, f, ok := m.Current(); !ok || r != 1 || f == nil || f.Path != "x.ts" {
		t.Fatalf("on file: %d %v %v", r, f, ok)
	}
}

func TestCurrentDir(t *testing.T) {
	if _, _, ok := New(theme.Default).CurrentDir(); ok {
		t.Fatal("no repos: ok must be false")
	}
	m := newModel(40, 20,
		repo("api", paths("M src/app/core/a.ts", "M src/app/core/b.ts", "M z.ts")...),
		repo("web", paths("M x.ts")...),
	)
	for _, tc := range []struct {
		down int
		repo int
		dir  string
		ok   bool
	}{
		{0, 0, "", true},             // repo node
		{1, 0, "src/app/core", true}, // compacted chain: its deepest directory
		{1, 0, "", false},            // a.ts
		{3, 1, "", true},             // web
		{1, 0, "", false},            // x.ts
	} {
		if tc.down > 0 {
			act(m, keys.Down, tc.down)
		}
		r, dir, ok := m.CurrentDir()
		if ok != tc.ok || ok && (r != tc.repo || dir != tc.dir) {
			t.Fatalf("on %s: CurrentDir() = %d %q %v, want %d %q %v", curLabel(m), r, dir, ok, tc.repo, tc.dir, tc.ok)
		}
	}
}

func TestActionsOnEmptyModel(t *testing.T) {
	m := New(theme.Default)
	for _, a := range []keys.Action{keys.Down, keys.Up, keys.Top, keys.Bottom, keys.HalfDown, keys.HalfUp,
		keys.PageDown, keys.PageUp, keys.ScrollDown, keys.ScrollUp, keys.CenterCursor, keys.CursorTop,
		keys.CursorBottom, keys.Expand, keys.Collapse, keys.Mark, keys.ClearMarks, keys.Search} {
		wantNoMsg(t, act(m, a, 3))
	}
	m.Scroll(5)
	m.Click(1)
	m.Select(0, "")
	if m.Files() != nil || m.Marked() != nil {
		t.Fatal("empty model has no files")
	}
}

func TestClickKeepsRowUnderPointer(t *testing.T) {
	m := longTree(100, 21)
	m.Scroll(50)
	m.Click(1)
	if m.offset != 50 || m.cursor != 50 {
		t.Fatalf("click on the top row: offset %d cursor %d, want the view to stay at 50", m.offset, m.cursor)
	}
	m.Click(20)
	if m.offset != 50 || m.cursor != 69 {
		t.Fatalf("click on the bottom row: offset %d cursor %d, want the view to stay at 50", m.offset, m.cursor)
	}

	// Toggling a directory on the last row leaves it where it was clicked,
	// so a second click on the same spot closes it again.
	var files []git.FileChange
	for i := 0; i < 30; i++ {
		files = append(files, fc("M d"+pad3(i)+"/a.ts", 1, 0))
	}
	m = newModel(40, 11, repo("api", files...))
	for i := range 30 {
		m.Select(0, "d"+pad3(i)+"/a.ts")
		act(m, keys.Collapse, 0)
		act(m, keys.Collapse, 0)
	}
	act(m, keys.Top, 0)
	m.Click(10)
	if got := curLabel(m); got != "d:d008" || m.offset != 0 {
		t.Fatalf("after the first click: cursor on %s offset %d", got, m.offset)
	}
	m.Click(10)
	if got := curLabel(m); got != "d:d008" || m.isExpanded(m.current()) {
		t.Fatalf("second click hit %s (expanded %v), want d:d008 closed again", got, m.isExpanded(m.current()))
	}
}
