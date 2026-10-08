package diffview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

var sizes = [][2]int{{0, 0}, {1, 1}, {0, 5}, {5, 0}, {10, 3}, {2, 5}, {7, 2}, {13, 4}, {24, 6}, {25, 8}, {40, 10}, {80, 24}, {120, 45}, {301, 3}}

// states puts a fresh model into each state worth rendering.
var states = map[string]func(t *testing.T, m *Model){
	"empty":   func(*testing.T, *Model) {},
	"message": func(_ *testing.T, m *Model) { m.SetMessage("no changes in a very long repository name\nsecond line") },
	"unified": func(t *testing.T, m *Model) { run(m, m.SetFile("revue", "src/app.ts", sample(t), false)) },
	"split": func(t *testing.T, m *Model) {
		run(m, m.SetFile("revue", "src/app.ts", sample(t), false))
		m.SetSplit(true)
	},
	"folded": func(t *testing.T, m *Model) {
		run(m, m.SetFile("revue", "src/deep/directory/structure/app.ts", sample(t), false))
		act(m, keys.ContextLess, 10)
		act(m, keys.Top, 0)
	},
	"folded-split": func(t *testing.T, m *Model) {
		run(m, m.SetFile("revue", "src/app.ts", sample(t), false))
		m.SetSplit(true)
		act(m, keys.ContextLess, 10)
		act(m, keys.Top, 0)
	},
	"visual-search": func(t *testing.T, m *Model) {
		run(m, m.SetFile("revue", "src/app.ts", sample(t), false))
		act(m, keys.VisualStart, 0)
		act(m, keys.Down, 3)
		m.SetSearch("const")
	},
	"scrolled": func(t *testing.T, m *Model) {
		run(m, m.SetFile("revue", "src/app.ts", fileDiff(t, " "+strings.Repeat("wide 世界 ", 30), "+\tx\t= 1"), false))
		act(m, keys.ScrollRight, 3)
	},
	"binary": func(t *testing.T, m *Model) {
		m.SetFile("revue", "logo.png", &diff.FileDiff{Binary: true}, false)
	},
}

func TestViewExactSize(t *testing.T) {
	for name, setup := range states {
		for _, sz := range sizes {
			for _, focused := range []bool{true, false} {
				m := New(themeDefault(), nil, 50)
				m.SetSize(sz[0], sz[1])
				setup(t, m)
				checkSize(t, m.View(focused), sz[0], sz[1])
				// Resizing after the content is set must work as well.
				m.SetSize(sz[1], sz[0])
				checkSize(t, m.View(focused), sz[1], sz[0])
			}
		}
		_ = name
	}
}

func TestActionsOnEveryStateDoNotPanic(t *testing.T) {
	var all []keys.Action
	for _, b := range keys.Bindings {
		all = append(all, b.Action)
	}
	for name, setup := range states {
		for _, sz := range [][2]int{{0, 0}, {1, 1}, {10, 3}, {80, 24}} {
			m := New(themeDefault(), nil, 50)
			m.SetSize(sz[0], sz[1])
			setup(t, m)
			for _, a := range all {
				for _, count := range []int{0, 1, 7, 1000} {
					if cmd := m.HandleAction(a, count); cmd != nil {
						cmd()
					}
					checkSize(t, m.View(true), sz[0], sz[1])
				}
			}
			m.Scroll(5)
			m.Scroll(-50)
			m.SetSplit(!m.Split())
			m.SetSearch("x")
			m.Selection()
			m.Cursor()
			m.Position()
			m.Stats()
			checkSize(t, m.View(false), sz[0], sz[1])
			_ = name
		}
	}
}

func TestEmptyBinaryLargeFiles(t *testing.T) {
	cases := map[string]*diff.FileDiff{
		"binary file":            {Binary: true},
		"file too large to show": {TooLarge: true},
		"empty file":             {},
		"nothing to show":        nil,
	}
	for want, fd := range cases {
		m := newModel(t, 60, 9, 50, nil)
		if cmd := m.SetFile("revue", "assets/logo.png", fd, false); cmd != nil {
			t.Errorf("%s: SetFile returned a highlight command", want)
		}
		got := plainLines(t, m, true)
		if !strings.Contains(got[0], "logo.png") {
			t.Errorf("%s: header %q", want, got[0])
		}
		if !strings.Contains(got[4], want) {
			t.Errorf("%s: middle line %q", want, got[4])
		}
		if l, total := m.Position(); l != 0 || total != 0 {
			t.Errorf("%s: position %d/%d", want, l, total)
		}
		if loc, ok := m.Cursor(); !ok || loc.Path != "assets/logo.png" {
			t.Errorf("%s: cursor %+v %v", want, loc, ok)
		}
		if y := yankOf(t, act(m, keys.YankRef, 0)); y.Text != "revue/assets/logo.png" {
			t.Errorf("%s: yp %q", want, y.Text)
		}
	}
}

func TestMessageCentred(t *testing.T) {
	m := newModel(t, 40, 11, 50, nil)
	m.SetMessage("no changes")
	got := plainLines(t, m, true)
	if strings.TrimSpace(got[0]) != "" {
		t.Fatalf("header %q", got[0])
	}
	line := got[5]
	if strings.TrimSpace(line) != "no changes" || strings.Index(line, "no") != 15 {
		t.Fatalf("message line %q", line)
	}
}

func TestMessageWrapsInNarrowPanes(t *testing.T) {
	const msg = "✓ atlas-front-end has no uncommitted changes"
	for _, w := range []int{40, 22, 12, 5, 1} {
		m := newModel(t, w, 12, 50, nil)
		m.SetMessage(msg)
		out := m.View(true)
		checkSize(t, out, w, 12)
		if w < 22 {
			continue // words longer than the pane are broken
		}
		if got := strings.Join(strings.Fields(ansi.Strip(out)), " "); got != msg {
			t.Errorf("width %d: message reads %q", w, got)
		}
	}
}

func TestHeader(t *testing.T) {
	m := newModel(t, 100, 10, 50, sample(t))
	m.SetFile("atlas-back-end", "src/projects/stats.service.ts", sample(t), false)
	h := plainLines(t, m, true)[0]
	for _, want := range []string{"src/projects/", "stats.service.ts", "+2 -3 ~1", "UNIFIED · ctx 50"} {
		if !strings.Contains(h, want) {
			t.Errorf("header %q lacks %q", h, want)
		}
	}
	if !strings.HasSuffix(h, "ctx 50 ") {
		t.Errorf("view mode is not right-aligned: %q", h)
	}
	m.SetSplit(true)
	if h := plainLines(t, m, true)[0]; !strings.Contains(h, "SPLIT · ctx 50") {
		t.Errorf("split header %q", h)
	}
	focused, unfocused := m.View(true), m.View(false)
	if !strings.Contains(focused, m.pal.seq(m.th.Accent, "", true)+"stats.service.ts") ||
		!strings.Contains(unfocused, m.pal.seq(m.th.FG, "", true)+"stats.service.ts") {
		t.Error("file name colour does not follow focus")
	}

	m.SetSize(30, 10)
	h = plainLines(t, m, true)[0]
	if !strings.Contains(h, "stats.service.ts") || strings.Contains(h, "SPLIT") {
		t.Errorf("narrow header %q", h)
	}
	m.SetSize(22, 10)
	if h = plainLines(t, m, true)[0]; !strings.Contains(h, "…") || !strings.Contains(h, "stats.service.ts") {
		t.Errorf("very narrow header %q", h)
	}
}

func TestUnifiedRows(t *testing.T) {
	m := newModel(t, 60, 40, 100, sample(t))
	act(m, keys.Top, 0)
	got := plainLines(t, m, true)
	want := map[int]string{
		1:  "▎  1   1   const c1 = 1;",
		31: "  31     ~ const a = 1;",
		32: "      31 ~ const a = 2;",
		33: "      32 + const b = 3;",
	}
	for i, w := range want {
		if strings.TrimRight(got[i], " ") != w {
			t.Errorf("line %d = %q, want %q", i, got[i], w)
		}
	}
	if unfocused := plainLines(t, m, false); strings.Contains(unfocused[1], "▎") {
		t.Error("cursor bar shown in an unfocused pane")
	}
}

func TestDeletedRowsAndSigns(t *testing.T) {
	m := newModel(t, 60, 12, 100, sample(t))
	act(m, keys.NextChange, 1)
	got := strings.Join(plainLines(t, m, true), "\n")
	for _, w := range []string{"  63     - gone2", "  64     - gone3", "▎ 62     - gone1"} {
		if !strings.Contains(got, w) {
			t.Errorf("view lacks %q:\n%s", w, got)
		}
	}
}

func TestSplitRows(t *testing.T) {
	m := newModel(t, 61, 40, 100, sample(t))
	m.SetSplit(true)
	act(m, keys.Top, 0)
	got := plainLines(t, m, true)
	if l := got[31]; !strings.Contains(l, "31 ~ const a = 1;") || !strings.Contains(l, "│ ") || !strings.HasSuffix(strings.TrimRight(l, " "), "31 ~ const a = 2;") {
		t.Errorf("pair row %q", l)
	}
	if l := got[32]; !strings.Contains(l, "╱╱╱") || !strings.Contains(l, "32 + const b = 3;") {
		t.Errorf("filler row %q", l)
	}
	left, _, _ := strings.Cut(got[32], "│")
	if ansi.StringWidth(left) != 30 {
		t.Errorf("left half is %d cells", ansi.StringWidth(left))
	}
}

func TestLongLineMarker(t *testing.T) {
	long := " const s = '" + strings.Repeat("x", 200) + "';"
	m := newModel(t, 50, 5, 100, fileDiff(t, long, " short", "+added"))
	got := plainLines(t, m, true)
	if !strings.HasSuffix(got[1], "x›") {
		t.Fatalf("long line %q does not end with ›", got[1])
	}
	if strings.Contains(got[2], "›") {
		t.Fatalf("short line %q has a ›", got[2])
	}
	act(m, keys.ScrollRight, 1000)
	if got = plainLines(t, m, true); strings.Contains(got[1], "›") || !strings.HasSuffix(strings.TrimRight(got[1], " "), "x';") {
		t.Fatalf("scrolled to the end: %q", got[1])
	}
}

func TestOnlyAllowedBackgrounds(t *testing.T) {
	m := newModel(t, 80, 20, 100, fileDiff(t, " a", " b", " c"))
	act(m, keys.Down, 1)
	out := m.View(true)
	cursor := "48;2;42;47;69" // theme.Default.CursorLineBG
	if n := strings.Count(out, "48;2;"); n == 0 || n != strings.Count(out, cursor) {
		t.Fatalf("context-only view paints backgrounds other than the cursor line:\n%q", out)
	}
}

func TestGlyphWidths(t *testing.T) {
	for _, g := range []string{barGlyph, moreGlyph, fillGlyph, splitGlyph, "⋯", "·", "…"} {
		if w := ansi.StringWidth(g); w != 1 {
			t.Errorf("%q is %d cells wide", g, w)
		}
	}
}

// Messages come from git errors, which can carry carriage returns and tabs.
// The terminal moves the cursor for those although ansi.StringWidth counts
// them as zero cells, so they must not reach the output.
func TestMessageControlCharacters(t *testing.T) {
	m := newModel(t, 40, 7, 50, nil)
	m.SetMessage("Could not read a.ts: exit status 128\r\nfatal:\tbad object\r")
	out := m.View(true)
	checkSize(t, out, 40, 7)
	for _, c := range ansi.Strip(out) {
		if c < 0x20 && c != '\n' {
			t.Fatalf("control character %q in the view:\n%q", c, out)
		}
	}
	if got := ansi.Strip(out); !strings.Contains(got, "fatal: bad object") {
		t.Fatalf("tab not expanded:\n%s", got)
	}
}
