package chat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/claude"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

// harness runs commands the way Bubble Tea does: each in a goroutine, its
// message fed back into Update.
type harness struct {
	t    *testing.T
	m    *Model
	msgs chan tea.Msg
	done chan struct{}

	timeout time.Duration // for waitFor
}

func newHarness(t *testing.T, mode string) *harness {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(fakeEnv, mode)
	t.Setenv(fakeLogEnv, filepath.Join(t.TempDir(), "calls.jsonl"))
	t.Setenv(fakeBeatEnv, filepath.Join(t.TempDir(), "beat"))
	return startHarness(t, Options{Theme: theme.Default, Dir: t.TempDir(), Model: "sonnet", Effort: "low", Binary: exe})
}

func startHarness(t *testing.T, opts Options) *harness {
	m := New(opts)
	m.SetSize(60, 30)
	h := &harness{t: t, m: m, msgs: make(chan tea.Msg, 64), done: make(chan struct{}), timeout: 15 * time.Second}
	t.Cleanup(func() {
		m.Cancel()
		close(h.done)
	})
	return h
}

func (h *harness) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() {
		msg := cmd()
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				h.run(c)
			}
			return
		}
		if msg == nil {
			return
		}
		select {
		case h.msgs <- msg:
		case <-h.done:
		}
	}()
}

// waitFor feeds messages until cond holds, rendering after each one like
// the real program.
func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.After(h.timeout)
	for !cond() {
		select {
		case msg := <-h.msgs:
			h.run(h.m.Update(msg))
			h.m.View(true)
		case <-deadline:
			h.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// pump feeds whatever arrives during d.
func (h *harness) pump(d time.Duration) {
	deadline := time.After(d)
	for {
		select {
		case msg := <-h.msgs:
			h.run(h.m.Update(msg))
			h.m.View(true)
		case <-deadline:
			return
		}
	}
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.run(h.m.Update(tea.KeyPressMsg{Code: r, Text: string(r)}))
	}
}

func (h *harness) press(k tea.KeyPressMsg) { h.run(h.m.Update(k)) }

// ask types question into a new prompt and sends it, then waits for the
// answer to finish.
func (h *harness) ask(items []claude.ContextItem, chips []string, question string) *turn {
	h.t.Helper()
	h.run(h.m.StartPrompt(items, chips))
	h.typeText(question)
	h.press(enter)
	if h.m.Prompting() || !h.m.Busy() {
		h.t.Fatalf("after enter: prompting=%v busy=%v, want a running question", h.m.Prompting(), h.m.Busy())
	}
	t := h.m.turns[len(h.m.turns)-1]
	h.waitFor("the answer to finish", func() bool { return !h.m.Busy() })
	return t
}

var (
	enter      = tea.KeyPressMsg{Code: tea.KeyEnter}
	shiftEnter = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}
	altEnter   = tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModAlt}
	escape     = tea.KeyPressMsg{Code: tea.KeyEscape}
	backspace  = tea.KeyPressMsg{Code: tea.KeyBackspace}
	ctrlC      = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
)

// plainView is the view without escape sequences or trailing spaces.
func plainView(m *Model) string {
	lines := strings.Split(ansi.Strip(m.View(true)), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " ")
	}
	return strings.Join(lines, "\n")
}

var sampleItems = []claude.ContextItem{{
	Repo: "atlas-back-end", RepoPath: `C:\repos\atlas-back-end`, Path: "src/app.ts",
	StartLine: 40, EndLine: 52, Selection: "+ const a = await load();", Patch: "@@ -40 +40 @@\n+const a = await load();",
}}

func TestSendStreamsAnswer(t *testing.T) {
	h := newHarness(t, "answer")
	m := h.m
	if !m.Empty() || m.Busy() || m.Prompting() {
		t.Fatal("new panel should be empty and idle")
	}
	tr := h.ask(sampleItems, []string{"atlas-back-end/src/app.ts:40-52"}, "why await?")

	if tr.state != stateDone || tr.answer() != "Hello **world**" {
		t.Fatalf("turn = state %d answer %q, want done with the streamed text", tr.state, tr.answer())
	}
	if tr.duration != 6100*time.Millisecond || tr.cost != 0.02 || tr.result != "Hello **world**" {
		t.Errorf("turn result = %v %v %q", tr.duration, tr.cost, tr.result)
	}
	if m.session != "sess-answer" {
		t.Errorf("session = %q, want the one from the result", m.session)
	}
	view := plainView(m)
	for _, want := range []string{"❯ why await?", "◆ atlas-back-end/src/app.ts:40-52", "Hello world", "sonnet · 6.1s", "1 turn"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}

	calls := readCalls(t)
	if len(calls) != 1 {
		t.Fatalf("%d claude runs, want 1", len(calls))
	}
	c := calls[0]
	if c.Stdin != claude.BuildPrompt("why await?", sampleItems) {
		t.Errorf("first question prompt = %q, want BuildPrompt of question and items", c.Stdin)
	}
	if flag(c.Args, "--model") != "sonnet" || flag(c.Args, "--effort") != "low" || slices.Contains(c.Args, "--resume") {
		t.Errorf("args = %q", c.Args)
	}
}

func TestFollowUpResumesSession(t *testing.T) {
	h := newHarness(t, "answer")
	m := h.m
	h.ask(sampleItems, nil, "first")

	h.run(m.HandleAction(keys.ChatFollowUp, 0))
	if !m.Prompting() {
		t.Fatal("ChatFollowUp did not open the prompt")
	}
	h.typeText("and then?")
	h.press(enter)
	h.waitFor("follow-up", func() bool { return !m.Busy() })

	items2 := []claude.ContextItem{{Repo: "atlas-front-end", Path: "src/main.tsx", Patch: "+x"}}
	h.ask(items2, []string{"atlas-front-end/src/main.tsx"}, "and this file?")

	calls := readCalls(t)
	if len(calls) != 3 {
		t.Fatalf("%d claude runs, want 3", len(calls))
	}
	if got := flag(calls[1].Args, "--resume"); got != "sess-answer" {
		t.Errorf("follow-up --resume = %q, want sess-answer", got)
	}
	if calls[1].Stdin != "and then?" {
		t.Errorf("follow-up prompt = %q, want the plain question", calls[1].Stdin)
	}
	if got := flag(calls[2].Args, "--resume"); got != "sess-answer" {
		t.Errorf("new-context question --resume = %q, want sess-answer", got)
	}
	if calls[2].Stdin != claude.BuildPrompt("and this file?", items2) {
		t.Errorf("new-context prompt = %q, want BuildPrompt with the new items", calls[2].Stdin)
	}

	h.run(m.HandleAction(keys.ChatNew, 0))
	if !m.Empty() || m.session != "" {
		t.Fatalf("ChatNew left %d turns, session %q", len(m.turns), m.session)
	}
	h.ask(nil, nil, "fresh")
	calls = readCalls(t)
	if last := calls[len(calls)-1]; slices.Contains(last.Args, "--resume") || last.Stdin != "fresh" {
		t.Errorf("question after ChatNew: args %q stdin %q, want no --resume and the plain question", last.Args, last.Stdin)
	}
}

// A question whose answer failed never reached a session, so a follow-up
// would arrive without the code it is about: its context goes again.
func TestFollowUpAfterFailedAnswerResendsContext(t *testing.T) {
	for _, fail := range []string{"error", "cancel"} {
		t.Run(fail, func(t *testing.T) {
			var h *harness
			if fail == "error" {
				h = newHarness(t, "error")
				h.ask(sampleItems, []string{"chip"}, "first")
			} else {
				h = newHarness(t, "hang")
				h.run(h.m.StartPrompt(sampleItems, []string{"chip"}))
				h.typeText("first")
				h.press(enter)
				tr := h.m.turns[0]
				h.waitFor("partial text", func() bool { return tr.answer() == "partial" })
				h.m.Cancel()
			}
			t.Setenv(fakeEnv, "answer")
			h.ask(nil, nil, "and then?")
			h.ask(nil, nil, "and after that?")

			calls := readCalls(t)
			if len(calls) != 3 {
				t.Fatalf("%d claude runs, want 3", len(calls))
			}
			if got, want := calls[1].Stdin, claude.BuildPrompt("and then?", sampleItems); got != want {
				t.Errorf("follow-up after %s: prompt %q, want the question with the unanswered context", fail, got)
			}
			if slices.Contains(calls[1].Args, "--resume") {
				t.Errorf("follow-up after %s resumed %q, but no session exists", fail, flag(calls[1].Args, "--resume"))
			}
			if calls[2].Stdin != "and after that?" || flag(calls[2].Args, "--resume") != "sess-answer" {
				t.Errorf("once answered, the context must not be sent again: %q %q", calls[2].Stdin, calls[2].Args)
			}
		})
	}
}

// claude can keep running after its result (hooks, a child holding it); the
// panel must not leave such a process behind for the rest of the session.
func TestLingeringProcessIsKilled(t *testing.T) {
	h := newHarness(t, "linger")
	h.m.linger = 300 * time.Millisecond
	tr := h.ask(nil, nil, "q")
	if tr.state != stateDone || h.m.session != "sess-linger" {
		t.Fatalf("state %d session %q", tr.state, h.m.session)
	}
	if !alive(t) {
		t.Fatal("the process was killed right at its result; claude may still be saving the session")
	}
	deadline := time.Now().Add(5 * time.Second)
	for alive(t) {
		if time.Now().After(deadline) {
			t.Fatal("claude still runs long after its answer finished")
		}
	}
}

func TestCancelKillsProcess(t *testing.T) {
	for _, how := range []keys.Action{keys.ChatCancel, keys.ChatNew} {
		t.Run(string(how), func(t *testing.T) {
			h := newHarness(t, "hang")
			h.run(h.m.StartPrompt(nil, nil))
			h.typeText("slow")
			h.press(enter)
			tr := h.m.turns[0]
			h.waitFor("partial text", func() bool { return tr.answer() == "partial" })
			if !alive(t) {
				t.Fatal("the fake claude is not running")
			}
			h.run(h.m.HandleAction(how, 0))
			deadline := time.Now().Add(5 * time.Second)
			for alive(t) {
				if time.Now().After(deadline) {
					t.Fatalf("claude still runs after %s", how)
				}
			}
		})
	}
}

func TestToolLines(t *testing.T) {
	h := newHarness(t, "tools")
	tr := h.ask(nil, nil, "where?")

	var got []string
	for _, s := range tr.segs {
		kind := "text"
		if s.kind == segTool {
			kind = "tool"
		}
		got = append(got, kind+":"+strings.TrimSpace(s.text))
	}
	want := []string{"text:Let me look.", "tool:Read src/app.ts", "tool:Grep findOne", "text:Found it."}
	if !slices.Equal(got, want) {
		t.Fatalf("segments = %q, want %q", got, want)
	}
	view := plainView(h.m)
	order := []string{"Let me look.", toolIcon + " Read src/app.ts", toolIcon + " Grep findOne", "Found it.", "sonnet · 0.9s"}
	at := 0
	for _, s := range order {
		i := strings.Index(view[at:], s)
		if i < 0 {
			t.Fatalf("view lacks %q after offset %d:\n%s", s, at, view)
		}
		at += i + len(s)
	}
}

func TestErrorTurn(t *testing.T) {
	h := newHarness(t, "error")
	tr := h.ask(nil, nil, "too much")
	if tr.state != stateError || tr.err == nil || !strings.Contains(tr.err.Error(), "Prompt is too long") {
		t.Fatalf("turn state %d err %v, want the CLI error", tr.state, tr.err)
	}
	if h.m.session != "" {
		t.Errorf("session = %q; a failed answer must not start a conversation", h.m.session)
	}
	view := plainView(h.m)
	if !strings.Contains(view, errorIcon+" Prompt is too long") || strings.Contains(view, "sonnet ·") {
		t.Errorf("error view:\n%s", view)
	}
	if !strings.Contains(h.m.View(true), ansiFG(theme.Default.Red)) {
		t.Error("error is not drawn in theme.Red")
	}
}

func TestCancel(t *testing.T) {
	h := newHarness(t, "hang")
	m := h.m
	h.run(m.StartPrompt(nil, nil))
	h.typeText("slow")
	h.press(enter)
	tr := m.turns[0]
	h.waitFor("partial text", func() bool { return tr.answer() == "partial" })

	h.run(m.HandleAction(keys.ChatCancel, 0))
	if m.Busy() || tr.state != stateCancelled {
		t.Fatalf("after ChatCancel: busy=%v state=%d", m.Busy(), tr.state)
	}
	h.pump(500 * time.Millisecond) // the killed process's last events must be ignored
	if tr.state != stateCancelled || tr.err != nil {
		t.Fatalf("late events changed the cancelled turn: state %d err %v", tr.state, tr.err)
	}
	if view := plainView(m); !strings.Contains(view, "cancelled") || !strings.Contains(view, "partial") {
		t.Errorf("cancelled view:\n%s", view)
	}

	// The conversation goes on after a cancel.
	t.Setenv(fakeEnv, "answer")
	tr2 := h.ask(nil, nil, "again")
	if tr2.state != stateDone {
		t.Fatalf("question after cancel: state %d", tr2.state)
	}
}

func TestSendWhileBusyCancelsRunningAnswer(t *testing.T) {
	h := newHarness(t, "hang")
	m := h.m
	h.run(m.StartPrompt(nil, nil))
	h.typeText("slow")
	h.press(enter)
	first := m.turns[0]
	h.waitFor("partial text", func() bool { return first.answer() == "partial" })

	t.Setenv(fakeEnv, "answer")
	second := h.ask(nil, nil, "next")
	if first.state != stateCancelled || second.state != stateDone {
		t.Fatalf("states = %d, %d; want cancelled, done", first.state, second.state)
	}
}

func TestStaleEventsIgnored(t *testing.T) {
	m := New(Options{Theme: theme.Default})
	m.SetSize(40, 20)
	old := m.begin("one", nil)
	m.Cancel()
	cur := m.begin("two", nil)

	text := func(s string) []claude.Event { return []claude.Event{{Kind: claude.EventText, Text: s}} }
	if cmd := m.Update(eventsMsg{turn: old.id, events: text("late")}); cmd != nil {
		t.Error("an event of a cancelled turn asked for more")
	}
	if old.answer() != "" || old.state != stateCancelled || cur.answer() != "" {
		t.Fatalf("a stale event changed a turn: old %q (%d), cur %q", old.answer(), old.state, cur.answer())
	}
	if cmd := m.Update(eventsMsg{turn: cur.id, events: text("fresh")}); cmd == nil {
		t.Error("an event of the running turn did not keep reading")
	}
	if cur.answer() != "fresh" {
		t.Fatalf("running turn answer = %q", cur.answer())
	}
	m.Update(eventsMsg{turn: cur.id, closed: true})
	if m.Busy() || cur.state != stateError || cur.err != errStreamEnded {
		t.Fatalf("closed stream: busy=%v state=%d err=%v", m.Busy(), cur.state, cur.err)
	}
	if cmd := m.Update(eventsMsg{turn: cur.id, events: text("after")}); cmd != nil || cur.answer() != "fresh" {
		t.Error("an event after the end changed the turn")
	}
}

func TestModelCycling(t *testing.T) {
	m := New(Options{Theme: theme.Default, Model: "sonnet"})
	m.SetSize(50, 10)
	for _, want := range []string{"opus", "haiku", "sonnet", "opus"} {
		m.HandleAction(keys.ChatModel, 0)
		if m.ModelName() != want {
			t.Fatalf("ModelName = %q, want %q", m.ModelName(), want)
		}
		if header := strings.Split(plainView(m), "\n")[0]; !strings.Contains(header, "Claude · "+want) {
			t.Errorf("header %q does not show %s", header, want)
		}
	}
	for cur, want := range map[string]string{"": "haiku", "claude-opus-4-5": "haiku", "sonnet[1m]": "opus", "HAIKU": "sonnet", "gpt": "haiku"} {
		if got := nextModel(cur); got != want {
			t.Errorf("nextModel(%q) = %q, want %q", cur, got, want)
		}
	}

	h := newHarness(t, "answer")
	h.m.HandleAction(keys.ChatModel, 0)
	h.m.HandleAction(keys.ChatModel, 0)
	tr := h.ask(nil, nil, "q")
	if tr.model != "haiku" || flag(readCalls(t)[0].Args, "--model") != "haiku" {
		t.Errorf("question used model %q / %q, want haiku", tr.model, flag(readCalls(t)[0].Args, "--model"))
	}
	if !strings.Contains(plainView(h.m), "haiku · 6.1s") {
		t.Errorf("footer does not name the model:\n%s", plainView(h.m))
	}
}

func TestYank(t *testing.T) {
	m := New(Options{Theme: theme.Default})
	if msg, ok := m.HandleAction(keys.ChatYank, 0)().(uimsg.Notify); !ok || msg.Text == "" {
		t.Fatalf("yank with no answer = %#v, want a notification", msg)
	}
	tr := m.begin("q", nil)
	m.Update(eventsMsg{turn: tr.id, events: []claude.Event{
		{Kind: claude.EventText, Text: "Use `await`:\n\n```ts\nawait x\n```"},
		{Kind: claude.EventTool, Text: "Read a.ts"},
		{Kind: claude.EventText, Text: "\n\nDone."},
		{Kind: claude.EventDone, Result: "Done."},
	}})
	m.begin("still thinking", nil)
	msg, ok := m.HandleAction(keys.ChatYank, 0)().(uimsg.Yank)
	if !ok {
		t.Fatalf("yank = %#v, want uimsg.Yank", msg)
	}
	if want := "Use `await`:\n\n```ts\nawait x\n```\n\nDone."; msg.Text != want || msg.What == "" {
		t.Errorf("yanked %q (%q), want %q", msg.Text, msg.What, want)
	}
}

func TestPromptInput(t *testing.T) {
	m := New(Options{Theme: theme.Default})
	m.SetSize(50, 40)
	key := func(k tea.KeyPressMsg) tea.Cmd { return m.Update(k) }
	typeText := func(s string) {
		for _, r := range s {
			key(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
	}
	for k, want := range map[tea.KeyPressMsg]string{enter: "enter", shiftEnter: "shift+enter", altEnter: "alt+enter", escape: "esc", ctrlC: "ctrl+c"} {
		if k.String() != want {
			t.Fatalf("test key %q, want %q", k.String(), want)
		}
	}

	if cmd := key(tea.KeyPressMsg{Code: 'x', Text: "x"}); cmd != nil || m.input.Value() != "" {
		t.Fatal("keys reached the input before StartPrompt")
	}
	m.StartPrompt(nil, nil)
	typeText("Привіт")
	key(shiftEnter)
	typeText("світ")
	key(altEnter)
	typeText("ok!")
	key(backspace)
	if got := m.input.Value(); got != "Привіт\nсвіт\nok" {
		t.Fatalf("value = %q", got)
	}
	// What win32-input-mode reports for Shift+Cyrillic, AltGr characters
	// and Caps Lock: modifiers set, the character in Text.
	for _, k := range []tea.KeyPressMsg{
		{Code: 'п', ShiftedCode: 'П', Text: "П", Mod: tea.ModShift},
		{Code: 'ґ', Text: "ґ", Mod: tea.ModCtrl | tea.ModAlt},
		{Code: '@', BaseCode: 'q', Text: "@", Mod: tea.ModCtrl | tea.ModAlt},
		{Code: 'і', Text: "І", Mod: tea.ModCapsLock},
	} {
		key(k)
	}
	if got := m.input.Value(); got != "Привіт\nсвіт\nokПґ@І" {
		t.Fatalf("value with modified keys = %q", got)
	}
	for range 4 {
		key(backspace)
	}
	if m.input.Height() != 3 {
		t.Errorf("input height = %d for 3 lines", m.input.Height())
	}
	m.Update(tea.PasteMsg{Content: " pasted\r\nline"})
	if got := m.input.Value(); !strings.HasSuffix(got, "ok pasted\nline") {
		t.Errorf("after paste value = %q", got)
	}
	for range 10 {
		key(shiftEnter)
	}
	if m.input.Height() != maxInputLines {
		t.Errorf("input height = %d with 14 lines, want it capped at %d", m.input.Height(), maxInputLines)
	}
	if view := plainView(m); !strings.Contains(view, "esc cancel") {
		t.Errorf("prompt view lacks the key hint:\n%s", view)
	}

	key(escape)
	if m.Prompting() || !m.Empty() {
		t.Fatal("esc did not cancel the prompt")
	}
	m.StartPrompt(nil, nil)
	if m.input.Value() != "" || m.input.Height() != 1 {
		t.Errorf("a cancelled draft came back: %q, %d lines high", m.input.Value(), m.input.Height())
	}
	typeText("   ")
	key(shiftEnter)
	if cmd := key(enter); cmd != nil || !m.Prompting() || !m.Empty() {
		t.Fatal("an empty question was sent")
	}
	key(ctrlC)
	if m.Prompting() {
		t.Fatal("ctrl+c did not close the prompt")
	}
}

// ctrl+v reads the Windows clipboard, whose line ends are CRLF; the
// textarea would turn each into two line breaks.
func TestCtrlVPasteNormalisesLineEnds(t *testing.T) {
	m := New(Options{Theme: theme.Default})
	m.SetSize(50, 30)
	m.readClipboard = func() (string, error) { return "one\r\ntwo\rthree", nil }
	m.StartPrompt(nil, nil)
	m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	cmd := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+v did not read the clipboard")
	}
	// The app routes the paste back like a terminal paste.
	m.Update(cmd())
	if got := m.input.Value(); got != "xone\ntwo\nthree" {
		t.Errorf("value after ctrl+v = %q", got)
	}
	m.readClipboard = func() (string, error) { return "", errors.New("no text") }
	if msg := m.Update(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})(); msg != nil {
		t.Errorf("an unreadable clipboard produced %#v", msg)
	}
}

func TestStreamingRendersOnTicksOnly(t *testing.T) {
	m := New(Options{Theme: theme.Default})
	m.SetSize(60, 30)
	tr := m.begin("q", nil)
	for i := range 200 {
		m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: "word" + string(rune('a'+i%26)) + " "}}})
		m.View(true)
	}
	if m.md.renders != 1 {
		t.Fatalf("%d markdown renders for 200 tokens before a tick, want 1", m.md.renders)
	}
	if m.Update(tickMsg{}) == nil {
		t.Fatal("tick stopped while streaming")
	}
	m.View(true)
	if m.md.renders != 2 || !strings.Contains(plainView(m), "wordr") {
		t.Fatalf("after a tick: %d renders, view:\n%s", m.md.renders, plainView(m))
	}
	m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: "**end**"}, {Kind: claude.EventDone}}})
	if view := plainView(m); !strings.Contains(view, "end") {
		t.Fatalf("finished answer not re-rendered:\n%s", view)
	}
	if m.Update(tickMsg{}) != nil || m.ticking {
		t.Error("tick kept running after the answer finished")
	}
}

func TestScrolling(t *testing.T) {
	m := New(Options{Theme: theme.Default})
	m.SetSize(40, 20)
	tr := m.begin("long", nil)
	var b strings.Builder
	for i := range 100 {
		b.WriteString("line " + strings.Repeat("x", i%7) + "\n\n")
	}
	m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: b.String()}}})
	m.View(true)
	bottom := m.maxOffset()
	if bottom < 100 || m.offset != bottom {
		t.Fatalf("offset %d of %d, want to follow the bottom", m.offset, bottom)
	}

	m.HandleAction(keys.Up, 3)
	if m.offset != bottom-3 || m.follow {
		t.Fatalf("3k: offset %d follow %v", m.offset, m.follow)
	}
	m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: "more\n\nand more"}}})
	renderTick(m)
	m.View(true)
	if m.offset != bottom-3 {
		t.Errorf("new text moved a scrolled-up view to %d", m.offset)
	}
	m.HandleAction(keys.Top, 0)
	if m.offset != 0 {
		t.Errorf("gg: offset %d", m.offset)
	}
	m.HandleAction(keys.HalfDown, 0)
	if half := m.bodyHeight() / 2; m.offset != half {
		t.Errorf("ctrl+d: offset %d, want %d", m.offset, half)
	}
	m.HandleAction(keys.Down, 0)
	m.Scroll(2)
	if half := m.bodyHeight() / 2; m.offset != half+3 {
		t.Errorf("j + wheel 2: offset %d, want %d", m.offset, half+3)
	}
	m.Scroll(-1000)
	if m.offset != 0 {
		t.Errorf("wheel up past the top: offset %d", m.offset)
	}
	m.HandleAction(keys.Bottom, 0)
	m.View(true)
	if !m.follow || m.offset != m.maxOffset() {
		t.Errorf("G: offset %d of %d follow %v", m.offset, m.maxOffset(), m.follow)
	}
	m.Scroll(-1)
	m.Scroll(1)
	if !m.follow {
		t.Error("scrolling back to the bottom did not resume following")
	}
	m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: "\n\nTHE END"}}})
	renderTick(m)
	lines := strings.Split(plainView(m), "\n")
	if !strings.Contains(strings.Join(lines[len(lines)-4:], "\n"), "THE END") {
		t.Errorf("following view does not show the newest text at the bottom:\n%s", strings.Join(lines, "\n"))
	}
}

// gg is a request to read from the top; the answer growing past the screen
// afterwards must not drag the view to the bottom.
func TestTopPinsViewWhileAnswerGrows(t *testing.T) {
	for _, jump := range []struct {
		a     keys.Action
		count int
	}{{keys.Top, 0}, {keys.Bottom, 1}} {
		m := New(Options{Theme: theme.Default})
		m.SetSize(40, 20)
		tr := m.begin("q", nil)
		m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: "short start"}}})
		m.View(true)
		m.HandleAction(jump.a, jump.count)
		var b strings.Builder
		for i := range 60 {
			b.WriteString("\n\npara " + strings.Repeat("y", i%5))
		}
		m.Update(eventsMsg{turn: tr.id, events: []claude.Event{{Kind: claude.EventText, Text: b.String()}}})
		renderTick(m)
		if !strings.Contains(plainView(m), "❯ q") {
			t.Errorf("%s (count %d) then a growing answer: the view left the top:\n%s", jump.a, jump.count, plainView(m))
		}
		m.HandleAction(keys.Bottom, 0)
		m.View(true)
		if !m.follow || m.offset != m.maxOffset() {
			t.Errorf("G after %s did not follow the bottom again", jump.a)
		}
	}
}

func TestFullToggle(t *testing.T) {
	m := New(Options{Theme: theme.Default})
	m.HandleAction(keys.ChatFull, 0)
	if !m.Full() {
		t.Fatal("ChatFull did not request the full width")
	}
	m.HandleAction(keys.ChatFull, 0)
	if m.Full() {
		t.Fatal("second ChatFull did not toggle back")
	}
	if m.HandleAction(keys.ChatHide, 0) != nil {
		t.Error("ChatHide is the app's business")
	}
}

// ansiFG is the SGR fragment lipgloss writes for a foreground colour.
func ansiFG(hex string) string {
	v, _ := strconv.ParseUint(hex[1:], 16, 32)
	return fmt.Sprintf("38;2;%d;%d;%d", v>>16, v>>8&0xff, v&0xff)
}

// renderTick delivers a tick that is due to re-render the streaming answer,
// as the next real tick would be.
func renderTick(m *Model) {
	m.nextRender = time.Time{}
	m.Update(tickMsg{})
}
