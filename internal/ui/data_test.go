package ui

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/git"
)

func TestFileSelectedOutOfOrderKeepsTheCursorFile(t *testing.T) {
	// Bubble Tea runs every command in its own goroutine, so the
	// FileSelected messages of two quick moves can arrive in either order.
	h := newHarness(t, 140, 40)
	_, first := h.m.Update(keyPress("j"))
	_, second := h.m.Update(keyPress("j"))
	_, cur := h.current()
	h.send(second())
	h.send(first())
	if want := key(0, cur); h.m.want != want || h.m.shown != want {
		t.Errorf("cursor on %q but want %+v, shown %+v", cur, h.m.want, h.m.shown)
	}
}

func TestSupersededDiffLoadIsCancelled(t *testing.T) {
	fg := sampleGit()
	var mu sync.Mutex
	var ctxs []context.Context
	d := fg.deps()
	d.fileDiff = func(ctx context.Context, _ string, fc git.FileChange) (*diff.FileDiff, error) {
		mu.Lock()
		ctxs = append(ctxs, ctx)
		mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(400 * time.Millisecond): // slow git
		}
		return script(fc.Path, "+x"), nil
	}
	h := startHarness(t, 140, 40, `C:\work\atlas`, []git.Repo{backEnd, frontEnd}, fg, d, 30*time.Millisecond)

	// Claude keeps editing the shown file: every poll sees a new status.
	for i := range 4 {
		fg.mu.Lock()
		st := fg.statuses[backEnd.Path]
		st.Fingerprint = "be-edit-" + string(rune('a'+i))
		fg.statuses[backEnd.Path] = st
		fg.mu.Unlock()
		h.send(pollTickMsg{})
	}

	mu.Lock()
	defer mu.Unlock()
	if len(ctxs) < 3 {
		t.Fatalf("only %d diff loads started", len(ctxs))
	}
	for i, ctx := range ctxs[:len(ctxs)-1] {
		if ctx.Err() == nil {
			t.Errorf("load %d of %d is still running after a newer load of the file started", i+1, len(ctxs))
		}
	}
	if ctxs[len(ctxs)-1].Err() != nil {
		t.Error("the latest load was cancelled")
	}
}

func TestShownFileGoneDoesNotPullTheCursorFromAnotherRepo(t *testing.T) {
	// Both repos have src/main.ts. The cursor already moved to the other
	// repo's copy (its FileSelected is still on the way) when the shown one
	// is committed.
	fg := sampleGit()
	fg.statuses[backEnd.Path] = git.Status{Branch: "main", Fingerprint: "be-1", Files: []git.FileChange{
		change(git.Modified, "src/main.ts", 1, 1),
		change(git.Modified, "src/zeta.ts", 1, 1),
	}}
	fg.statuses[frontEnd.Path] = git.Status{Branch: "main", Fingerprint: "fe-1", Files: []git.FileChange{
		change(git.Modified, "src/main.ts", 2, 0),
	}}
	h := startHarness(t, 140, 40, `C:\work\atlas`, []git.Repo{backEnd, frontEnd}, fg, fg.deps(), 80*time.Millisecond)
	if h.m.shown != key(0, "src/main.ts") {
		t.Fatalf("shown %+v", h.m.shown)
	}
	h.m.sidebar.Select(1, "src/main.ts")

	st := fg.statuses[backEnd.Path]
	st.Fingerprint, st.Files = "be-2", st.Files[1:]
	h.send(statusMsg{repo: 0, status: st})
	if r, p := h.current(); r != 1 || p != "src/main.ts" {
		t.Errorf("cursor pulled to repo %d %q", r, p)
	}
}

func TestNextFileAfterTheShownFileWasCommitted(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("k") // onto src/auth, the shown file stays auth.service.ts
	if r, p := h.current(); r != 0 || p != "" {
		t.Fatalf("k: repo %d %q", r, p)
	}
	st := h.git.statuses[backEnd.Path]
	st.Fingerprint = "be-2"
	st.Files = slices.Delete(slices.Clone(st.Files), 1, 2) // auth.service.ts committed
	h.send(statusMsg{repo: 0, status: st})
	if h.m.shown.ok || !strings.Contains(h.plain(), "no longer changed") {
		t.Fatalf("shown %+v after its commit", h.m.shown)
	}
	h.keys("tab")
	if r, p := h.current(); r != 0 || p == "" {
		t.Errorf("tab: repo %d %q, want a file of repo 0", r, p)
	}
	h.keys("shift+tab", "shift+tab")
	if _, p := h.current(); p == "" {
		t.Error("shift+tab did not land on a file")
	}
}

func TestRepoBecomesCleanWhileItsFileIsShown(t *testing.T) {
	h := newHarness(t, 140, 40)
	st := h.git.statuses[backEnd.Path]
	st.Fingerprint, st.Files = "be-clean", nil
	h.send(statusMsg{repo: 0, status: st})
	if r, p := h.current(); r != 0 || p != "" {
		t.Errorf("cursor on repo %d %q, want the clean repo's node", r, p)
	}
	if h.m.shown.ok || !strings.Contains(h.plain(), "no uncommitted changes") {
		t.Errorf("shown %+v:\n%s", h.m.shown, h.plain())
	}
	h.keys("tab")
	if r, p := h.current(); r != 1 || p == "" {
		t.Errorf("tab from the clean repo: repo %d %q", r, p)
	}
}

func TestShownFileRenamedFollowsTheNewName(t *testing.T) {
	h := newHarness(t, 140, 40)
	if h.m.shown != key(0, "src/auth/auth.service.ts") {
		t.Fatalf("shown %+v", h.m.shown)
	}
	st := h.git.statuses[backEnd.Path]
	st.Fingerprint = "be-mv"
	st.Files = slices.Clone(st.Files)
	st.Files[1] = git.FileChange{Path: "src/auth/session.service.ts", OldPath: "src/auth/auth.service.ts", Kind: git.Renamed, Added: 9, Deleted: 4}
	h.send(statusMsg{repo: 0, status: st})
	want := key(0, "src/auth/session.service.ts")
	if h.m.shown != want {
		t.Errorf("shown %+v, want the renamed file", h.m.shown)
	}
	if r, p := h.current(); key(r, p) != want {
		t.Errorf("cursor on repo %d %q, want the renamed file", r, p)
	}
	if strings.Contains(h.plain(), "no longer changed") {
		t.Errorf("a rename was reported as gone:\n%s", h.plain())
	}
}
