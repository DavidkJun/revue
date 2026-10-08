package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/config"
	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/theme"
)

// lockedBuffer is a terminal for a real Bubble Tea program, whose renderer
// writes from its own goroutine.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

func TestRunResetsTheTerminalWhenItDies(t *testing.T) {
	t.Setenv("WEZTERM_PANE", "7")
	fg := sampleGit()
	d := fg.deps()
	d.status = func(context.Context, string) (git.Status, error) { panic("git exploded") }
	m := newModel(`C:\work\atlas`, []git.Repo{backEnd}, config.Default(), theme.Default, d)
	var out lockedBuffer
	err := run(m, &out, tea.WithInput(nil), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler())
	if !errors.Is(err, tea.ErrProgramPanic) {
		t.Fatalf("run returned %v, want a caught panic", err)
	}
	// It may die before or after turning the modes on; either way the
	// reset comes last.
	if got := out.String(); !strings.HasSuffix(got, win32InputOff+isNvimFalse) {
		t.Errorf("the terminal modes were not reset last: ...%q", got[max(len(got)-120, 0):])
	}
}

func TestRunResetsTheTerminalOnQuit(t *testing.T) {
	t.Setenv("WEZTERM_PANE", "7")
	fg := sampleGit()
	m := newModel(`C:\work\atlas`, []git.Repo{backEnd}, config.Default(), theme.Default, fg.deps())
	in, keys := io.Pipe()
	defer keys.Close()
	go keys.Write([]byte("Q")) // blocks until revue reads it
	var out lockedBuffer
	if err := run(m, &out, tea.WithInput(in), tea.WithWindowSize(80, 24), tea.WithoutSignalHandler()); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "atlas-back-end") || !strings.HasSuffix(got, win32InputOff+isNvimFalse) {
		t.Errorf("after Q the terminal modes were not reset last: ...%q", got[max(len(got)-120, 0):])
	}
}

func TestEditorHandsOverTheTerminalModes(t *testing.T) {
	for _, wez := range []bool{false, true} {
		pane := ""
		if wez {
			pane = "7"
		}
		t.Setenv("WEZTERM_PANE", pane) // the tests may run inside WezTerm
		h := newHarness(t, 140, 40)
		h.raw = nil
		h.keys("e")
		if len(h.raw) != 1 || !strings.Contains(h.raw[0], win32InputOff) || wez != strings.Contains(h.raw[0], isNvimFalse) {
			t.Fatalf("wezterm %v: before the editor wrote %q", wez, h.raw)
		}
		h.raw = nil
		h.send(editorDoneMsg{err: errors.New("exit status 1")})
		if len(h.raw) != 1 || !strings.Contains(h.raw[0], win32InputOn) || wez != strings.Contains(h.raw[0], isNvimTrue) {
			t.Errorf("wezterm %v: after the editor wrote %q", wez, h.raw)
		}
		if !h.m.notice.err {
			t.Errorf("wezterm %v: editor failure not reported: %+v", wez, h.m.notice)
		}
	}
}

func TestEveryQuitTurnsTheModesOff(t *testing.T) {
	t.Setenv("WEZTERM_PANE", "7")
	for _, k := range []string{"q", "Q", "ctrl+c"} {
		h := newHarness(t, 140, 40)
		h.raw = nil
		h.keys(k)
		if !h.quit || len(h.raw) == 0 || !strings.Contains(h.raw[0], win32InputOff+isNvimFalse) {
			t.Errorf("%s: quit %v, raw %q", k, h.quit, h.raw)
		}
	}
}
