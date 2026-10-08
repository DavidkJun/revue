package chat

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/claude"
	"github.com/DavidkJun/revue/internal/theme"
)

const tortureMarkdown = "# Title\n\nSome *emphasis*, **bold**, `code`, a [link](https://example.com) and " +
	"https://example.com/a/very/long/url/that/does/not/fit/anywhere/in/a/narrow/panel/at/all.\n\n" +
	"Відповідь українською, 中文字符 and emoji 🎉 in one paragraph that wraps.\n\n" +
	"1. first\n2. second with a long tail that needs wrapping across the panel width\n" +
	"   ```go\n   if err != nil { return fmt.Errorf(\"nested fence inside a list item: %w\", err) }\n   ```\n\n" +
	"> quoted text that is long enough to wrap around the panel width at least once\n\n" +
	"| col | value |\n|-----|-------|\n| a | a cell with a fairly long value inside |\n\n" +
	"```ts\nconst x = await this.repository.findOneOrFail({ where: { id }, relations: ['owner', 'tags'] });\n" +
	"\tif (!x) {\n\t\tthrow new Error(`missing ${id} — ${'中文'.repeat(30)}`);\n\t}\n```\n\n" +
	"```diff\n- old line\n+ new line\n```\n\n" +
	"```\nunlabelled\n```\n\n" +
	"---\n\nTrailing text.\n\n```python\ndef unclosed():\n    return 'streaming'"

func TestMarkdownFitsWidth(t *testing.T) {
	md := newMarkdown(theme.Default)
	for w := 1; w <= 130; w += 3 {
		lines, _ := md.render(tortureMarkdown, w, nil)
		if len(lines) == 0 {
			t.Fatalf("width %d: nothing rendered", w)
		}
		for i, l := range lines {
			if got := ansi.StringWidth(l); got != w {
				t.Fatalf("width %d line %d is %d cells: %q", w, i, got, l)
			}
			if stripBackground(l) != l {
				t.Fatalf("width %d line %d paints a background: %q", w, i, l)
			}
		}
		if strings.TrimSpace(ansi.Strip(lines[0])) == "" || strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
			t.Errorf("width %d: leading or trailing blank line", w)
		}
		// The renderer is cached per width; a second render must not
		// inherit state from the first.
		if again, _ := md.render(tortureMarkdown, w, nil); strings.Join(again, "\n") != strings.Join(lines, "\n") {
			t.Fatalf("width %d: a reused renderer rendered differently", w)
		}
	}
}

// Glamour wraps punctuation after styled text one cell past its width; the
// panel must not cut that punctuation off.
func TestMarkdownKeepsPunctuationAtWrap(t *testing.T) {
	md := newMarkdown(theme.Default)
	for _, c := range []struct{ src, want string }{
		{"1. `validate` calls `user.checkPassword`, which hashes with **bcrypt** and returns a promise.",
			"1. validate calls user.checkPassword, which hashes with bcrypt and returns a promise."},
		{"- `a`, `b`, and `c` are *all* required; see the docs.",
			"• a, b, and c are all required; see the docs."},
	} {
		for w := 20; w <= 60; w++ {
			lines, _ := md.render(c.src, w, nil)
			// Wraps may break inside "user.checkPassword", so spaces are not compared.
			got := strings.Join(strings.Fields(ansi.Strip(strings.Join(lines, " "))), "")
			if want := strings.ReplaceAll(c.want, " ", ""); got != want {
				t.Errorf("width %d: %q, want %q", w, got, want)
			}
		}
	}
}

func TestMarkdownKeepsCode(t *testing.T) {
	md := newMarkdown(theme.Default)
	long := "const x = await this.repository.findOneOrFail({ where: { id }, relations: ['owner', 'tags'] });"
	src := "Intro.\n\n```ts\n" + long + "\n\tindented()\n```\n\nOutro."
	lines, _ := md.render(src, 40, nil)
	plain := make([]string, len(lines))
	for i, l := range lines {
		plain[i] = strings.TrimRight(ansi.Strip(l), " ")
	}
	var code []string
	for _, l := range plain {
		if rest, ok := strings.CutPrefix(l, codeGutter); ok {
			code = append(code, rest)
		}
	}
	if len(code) < 4 {
		t.Fatalf("code lines = %q", code)
	}
	if !strings.HasPrefix(code[1], codeHang) {
		t.Errorf("continuation %q lacks the hanging indent", code[1])
	}
	var joined string
	for _, c := range code[:len(code)-1] {
		joined += strings.TrimPrefix(c, codeHang)
	}
	if strings.ReplaceAll(joined, " ", "") != strings.ReplaceAll(long, " ", "") {
		t.Errorf("wrapped code lost text:\n%q\n%q", joined, long)
	}
	if last := code[len(code)-1]; last != "    indented()" {
		t.Errorf("tab not expanded: %q", last)
	}
	if plain[0] != "Intro." || plain[len(plain)-1] != "Outro." {
		t.Errorf("text around the code: %q", plain)
	}
}

// Claude writes steps as "1. Do this:" with the code fenced three spaces in;
// the code must stay under its item's text, not jump to the list's edge.
func TestMarkdownCodeInListItemsStaysIndented(t *testing.T) {
	md := newMarkdown(theme.Default)
	src := "1. Install the package:\n   ```bash\n   npm i zod\n   ```\n2. Run it.\n\n- bullet:\n  ```go\n  x := 1\n  ```\n- next"
	lines, _ := md.render(src, 40, nil)
	var plain []string
	for _, l := range lines {
		plain = append(plain, strings.TrimRight(ansi.Strip(l), " "))
	}
	for _, want := range []string{"1. Install the package:", "   " + codeGutter + "npm i zod", "2. Run it.", "  " + codeGutter + "x := 1", "• next"} {
		if !slices.Contains(plain, want) {
			t.Errorf("no line %q in:\n%s", want, strings.Join(plain, "\n"))
		}
	}
	for _, l := range lines {
		if ansi.StringWidth(l) != 40 {
			t.Fatalf("line %q is %d cells", l, ansi.StringWidth(l))
		}
	}
}

func TestMarkdownReusesFinishedBlocks(t *testing.T) {
	md := newMarkdown(theme.Default)
	partial := "Intro.\n\n```go\nx := 1\n```\n\nThe answer goes"
	first, cache := md.render(partial, 50, nil)
	more, cache2 := md.render(partial+" on and on.", 50, cache)
	if len(cache2) != 3 || &cache2[1].lines[0] != &cache[1].lines[0] {
		t.Fatal("the finished code block was rendered again")
	}
	if len(more) != len(first) || ansi.Strip(more[len(more)-1]) == ansi.Strip(first[len(first)-1]) {
		t.Fatalf("the growing block was not re-rendered:\n%q\n%q", first, more)
	}
	if again, _ := md.render(partial, 40, cache); ansi.StringWidth(again[0]) != 40 {
		t.Fatal("a cached block of another width was reused")
	}
}

func TestStripBackground(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b[38;2;1;2;3;48;2;4;5;6mX":  "\x1b[38;2;1;2;3mX",
		"\x1b[48;5;17mX\x1b[0m":         "X\x1b[0m",
		"\x1b[38;2;48;48;48mX":          "\x1b[38;2;48;48;48mX",
		"\x1b[41;1mX":                   "\x1b[1mX",
		"\x1b[mX":                       "\x1b[mX",
		"\x1b[7;4mX":                    "\x1b[4mX",
		"\x1b[48:2::1:2:3;3mX":          "\x1b[3mX",
		"\x1b[38;5;48;104mX":            "\x1b[38;5;48mX",
		"\x1b]8;;http://x\aY\x1b]8;;\a": "\x1b]8;;http://x\aY\x1b]8;;\a",
		"\x1b[2KX":                      "\x1b[2KX",
		"plain":                         "plain",
		"cut \x1b[48;2":                 "cut \x1b[48;2",
	} {
		if got := stripBackground(in); got != want {
			t.Errorf("stripBackground(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitFences(t *testing.T) {
	type b = mdBlock
	for _, tc := range []struct {
		src  string
		want []mdBlock
	}{
		{"just text", []b{{text: "just text"}}},
		{"a\n```go\nx := 1\n```\nb", []b{{text: "a"}, {code: true, lang: "go", text: "x := 1"}, {text: "b"}}},
		{"~~~ py extra\nprint()\n~~~", []b{{code: true, lang: "py", text: "print()"}}},
		{"````\n```\ninner\n```\n````", []b{{code: true, text: "```\ninner\n```"}}},
		{"```ts\nstreaming", []b{{code: true, lang: "ts", text: "streaming"}}},
		{"  ```\n  two\n   three\n  ```", []b{{code: true, text: "two\n three", indent: 2}}},
		{"    ```\n    indented code", []b{{text: "    ```\n    indented code"}}},
		{"``` a`b\nnot a fence", []b{{text: "``` a`b\nnot a fence"}}},
		{"``\ninline", []b{{text: "``\ninline"}}},
	} {
		got := splitFences(tc.src)
		if fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("splitFences(%q) = %+v, want %+v", tc.src, got, tc.want)
		}
	}
}

func TestBreakRows(t *testing.T) {
	for _, tc := range []struct {
		s           string
		first, rest int
	}{
		{"short", 10, 8},
		{"a b c d e f g h i j k l m n o p", 7, 5},
		{"nobreaksinthislongwordatall", 6, 4},
		{"中文中文中文中文", 5, 3},
		{"x 🎉🎉🎉🎉 y", 4, 2},
		{"", 3, 3},
		{"abc", 1, 1},
	} {
		rows := breakRows(tc.s, tc.first, tc.rest)
		total := ansi.StringWidth(tc.s)
		pos := 0
		for i, r := range rows {
			limit := tc.rest
			if i == 0 {
				limit = tc.first
			}
			if r[0] != pos || r[1] < r[0] || (r[1]-r[0] > limit && r[1]-r[0] > 2) {
				t.Errorf("breakRows(%q, %d, %d) = %v: bad row %d", tc.s, tc.first, tc.rest, rows, i)
			}
			pos = r[1]
		}
		if pos != total {
			t.Errorf("breakRows(%q) = %v covers %d of %d cells", tc.s, rows, pos, total)
		}
	}
}

// panels returns the panel in every state worth checking, at a size.
func panels(w, h int) map[string]*Model {
	newPanel := func() *Model {
		m := New(Options{Theme: theme.Default, Model: "sonnet"})
		m.SetSize(w, h)
		return m
	}
	feed := func(m *Model, evs ...claude.Event) { m.Update(eventsMsg{turn: m.active.id, events: evs}) }
	out := map[string]*Model{}

	out["empty"] = newPanel()

	m := newPanel()
	m.StartPrompt(sampleItems, []string{"a/b.ts:1-2", "atlas-back-end/src/very/long/path/to/some/file.service.ts:120-140", "c", "d", "e"})
	for _, r := range "Чому тут await? 中文 🎉 and a long question that wraps" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.Update(shiftEnter)
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	out["prompting"] = m

	m = newPanel()
	m.begin("why is this async? "+strings.Repeat("long question ", 8), []string{"atlas-back-end/src/app.ts:40-52"})
	feed(m, claude.Event{Kind: claude.EventThinking})
	out["thinking"] = m

	m = newPanel()
	m.begin("q", []string{"x"})
	feed(m, claude.Event{Kind: claude.EventText, Text: "Let me look."}, claude.Event{Kind: claude.EventTool, Text: "Read " + strings.Repeat("dir/", 20) + "file.ts"})
	feed(m, claude.Event{Kind: claude.EventText, Text: "\n\n" + tortureMarkdown})
	out["streaming"] = m

	m = newPanel()
	for i := range 3 {
		m.begin(fmt.Sprintf("question %d", i), []string{"a", "b", "c", "d", "e", "f", "g", "h"})
		feed(m, claude.Event{Kind: claude.EventText, Text: tortureMarkdown}, claude.Event{Kind: claude.EventDone, Duration: 61 * time.Second})
	}
	out["answered"] = m

	m = newPanel()
	m.begin("fails", nil)
	feed(m, claude.Event{Kind: claude.EventError, Err: errors.New("claude: exited without a result:\n" + strings.Repeat("stderr ", 30))})
	m.begin("cancelled", nil)
	m.Cancel()
	m.StartPrompt(nil, nil)
	out["error+prompt"] = m
	return out
}

func TestViewExactSize(t *testing.T) {
	sizes := [][2]int{{0, 0}, {1, 1}, {1, 6}, {2, 2}, {6, 1}, {3, 12}, {12, 3}, {17, 9}, {30, 12}, {44, 7}, {60, 40}, {83, 25}, {141, 50}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		for name, m := range panels(w, h) {
			for _, focused := range []bool{true, false} {
				v := m.View(focused)
				if h == 0 {
					if v != "" {
						t.Errorf("%s %dx%d: want an empty view, got %q", name, w, h, v)
					}
					continue
				}
				lines := strings.Split(v, "\n")
				if len(lines) != h {
					t.Fatalf("%s %dx%d: %d lines", name, w, h, len(lines))
				}
				for i, l := range lines {
					if got := ansi.StringWidth(l); got != w {
						t.Fatalf("%s %dx%d: line %d is %d cells: %q", name, w, h, i, got, l)
					}
				}
				// The prompt's cursor is reverse video; everything else must
				// leave the terminal background alone.
				if name != "prompting" && name != "error+prompt" && stripBackground(v) != v {
					t.Errorf("%s %dx%d paints a background", name, w, h)
				}
			}
		}
	}
}

func TestViewResizeKeepsContent(t *testing.T) {
	m := panels(60, 30)["answered"]
	before := m.md.renders
	m.SetSize(45, 30)
	v := plainView(m)
	if m.md.renders == before || !strings.Contains(v, "Trailing text.") {
		t.Fatalf("resize did not re-render the answers:\n%s", v)
	}
	if !strings.Contains(v, "sonnet · 1m 1s") {
		t.Errorf("footer duration:\n%s", v)
	}
}

// Toggling the full width (or resizing the terminal) reflows the whole
// conversation; a reader scrolled into an earlier answer must stay there.
func TestResizeKeepsReadingPosition(t *testing.T) {
	m := New(Options{Theme: theme.Default})
	m.SetSize(40, 20)
	for _, turn := range "ABC" {
		tr := m.begin("question "+string(turn), nil)
		var b strings.Builder
		for _, p := range "abcdefghijkl" {
			fmt.Fprintf(&b, "Turn %c paragraph %c has enough words to wrap at a narrow width.\n\n", turn, p)
		}
		m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: b.String()}, {Kind: claude.EventDone}}})
	}
	m.View(true)
	for i, l := range m.body() {
		if strings.Contains(ansi.Strip(l), "Turn B paragraph f") {
			m.jumpTo(i)
			break
		}
	}
	// The anchor is proportional within the turn, so after a reflow the top
	// may land a few lines into a neighbouring paragraph; six lines always
	// show a paragraph start, even at the narrowest width.
	near := regexp.MustCompile(`Turn B paragraph [efg]`)
	top := func() string { return strings.Join(strings.Split(plainView(m), "\n")[1:7], "\n") }
	if got := top(); !strings.Contains(strings.Split(got, "\n")[0], "Turn B paragraph f") {
		t.Fatalf("setup: reading\n%s", got)
	}
	for _, w := range []int{100, 40, 23} {
		m.SetSize(w, 20)
		if got := top(); !near.MatchString(got) {
			t.Errorf("width %d: the view moved to\n%s\nwant to stay near Turn B paragraph f", w, got)
		}
	}
	m.SetSize(40, 20)
	if got := top(); !strings.Contains(strings.Split(got, "\n")[0], "Turn B paragraph f") {
		t.Errorf("back at the first width the view drifted to\n%s", got)
	}
}

func TestEmptyHint(t *testing.T) {
	m := New(Options{Theme: theme.Default, Model: "sonnet"})
	m.SetSize(60, 20)
	v := plainView(m)
	if !strings.Contains(v, "Select lines with v and press a to ask Claude") {
		t.Fatalf("empty panel:\n%s", v)
	}
	lines := strings.Split(v, "\n")
	for i, l := range lines {
		if strings.Contains(l, "Select lines") {
			if i < 6 || i > 14 {
				t.Errorf("hint on line %d of 20 is not centred", i)
			}
			if lead := len(l) - len(strings.TrimLeft(l, " ")); lead < 4 {
				t.Errorf("hint is not centred horizontally: %q", l)
			}
		}
	}
}

func TestEmptyHintIsWholeOrGone(t *testing.T) {
	const hint = "Claude can read the repos for more context"
	for _, size := range [][2]int{{30, 10}, {30, 8}, {24, 9}, {30, 6}, {60, 7}, {14, 12}} {
		m := New(Options{Theme: theme.Default, Model: "sonnet"})
		m.SetSize(size[0], size[1])
		m.StartPrompt([]claude.ContextItem{{Repo: "r", Path: "a.ts"}}, []string{"r/a.ts:1"})
		v := plainView(m)
		got := strings.Join(strings.Fields(v), " ")
		if strings.Contains(got, "Claude") && strings.Contains(got, "read") && !strings.Contains(got, hint) {
			t.Errorf("%dx%d: the hint is cut short:\n%s", size[0], size[1], v)
		}
	}
}

func TestHeader(t *testing.T) {
	m := New(Options{Theme: theme.Default, Model: "opus"})
	m.SetSize(50, 5)
	if h := m.header(true); !strings.Contains(h, ansiFG(theme.Default.Accent)) || !strings.Contains(ansi.Strip(h), icon+" Claude · opus") {
		t.Errorf("focused header %q", h)
	}
	if h := m.header(false); strings.Contains(h, ansiFG(theme.Default.Accent)) {
		t.Errorf("unfocused header uses the accent: %q", h)
	}
	tr := m.begin("q", nil)
	m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventThinking}}})
	if h := ansi.Strip(m.header(true)); !strings.Contains(h, "thinking…") {
		t.Errorf("thinking header %q", h)
	}
	m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: "x"}}})
	if h := ansi.Strip(m.header(true)); !strings.Contains(h, "answering…") {
		t.Errorf("answering header %q", h)
	}
	m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventDone}}})
	if h := ansi.Strip(m.header(true)); !strings.HasSuffix(h, "1 turn ") {
		t.Errorf("idle header %q", h)
	}
}
