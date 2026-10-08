package diffview

// lineID names a line by its file line numbers, which survive a rebuild of
// the rows and a switch between views.
type lineID struct{ newNo, oldNo int }

// snapshot is the position state worth keeping across a reload of the same
// file.
type snapshot struct {
	cursor lineID
	screen int // cursor row on screen
	visual bool
	anchor [2]lineID
	open   [2][]bool // per view: new-file lines inside open folds
}

func (m *Model) idOf(v, l int) lineID {
	r := m.f.rows[m.primary(v, l)]
	return lineID{newNo: r.NewNo, oldNo: r.OldNo}
}

// lineOfID finds the line of view v showing id: by new-file line, else by
// old-file line (deleted rows), else the end of the file.
func (m *Model) lineOfID(v int, id lineID) int {
	f := m.f
	var row int32
	switch {
	case id.newNo > 0 && len(f.newRow) > 0:
		row = f.newRow[min(id.newNo, len(f.newRow))-1]
	case id.oldNo > 0 && len(f.oldRow) > 0:
		row = f.oldRow[min(id.oldNo, len(f.oldRow))-1]
	default:
		row = int32(len(f.rows) - 1)
	}
	return m.lineOfRow(v, int(row))
}

func (m *Model) snapshot() snapshot {
	p := m.cur()
	v := m.view()
	s := snapshot{
		cursor: m.idOf(v, m.cursorLine()),
		screen: p.cursor - p.offset,
		visual: m.visual,
	}
	if m.visual {
		s.anchor = [2]lineID{m.idOf(v, m.anchor[0]), m.idOf(v, m.anchor[1]-1)}
	}
	for w := range m.panes {
		s.open[w] = m.openLines(w)
	}
	return s
}

// openLines marks the new-file lines that open folds of view v show.
func (m *Model) openLines(v int) []bool {
	f := m.f
	folds := m.panes[v].folds
	var open []bool
	for l := range m.lines(v) {
		k := folds.FoldAt(l)
		if k < 0 || !folds.IsOpen(k) {
			continue
		}
		if open == nil {
			open = make([]bool, len(f.newLines)+1)
		}
		if n := f.rows[m.primary(v, l)].NewNo; n > 0 && n < len(open) {
			open[n] = true
		}
	}
	return open
}

func (m *Model) lines(v int) int {
	if v == viewSplit {
		return len(m.f.split)
	}
	return len(m.f.rows)
}

// restore applies a snapshot taken before the file was rebuilt.
func (m *Model) restore(s snapshot) {
	f := m.f
	for v, open := range s.open {
		if open == nil {
			continue
		}
		folds := m.panes[v].folds
		for l := range m.lines(v) {
			k := folds.FoldAt(l)
			if k < 0 || folds.IsOpen(k) {
				continue
			}
			if n := f.rows[m.primary(v, l)].NewNo; n > 0 && n < len(open) && open[n] {
				folds.Open(k)
			}
		}
	}
	v := m.view()
	p := m.cur()
	p.cursor = p.folds.ItemOfLine(m.lineOfID(v, s.cursor))
	p.offset = p.cursor - s.screen
	m.follow(p)
	m.visual = s.visual
	if s.visual {
		a, b := m.lineOfID(v, s.anchor[0]), m.lineOfID(v, s.anchor[1])
		m.anchor = [2]int{min(a, b), max(a, b) + 1}
	}
	if m.xoff > 0 {
		// maxXOffset measures every row; most reloads are not scrolled.
		m.xoff = min(m.xoff, m.maxXOffset())
	}
}

// setSplit switches views. The cursor stays on the same row of the diff at
// the same screen row, and the visual anchor covers the same rows.
func (m *Model) setSplit(on bool) {
	if on == m.split {
		return
	}
	// A message may cover a file that is kept for a reload of it, whose
	// cursor has to move to the other view all the same.
	if m.f == nil {
		m.split = on
		return
	}
	m.fresh = false
	from, to := m.view(), 1-m.view()
	p := m.cur()
	it := p.folds.Items()[p.cursor]
	start, _ := span(it)
	row := m.primary(from, start)
	screen := p.cursor - p.offset
	var aLo, aHi int
	if m.visual {
		aLo, _ = m.rowRange(from, m.anchor[0])
		_, aHi = m.rowRange(from, m.anchor[1]-1)
	}

	m.split = on
	q := m.cur()
	// A split row can show two unified rows; coming back to the one the
	// unified cursor was on makes toggling twice a no-op.
	if items := q.folds.Items(); to == viewUnified && it.Fold < 0 && q.cursor < len(items) {
		if prev := items[q.cursor]; prev.Fold < 0 && m.lineOfRow(viewSplit, prev.Line) == start {
			row = prev.Line
		}
	}
	line := m.lineOfRow(to, row)
	if it.Fold < 0 {
		if k := q.folds.FoldAt(line); k >= 0 && !q.folds.IsOpen(k) {
			q.folds.Open(k)
		}
	}
	q.cursor = q.folds.ItemOfLine(line)
	q.offset = q.cursor - screen
	m.follow(q)
	if m.visual {
		a, b := m.lineOfRow(to, aLo), m.lineOfRow(to, aHi)
		m.anchor = [2]int{min(a, b), max(a, b) + 1}
	}
	m.search.valid = false
}
