package diff

import (
	"slices"
	"unicode"
	"unicode/utf8"
)

const (
	// maxDensePairs is the largest block (deletions × additions) in which
	// every line pair is scored. A bigger block scores only the pairs within
	// ±w of a few diagonals, w chosen so that each diagonal holds about
	// maxDensePairs pairs but never below minBand, which keeps the work
	// linear in the block size.
	maxDensePairs = 2500
	minBand       = 4

	// pairCells bounds the LCS table of one candidate pair and pairBudget
	// the cells of all of a block's candidates (and, separately, of its
	// paired lines' word diffs); past either, lines are compared by common
	// prefix and suffix only.
	pairCells  = 1 << 16
	pairBudget = 1 << 25

	// scoreScale turns similarities into integers so that equal totals are
	// recognised exactly when choosing between alignments.
	scoreScale = 1 << 20

	// charBudget bounds the character comparisons of one block, separately
	// from pairBudget so that they never starve the word matching.
	charBudget = 1 << 23

	// shortLine is the longest line (in non-whitespace runes) that is also
	// compared character by character. Word matching undervalues a short edit
	// inside one token: "x := 1234" -> "x := 1235" shares only 3 of 7 bytes as
	// whole tokens, yet it is plainly the same line with a new value.
	shortLine = 80
)

// candidate is a deletion/addition pair similar enough to show as modified.
type candidate struct {
	i, j  int   // index among the block's deletions / additions
	score int64 // similarity × scoreScale
}

// pairer finds which deleted lines of a change block were replaced by which
// added lines.
type pairer struct {
	dels, adds       []tokLine
	delBags, addBags [][]int32 // sorted ids of each line's non-whitespace tokens
	weight           []int32
	sc               *scratch
	spent            int // LCS cells used so far
	charSpent        int // character comparison cells used so far
}

func newPairer(dels, adds []tokLine, weight []int32, sc *scratch) *pairer {
	p := &pairer{dels: dels, adds: adds, weight: weight, sc: sc}
	p.delBags = make([][]int32, len(dels))
	for i := range dels {
		p.delBags[i] = bag(&dels[i])
	}
	p.addBags = make([][]int32, len(adds))
	for j := range adds {
		p.addBags[j] = bag(&adds[j])
	}
	return p
}

// pairs returns the order-preserving pairing with the largest total
// similarity, counting only pairs with word similarity >= 0.5 (or, for short
// lines, character similarity >= 0.6), sorted by i.
func (p *pairer) pairs() []candidate {
	n, m := len(p.dels), len(p.adds)
	if n == 0 || m == 0 {
		return nil
	}
	var cands []candidate
	if int64(n)*int64(m) <= maxDensePairs {
		for i := range n {
			for j := range m {
				cands = p.consider(cands, i, j)
			}
		}
	} else {
		w := max(minBand, maxDensePairs/(2*n))
		for i := range n {
			spans, k := band(i, n, m, w)
			for _, s := range spans[:k] {
				for j := s.lo; j < s.hi; j++ {
					cands = p.consider(cands, i, j)
				}
			}
		}
	}
	return bestChain(cands, m)
}

// consider appends (i, j) to cands when the pair reaches the similarity
// threshold. Cheap upper bounds on the similarity (length ratio, shared
// tokens regardless of order) reject most unrelated pairs before the LCS.
func (p *pairer) consider(cands []candidate, i, j int) []candidate {
	d, a := &p.dels[i], &p.adds[j]
	// Blank lines carry nothing to recognise, so they never anchor a pair.
	if d.nonWS == 0 || a.nonWS == 0 {
		return cands
	}
	total := d.nonWS + a.nonWS
	if 4*min(d.nonWS, a.nonWS) >= total && 4*bagOverlap(p.delBags[i], p.addBags[j], p.weight) >= total {
		matched := d.nonWS
		if d.text != a.text {
			matched = p.matched(d, a)
		}
		if 4*matched >= total {
			return append(cands, candidate{i: i, j: j, score: int64(matched) * 2 * scoreScale / int64(total)})
		}
	}
	return p.considerChars(cands, i, j)
}

// considerChars is the fallback for short lines whose words differ too much:
// it pairs them when at least 60% of their non-whitespace characters align.
// The bar is higher than for words because unrelated code lines share many
// letters by chance. Such a pair scores half its character similarity, which
// keeps it below every word-matched pair, so it only fills gaps and never
// pulls a block's alignment away from lines that match by words.
func (p *pairer) considerChars(cands []candidate, i, j int) []candidate {
	if p.charSpent >= charBudget {
		return cands
	}
	da, aa := nonSpaceRunes(p.dels[i].text), nonSpaceRunes(p.adds[j].text)
	if len(da) > shortLine || len(aa) > shortLine {
		return cands
	}
	total := len(da) + len(aa)
	matched := runeLCS(da, aa)
	p.charSpent += len(da) * len(aa)
	if 10*matched < 3*total {
		return cands
	}
	return append(cands, candidate{i: i, j: j, score: int64(matched) * scoreScale / int64(total)})
}

// nonSpaceRunes lists the runes of s outside whitespace. Each invalid byte
// becomes a value of its own (negative, so no real rune), otherwise any two
// broken sequences would compare equal as U+FFFD.
func nonSpaceRunes(s string) []rune {
	out := make([]rune, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			r = -rune(s[i]) - 1
		}
		if !unicode.IsSpace(r) {
			out = append(out, r)
		}
		i += size
	}
	return out
}

// runeLCS is the length of the longest common subsequence of a and b.
func runeLCS(a, b []rune) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	row := make([]int, len(b)+1)
	for _, ra := range a {
		diag := 0
		for k, rb := range b {
			up := row[k+1]
			if ra == rb {
				row[k+1] = diag + 1
			} else {
				row[k+1] = max(row[k+1], row[k])
			}
			diag = up
		}
	}
	return row[len(b)]
}

func (p *pairer) matched(d, a *tokLine) int {
	budget := pairCells
	if p.spent >= pairBudget {
		budget = 0
	}
	ma, _, cells := p.sc.align(d.ids, a.ids, budget)
	p.spent += cells
	return matchedWeight(d.ids, ma, p.weight)
}

func bag(l *tokLine) []int32 {
	out := make([]int32, 0, len(l.ids))
	for k, t := range l.toks {
		if t.kind != tokSpace {
			out = append(out, l.ids[k])
		}
	}
	slices.Sort(out)
	return out
}

// bagOverlap is the weight of the multiset intersection of two sorted id
// lists: an upper bound on the bytes any alignment can match.
func bagOverlap(a, b []int32, weight []int32) int {
	sum := 0
	for i, j := 0, 0; i < len(a) && j < len(b); {
		switch {
		case a[i] < b[j]:
			i++
		case a[i] > b[j]:
			j++
		default:
			sum += int(weight[a[i]])
			i++
			j++
		}
	}
	return sum
}

type span struct{ lo, hi int } // [lo, hi)

// band returns the additions scored against deletion i of a large block:
// windows of ±w around the top-aligned, bottom-aligned and proportional
// diagonals, because the edits of a big block usually sit at its start, at
// its end, or spread evenly through it.
func band(i, n, m, w int) (spans [3]span, k int) {
	centers := [3]int{i, i + m - n, (2*i + 1) * m / (2 * n)}
	slices.Sort(centers[:])
	for _, c := range centers {
		lo, hi := max(c-w, 0), min(c+w+1, m)
		if lo >= hi {
			continue
		}
		if k > 0 && lo <= spans[k-1].hi {
			spans[k-1].hi = max(spans[k-1].hi, hi)
			continue
		}
		spans[k] = span{lo: lo, hi: hi}
		k++
	}
	return spans, k
}

// bestChain picks the subset of candidates with strictly increasing i and j
// and the largest total score. Among equal totals it pairs earlier lines,
// which reads more naturally (old line, its new version, then extra
// additions). cands must be sorted by i, then j; m bounds every j.
//
// Rows are processed bottom-up; a Fenwick tree over reversed j answers "best
// chain starting strictly below and to the right", so the whole solve is
// O(K log m) for K candidates.
func bestChain(cands []candidate, m int) []candidate {
	if len(cands) == 0 {
		return nil
	}
	dp := make([]int64, len(cands))
	next := make([]int, len(cands))
	better := func(x, y int) bool {
		switch {
		case x < 0:
			return false
		case y < 0:
			return true
		case dp[x] != dp[y]:
			return dp[x] > dp[y]
		case cands[x].i != cands[y].i:
			return cands[x].i < cands[y].i
		default:
			return cands[x].j < cands[y].j
		}
	}
	tree := make([]int, m+1)
	for k := range tree {
		tree[k] = -1
	}
	for hi := len(cands); hi > 0; {
		lo := hi - 1
		for lo > 0 && cands[lo-1].i == cands[hi-1].i {
			lo--
		}
		// Query the whole row before inserting it: two pairs on one deleted
		// line must never chain.
		for k := lo; k < hi; k++ {
			best := -1
			for x := m - 1 - cands[k].j; x > 0; x -= x & -x {
				if better(tree[x], best) {
					best = tree[x]
				}
			}
			next[k] = best
			dp[k] = cands[k].score
			if best >= 0 {
				dp[k] += dp[best]
			}
		}
		for k := lo; k < hi; k++ {
			for x := m - cands[k].j; x <= m; x += x & -x {
				if better(k, tree[x]) {
					tree[x] = k
				}
			}
		}
		hi = lo
	}
	start := -1
	for k := range cands {
		if better(k, start) {
			start = k
		}
	}
	var chain []candidate
	for k := start; k >= 0; k = next[k] {
		chain = append(chain, cands[k])
	}
	return chain
}
