package highlight

import (
	"math/rand/v2"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
)

var tsSample = []string{
	"import { Injectable } from '@nestjs/common';",
	"/* first line of a comment",
	" * second line",
	" */",
	"export const answer: number = 42;",
	"function greet(name: string) {",
	"\treturn `hi ${name}`;",
	"}",
}

// spanAt returns the span covering byte off, if any.
func spanAt(spans []Span, off int) (Span, bool) {
	for _, s := range spans {
		if s.Start <= off && off < s.End {
			return s, true
		}
	}
	return Span{}, false
}

// checkSpans verifies spans are what Render's fast path expects: inside the
// line, non-empty, sorted and disjoint.
func checkSpans(t *testing.T, name string, lines []string, out [][]Span) {
	t.Helper()
	if len(out) != len(lines) {
		t.Fatalf("%s: %d span lines for %d lines", name, len(out), len(lines))
	}
	for i, spans := range out {
		end := 0
		for _, s := range spans {
			if s.Start < end || s.Start >= s.End || s.End > len(lines[i]) {
				t.Fatalf("%s line %d: bad span %+v in %+v (line %q)", name, i, s, spans, lines[i])
			}
			end = s.End
		}
	}
}

func TestFileTypeScript(t *testing.T) {
	h := New("")
	out := h.File("src/app/answer.ts", tsSample)
	checkSpans(t, "ts", tsSample, out)

	for _, kw := range []struct {
		line int
		word string
	}{{0, "import"}, {4, "export"}, {4, "const"}, {5, "function"}, {6, "return"}} {
		off := strings.Index(tsSample[kw.line], kw.word)
		s, ok := spanAt(out[kw.line], off)
		if !ok || s.FG == "" || !strings.HasPrefix(s.FG, "#") {
			t.Errorf("keyword %q on line %d has no colour: %+v", kw.word, kw.line, out[kw.line])
		}
	}

	comment := h.pal.of(chroma.CommentMultiline)
	if comment.fg == "" {
		t.Fatalf("the style has no comment colour")
	}
	for line := 1; line <= 3; line++ {
		text := tsSample[line]
		for off := 0; off < len(text); off++ {
			if text[off] == ' ' {
				continue
			}
			s, ok := spanAt(out[line], off)
			if !ok || s.FG != comment.fg || s.Italic != comment.italic {
				t.Fatalf("comment line %d byte %d (%q) is not comment-coloured: %+v", line, off, text[off], out[line])
			}
		}
	}

	// Text in the style's base colour (here the parenthesis) carries no span,
	// so it takes the line's foreground.
	off := strings.Index(tsSample[5], "(")
	if s, ok := spanAt(out[5], off); ok && s.FG != "" {
		t.Errorf("base-coloured text got a colour: %+v", s)
	}
}

func TestFileCommonTypes(t *testing.T) {
	samples := map[string][]string{
		"a.ts":                  {"const x: number = 1;"},
		"a.tsx":                 {"export const App = () => <div className=\"x\">{1}</div>;"},
		"a.js":                  {"function f() { return 1; }"},
		"a.jsx":                 {"const el = <a href=\"x\">y</a>;"},
		"a.json":                {`{"a": 1, "b": [true, null]}`},
		"a.html":                {`<div class="x">hi</div>`},
		"a.scss":                {"$c: #fff;", ".a { color: $c; }"},
		"a.css":                 {".a { color: red; }"},
		"a.md":                  {"# Title", "", "Some *text* and `code`."},
		"a.yml":                 {"key: value", "list:", "  - 1"},
		"a.yaml":                {"key: value"},
		"a.go":                  {"package main", "", "func main() {}"},
		"a.py":                  {"def f(x):", "    return x"},
		"a.sql":                 {"SELECT id FROM users WHERE id = 1;"},
		"a.sh":                  {"echo \"hi\" # comment"},
		"a.ps1":                 {"$x = Get-Item .", "Write-Host $x"},
		"Dockerfile":            {"FROM node:20", "RUN npm ci"},
		".env":                  {"API_URL=http://localhost", "# comment"},
		".env.local":            {"KEY=value"},
		`C:\repo\src\main.ts`:   {"let a = 1;"},
		"atlas-front-end/x.tsx": {"let a = 1;"},
	}
	h := New("tokyonight-storm")
	for name, lines := range samples {
		out := h.File(name, lines)
		if out == nil {
			t.Errorf("%s: no highlighting", name)
			continue
		}
		checkSpans(t, name, lines, out)
		coloured := false
		for _, spans := range out {
			for _, s := range spans {
				coloured = coloured || s.FG != ""
			}
		}
		if !coloured {
			t.Errorf("%s: nothing got a colour: %+v", name, out)
		}
	}
}

// Callers split file content without its final newline, but lexers match
// line endings, so the last line must colour as it does when one follows.
func TestFileLastLineColoursLikeTheOthers(t *testing.T) {
	h := New("")
	cases := []struct{ name, line string }{
		{"README.md", "# Title"},
		{"README.md", "- item"},
		{"README.md", "> quote"},
		{"README.md", "1. first"},
		{"App.jsx", "// trailing comment"},
		{"app.scss", "// trailing comment"},
		{"a.ts", "// trailing comment"},
		{"a.go", "// trailing comment"},
		{"a.yaml", "key: value # note"},
	}
	for _, tc := range cases {
		last := h.File(tc.name, []string{"x", tc.line})
		inner := h.File(tc.name, []string{"x", tc.line, "x"})
		if last == nil || inner == nil {
			t.Errorf("%s: no highlighting", tc.name)
			continue
		}
		if !reflect.DeepEqual(last[1], inner[1]) {
			t.Errorf("%s: %q as the last line gets %+v, elsewhere %+v", tc.name, tc.line, last[1], inner[1])
		}
	}
}

func TestFileNoLexer(t *testing.T) {
	h := New("")
	for _, name := range []string{"notes.unknownext", "README.txt", "noext", ".gitignore"} {
		if out := h.File(name, []string{"const a = 1;"}); out != nil {
			t.Errorf("%s: got %+v, want nil", name, out)
		}
	}
	if out := h.File("a.ts", nil); out != nil {
		t.Errorf("no lines: got %+v, want nil", out)
	}
	var nilH *Highlighter
	if out := nilH.File("a.ts", tsSample); out != nil {
		t.Errorf("nil highlighter: got %+v, want nil", out)
	}
}

func TestFileLimits(t *testing.T) {
	h := New("")
	repeat := func(line string, n int) []string {
		lines := make([]string, n)
		for i := range lines {
			lines[i] = line
		}
		return lines
	}
	cases := []struct {
		name  string
		lines []string
		ok    bool
	}{
		{"20000 lines", repeat("x;", maxFileLines), true},
		{"20001 lines", repeat("x;", maxFileLines+1), false},
		{"10000-byte line", []string{"let a = '" + strings.Repeat("a", maxLineBytes-11) + "';"}, true},
		{"10001-byte line", []string{"let a = '" + strings.Repeat("a", maxLineBytes-10) + "';"}, false},
		{"over 1 MB", repeat(strings.Repeat("a", 9000), 120), false},
	}
	for _, tc := range cases {
		out := h.File("a.ts", tc.lines)
		if (out != nil) != tc.ok {
			t.Errorf("%s: highlighted = %v, want %v", tc.name, out != nil, tc.ok)
		}
	}
}

func TestFileBlankLines(t *testing.T) {
	lines := []string{"   ", "\t\t", "", " \t "}
	out := New("").File("a.ts", lines)
	if out == nil {
		t.Fatalf("whitespace-only file: no highlighting")
	}
	for i, spans := range out {
		if len(spans) != 0 {
			t.Errorf("line %d (%q) got spans %+v", i, lines[i], spans)
		}
	}
}

// A line holding a newline is not a line; the spans must still fit the
// lines they are returned for.
func TestFileNewlineInsideALine(t *testing.T) {
	lines := []string{"let a\n= 'a long string here';", "x"}
	if out := New("").File("a.ts", lines); out != nil {
		checkSpans(t, "newline inside a line", lines, out)
	}
}

// Within every size limit, markdown full of unclosed '[' makes chroma's link
// rule rescan the rest of the text for each one: minutes for a megabyte.
func TestFileGivesUpOnSlowInput(t *testing.T) {
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("[", maxLineBytes)
	}
	const budget = 100 * time.Millisecond
	done := make(chan [][]Span, 1)
	start := time.Now()
	go func() { done <- highlightFile(newPalette(""), "notes.md", lines, budget) }()
	select {
	case out := <-done:
		if out != nil {
			t.Errorf("slow input was highlighted after all")
		}
		// One token past the deadline is the most it may overrun by.
		if took := time.Since(start); took > budget+time.Second {
			t.Errorf("gave up after %v on a %v budget", took, budget)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("still lexing after 20s on a %v budget", budget)
	}
}

func TestFileOffsetsSurviveOddBytes(t *testing.T) {
	h := New("")
	lines := []string{
		"const a = \"\xff\xfe\"; const b = 1;",
		"let c = 'привет'; let d = 2;\r",
		"\tconst e = `中文 \U0001F600`; const f = 3;",
	}
	out := h.File("a.ts", lines)
	checkSpans(t, "odd bytes", lines, out)
	// The keyword after the odd bytes must get exactly the span the same
	// keyword before them got.
	for i, word := range []string{"const", "let", "const"} {
		first, _ := spanAt(out[i], strings.Index(lines[i], word))
		off := strings.LastIndex(lines[i], word)
		s, ok := spanAt(out[i], off)
		if first.FG == "" || !ok || s.FG != first.FG || s.Start != off || s.End != off+len(word) {
			t.Errorf("line %d: %q at byte %d not covered exactly by a keyword span: %+v", i, word, off, out[i])
		}
	}
}

func TestNewFallsBackToDefaultStyle(t *testing.T) {
	want := New(VSCodeDarkPlus).File("a.ts", tsSample)
	for _, name := range []string{"", "no-such-style", "VSCode-Dark-Plus"} {
		if got := New(name).File("a.ts", tsSample); !reflect.DeepEqual(got, want) {
			t.Errorf("New(%q) colours differently from %s", name, VSCodeDarkPlus)
		}
	}
	if got := New("monokai").File("a.ts", tsSample); reflect.DeepEqual(got, want) {
		t.Errorf("New(\"monokai\") ignored the style")
	}
	var zero Highlighter
	if got := zero.File("a.ts", tsSample); !reflect.DeepEqual(got, want) {
		t.Errorf("zero Highlighter does not use the default style")
	}
}

func TestFileConcurrent(t *testing.T) {
	h := New("")
	inputs := []struct {
		name  string
		lines []string
	}{
		{"a.ts", tsSample},
		{"b.go", []string{"package b", "", "// Doc.", "func B() int { return 1 }"}},
		{"c.html", []string{"<html>", "<style>a { color: red }</style>", "<script>let x = 1;</script>", "</html>"}},
		{"d.md", []string{"# T", "", "```ts", "const a = 1;", "```"}},
		{"e.json", []string{`{"a": [1, 2, {"b": null}]}`}},
	}
	want := make([][][]Span, len(inputs))
	for i, in := range inputs {
		want[i] = h.File(in.name, in.lines)
	}
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := range 10 {
				i := (g + k) % len(inputs)
				if got := h.File(inputs[i].name, inputs[i].lines); !reflect.DeepEqual(got, want[i]) {
					t.Errorf("goroutine %d: %s differs from the sequential result", g, inputs[i].name)
				}
			}
		}()
	}
	wg.Wait()
}

func TestFileSpansRender(t *testing.T) {
	h := New("")
	out := h.File("a.ts", tsSample)
	for i, line := range tsSample {
		checkRender(t, line, out[i], nil, LineStyle{Width: 50, FG: "#d5dbff"})
	}
}

// codeBits are what random source lines are made of: token starters of the
// common languages plus the bytes that shift offsets.
var codeBits = []string{
	"const ", "let ", "x", " = ", "1", "'", "\"", "`", "${", "}", "{", "(", ")", ";", "//", "/*", "*/", "#", "<", ">",
	"</", "-", "*", "_", "[", "]", ":", " ", "\t", "\r", "\x00", "\xff", "\xe2\x82", "\U0000FFFD", "п", "中",
	"\U0001F600", "e\U00000301", "\\", "@", "$", "%", "!", "?", "|", "&", "---", "```", "~~", "**",
}

func TestFileRandomLines(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	names := []string{"a.ts", "a.tsx", "a.js", "a.jsx", "a.json", "a.html", "a.scss", "a.css", "a.md", "a.yml",
		"a.go", "a.py", "a.sql", "a.sh", "a.ps1", "Dockerfile"}
	h := New("")
	for range 400 {
		lines := make([]string, 1+r.IntN(6))
		for i := range lines {
			var b strings.Builder
			for k := r.IntN(12); k >= 0; k-- {
				b.WriteString(codeBits[r.IntN(len(codeBits))])
			}
			lines[i] = b.String()
		}
		name := names[r.IntN(len(names))]
		out := h.File(name, lines)
		if out == nil {
			t.Fatalf("%s: no highlighting for %+q", name, lines)
		}
		checkSpans(t, name, lines, out)
		for i, l := range lines {
			checkRender(t, l, out[i], nil, LineStyle{Width: 30})
		}
	}
}
