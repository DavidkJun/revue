package ui

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/chat"
)

// TestDumpFrames writes full frames, ANSI styling included, to
// $REVUE_FRAME_DIR so they can be turned into screenshots with termshot and
// looked at. With $REVUE_LIVE_ROOT set it also renders the repos found
// there; they are only read.
func TestDumpFrames(t *testing.T) {
	if os.Getenv("REVUE_FRAME_DIR") == "" {
		t.Skip("set REVUE_FRAME_DIR to a directory to write frames")
	}
	for _, size := range [][2]int{{170, 45}, {100, 30}} {
		name := fmt.Sprintf("%dx%d", size[0], size[1])
		fg := sampleGit()
		h := startHarness(t, size[0], size[1], `C:\work\atlas`, []git.Repo{backEnd, frontEnd, docsRepo}, fg, fg.deps(), 400*time.Millisecond)
		dumpStates(t, h, "sample-"+name)
	}

	for _, size := range [][2]int{{60, 16}, {30, 8}} {
		fg := sampleGit()
		h := startHarness(t, size[0], size[1], `C:\work\atlas`, []git.Repo{backEnd, frontEnd, docsRepo}, fg, fg.deps(), 400*time.Millisecond)
		dumpFrame(t, fmt.Sprintf("tiny-%dx%d", size[0], size[1]), h.frame())
	}

	root := os.Getenv("REVUE_LIVE_ROOT")
	if root == "" {
		return
	}
	repos, err := git.Discover(root, 2)
	if err != nil || len(repos) == 0 {
		t.Fatalf("discover %s: %v (%d repos)", root, err, len(repos))
	}
	d := defaultDeps()
	d.clipboard = func(string) error { return nil }
	d.navigate = func(context.Context, string) error { return nil }
	for _, size := range [][2]int{{170, 45}, {100, 30}} {
		name := fmt.Sprintf("%dx%d", size[0], size[1])
		h := startHarness(t, size[0], size[1], root, repos, &fakeGit{}, d, time.Second)
		dumpStates(t, h, "live-"+name)
	}
}

func dumpStates(t *testing.T, h *harness, prefix string) {
	t.Helper()
	dumpFrame(t, prefix+"-start", h.frame())

	h.keys("ctrl+l", "j", "j", "j")
	dumpFrame(t, prefix+"-diff", h.frame())

	h.send(keyPress("ctrl+shift+d"))
	dumpFrame(t, prefix+"-split", h.frame())
	h.send(keyPress("ctrl+shift+d"))

	h.keys("V", "j", "j", "j")
	dumpFrame(t, prefix+"-visual", h.frame())
	h.keys("esc")

	h.keys("/", "r", "e", "t", "u", "r", "n")
	dumpFrame(t, prefix+"-search", h.frame())
	h.keys("enter")

	h.keys("?")
	dumpFrame(t, prefix+"-help", h.frame())
	h.keys("q")

	h.keys("1", "2", "g")
	dumpFrame(t, prefix+"-pending", h.frame())
	h.keys("esc")

	h.keys("a")
	dumpFrame(t, prefix+"-ask", h.frame())
	h.keys("esc")

	h.keys("L")
	dumpFrame(t, prefix+"-next-repo", h.frame())

	// $REVUE_FAKE_CLAUDE is a stand-in claude binary that prints a canned
	// stream-json answer, so the panel can be shown answering for free.
	if bin := os.Getenv("REVUE_FAKE_CLAUDE"); bin != "" {
		h.m.chat = chat.New(chat.Options{Theme: theme.Default, Dir: t.TempDir(), Model: "sonnet", Binary: bin})
		h.m.sizes[paneChat] = [2]int{}
		h.keys("H")
		h.m.focus = paneDiff
		h.keys("V", "j", "j", "a")
		for _, r := range "why is this async now?" {
			h.keys(string(r))
		}
		h.keys("enter")
		dumpFrame(t, prefix+"-chat-answer", h.frame())
	}
}
