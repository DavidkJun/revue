package highlight

import (
	"strings"
	"testing"
)

// Dark+ colours as the user's Neovim (vscode.nvim) shows them.
const (
	cKeyword  = "#569cd6"
	cControl  = "#c586c0"
	cVariable = "#9cdcfe"
	cFunction = "#dcdcaa"
	cType     = "#4ec9b0"
	cConstant = "#4fc1ff"
	cString   = "#ce9178"
	cNumber   = "#b5cea8"
	cComment  = "#6a9955"
)

// colourCase expects word (its n-th occurrence on the line, 1-based) to be
// painted entirely in want.
type colourCase struct {
	line int
	word string
	nth  int
	want string
}

func checkColours(t *testing.T, filename string, lines []string, cases []colourCase) {
	t.Helper()
	out := New(VSCodeDarkPlus).File(filename, lines)
	if out == nil {
		t.Fatalf("%s: no highlighting", filename)
	}
	for _, c := range cases {
		off := -1
		for n, from := max(c.nth, 1), 0; n > 0; n-- {
			k := strings.Index(lines[c.line][from:], c.word)
			if k < 0 {
				t.Fatalf("line %d: %q (occurrence %d) not found", c.line, c.word, c.nth)
			}
			off, from = from+k, from+k+len(c.word)
		}
		s, ok := spanAt(out[c.line], off)
		if !ok || s.FG != c.want || s.End < off+len(c.word) {
			t.Errorf("line %d %q: span %+v (found %v), want FG %s over the whole word", c.line, c.word, s, ok, c.want)
		}
	}
}

func TestRefineTypeScriptLooksLikeNeovim(t *testing.T) {
	lines := []string{
		`import { Injectable } from '@nestjs/common';`,
		`export const LIMIT = 3000;`,
		`export const format = (date: Date): string => dayjs(date).format('YYYY');`,
		`@Injectable()`,
		`export class ReportService extends Base {`,
		`  constructor(private node: BaseNode) {}`,
		`  get domain() { return this.node.name; }`,
		`  build(items: string[]) {`,
		`    for (const item of items) { if (!item) continue; }`,
		`    return get(this.node, 'x', Array.from(items));`,
		`  }`,
		`}`,
		`const total = items.map((i) => i.id).length; // done`,
		`const doubled = (n: number): number => n * 2;`,
		`const sum = (a + b) * 2;`,
		`const run = async () => {};`,
	}
	checkColours(t, "a.ts", lines, []colourCase{
		{0, "import", 1, cControl},
		{0, "Injectable", 1, cType},
		{0, "from", 1, cControl},
		{0, "'@nestjs/common'", 1, cString},
		{1, "export", 1, cControl},
		{1, "const", 1, cKeyword},
		{1, "LIMIT", 1, cConstant},
		{1, "3000", 1, cNumber},
		{2, "format", 1, cFunction},
		{2, "date", 1, cVariable},
		{2, "Date", 1, cType},
		{2, "string", 1, cType},
		{2, "dayjs", 1, cFunction},
		{2, "format", 2, cFunction},
		{3, "@Injectable", 1, cFunction},
		{4, "class", 1, cKeyword},
		{4, "ReportService", 1, cType},
		{4, "Base", 1, cType},
		{5, "constructor", 1, cKeyword},
		{5, "private", 1, cKeyword},
		{5, "node", 1, cVariable},
		{5, "BaseNode", 1, cType},
		{6, "get", 1, cKeyword},
		{6, "domain", 1, cFunction},
		{6, "return", 1, cControl},
		{6, "this", 1, cKeyword},
		{6, "node", 1, cVariable},
		{6, "name", 1, cVariable},
		{7, "build", 1, cFunction},
		{8, "for", 1, cControl},
		{8, "of", 1, cControl},
		{8, "if", 1, cControl},
		{8, "item", 2, cVariable},
		{9, "get", 1, cFunction},
		{9, "Array", 1, cType},
		{9, "from", 1, cFunction},
		{12, "total", 1, cVariable},
		{12, "map", 1, cFunction},
		{12, "id", 1, cVariable},
		{12, "length", 1, cVariable},
		{12, "// done", 1, cComment},
		{13, "doubled", 1, cFunction},
		{14, "sum", 1, cVariable},
		{15, "run", 1, cFunction},
	})
}

func TestRefineTSX(t *testing.T) {
	lines := []string{`const el = <Button onClick={() => setOpen(true)} disabled />;`}
	checkColours(t, "a.tsx", lines, []colourCase{
		{0, "Button", 1, cType},
		{0, "onClick", 1, cVariable},
		{0, "setOpen", 1, cFunction},
	})
}

func TestRefineOtherLanguages(t *testing.T) {
	checkColours(t, "a.py", []string{"import os", "def f(x):", "    if x:", "        return 1"}, []colourCase{
		{0, "import", 1, cControl},
		{1, "def", 1, cKeyword},
		{2, "if", 1, cControl},
		{3, "return", 1, cControl},
	})
	checkColours(t, "a.go", []string{"package main", "", "func main() {", "\tif ok {", "\t\treturn", "\t}", "}"}, []colourCase{
		{2, "func", 1, cKeyword},
		{3, "if", 1, cControl},
		{4, "return", 1, cControl},
	})
	checkColours(t, "a.json", []string{`{"name": "revue", "count": 2}`}, []colourCase{
		{0, `"name"`, 1, cVariable},
		{0, `"revue"`, 1, cString},
		{0, "2", 1, cNumber},
	})
}
