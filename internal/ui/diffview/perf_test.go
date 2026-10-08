package diffview

import (
	"fmt"
	"testing"
	"time"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/highlight"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// bigFile is a 10000-line TypeScript-like file with a change block every 40
// lines: a modified line, an added line and a deleted line.
func bigFile(t testing.TB) *diff.FileDiff {
	var spec []string
	for i := 0; len(spec) < 10000; i++ {
		switch i % 40 {
		case 10:
			spec = append(spec,
				fmt.Sprintf("-  const value%d = await this.repository.findOne({ where: { id: %d } });", i, i),
				fmt.Sprintf("+  const value%d = await this.repository.findOne({ where: { id: %d, deleted: false } });", i, i),
				fmt.Sprintf("+  this.logger.debug(`loaded ${value%d.id}`);", i))
		case 25:
			spec = append(spec, fmt.Sprintf("-  // TODO(%d): remove this once the migration ran", i))
		default:
			spec = append(spec, fmt.Sprintf("   if (item.status === Status.Active && item.count > %d) { return mapItem(item, '%d'); }", i, i))
		}
	}
	return fileDiff(t, spec...)
}

func TestPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("timing test")
	}
	fd := bigFile(t)
	m := New(theme.Default, highlight.New(theme.Default.SyntaxStyle), 50)
	m.SetSize(120, 45)

	start := time.Now()
	cmd := m.SetFile("revue", "src/big.service.ts", fd, false)
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Errorf("SetFile took %v, want < 50ms", d)
	}
	m.Update(cmd())

	views := map[string]func(){
		"unified": func() {},
		"split":   func() { m.SetSplit(true) },
		"all open + search + visual": func() {
			m.SetSplit(false)
			act(m, keys.FoldOpenAll, 0)
			m.SetSearch("item")
			act(m, keys.VisualStart, 0)
			act(m, keys.Down, 20)
		},
	}
	for _, name := range []string{"unified", "split", "all open + search + visual"} {
		views[name]()
		m.View(true)
		const n = 50
		start = time.Now()
		for range n {
			act(m, keys.Down, 1)
			m.View(true)
		}
		if avg := time.Since(start) / n; avg > 2*time.Millisecond {
			t.Errorf("%s: View took %v on average, want < 2ms", name, avg)
		}
	}
}

func BenchmarkView(b *testing.B) {
	fd := bigFile(b)
	m := New(theme.Default, highlight.New(theme.Default.SyntaxStyle), 50)
	m.SetSize(120, 45)
	m.Update(m.SetFile("revue", "src/big.service.ts", fd, false)())
	b.ResetTimer()
	for range b.N {
		m.View(true)
	}
}

func BenchmarkSetFile(b *testing.B) {
	fd := bigFile(b)
	m := New(theme.Default, highlight.New(theme.Default.SyntaxStyle), 50)
	m.SetSize(120, 45)
	b.ResetTimer()
	for range b.N {
		m.SetFile("revue", "src/big.service.ts", fd, false)
	}
}
