package diff

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"
)

// perfLimit is the bound from the product spec; typical results are far
// below it (see the test logs with -v).
const perfLimit = 200 * time.Millisecond

// timePerRun reports the time of one call of f: the best of three batches,
// each averaged over enough calls to span 30ms. Batching beats the Windows
// clock, which may tick only every 15.6ms; taking the best batch ignores a
// burst of load from other processes on a shared machine.
func timePerRun(f func()) time.Duration {
	best := time.Duration(1<<63 - 1)
	for range 3 {
		runs := 0
		start := time.Now()
		for runs == 0 || time.Since(start) < 30*time.Millisecond {
			f()
			runs++
		}
		best = min(best, time.Since(start)/time.Duration(runs))
	}
	return best
}

// blockDiff wraps one change block (all deletions, then all additions) in a
// line of context on each side.
func blockDiff(dels, adds []string) *FileDiff {
	fd := &FileDiff{Lines: []Line{{Kind: Context, OldNo: 1, NewNo: 1, Text: "package x"}}}
	for i, s := range dels {
		fd.Lines = append(fd.Lines, Line{Kind: Deleted, OldNo: i + 2, Text: s})
	}
	for j, s := range adds {
		fd.Lines = append(fd.Lines, Line{Kind: Added, NewNo: j + 2, Text: s})
	}
	return fd
}

func countPairs(rows []Row) int {
	n := 0
	for _, r := range rows {
		if r.Kind == RowModOld {
			n++
		}
	}
	return n
}

func randWord(rng *rand.Rand, alphabet string) string {
	b := make([]byte, 3+rng.IntN(6))
	for i := range b {
		b[i] = alphabet[rng.IntN(len(alphabet))]
	}
	return string(b)
}

func largeBlocks() []struct {
	name       string
	dels, adds []string
	minPairs   int
	maxPairs   int
} {
	const n = 2000
	rng := rand.New(rand.NewPCG(7, 7))
	var similarD, similarA, unrelatedD, unrelatedA, braces, longD, longA []string
	for i := range n {
		similarD = append(similarD, fmt.Sprintf("\tresult%d := compute(input%d, \"key-%d\")", i, i, i))
		similarA = append(similarA, fmt.Sprintf("\tresult%d := computeV2(ctx, input%d, \"key-%d\")", i, i, i))
		unrelatedD = append(unrelatedD, fmt.Sprintf("%s(%s, %s) + %s;", randWord(rng, "abcdefghijklm"), randWord(rng, "abcdefghijklm"), randWord(rng, "abcdefghijklm"), randWord(rng, "abcdefghijklm")))
		unrelatedA = append(unrelatedA, fmt.Sprintf("if %s {%s = %s}", randWord(rng, "nopqrstuvwxyz"), randWord(rng, "nopqrstuvwxyz"), randWord(rng, "nopqrstuvwxyz")))
		braces = append(braces, "}")
		var d, a []string
		for k := range 40 {
			d = append(d, fmt.Sprintf("arg%d_%d", k, i%7))
			if k%9 == 4 {
				a = append(a, fmt.Sprintf("changed%d", k))
			} else {
				a = append(a, fmt.Sprintf("arg%d_%d", k, i%7))
			}
		}
		longD = append(longD, "call("+strings.Join(d, ", ")+")")
		longA = append(longA, "call("+strings.Join(a, ", ")+")")
	}
	wordsD, wordsA := shuffledWords(rng, n, 64), shuffledWords(rng, n, 64)
	shiftedA := append([]string(nil), similarA...)
	for k := range 10 {
		shiftedA = append([]string{fmt.Sprintf("// new header line %d", k)}, shiftedA...)
	}
	return []struct {
		name       string
		dels, adds []string
		minPairs   int
		maxPairs   int
	}{
		{"similar lines", similarD, similarA, n, n},
		{"unrelated lines", unrelatedD, unrelatedA, 0, 0},
		{"shifted by inserted lines", similarD, shiftedA, n, n},
		{"identical repeated lines", braces, braces, n, n},
		{"long lines with scattered edits", longD, longA, n, n},
		// Every pair in the band is similar and needs a big LCS table, so the
		// block's LCS budget runs out; the bound must hold regardless.
		{"long lines all alike", alike(longD[0], n), alike(longA[0], n), 0, n},
		// Lines drawn from one small vocabulary pass the cheap token-overlap
		// bound, so every candidate needs an LCS and the budget runs out.
		{"long lines of shuffled common words", wordsD, wordsA, 0, n},
	}
}

// shuffledWords returns n lines of k words drawn from a 20-word vocabulary.
func shuffledWords(rng *rand.Rand, n, k int) []string {
	vocab := strings.Fields("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon")
	out := make([]string, n)
	for i := range out {
		w := make([]string, k)
		for j := range w {
			w[j] = vocab[rng.IntN(len(vocab))]
		}
		out[i] = strings.Join(w, " ")
	}
	return out
}

// TestUnifiedPathologicalBlocks runs 5000 x 5000 blocks, beyond the size the
// spec bounds, to show the cost stays linear in the block size.
func TestUnifiedPathologicalBlocks(t *testing.T) {
	const n = 5000
	rng := rand.New(rand.NewPCG(5, 5))
	same := make([]string, n)
	var similarD, similarA, unrelatedD, unrelatedA []string
	for i := range n {
		same[i] = "\treturn nil"
		similarD = append(similarD, fmt.Sprintf("\tv%d := f(a%d)", i, i))
		similarA = append(similarA, fmt.Sprintf("\tv%d := g(ctx, a%d)", i, i))
		unrelatedD = append(unrelatedD, randWord(rng, "abcdefghijklm")+"("+randWord(rng, "abcdefghijklm")+");")
		unrelatedA = append(unrelatedA, "if "+randWord(rng, "nopqrstuvwxyz")+" {}")
	}
	huge := strings.Repeat("abc def ", 12_500)
	tests := []struct {
		name       string
		dels, adds []string
		minPairs   int
	}{
		{"all lines identical", same, same, n},
		{"all lines different", unrelatedD, unrelatedA, 0},
		{"all lines similar", similarD, similarA, n},
		{"shuffled common words", shuffledWords(rng, n, 20), shuffledWords(rng, n, 20), 0},
		{"one huge line among similar lines", append([]string{huge}, similarD...), append([]string{huge + "x"}, similarA...), n + 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fd := blockDiff(tc.dels, tc.adds)
			var rows []Row
			d := timePerRun(func() { rows = Unified(fd) })
			t.Logf("%d x %d block: %v, %d pairs", len(tc.dels), len(tc.adds), d, countPairs(rows))
			if d > perfLimit {
				t.Errorf("took %v, limit %v", d, perfLimit)
			}
			if p := countPairs(rows); p < tc.minPairs {
				t.Errorf("%d pairs, want at least %d", p, tc.minPairs)
			}
			checkRows(t, fd, rows)
			checkSplit(t, rows, Split(rows))
		})
	}
}

// alike returns n distinct lines that differ from s only in a short suffix,
// so that no two are textually identical.
func alike(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s // %d", s, i)
	}
	return out
}

func TestUnifiedLargeBlockIsFast(t *testing.T) {
	for _, tc := range largeBlocks() {
		t.Run(tc.name, func(t *testing.T) {
			fd := blockDiff(tc.dels, tc.adds)
			var rows []Row
			d := timePerRun(func() { rows = Unified(fd) })
			t.Logf("%d x %d block: %v, %d pairs", len(tc.dels), len(tc.adds), d, countPairs(rows))
			if d > perfLimit {
				t.Errorf("took %v, limit %v", d, perfLimit)
			}
			if p := countPairs(rows); p < tc.minPairs || p > tc.maxPairs {
				t.Errorf("%d pairs, want %d..%d", p, tc.minPairs, tc.maxPairs)
			}
			checkRows(t, fd, rows)
		})
	}
}

func longLines() []struct{ name, a, b string } {
	const size = 100_000
	rng := rand.New(rand.NewPCG(3, 4))
	var words []string
	for n := 0; n < size; {
		w := randWord(rng, "abcdefghijklmnopqrstuvwxyz")
		words = append(words, w)
		n += len(w) + 1
	}
	base := strings.Join(words, " ")

	mid := []byte(base)
	mid[len(mid)/2] = '#'

	scattered := append([]string(nil), words...)
	for k := 0; k < len(scattered); k += 50 {
		scattered[k] = "CHANGED"
	}

	var other []string
	for n := 0; n < size; {
		w := randWord(rng, "abcdefghijklmnopqrstuvwxyz")
		other = append(other, w)
		n += len(w) + 1
	}

	cyr := strings.Repeat("значення ", size/17)
	cyrB := []rune(cyr)
	cyrB[len(cyrB)/2] = 'ж'

	return []struct{ name, a, b string }{
		{"one edit in the middle", base, string(mid)},
		{"unrelated", base, strings.Join(other, " ")},
		{"scattered edits", base, strings.Join(scattered, " ")},
		{"cyrillic", cyr, string(cyrB)},
		{"emoji", strings.Repeat("ok 👍🏽 ", size/12), strings.Repeat("ok 👍🏿 ", size/12)},
	}
}

func TestWordDiffLongLineIsFast(t *testing.T) {
	for _, tc := range longLines() {
		t.Run(tc.name, func(t *testing.T) {
			var ra, rb []Range
			d := timePerRun(func() { ra, rb, _ = WordDiff(tc.a, tc.b) })
			t.Logf("%d / %d bytes: %v", len(tc.a), len(tc.b), d)
			if d > perfLimit {
				t.Errorf("took %v, limit %v", d, perfLimit)
			}
			checkRanges(t, tc.a, ra)
			checkRanges(t, tc.b, rb)
		})
	}
}

func TestUnifiedLongLineIsFast(t *testing.T) {
	for _, tc := range longLines() {
		t.Run(tc.name, func(t *testing.T) {
			fd := blockDiff([]string{"x := 1", tc.a, "y := 2"}, []string{"x := 10", tc.b, "z()", "y := 20"})
			var rows []Row
			d := timePerRun(func() { rows = Unified(fd) })
			t.Logf("block with a %d-byte line: %v", len(tc.a), d)
			if d > perfLimit {
				t.Errorf("took %v, limit %v", d, perfLimit)
			}
			checkRows(t, fd, rows)
		})
	}
}

func BenchmarkUnifiedLargeBlock(b *testing.B) {
	for _, tc := range largeBlocks() {
		fd := blockDiff(tc.dels, tc.adds)
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				Unified(fd)
			}
		})
	}
}

func BenchmarkWordDiffLongLine(b *testing.B) {
	for _, tc := range longLines() {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				WordDiff(tc.a, tc.b)
			}
		})
	}
}

func BenchmarkWordDiffTypical(b *testing.B) {
	for b.Loop() {
		WordDiff("\tresult := compute(input, \"key\", opts)", "\tresult, err := computeV2(ctx, input, \"key\")")
	}
}

func BenchmarkFoldsItems(b *testing.B) {
	flags := make([]bool, 100_000)
	for i := 0; i < len(flags); i += 997 {
		flags[i] = true
	}
	f := NewFolds(flags, 50)
	b.Run("cached", func(b *testing.B) {
		for b.Loop() {
			f.Items()
		}
	})
	b.Run("after toggle", func(b *testing.B) {
		for b.Loop() {
			f.Open(3)
			f.Items()
			f.Close(3)
		}
	})
	b.Run("ItemOfLine", func(b *testing.B) {
		for b.Loop() {
			f.ItemOfLine(54_321)
		}
	})
}
