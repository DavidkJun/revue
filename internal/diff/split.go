package diff

func split(rows []Row) []SplitRow {
	out := make([]SplitRow, 0, len(rows))
	consumed := make([]bool, len(rows))
	var dels, adds []int
	// Unpaired deletions and additions between two pairs are laid side by
	// side positionally, the way GitHub's split view does it.
	flush := func() {
		for k := range max(len(dels), len(adds)) {
			r := SplitRow{Left: -1, Right: -1}
			if k < len(dels) {
				r.Left = dels[k]
			}
			if k < len(adds) {
				r.Right = adds[k]
			}
			out = append(out, r)
		}
		dels, adds = dels[:0], adds[:0]
	}
	segment := -1
	for i, r := range rows {
		if consumed[i] {
			continue
		}
		if p := partner(rows, i); p >= 0 {
			flush()
			consumed[p] = true
			if r.Kind == RowModOld {
				out = append(out, SplitRow{Left: i, Right: p})
			} else {
				out = append(out, SplitRow{Left: p, Right: i})
			}
			continue
		}
		switch r.Kind {
		case RowDeleted, RowModOld, RowAdded, RowModNew:
			if r.Block != segment {
				flush()
				segment = r.Block
			}
			if r.Kind == RowDeleted || r.Kind == RowModOld {
				dels = append(dels, i)
			} else {
				adds = append(adds, i)
			}
		default:
			flush()
			out = append(out, SplitRow{Left: i, Right: i})
		}
	}
	flush()
	return out
}

// partner returns the other half of row i's modified pair, or -1 when row i
// is not one half of a consistent pair.
func partner(rows []Row, i int) int {
	r := rows[i]
	p := r.Pair
	if p < 0 || p >= len(rows) || p == i || rows[p].Pair != i {
		return -1
	}
	switch {
	case r.Kind == RowModOld && rows[p].Kind == RowModNew,
		r.Kind == RowModNew && rows[p].Kind == RowModOld:
		return p
	}
	return -1
}
