package chat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidkJun/revue/internal/theme"
)

// TestRealClaude asks the user's own claude CLI one cheap question, end to
// end through the panel. It costs a real request, so it only runs with
// REVUE_CHAT_REAL=1; with REVUE_CHAT_PREVIEW set it also writes the view.
func TestRealClaude(t *testing.T) {
	if os.Getenv("REVUE_CHAT_REAL") == "" {
		t.Skip("set REVUE_CHAT_REAL=1 to call the real claude CLI")
	}
	h := startHarness(t, Options{Theme: theme.Default, Dir: t.TempDir(), Model: "haiku", Effort: "low"})
	h.m.SetSize(60, 40)
	h.timeout = 3 * time.Minute
	tr := h.ask(nil, nil, "Answer in markdown: one short sentence on what Go's defer does, "+
		"then a Go code block of at most 6 lines showing it. No tools needed.")

	if tr.state != stateDone {
		t.Fatalf("state %d, err %v", tr.state, tr.err)
	}
	if !strings.Contains(tr.answer(), "```") || h.m.session == "" || tr.duration <= 0 {
		t.Errorf("answer %q, session %q, duration %v", tr.answer(), h.m.session, tr.duration)
	}
	t.Logf("session %s, %v, $%.4f\n%s", h.m.session, tr.duration, tr.cost, plainView(h.m))
	if dir := os.Getenv("REVUE_CHAT_PREVIEW"); dir != "" {
		if err := os.WriteFile(filepath.Join(dir, "real.ansi"), []byte(h.m.View(true)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
