package diffview

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// randomDiff builds a diff of random runs of context, deletions and
// additions. Deleted and added lines often differ in one word, so the
// pairing produces modified pairs as well as plain red and green rows.
func randomDiff(t testing.TB, r *rand.Rand) *diff.FileDiff {
	texts := []string{
		"const value = compute(a, b);",
		"\treturn x\t// tab",
		"  const s = 'Привіт світ';",
		"wide 世界 text 世界",
		"",
		strings.Repeat("long ", 40),
		"if (item.status === Status.Active) {",
	}
	var spec []string
	n := 1 + r.IntN(12)
	for range n {
		k := 1 + r.IntN(15)
		switch r.IntN(4) {
		case 0, 1:
			for range k {
				spec = append(spec, " "+texts[r.IntN(len(texts))]+fmt.Sprint(len(spec)))
			}
		case 2:
			for range k {
				spec = append(spec, "-"+texts[r.IntN(len(texts))]+fmt.Sprint(len(spec)%3))
			}
		default:
			for range k {
				spec = append(spec, "+"+texts[r.IntN(len(texts))]+fmt.Sprint(len(spec)%5))
			}
		}
	}
	return fileDiff(t, spec...)
}

// plainModel is newModel without syntax colours, which cost time and do
// not change where the cursor goes.
func plainModel(t testing.TB, w, h, context int, fd *diff.FileDiff) *Model {
	t.Helper()
	m := New(themeDefault(), nil, context)
	m.SetSize(w, h)
	m.SetFile("revue", "src/app.ts", fd, false)
	return m
}

var randomActions = []keys.Action{
	keys.Down, keys.Up, keys.Top, keys.Bottom, keys.HalfDown, keys.HalfUp,
	keys.PageDown, keys.PageUp, keys.ScrollDown, keys.ScrollUp,
	keys.CenterCursor, keys.CursorTop, keys.CursorBottom,
	keys.NextChange, keys.PrevChange, keys.ScrollLeft, keys.ScrollRight, keys.ScrollHome,
	keys.FoldOpen, keys.FoldClose, keys.FoldToggle, keys.FoldOpenAll, keys.FoldCloseAll,
	keys.ContextMore, keys.ContextLess, keys.VisualStart, keys.VisualEnd,
	keys.SearchNext, keys.SearchPrev,
}

// checkInvariants verifies what must hold after any sequence of actions.
func checkInvariants(t *testing.T, m *Model, step string) {
	t.Helper()
	checkSize(t, m.View(true), m.width, m.height)
	if !m.active() {
		return
	}
	p := m.cur()
	n, h := len(p.folds.Items()), m.bodyHeight()
	if p.cursor < 0 || p.cursor >= n {
		t.Fatalf("%s: cursor %d outside %d items", step, p.cursor, n)
	}
	if p.offset < 0 || p.offset > max(n-h, 0) {
		t.Fatalf("%s: offset %d outside [0,%d]", step, p.offset, max(n-h, 0))
	}
	if h > 0 && (p.cursor < p.offset || p.cursor >= p.offset+h) {
		t.Fatalf("%s: cursor %d outside the window [%d,%d)", step, p.cursor, p.offset, p.offset+h)
	}
	loc, ok := m.Cursor()
	if !ok || loc.NewLine < 0 || loc.NewLine > len(m.f.newLines) || loc.OldLine < 0 || loc.OldLine > len(m.f.oldLines) {
		t.Fatalf("%s: cursor location %+v (old %d, new %d lines)", step, loc, len(m.f.oldLines), len(m.f.newLines))
	}
	if len(m.f.newLines) > 0 && loc.NewLine == 0 {
		t.Fatalf("%s: no new-file line although the new file has %d", step, len(m.f.newLines))
	}
	if sel, ok := m.Selection(); ok {
		if sel.StartLine > sel.EndLine || sel.EndLine > len(m.f.newLines) || (len(m.f.newLines) > 0 && sel.StartLine < 1) {
			t.Fatalf("%s: selection range %d-%d of %d lines", step, sel.StartLine, sel.EndLine, len(m.f.newLines))
		}
		if m.anchor[0] < 0 || m.anchor[1] > m.lines(m.view()) || m.anchor[0] >= m.anchor[1] {
			t.Fatalf("%s: anchor %v of %d lines", step, m.anchor, m.lines(m.view()))
		}
	}
}

func TestRandomActionsKeepInvariants(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for iter := range 300 {
		fd := randomDiff(t, r)
		w, h := 10+r.IntN(140), 1+r.IntN(40)
		m := plainModel(t, w, h, r.IntN(12), fd)
		if r.IntN(3) == 0 {
			m.SetSearch("const")
		}
		for s := range 60 {
			step := fmt.Sprintf("iter %d step %d", iter, s)
			switch r.IntN(10) {
			case 0:
				m.SetSplit(!m.Split())
			case 1:
				m.Scroll(r.IntN(21) - 10)
			case 2:
				m.SetSize(10+r.IntN(140), 1+r.IntN(40))
			default:
				a := randomActions[r.IntN(len(randomActions))]
				count := 0
				if r.IntN(3) == 0 {
					count = r.IntN(60)
				}
				step += fmt.Sprintf(" %s x%d", a, count)
				act(m, a, count)
			}
			checkInvariants(t, m, step)
		}
		if r.IntN(2) == 0 {
			run(m, m.SetFile("revue", "src/app.ts", randomDiff(t, r), true))
			checkInvariants(t, m, fmt.Sprintf("iter %d reload", iter))
		}
	}
}

// TestSplitToggleKeepsLineEverywhere puts the cursor on every line of both
// views and checks that switching views keeps it on the same diff line and
// that switching back is a no-op.
func TestSplitToggleKeepsLineEverywhere(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	for iter := range 200 {
		fd := randomDiff(t, r)
		m := plainModel(t, 100, 1+r.IntN(30), r.IntN(6), fd)
		for _, split := range []bool{false, true} {
			m.SetSplit(split)
			act(m, keys.FoldOpenAll, 0)
			for l := range m.lines(m.view()) {
				p := m.cur()
				p.cursor = p.folds.ItemOfLine(l)
				m.follow(p)
				from := m.view()
				rowsBefore := m.rowsOn(from, l)
				screen := p.cursor - p.offset
				m.SetSplit(!split)
				to := m.view()
				got := m.cursorLine()
				// The other view must show one of the rows the line showed.
				if !overlaps(m.rowsOn(to, got), rowsBefore) {
					t.Fatalf("iter %d: view %d line %d (rows %v) -> view %d line %d (rows %v)",
						iter, from, l, rowsBefore, to, got, m.rowsOn(to, got))
				}
				inner := screen >= m.so() && screen <= m.bodyHeight()-1-m.so()
				if q := m.cur(); inner && q.cursor-q.offset != screen && q.offset != 0 && q.offset != max(len(q.folds.Items())-m.bodyHeight(), 0) {
					t.Fatalf("iter %d: screen row %d -> %d", iter, screen, q.cursor-q.offset)
				}
				m.SetSplit(split)
				if back := m.cursorLine(); back != l {
					t.Fatalf("iter %d: view %d line %d toggled twice is line %d", iter, from, l, back)
				}
			}
		}
	}
}

// rowsOn lists the unified rows line l of view v shows.
func (m *Model) rowsOn(v, l int) []int {
	if v == viewUnified {
		return []int{l}
	}
	sr := m.f.split[l]
	var out []int
	if sr.Left >= 0 {
		out = append(out, sr.Left)
	}
	if sr.Right >= 0 && sr.Right != sr.Left {
		out = append(out, sr.Right)
	}
	return out
}

func overlaps(a, b []int) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}

// TestFoldOpsKeepLineAndRow opens and closes every fold of random diffs from
// every position and checks where the cursor ends up.
func TestFoldOpsKeepLineAndRow(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	for iter := range 150 {
		m := plainModel(t, 100, 5+r.IntN(30), r.IntN(5), randomDiff(t, r))
		m.SetSplit(r.IntN(2) == 0)
		p := m.cur()
		for k := 0; ; k++ {
			items := p.folds.Items()
			fold := slices.IndexFunc(items, func(it diff.Item) bool { return it.Fold == k })
			if fold < 0 {
				break
			}
			it := items[fold]
			p.cursor = fold
			m.follow(p)
			row := p.cursor - p.offset
			// At the ends of the file the cursor may sit inside the
			// scrolloff margin, which applies again once the fold opens.
			inner := row >= m.so() && row <= m.bodyHeight()-1-m.so()
			act(m, keys.FoldOpen, 0)
			if m.cursorLine() != it.Start || inner && p.cursor-p.offset != row {
				t.Fatalf("iter %d fold %d: zo put the cursor on line %d row %d, want %d row %d",
					iter, k, m.cursorLine(), p.cursor-p.offset, it.Start, row)
			}
			l := it.Start + r.IntN(it.End-it.Start)
			p.cursor = p.folds.ItemOfLine(l)
			m.follow(p)
			row = p.cursor - p.offset
			act(m, keys.FoldClose, 0)
			got := p.folds.Items()[p.cursor]
			if got.Fold != k {
				t.Fatalf("iter %d fold %d: zc from line %d left the cursor on %+v", iter, k, l, got)
			}
			atEdge := p.offset == 0 || p.offset == max(len(p.folds.Items())-m.bodyHeight(), 0)
			if p.cursor-p.offset != row && !atEdge {
				t.Fatalf("iter %d fold %d: zc moved the cursor from row %d to %d", iter, k, row, p.cursor-p.offset)
			}
		}
	}
}

// TestSearchOffsetsUnicode checks that every match is a case-insensitive
// occurrence of the pattern at rune boundaries of the original text, for
// letters whose lower case has a different UTF-8 length.
func TestSearchOffsetsUnicode(t *testing.T) {
	alphabet := []rune("aAkKȺⱥİiσΣςßẞé́世 Kx")
	r := rand.New(rand.NewPCG(7, 8))
	for range 2000 {
		var b, p strings.Builder
		for range 1 + r.IntN(30) {
			b.WriteRune(alphabet[r.IntN(len(alphabet))])
		}
		for range 1 + r.IntN(3) {
			p.WriteRune(alphabet[r.IntN(len(alphabet))])
		}
		text, pattern := b.String(), p.String()
		fold := !hasUpper(pattern)
		lp := pattern
		if fold {
			lp = strings.ToLower(pattern)
		}
		last := 0
		find(text, lp, fold, func(s, e int) {
			if s < last || !utf8.RuneStart(text[s]) || (e < len(text) && !utf8.RuneStart(text[e])) {
				t.Fatalf("%q in %q: match [%d,%d) overlaps or splits a rune", pattern, text, s, e)
			}
			if got := text[s:e]; fold && strings.ToLower(got) != lp || !fold && got != pattern {
				t.Fatalf("%q in %q: match [%d,%d) is %q", pattern, text, s, e, got)
			}
			last = e
		})
	}
}
