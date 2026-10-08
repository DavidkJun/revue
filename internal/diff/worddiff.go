package diff

// wordCells bounds the LCS table of one word diff (about 2 MB). Past it, a
// line pair is compared by common prefix and suffix only, which keeps a
// minified 100k-character line from stalling the viewer.
const wordCells = 1 << 20

func wordDiff(a, b string) (aChanged, bChanged []Range, similarity float64) {
	if a == b {
		return nil, nil, 1
	}
	in := newInterner()
	la, lb := in.line(a), in.line(b)
	var sc scratch
	aChanged, bChanged, similarity, _ = sc.compare(&la, &lb, in.weight, wordCells)
	return aChanged, bChanged, similarity
}

// compare word-diffs two prepared lines and reports the LCS cells it used.
func (sc *scratch) compare(a, b *tokLine, weight []int32, maxCells int) (aChanged, bChanged []Range, similarity float64, cells int) {
	ma, mb, cells := sc.align(a.ids, b.ids, maxCells)
	aChanged = changedRanges(a.toks, ma)
	bChanged = changedRanges(b.toks, mb)
	similarity = 1
	if total := a.nonWS + b.nonWS; total > 0 {
		similarity = float64(2*matchedWeight(a.ids, ma, weight)) / float64(total)
	}
	return aChanged, bChanged, similarity, cells
}

// matchedWeight is the number of non-whitespace bytes among matched tokens;
// matched tokens are equal on both sides, so either side gives the same sum.
func matchedWeight(ids []int32, matched []bool, weight []int32) int {
	sum := 0
	for k, id := range ids {
		if matched[k] {
			sum += int(weight[id])
		}
	}
	return sum
}

// changedRanges turns the unmatched tokens into byte ranges. Two changes
// separated only by an unchanged whitespace run read as one edit ("foo bar"
// replaced as a phrase), so they merge into one range.
func changedRanges(toks []token, matched []bool) []Range {
	var out []Range
	for k := 0; k < len(toks); k++ {
		if matched[k] {
			continue
		}
		r := Range{Start: toks[k].start, End: toks[k].end}
		for k+1 < len(toks) {
			if !matched[k+1] {
				k++
			} else if toks[k+1].kind == tokSpace && k+2 < len(toks) && !matched[k+2] {
				k += 2
			} else {
				break
			}
			r.End = toks[k].end
		}
		out = append(out, r)
	}
	return out
}
