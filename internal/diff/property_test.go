package diff

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/rivo/uniseg"
)

// pieces are the building blocks of random lines: plain code, whitespace of
// every kind, multi-byte letters, emoji sequences, combining marks (also
// detached ones), CR LF and invalid UTF-8.
var pieces = []string{
	"x", "y", "foo", "bar_1", "42", ":=", "(", ")", ",", ".", "\"", "{", "}",
	" ", "  ", "\t", " ", "\r\n",
	"привіт", "значення", "світ",
	"👍", "👍🏽", "👨‍👩‍👧", "🇺🇦", "́", "é", "é",
	"\xff", "\xc3", "\xe2\x82",
}

func randLine(rng *rand.Rand, maxPieces int) string {
	var b strings.Builder
	for range rng.IntN(maxPieces + 1) {
		b.WriteString(pieces[rng.IntN(len(pieces))])
	}
	return b.String()
}

// mutate returns s with a few pieces inserted, removed or replaced, so that
// the result is often similar enough to s to pair with it.
func mutate(rng *rand.Rand, s string) string {
	for range rng.IntN(3) {
		at := 0
		if len(s) > 0 {
			at = rng.IntN(len(s) + 1)
		}
		switch rng.IntN(3) {
		case 0:
			s = s[:at] + pieces[rng.IntN(len(pieces))] + s[at:]
		case 1:
			s = s[:at] + s[min(len(s), at+1+rng.IntN(3)):]
		default:
			s = s[:at] + pieces[rng.IntN(len(pieces))] + s[min(len(s), at+2):]
		}
	}
	return s
}

// randomFileDiff builds a diff of up to 40 lines with change blocks whose
// deletions and additions come in any interleaving. Texts are drawn from a
// small pool, so identical, similar, blank and unrelated lines all occur.
func randomFileDiff(rng *rand.Rand) *FileDiff {
	pool := []string{"", "   ", "\t"}
	for range 3 + rng.IntN(6) {
		pool = append(pool, randLine(rng, 14))
	}
	text := func() string {
		s := pool[rng.IntN(len(pool))]
		if rng.IntN(2) == 0 {
			s = mutate(rng, s)
		}
		return s
	}
	fd := &FileDiff{}
	oldNo, newNo := 0, 0
	for range rng.IntN(41) {
		switch rng.IntN(5) {
		case 0, 1:
			oldNo++
			newNo++
			fd.Lines = append(fd.Lines, Line{Kind: Context, OldNo: oldNo, NewNo: newNo, Text: text()})
		case 2, 3:
			oldNo++
			fd.Lines = append(fd.Lines, Line{Kind: Deleted, OldNo: oldNo, Text: text()})
		default:
			newNo++
			fd.Lines = append(fd.Lines, Line{Kind: Added, NewNo: newNo, Text: text()})
		}
	}
	return fd
}

func describe(fd *FileDiff) string {
	var b strings.Builder
	for _, l := range fd.Lines {
		fmt.Fprintf(&b, "\n  %c%q", " +-"[l.Kind], l.Text)
	}
	return b.String()
}

func TestUnifiedProperties(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 12))
	for iter := range 3000 {
		fd := randomFileDiff(rng)
		rows := Unified(fd)
		checkRows(t, fd, rows)
		checkRowSources(t, fd, rows)
		checkPairs(t, rows)
		checkEmissionOrder(t, rows)
		checkPairingIsOptimal(t, fd, rows)
		if t.Failed() {
			t.Fatalf("iteration %d, input:%s", iter, describe(fd))
		}
	}
}

// Blocks above maxDensePairs are paired within diagonal bands; the result
// must keep every invariant even though it is no longer exhaustive.
func TestUnifiedLargeBlockProperties(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 14))
	for iter := range 200 {
		fd := randomLargeBlock(rng)
		rows := Unified(fd)
		checkRows(t, fd, rows)
		checkRowSources(t, fd, rows)
		checkPairs(t, rows)
		checkEmissionOrder(t, rows)
		checkSplit(t, rows, Split(rows))
		if t.Failed() {
			t.Fatalf("iteration %d, input:%s", iter, describe(fd))
		}
	}
}

// randomLargeBlock builds one interleaved change block of 60 to 300 lines a
// side, or a skewed one of a few lines against up to 2700, so that both the
// banded pairing and its wide-window case run.
func randomLargeBlock(rng *rand.Rand) *FileDiff {
	pool := []string{"", "   "}
	for range 3 + rng.IntN(10) {
		pool = append(pool, randLine(rng, 14))
	}
	text := func() string {
		s := pool[rng.IntN(len(pool))]
		if rng.IntN(2) == 0 {
			s = mutate(rng, s)
		}
		return s
	}
	nd, na := 60+rng.IntN(241), 60+rng.IntN(241)
	if rng.IntN(3) == 0 {
		nd, na = 1+rng.IntN(4), 700+rng.IntN(2000)
	}
	if rng.IntN(2) == 0 {
		nd, na = na, nd
	}
	fd := &FileDiff{Lines: []Line{{Kind: Context, OldNo: 1, NewNo: 1, Text: text()}}}
	oldNo, newNo := 1, 1
	for nd+na > 0 {
		if rng.IntN(nd+na) < nd {
			oldNo++
			nd--
			fd.Lines = append(fd.Lines, Line{Kind: Deleted, OldNo: oldNo, Text: text()})
		} else {
			newNo++
			na--
			fd.Lines = append(fd.Lines, Line{Kind: Added, NewNo: newNo, Text: text()})
		}
	}
	return fd
}

// checkRowSources verifies every row carries the text of the input line its
// numbers name, and the block index of the change run that line is in.
func checkRowSources(t *testing.T, fd *FileDiff, rows []Row) {
	t.Helper()
	type src struct {
		text  string
		block int
	}
	oldSrc, newSrc := map[int]src{}, map[int]src{}
	block := -1
	for i, l := range fd.Lines {
		b := -1
		if l.Kind != Context {
			if i == 0 || fd.Lines[i-1].Kind == Context {
				block++
			}
			b = block
		}
		if l.Kind != Added {
			oldSrc[l.OldNo] = src{l.Text, b}
		}
		if l.Kind != Deleted {
			newSrc[l.NewNo] = src{l.Text, b}
		}
	}
	for i, r := range rows {
		for _, s := range []struct {
			no int
			m  map[int]src
		}{{r.OldNo, oldSrc}, {r.NewNo, newSrc}} {
			if s.no == 0 {
				continue
			}
			want, ok := s.m[s.no]
			if !ok || want.text != r.Text || want.block != r.Block {
				t.Errorf("row %d (%+v) does not match its input line %+v", i, r, want)
			}
		}
	}
}

// checkPairs verifies each modified pair is similar enough, joins two lines
// with visible content and carries exactly WordDiff's ranges.
func checkPairs(t *testing.T, rows []Row) {
	t.Helper()
	for i, r := range rows {
		if r.Kind != RowModOld {
			continue
		}
		n := rows[i+1]
		ra, rb, si := WordDiff(r.Text, n.Text)
		if _, ok := pairScore(r.Text, n.Text); !ok {
			t.Errorf("rows %d/%d paired with similarity %v: %q / %q", i, i+1, si, r.Text, n.Text)
		}
		if strings.TrimSpace(r.Text) == "" || strings.TrimSpace(n.Text) == "" {
			t.Errorf("rows %d/%d pair a blank line: %q / %q", i, i+1, r.Text, n.Text)
		}
		if !slices.Equal(ra, r.Changed) || !slices.Equal(rb, n.Changed) {
			t.Errorf("rows %d/%d changed %v / %v, WordDiff gives %v / %v", i, i+1, r.Changed, n.Changed, ra, rb)
		}
	}
}

// checkEmissionOrder verifies that between two pairs of a block the unpaired
// deletions come before the unpaired additions.
func checkEmissionOrder(t *testing.T, rows []Row) {
	t.Helper()
	sawAdd := false
	for i, r := range rows {
		switch r.Kind {
		case RowAdded:
			sawAdd = true
		case RowDeleted:
			if sawAdd && rows[i-1].Block == r.Block {
				t.Errorf("row %d: deletion after an addition in the same gap", i)
			}
		default:
			sawAdd = false
		}
	}
}

// checkPairingIsOptimal compares the total similarity of the emitted pairs
// with an exhaustive order-preserving alignment of each block's lines.
func checkPairingIsOptimal(t *testing.T, fd *FileDiff, rows []Row) {
	t.Helper()
	got := map[int]float64{}
	for i, r := range rows {
		if r.Kind == RowModOld {
			score, _ := pairScore(r.Text, rows[i+1].Text)
			got[r.Block] += score
		}
	}
	block := -1
	for i := 0; i < len(fd.Lines); {
		if fd.Lines[i].Kind == Context {
			i++
			continue
		}
		block++
		var dels, adds []string
		for ; i < len(fd.Lines) && fd.Lines[i].Kind != Context; i++ {
			if fd.Lines[i].Kind == Deleted {
				dels = append(dels, fd.Lines[i].Text)
			} else {
				adds = append(adds, fd.Lines[i].Text)
			}
		}
		if want := bestAlignment(dels, adds); math.Abs(got[block]-want) > 1e-5 {
			t.Errorf("block %d: pairs total %v, best alignment %v", block, got[block], want)
		}
	}
}

// pairScore restates the pairing rule independently of the pairer: word
// similarity of at least one half, or for short lines a character similarity
// of at least 0.6, which then counts half.
func pairScore(a, b string) (float64, bool) {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return 0, false
	}
	if _, _, si := WordDiff(a, b); si >= 0.5 {
		return si, true
	}
	ra, rb := nonSpaceRunes(a), nonSpaceRunes(b)
	if len(ra) == 0 || len(rb) == 0 || len(ra) > shortLine || len(rb) > shortLine {
		return 0, false
	}
	total := len(ra) + len(rb)
	matched := runeLCS(ra, rb)
	if 10*matched < 3*total {
		return 0, false
	}
	return float64(matched) / float64(total), true
}

// bestAlignment is the plain O(n·m) alignment DP over pairScore, skipping
// pairs the rule rejects.
func bestAlignment(dels, adds []string) float64 {
	n, m := len(dels), len(adds)
	best := make([][]float64, n+1)
	for i := range best {
		best[i] = make([]float64, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			v := max(best[i+1][j], best[i][j+1])
			if strings.TrimSpace(dels[i]) != "" && strings.TrimSpace(adds[j]) != "" {
				if si, ok := pairScore(dels[i], adds[j]); ok {
					v = max(v, si+best[i+1][j+1])
				}
			}
			best[i][j] = v
		}
	}
	return best[0][0]
}

func TestWordDiffProperties(t *testing.T) {
	rng := rand.New(rand.NewPCG(21, 22))
	for range 20000 {
		a := randLine(rng, 10)
		b := mutate(rng, a)
		if rng.IntN(4) == 0 {
			b = randLine(rng, 10)
		}
		checkWordDiff(t, a, b)
		if t.Failed() {
			t.Fatalf("WordDiff(%q, %q)", a, b)
		}
	}
}

func checkWordDiff(t *testing.T, a, b string) {
	t.Helper()
	ra, rb, si := WordDiff(a, b)
	checkRanges(t, a, ra)
	checkRanges(t, b, rb)
	if math.IsNaN(si) || si < 0 || si > 1 {
		t.Errorf("similarity %v outside [0, 1]", si)
	}
	if a == b && (ra != nil || rb != nil || si != 1) {
		t.Errorf("identical lines: %v / %v, %v", ra, rb, si)
	}
	if a != b && len(ra) == 0 && len(rb) == 0 {
		t.Errorf("different lines without changed ranges")
	}
	checkGraphemeBoundaries(t, a, ra)
	checkGraphemeBoundaries(t, b, rb)

	keptA, wsA := unchangedWords(t, a, ra)
	keptB, wsB := unchangedWords(t, b, rb)
	if !slices.Equal(keptA, keptB) {
		t.Errorf("unchanged words differ: %q / %q", keptA, keptB)
	}
	want := 1.0
	if total := wsA + wsB; total > 0 {
		kept := 0
		for _, w := range keptA {
			kept += len(w)
		}
		want = float64(2*kept) / float64(total)
	}
	if math.Abs(si-want) > 1e-12 {
		t.Errorf("similarity %v, unchanged words give %v", si, want)
	}
}

// checkGraphemeBoundaries verifies no range starts or ends inside a grapheme
// cluster, so a highlight never cuts a visible character apart.
func checkGraphemeBoundaries(t *testing.T, s string, rs []Range) {
	t.Helper()
	at := map[int]bool{len(s): true}
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		from, _ := g.Positions()
		at[from] = true
	}
	for _, r := range rs {
		if !at[r.Start] || !at[r.End] {
			t.Errorf("range %v of %q splits a grapheme cluster", r, s)
		}
	}
}

// unchangedWords returns the non-whitespace tokens of s outside rs and the
// number of non-whitespace bytes of s, checking that no range cuts a token.
func unchangedWords(t *testing.T, s string, rs []Range) (kept []string, nonWS int) {
	t.Helper()
	inside := func(p int) bool {
		for _, r := range rs {
			if r.Start <= p && p < r.End {
				return true
			}
		}
		return false
	}
	for _, tok := range tokenize(s) {
		in := inside(tok.start)
		for p := tok.start; p < tok.end; p++ {
			if inside(p) != in {
				t.Errorf("a range cuts token %q of %q (ranges %v)", s[tok.start:tok.end], s, rs)
				break
			}
		}
		if tok.kind == tokSpace {
			continue
		}
		nonWS += tok.end - tok.start
		if !in {
			kept = append(kept, s[tok.start:tok.end])
		}
	}
	return kept, nonWS
}

// align must mark one longest common subsequence: a common subsequence
// whose length matches a brute-force LCS.
func TestAlignIsLCS(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 32))
	var sc scratch
	for range 5000 {
		a := make([]int32, rng.IntN(12))
		b := make([]int32, rng.IntN(12))
		for k := range a {
			a[k] = rng.Int32N(4)
		}
		for k := range b {
			b[k] = rng.Int32N(4)
		}
		ma, mb, _ := sc.align(a, b, wordCells)
		var subA, subB []int32
		for k, ok := range ma {
			if ok {
				subA = append(subA, a[k])
			}
		}
		for k, ok := range mb {
			if ok {
				subB = append(subB, b[k])
			}
		}
		if !slices.Equal(subA, subB) || len(subA) != lcsLen(a, b) {
			t.Fatalf("align(%v, %v) marks %v / %v, LCS length %d", a, b, subA, subB, lcsLen(a, b))
		}
	}
}

func lcsLen(a, b []int32) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if a[0] == b[0] {
		return 1 + lcsLen(a[1:], b[1:])
	}
	return max(lcsLen(a[1:], b), lcsLen(a, b[1:]))
}

func TestSplitProperties(t *testing.T) {
	rng := rand.New(rand.NewPCG(41, 42))
	for iter := range 3000 {
		fd := randomFileDiff(rng)
		rows := Unified(fd)
		checkSplit(t, rows, Split(rows))
		if t.Failed() {
			t.Fatalf("iteration %d, input:%s", iter, describe(fd))
		}
	}
}

// checkSplit verifies each unified row appears exactly once on each side it
// belongs to, sides stay in order, pairs share a row, and unpaired lines
// between two pairs fill both sides from the top.
func checkSplit(t *testing.T, rows []Row, split []SplitRow) {
	t.Helper()
	onLeft, onRight := make([]int, len(rows)), make([]int, len(rows))
	lastL, lastR := -1, -1
	for k, s := range split {
		if s.Left < 0 && s.Right < 0 {
			t.Errorf("split row %d is empty", k)
		}
		if s.Left >= 0 {
			onLeft[s.Left]++
			if s.Left <= lastL {
				t.Errorf("split row %d: left %d after %d", k, s.Left, lastL)
			}
			lastL = s.Left
		}
		if s.Right >= 0 {
			onRight[s.Right]++
			if s.Right <= lastR {
				t.Errorf("split row %d: right %d after %d", k, s.Right, lastR)
			}
			lastR = s.Right
		}
		if (s.Left >= 0 && rows[s.Left].Kind == RowContext || s.Right >= 0 && rows[s.Right].Kind == RowContext) && s.Left != s.Right {
			t.Errorf("split row %d: context row not on both sides: %v", k, s)
		}
		if s.Left >= 0 && rows[s.Left].Kind == RowModOld && s.Right != rows[s.Left].Pair {
			t.Errorf("split row %d: pair %d/%d split apart: %v", k, s.Left, rows[s.Left].Pair, s)
		}
	}
	for i, r := range rows {
		wantL := r.Kind == RowContext || r.Kind == RowDeleted || r.Kind == RowModOld
		wantR := r.Kind == RowContext || r.Kind == RowAdded || r.Kind == RowModNew
		if onLeft[i] != b2i(wantL) || onRight[i] != b2i(wantR) {
			t.Errorf("row %d (kind %d) appears %d times left, %d times right", i, r.Kind, onLeft[i], onRight[i])
		}
	}
	// A filler may only follow another filler until the gap ends.
	fillerL, fillerR := false, false
	for k, s := range split {
		gap := s.Left < 0 || s.Right < 0 || rows[s.Left].Kind == RowDeleted
		if !gap || k > 0 && blockOf(rows, split[k-1]) != blockOf(rows, s) {
			fillerL, fillerR = false, false
		}
		if !gap {
			continue
		}
		if s.Left >= 0 && fillerL || s.Right >= 0 && fillerR {
			t.Errorf("split row %d: a line below a filler: %v", k, split)
		}
		fillerL, fillerR = fillerL || s.Left < 0, fillerR || s.Right < 0
	}
}

func blockOf(rows []Row, s SplitRow) int {
	if s.Left >= 0 {
		return rows[s.Left].Block
	}
	return rows[s.Right].Block
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestFoldsProperties(t *testing.T) {
	rng := rand.New(rand.NewPCG(51, 52))
	for iter := range 2000 {
		n := rng.IntN(60)
		flags := make([]bool, n)
		density := 1 + rng.IntN(15)
		for i := range flags {
			flags[i] = rng.IntN(density) == 0
		}
		f := NewFolds(flags, rng.IntN(8))
		for step := range 12 {
			checkFoldsAgainstModel(t, f, flags)
			if t.Failed() {
				t.Fatalf("iteration %d step %d: flags %v context %d", iter, step, flags, f.Context())
			}
			switch k := len(f.regions); rng.IntN(6) {
			case 0:
				f.OpenAll()
			case 1:
				f.CloseAll()
			case 2:
				f.SetContext(rng.IntN(8) - 1)
			default:
				if k > 0 {
					r := rng.IntN(k)
					if f.IsOpen(r) {
						f.Close(r)
					} else {
						f.Open(r)
					}
				}
			}
		}
	}
}

// checkFoldsAgainstModel compares the folds with a direct definition: a line
// is foldable when it is farther than context lines from every change, the
// file has a change, and the run of foldable lines it is in has at least
// minFold lines.
func checkFoldsAgainstModel(t *testing.T, f *Folds, flags []bool) {
	t.Helper()
	n, ctx := len(flags), f.Context()
	far := make([]bool, n)
	for i := range flags {
		far[i] = !flags[i] && slices.Contains(flags, true)
		for j, c := range flags {
			if c && (j-i <= ctx && i-j <= ctx) {
				far[i] = false
			}
		}
	}
	var want [][2]int
	for i := 0; i < n; {
		if !far[i] {
			i++
			continue
		}
		s := i
		for i < n && far[i] {
			i++
		}
		if i-s >= minFold {
			want = append(want, [2]int{s, i})
		}
	}
	if got := regionSpans(f); !slices.Equal(got, want) {
		t.Errorf("regions %v, want %v", got, want)
		return
	}

	var items []Item
	for i := 0; i < n; {
		if k := slices.IndexFunc(want, func(r [2]int) bool { return r[0] == i }); k >= 0 && !f.IsOpen(k) {
			items = append(items, Item{Line: -1, Fold: k, Start: i, End: want[k][1]})
			i = want[k][1]
			continue
		}
		items = append(items, Item{Line: i, Fold: -1})
		i++
	}
	if got := f.Items(); !slices.Equal(got, items) {
		t.Errorf("items %v, want %v", got, items)
		return
	}
	checkItems(t, f, n)
	if n > 0 {
		last := len(items) - 1
		for _, c := range []struct{ line, want int }{{-1, 0}, {-100, 0}, {n, last}, {n + 7, last}} {
			if got := f.ItemOfLine(c.line); got != c.want {
				t.Errorf("ItemOfLine(%d) = %d, want %d", c.line, got, c.want)
			}
		}
	}
	for _, line := range []int{-1, n, n + 3} {
		if got := f.FoldAt(line); got != -1 {
			t.Errorf("FoldAt(%d) = %d outside the file", line, got)
		}
	}
	for i := range n {
		k := slices.IndexFunc(want, func(r [2]int) bool { return r[0] <= i && i < r[1] })
		if got := f.FoldAt(i); got != k {
			t.Errorf("FoldAt(%d) = %d, want %d", i, got, k)
		}
	}
}
