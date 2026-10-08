package diffview

// contextStep is how many context lines + and - add or remove.
const contextStep = 10

// foldOpen opens the fold under the cursor. The items before it do not
// change, so the cursor index now points at the fold's first line.
func (m *Model) foldOpen(p *pane) {
	if it := p.folds.Items()[p.cursor]; it.Fold >= 0 {
		p.folds.Open(it.Fold)
		m.follow(p)
	}
}

// foldClose closes the foldable region the cursor is in.
func (m *Model) foldClose(p *pane) {
	it := p.folds.Items()[p.cursor]
	if it.Fold >= 0 {
		return
	}
	if k := p.folds.FoldAt(it.Line); k >= 0 && p.folds.IsOpen(k) {
		m.remap(p, func() { p.folds.Close(k) })
	}
}

func (m *Model) foldToggle(p *pane) {
	if p.folds.Items()[p.cursor].Fold >= 0 {
		m.foldOpen(p)
		return
	}
	m.foldClose(p)
}

// setContext refolds both views around changes with n context lines; folds
// start closed again, as after zM.
func (m *Model) setContext(n int) {
	n = max(n, 0)
	if n == m.context {
		return
	}
	m.remap(m.cur(), func() {
		m.context = n
		for v := range m.panes {
			m.panes[v].folds.SetContext(n)
		}
	})
}
