package diff

func unified(fd *FileDiff) []Row {
	if fd == nil {
		return nil
	}
	lines := fd.Lines
	rows := make([]Row, 0, len(lines))
	var sc scratch
	block := 0
	for i := 0; i < len(lines); {
		if !isChangeLine(lines[i]) {
			l := lines[i]
			rows = append(rows, Row{Kind: RowContext, OldNo: l.OldNo, NewNo: l.NewNo, Text: l.Text, Pair: -1, Block: -1})
			i++
			continue
		}
		end := i + 1
		for end < len(lines) && isChangeLine(lines[end]) {
			end++
		}
		rows = appendBlock(rows, lines[i:end], block, &sc)
		block++
		i = end
	}
	return rows
}

// isChangeLine treats unknown kinds as context, so malformed input degrades
// to plain lines instead of failing.
func isChangeLine(l Line) bool { return l.Kind == Added || l.Kind == Deleted }

// appendBlock emits one change block: walking the pairing, the unpaired
// deletions before a pair, then the unpaired additions before it, then the
// pair itself; whatever is left over trails as deletions, then additions.
func appendBlock(rows []Row, lines []Line, block int, sc *scratch) []Row {
	var dels, adds []Line
	for _, l := range lines {
		if l.Kind == Deleted {
			dels = append(dels, l)
		} else {
			adds = append(adds, l)
		}
	}
	deleted := func(l Line) Row {
		return Row{Kind: RowDeleted, OldNo: l.OldNo, Text: l.Text, Pair: -1, Block: block}
	}
	added := func(l Line) Row {
		return Row{Kind: RowAdded, NewNo: l.NewNo, Text: l.Text, Pair: -1, Block: block}
	}

	var (
		pairs  []candidate
		dl, al []tokLine
		in     *interner
	)
	if len(dels) > 0 && len(adds) > 0 {
		in = newInterner()
		dl, al = prepare(in, dels), prepare(in, adds)
		pairs = newPairer(dl, al, in.weight, sc).pairs()
	}

	// The word diffs of one block share a cell budget like the pairing does,
	// so a block of thousands of huge paired lines cannot stall the viewer.
	budget := pairBudget
	di, ai := 0, 0
	for _, p := range pairs {
		for ; di < p.i; di++ {
			rows = append(rows, deleted(dels[di]))
		}
		for ; ai < p.j; ai++ {
			rows = append(rows, added(adds[ai]))
		}
		oldChanged, newChanged, _, cells := sc.compare(&dl[p.i], &al[p.j], in.weight, min(wordCells, budget))
		budget -= cells
		k := len(rows)
		rows = append(rows,
			Row{Kind: RowModOld, OldNo: dels[p.i].OldNo, Text: dels[p.i].Text, Changed: oldChanged, Pair: k + 1, Block: block},
			Row{Kind: RowModNew, NewNo: adds[p.j].NewNo, Text: adds[p.j].Text, Changed: newChanged, Pair: k, Block: block},
		)
		di, ai = p.i+1, p.j+1
	}
	for ; di < len(dels); di++ {
		rows = append(rows, deleted(dels[di]))
	}
	for ; ai < len(adds); ai++ {
		rows = append(rows, added(adds[ai]))
	}
	return rows
}

func prepare(in *interner, lines []Line) []tokLine {
	out := make([]tokLine, len(lines))
	for k, l := range lines {
		out[k] = in.line(l.Text)
	}
	return out
}
