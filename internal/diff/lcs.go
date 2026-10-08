package diff

// scratch holds buffers reused across many line comparisons so that pairing
// a large block does not allocate per candidate.
type scratch struct {
	table  []uint16
	ma, mb []bool
}

// align marks in sc.ma / sc.mb (valid until the next call) the tokens of a
// and b that belong to one longest common subsequence. The common prefix and
// suffix are matched directly; the middle gets a full LCS only when its table
// fits in maxCells, otherwise it stays entirely unmatched. It returns the
// number of table cells it used.
func (sc *scratch) align(a, b []int32, maxCells int) (ma, mb []bool, cells int) {
	ma = resetFlags(&sc.ma, len(a))
	mb = resetFlags(&sc.mb, len(b))

	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		ma[p], mb[p] = true, true
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		ma[len(a)-1-s], mb[len(b)-1-s] = true, true
		s++
	}
	cells = sc.lcs(a[p:len(a)-s], b[p:len(b)-s], ma[p:len(a)-s], mb[p:len(b)-s], maxCells)
	return ma, mb, cells
}

// lcs is the classic suffix-table LCS. The budget keeps min(n, m) far below
// the uint16 range, so the table cells cannot overflow.
func (sc *scratch) lcs(a, b []int32, ma, mb []bool, maxCells int) int {
	n, m := len(a), len(b)
	if n == 0 || m == 0 {
		return 0
	}
	if int64(n)*int64(m) > int64(maxCells) {
		return 0
	}
	w := m + 1
	size := (n + 1) * w
	if cap(sc.table) < size {
		sc.table = make([]uint16, size)
	}
	t := sc.table[:size]
	clear(t[n*w:])
	for i := n - 1; i >= 0; i-- {
		row, next := t[i*w:(i+1)*w], t[(i+1)*w:(i+2)*w]
		row[m] = 0
		ai := a[i]
		// Branch-free on purpose: when two lines share many tokens, which
		// neighbour is larger is unpredictable, and the mispredictions about
		// doubled the cost of a block that spends its whole cell budget.
		right := uint16(0)
		for j := m - 1; j >= 0; j-- {
			v := max(next[j], right)
			if ai == b[j] {
				v = next[j+1] + 1
			}
			row[j] = v
			right = v
		}
	}
	for i, j := 0, 0; i < n && j < m; {
		switch {
		case a[i] == b[j]:
			ma[i], mb[j] = true, true
			i++
			j++
		case t[(i+1)*w+j] >= t[i*w+j+1]:
			i++
		default:
			j++
		}
	}
	return n * m
}

func resetFlags(buf *[]bool, n int) []bool {
	if cap(*buf) < n {
		*buf = make([]bool, n)
	}
	f := (*buf)[:n]
	clear(f)
	return f
}
