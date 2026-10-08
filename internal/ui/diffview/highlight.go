package diffview

import (
	"path"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/highlight"
)

// highlightMsg carries the syntax colours of a file version; gen ties it to
// the SetFile call it was made for.
type highlightMsg struct {
	gen      int
	old, new [][]highlight.Span
}

// highlightCmd lexes both sides of the file off the UI goroutine. The line
// slices are never modified after SetFile, so sharing them is safe.
func (m *Model) highlightCmd() tea.Cmd {
	if m.hl == nil || m.f == nil {
		return nil
	}
	hl, gen, name := m.hl, m.gen, path.Base(m.path)
	oldLines, newLines := m.f.oldLines, m.f.newLines
	return func() tea.Msg {
		msg := highlightMsg{gen: gen}
		var wg sync.WaitGroup
		if len(oldLines) > 0 {
			wg.Go(func() { msg.old = hl.File(name, oldLines) })
		}
		if len(newLines) > 0 {
			msg.new = hl.File(name, newLines)
		}
		wg.Wait()
		return msg
	}
}

func (m *Model) applyHighlight(msg highlightMsg) {
	if msg.gen != m.gen || m.f == nil {
		return
	}
	m.f.oldSpans, m.f.newSpans = msg.old, msg.new
}
