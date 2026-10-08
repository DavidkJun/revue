package sidebar

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

func checkSize(t *testing.T, m *Model, w, h int, focused bool) []string {
	t.Helper()
	v := m.View(focused)
	if h == 0 {
		if v != "" {
			t.Fatalf("%dx%d: want empty view, got %q", w, h, v)
		}
		return nil
	}
	lines := strings.Split(v, "\n")
	if len(lines) != h {
		t.Fatalf("%dx%d: %d lines", w, h, len(lines))
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != w {
			t.Fatalf("%dx%d: line %d is %d cells: %q", w, h, i, got, ansi.Strip(l))
		}
	}
	return lines
}

func TestViewExactSizeEverywhere(t *testing.T) {
	models := map[string]func() *Model{
		"empty":  func() *Model { return New(theme.Default) },
		"norepo": func() *Model { return newModel(1, 1) },
		"sample": func() *Model { return newModel(1, 1, sampleRepos()...) },
		"long": func() *Model {
			return newModel(1, 1, repo("a-very-long-repository-name-that-goes-on", paths(
				"M deeply/nested/one/two/three/four/five/six/seven/eight/nine/ten/x.ts",
				"M deeply/nested/one/two/a-file-name-that-is-really-much-too-long-for-any-pane.service.spec.ts",
				"? docs/звіти/квартальний-звіт-про-зміни-в-архітектурі-системи.md",
				"? docs/日本語のファイル名です.md",
			)...))
		},
		"hostile": func() *Model {
			r := repo("репо-日本語-👨‍👩‍👧", paths(
				"M 👨‍👩‍👧‍👦-family.ts", "M é́combining.ts", "M ❤️heart.md", "M bad\xff\xfeutf8.ts",
				"M 日本語/中文目录名字很长/한국어파일이름.ts", "M ｆｕｌｌｗｉｄｔｈ.ts", "M zero​width.ts",
				"M tab\there.ts", "? 日/本/語.md",
			)...)
			r.Status.Branch = "feature/日本語-ветка-👍"
			r.Status.Ahead, r.Status.Behind = 123, 4567
			bad := errRepo("错误")
			bad.Err = errors.New("git status: fatal: 日本語のエラー\x1b[31m (exit status 128)")
			return newModel(1, 1, r, bad)
		},
	}
	for name, mk := range models {
		t.Run(name, func(t *testing.T) {
			for _, w := range []int{0, 1, 2, 3, 5, 8, 12, 16, 20, 28, 36, 50, 120} {
				for _, h := range []int{0, 1, 2, 3, 10, 40} {
					m := mk()
					m.SetSize(w, h)
					m.Select(0, "")
					act(m, keys.Bottom, 0)
					checkSize(t, m, w, h, true)
					checkSize(t, m, w, h, false)
					act(m, keys.Up, 2)
					checkSize(t, m, w, h, true)
				}
			}
		})
	}
}

func TestViewLayout(t *testing.T) {
	m := newModel(40, 12,
		repo("api", fc("M src/app.ts", 12, 3), fc("A src/new.ts", 40, 0), fc("D old.ts", 0, 9)),
		repo("web"),
	)
	lines := plainLines(m.View(true))
	if !strings.Contains(lines[0], "Files") || !strings.HasSuffix(lines[0], "3 ") {
		t.Errorf("header %q", lines[0])
	}
	for i, frag := range []string{"api", "src", "app.ts", "new.ts", "old.ts", "web ✓"} {
		if !strings.Contains(lines[i+1], frag) {
			t.Errorf("row %d %q lacks %q", i+1, lines[i+1], frag)
		}
	}
	if !strings.HasSuffix(lines[3], "+12 -3 ") || !strings.HasSuffix(lines[4], "+40 ") || !strings.HasSuffix(lines[5], "-9 ") {
		t.Errorf("counts not right-aligned:\n%s", strings.Join(lines[3:6], "\n"))
	}
	// Two cells per level: the file letters sit one level right of "src".
	if col(lines[3], "M") != col(lines[2], "▾")+2 {
		t.Errorf("indentation:\n%s\n%s", lines[2], lines[3])
	}
	if col(lines[5], "D") != col(lines[2], "▾") {
		t.Errorf("a repo-level file aligns with repo-level directories:\n%s\n%s", lines[2], lines[5])
	}
}

func TestViewOnlyCursorRowHasBackground(t *testing.T) {
	m := newModel(40, 20, sampleRepos()...)
	m.Select(0, "package.json")
	act(m, keys.Mark, 0)
	lines := strings.Split(m.View(true), "\n")
	bgRows := 0
	for i, l := range lines {
		hasBG := strings.Contains(l, ";48;2;") || strings.Contains(l, "[48;2;")
		if i == m.cursor-m.offset+1 {
			if !hasBG {
				t.Errorf("cursor row %d has no background", i)
			}
			bgRows++
			continue
		}
		if hasBG {
			t.Errorf("row %d paints a background: %q", i, l)
		}
	}
	if bgRows != 1 {
		t.Fatalf("%d cursor rows", bgRows)
	}
	sel := ansiBG(theme.Default.SelectedBG)
	if row := lines[m.cursor-m.offset+1]; strings.Count(row, sel) < 5 {
		t.Errorf("cursor row should carry the selection background through every segment: %q", row)
	}
}

func ansiBG(hex string) string {
	c, _ := parseHex(hex)
	return "48;2;" + strconv.Itoa(int(c.R)) + ";" + strconv.Itoa(int(c.G)) + ";" + strconv.Itoa(int(c.B))
}

func TestViewHeaderFocus(t *testing.T) {
	m := newModel(30, 3, repo("api", paths("M a.ts")...))
	accent := fgSGR(theme.Default.Accent)
	if head := strings.Split(m.View(true), "\n")[0]; !strings.Contains(head, accent) {
		t.Errorf("focused header should use the accent: %q", head)
	}
	if head := strings.Split(m.View(false), "\n")[0]; strings.Contains(head, accent) {
		t.Errorf("unfocused header should be muted: %q", head)
	}
}

func fgSGR(hex string) string { return "3" + ansiBG(hex)[1:] }

func TestViewTruncatesLongNamesKeepingCounts(t *testing.T) {
	long := "a-file-name-that-is-really-much-too-long-for-the-pane.service.ts"
	m := newModel(36, 5, repo("api", fc("M "+long, 120, 45)))
	row := plainLines(m.View(true))[2]
	if !strings.HasSuffix(row, "+120 -45 ") {
		t.Errorf("counts must stay visible: %q", row)
	}
	if !strings.Contains(row, "…") || strings.Contains(row, long) {
		t.Errorf("name should be cut with an ellipsis: %q", row)
	}
	if ansi.StringWidth(row) != 36 {
		t.Errorf("width %d", ansi.StringWidth(row))
	}
}

func TestViewCyrillicAndWideNames(t *testing.T) {
	m := newModel(30, 6, repo("api",
		fc("? Звіт-про-зміни-в-системі.md", 15, 0),
		fc("? 日本語のファイル名です.md", 3, 0),
	))
	lines := plainLines(m.View(true))
	if !strings.Contains(lines[2], "Звіт") || !strings.Contains(lines[2], "…") {
		t.Errorf("cyrillic row %q", lines[2])
	}
	if !strings.Contains(lines[3], "日本") {
		t.Errorf("wide row %q", lines[3])
	}
	checkSize(t, m, 30, 6, true)
}

func TestViewRenameShowsOldNameWhenItFits(t *testing.T) {
	rn := fc("R src/new-name.ts", 1, 1)
	rn.OldPath = "src/old-name.ts"
	moved := fc("R lib/moved.ts", 0, 0)
	moved.OldPath = "src/moved.ts"
	m := newModel(60, 6, repo("api", rn, moved))
	lines := plainLines(m.View(true))
	if !strings.Contains(lines[5], "old-name.ts → new-name.ts") {
		t.Errorf("same-dir rename: %q", lines[5])
	}
	if !strings.Contains(lines[3], "src/moved.ts → moved.ts") {
		t.Errorf("move: %q", lines[3])
	}
	m.SetSize(28, 6)
	lines = plainLines(m.View(true))
	if strings.Contains(lines[5], "→") || !strings.Contains(lines[5], "new-name.ts") {
		t.Errorf("narrow rename should show just the new name: %q", lines[5])
	}
}

func TestViewMarksStatusAndRepoStates(t *testing.T) {
	bin := fc("A logo.png", 0, 0)
	bin.Binary = true
	conflict := fc("U merge.go", 3, 3)
	loading := repo("loading")
	loading.Loaded = false
	detached := repo("detached", paths("M a.ts")...)
	detached.Status.Branch = ""
	detached.Status.Ahead, detached.Status.Behind = 2, 5
	m := newModel(50, 12, repo("api", bin, conflict), loading, errRepo("broken"), detached)
	m.Select(0, "merge.go")
	act(m, keys.Mark, 0)
	lines := plainLines(m.View(true))
	if !strings.HasPrefix(lines[3], markGlyph) || !strings.Contains(lines[3], "U") {
		t.Errorf("marked conflicted file: %q", lines[3])
	}
	if !strings.HasSuffix(lines[2], "bin ") {
		t.Errorf("binary file: %q", lines[2])
	}
	if !strings.Contains(lines[4], "loading …") {
		t.Errorf("loading repo: %q", lines[4])
	}
	if !strings.Contains(lines[5], "broken ! bad object HEAD") || strings.Contains(lines[5], "fatal") {
		t.Errorf("error repo shows git's explanation without the lead-in: %q", lines[5])
	}
	if !strings.Contains(lines[6], iconCommit+" abc1234 ↑2 ↓5") {
		t.Errorf("detached repo: %q", lines[6])
	}
	raw := strings.Split(m.View(true), "\n")
	if !strings.Contains(raw[3], fgSGR(theme.Default.Magenta)) {
		t.Errorf("mark should be magenta: %q", raw[3])
	}
}

func TestViewCollapsedRepoShowsFileCount(t *testing.T) {
	m := newModel(40, 4, repo("api", paths("M a.ts", "M b.ts")...))
	act(m, keys.Collapse, 0)
	if row := plainLines(m.View(true))[1]; !strings.HasSuffix(row, "2 ") {
		t.Errorf("collapsed repo row %q", row)
	}
}

func TestViewNoRepos(t *testing.T) {
	m := newModel(30, 4)
	if row := plainLines(m.View(true))[1]; !strings.Contains(row, "No git repositories") {
		t.Errorf("empty state %q", row)
	}
	m = New(theme.Default)
	m.SetSize(30, 4)
	if strings.TrimSpace(ansi.Strip(m.View(true))) != iconTree+" Files" {
		t.Errorf("before the first SetRepos only the header shows: %q", ansi.Strip(m.View(true)))
	}
}

func TestViewSanitizesControlCharacters(t *testing.T) {
	m := newModel(40, 4, repo("api", fc("M bad\x1b[31mname\t.ts", 1, 0)))
	row := strings.Split(m.View(true), "\n")[2]
	if strings.Contains(row, "\x1b[31m") || strings.Contains(row, "\t") {
		t.Errorf("control characters leaked: %q", row)
	}
	checkSize(t, m, 40, 4, true)
}

func TestShortError(t *testing.T) {
	if got := shortError(errors.New("  first line \n second")); got != "first line" {
		t.Errorf("shortError = %q", got)
	}
}

// col is the cell column where sub starts in s.
func col(s, sub string) int {
	i := strings.Index(s, sub)
	if i < 0 {
		return -1
	}
	return ansi.StringWidth(s[:i])
}

func TestViewReplacesInvalidUTF8(t *testing.T) {
	// The terminal gives every invalid byte a cell of its own while
	// ansi.StringWidth counts none, so such a name would overflow the row.
	m := newModel(30, 3, repo("api", fc("M bad\xff\xfename.ts", 1, 0)))
	row := strings.Split(m.View(true), "\n")[2]
	if !utf8.ValidString(row) {
		t.Errorf("row carries invalid UTF-8: %q", row)
	}
	if !strings.Contains(ansi.Strip(row), "bad?name.ts") {
		t.Errorf("row %q", ansi.Strip(row))
	}
}

func TestShortErrorKeepsGitsExplanation(t *testing.T) {
	for in, want := range map[string]string{
		"git status: fatal: not a git repository (or any of the parent directories): .git (exit status 128)": "not a git repository (or any of the parent directories): .git (exit status 128)",
		"git diff: error: unable to read tree (exit status 128)":                                             "unable to read tree (exit status 128)",
		"git status: exit status 1": "exit status 1",
		"context deadline exceeded": "context deadline exceeded",
	} {
		if got := shortError(errors.New(in)); got != want {
			t.Errorf("shortError(%q) = %q, want %q", in, got, want)
		}
	}
}
