package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

func TestStatuslineMakesRoomForANotice(t *testing.T) {
	position := regexp.MustCompile(`\d+/\d+ *$`)
	cases := []struct {
		w       int
		pending bool
		want    []string
		pos     bool
	}{
		{120, true, []string{"47  yanked atlas", "UNIFIED"}, true},
		{100, true, []string{"47  yanked atlas"}, true},
		{80, false, []string{"yanked atlas"}, false},
		{80, true, []string{"47"}, false},
	}
	for _, c := range cases {
		h := newHarness(t, c.w, 30)
		h.send(uimsg.Notify{Text: "yanked atlas-back-end/src/auth/refresh-token.guard.ts:118"})
		if c.pending {
			h.keys("4", "7")
		}
		line := lastLine(h.plain())
		for _, w := range c.want {
			if !strings.Contains(line, w) {
				t.Errorf("width %d: %q missing from the statusline %q", c.w, w, line)
			}
		}
		if c.pos && !position.MatchString(line) {
			t.Errorf("width %d: no cursor position in %q", c.w, line)
		}
	}
}

func TestStatuslineDescribesTheShownFile(t *testing.T) {
	h := newHarness(t, 140, 30)
	h.m.sidebar.Select(1, "") // the tree cursor moves to the other repo's node
	line := lastLine(h.plain())
	if !strings.Contains(line, "atlas-back-end") || !strings.Contains(line, "auth.service.ts") || strings.Contains(line, "atlas-front-end") {
		t.Errorf("statusline %q, want the shown file's repo and path", line)
	}
}
