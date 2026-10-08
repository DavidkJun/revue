package claude

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func fakeRequest(t *testing.T, mode string) Request {
	t.Helper()
	t.Setenv(fakeEnv, mode)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return Request{Binary: exe, Dir: t.TempDir(), Prompt: "question"}
}

// collect reads ch until it is closed and checks that exactly one final
// event arrived, as the last one.
func collect(t *testing.T, ch <-chan Event, timeout time.Duration) []Event {
	t.Helper()
	deadline := time.After(timeout)
	var evs []Event
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				checkFinal(t, evs)
				return evs
			}
			evs = append(evs, ev)
		case <-deadline:
			t.Fatalf("channel not closed after %v; got %d events", timeout, len(evs))
		}
	}
}

func checkFinal(t *testing.T, evs []Event) {
	t.Helper()
	if len(evs) == 0 {
		t.Fatal("channel closed without a final event")
	}
	for i, ev := range evs {
		final := ev.Kind == EventDone || ev.Kind == EventError
		if final != (i == len(evs)-1) {
			t.Fatalf("event %d (kind %d) breaks the one-final-event-last rule", i, ev.Kind)
		}
	}
}

func joinText(evs []Event) string {
	var b strings.Builder
	for _, ev := range evs {
		if ev.Kind == EventText {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

func kinds(evs []Event, k EventKind) []string {
	var out []string
	for _, ev := range evs {
		if ev.Kind == k {
			out = append(out, ev.Text)
		}
	}
	return out
}

func TestAskArgv(t *testing.T) {
	req := fakeRequest(t, "args")
	req.Prompt = "Привіт — what does `x` do?\nsecond line"
	req.AddDirs = []string{`C:\repos\front end`, `C:\repos\back`}
	req.SessionID = "0b6c-session"
	req.Model = "sonnet"
	req.Effort = "low"
	req.SystemPrompt = `Be "brief" & kind\`
	req.ExtraArgs = []string{"--max-turns", "3"}

	ch, err := Ask(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch, 30*time.Second)
	done := evs[len(evs)-1]
	if done.Kind != EventDone {
		t.Fatalf("final event = %+v", done)
	}
	var got struct {
		Args  []string
		Stdin string
		Cwd   string
	}
	if err := json.Unmarshal([]byte(done.Result), &got); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--tools", "Read,Grep,Glob", "--permission-mode", "dontAsk",
		"--settings", settingsArg(), "--strict-mcp-config",
		"--append-system-prompt", `Be "brief" & kind\`,
		"--model", "sonnet", "--effort", "low", "--resume", "0b6c-session",
		`--add-dir=C:\repos\front end`, `--add-dir=C:\repos\back`,
		"--max-turns", "3",
	}
	if !reflect.DeepEqual(got.Args, want) {
		t.Errorf("argv\n got %q\nwant %q", got.Args, want)
	}
	if got.Stdin != req.Prompt {
		t.Errorf("stdin = %q, want %q", got.Stdin, req.Prompt)
	}
	if !samePath(got.Cwd, req.Dir) {
		t.Errorf("cwd = %q, want %q", got.Cwd, req.Dir)
	}
	if done.SessionID != "sess-args" {
		t.Errorf("SessionID = %q", done.SessionID)
	}
}

func TestBuildArgsDefaults(t *testing.T) {
	got := buildArgs(Request{})
	want := []string{
		"-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
		"--tools", "Read,Grep,Glob", "--permission-mode", "dontAsk",
		"--settings", settingsArg(), "--strict-mcp-config",
		"--append-system-prompt", DefaultSystemPrompt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv\n got %q\nwant %q", got, want)
	}
	data, err := os.ReadFile(settingsArg())
	if err != nil || string(data) != questionSettings {
		t.Errorf("settings file %s = %q, %v; want %q", settingsArg(), data, err, questionSettings)
	}
}

func TestAskStreamsTextAndTools(t *testing.T) {
	req := fakeRequest(t, "stream")
	ch, err := Ask(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch, 30*time.Second)

	if got, want := joinText(evs), "Let me look.\n\nIt is fine."; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if got, want := kinds(evs, EventTool), []string{"Read src/app.ts"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tools = %q, want %q", got, want)
	}
	if got := kinds(evs, EventThinking); !reflect.DeepEqual(got, []string{"", "hmm"}) {
		t.Errorf("thinking = %q", got)
	}
	done := evs[len(evs)-1]
	if done.Kind != EventDone || done.Result != "It is fine." || done.SessionID != "sess-stream" ||
		done.CostUSD != 0.5 || done.Duration != 1234*time.Millisecond {
		t.Errorf("final event = %+v", done)
	}
}

func TestAskErrorResult(t *testing.T) {
	ch, err := Ask(context.Background(), fakeRequest(t, "error"))
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch, 30*time.Second)
	last := evs[len(evs)-1]
	if last.Kind != EventError || last.Err == nil || !strings.Contains(last.Err.Error(), "Prompt is too long") {
		t.Fatalf("final event = %+v", last)
	}
	if last.SessionID != "sess-err" {
		t.Errorf("SessionID = %q", last.SessionID)
	}
}

func TestAskExitWithoutResult(t *testing.T) {
	ch, err := Ask(context.Background(), fakeRequest(t, "noresult"))
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch, 30*time.Second)
	if got := joinText(evs); got != "partial" {
		t.Errorf("text = %q", got)
	}
	last := evs[len(evs)-1]
	if last.Kind != EventError || last.Err == nil {
		t.Fatalf("final event = %+v", last)
	}
	msg := last.Err.Error()
	for _, want := range []string{"without a result", "exit status 3", "fatal: boom"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not mention %q", msg, want)
		}
	}
	if strings.Contains(msg, "BEGIN") || len(msg) > stderrTail+200 {
		t.Errorf("error keeps more than the stderr tail (%d bytes)", len(msg))
	}
	if last.SessionID != "sess-noresult" {
		t.Errorf("SessionID = %q", last.SessionID)
	}
}

func TestAskHugeLine(t *testing.T) {
	ch, err := Ask(context.Background(), fakeRequest(t, "huge"))
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch, 60*time.Second)
	text := joinText(evs)
	if len(text) != 6<<20 || strings.Trim(text, "a") != "" {
		t.Errorf("text has %d bytes, want %d of 'a'", len(text), 6<<20)
	}
	if last := evs[len(evs)-1]; last.Kind != EventDone || last.Result != "done" {
		t.Errorf("final event = %+v", last)
	}
}

func TestAskCancelKillsProcessTree(t *testing.T) {
	mode := "sleep"
	if killsTree {
		mode = "sleeptree"
	}
	req := fakeRequest(t, mode)
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	t.Setenv(fakePIDEnv, pidFile)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Ask(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if killsTree {
		select {
		case ev := <-ch:
			if ev.Kind != EventText || ev.Text != "ready" {
				t.Fatalf("first event = %+v", ev)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("fake claude never got ready")
		}
	}

	start := time.Now()
	cancel()
	evs := collect(t, ch, 10*time.Second)
	elapsed := time.Since(start)
	last := evs[len(evs)-1]
	if last.Kind != EventError || !errors.Is(last.Err, context.Canceled) {
		t.Fatalf("final event = %+v", last)
	}
	// Faster than waitDelay: the stream ended because the whole tree died,
	// not because Wait gave up on a grandchild holding stdout.
	if elapsed > waitDelay/2 {
		t.Errorf("cancellation took %v", elapsed)
	}
	if !killsTree {
		return
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if !processGone(pid, 5*time.Second) {
		t.Errorf("grandchild %d still running after cancel", pid)
	}
}

func TestAskCancelWithoutReading(t *testing.T) {
	req := fakeRequest(t, "flood")
	before := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Ask(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	// Let the flood fill the channel buffer while nobody reads it.
	time.Sleep(500 * time.Millisecond)
	cancel()
	// Without a reader, Ask's goroutines must still finish.
	deadline := time.Now().Add(10 * time.Second)
	for runtime.NumGoroutine() > before {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines still running, %d before Ask", runtime.NumGoroutine(), before)
		}
		time.Sleep(10 * time.Millisecond)
	}
	evs := collect(t, ch, time.Second)
	if last := evs[len(evs)-1]; last.Kind != EventError || !errors.Is(last.Err, context.Canceled) {
		t.Fatalf("final event = %+v", last)
	}
}

func TestAskMissingBinary(t *testing.T) {
	ch, err := Ask(context.Background(), Request{Binary: "revue-no-such-claude-binary"})
	if ch != nil || !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("Ask = %v, %v; want no channel and exec.ErrNotFound", ch, err)
	}
}

func samePath(a, b string) bool {
	ea, errA := filepath.EvalSymlinks(a)
	eb, errB := filepath.EvalSymlinks(b)
	if errA == nil && errB == nil {
		a, b = ea, eb
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// A reader that stops for a while (the UI is blocked while the user is in
// their editor) must still get the end of the answer, even though claude
// exited long before Ask handed over everything it wrote.
func TestAskPausedReaderGetsWholeAnswer(t *testing.T) {
	ch, err := Ask(context.Background(), fakeRequest(t, "burst"))
	if err != nil {
		t.Fatal(err)
	}
	var evs []Event
	select {
	case ev := <-ch:
		evs = append(evs, ev)
	case <-time.After(30 * time.Second):
		t.Fatal("no first event")
	}
	time.Sleep(waitDelay + 1500*time.Millisecond)
	evs = append(evs, collect(t, ch, 30*time.Second)...)
	checkFinal(t, evs)
	if got := joinText(evs); len(got) != burstDeltas+1 {
		t.Errorf("got %d bytes of text, want %d", len(got), burstDeltas+1)
	}
	if last := evs[len(evs)-1]; last.Kind != EventDone || last.SessionID != "sess-burst" {
		t.Fatalf("final event = %+v", last)
	}
}

func TestAskCancelMidStream(t *testing.T) {
	req := fakeRequest(t, "drip")
	pidFile := filepath.Join(t.TempDir(), "claude.pid")
	t.Setenv(fakePIDEnv, pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := Ask(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		select {
		case ev := <-ch:
			if ev.Kind != EventText {
				t.Fatalf("event %d = %+v", i, ev)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("no text streamed")
		}
	}

	start := time.Now()
	cancel()
	evs := collect(t, ch, 10*time.Second)
	if elapsed := time.Since(start); elapsed > waitDelay/2 {
		t.Errorf("cancellation took %v", elapsed)
	}
	if last := evs[len(evs)-1]; last.Kind != EventError || !errors.Is(last.Err, context.Canceled) {
		t.Fatalf("final event = %+v", last)
	}
	if len(evs) > 64+1 {
		t.Errorf("%d events after cancel; streaming should stop", len(evs))
	}
	b, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if !processGone(pid, 5*time.Second) {
		t.Errorf("claude %d still running after cancel", pid)
	}
}

func TestAskStderrFloodDoesNotBlock(t *testing.T) {
	ch, err := Ask(context.Background(), fakeRequest(t, "stderrflood"))
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch, 30*time.Second)
	if last := evs[len(evs)-1]; last.Kind != EventDone || joinText(evs) != "ok" {
		t.Fatalf("events = %+v", evs)
	}
}

// The answer is complete at the result line; the UI must not wait for
// claude's shutdown, and cancelling afterwards still stops the process.
func TestAskDoneBeforeExit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	start := time.Now()
	ch, err := Ask(ctx, fakeRequest(t, "linger"))
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch, 30*time.Second)
	if last := evs[len(evs)-1]; last.Kind != EventDone || last.Result != "answer" || joinText(evs) != "answer" {
		t.Fatalf("events = %+v", evs)
	}
	if elapsed := time.Since(start); elapsed > 20*time.Second {
		t.Errorf("EventDone took %v; it waited for the process to exit", elapsed)
	}
}

func TestAskAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch, err := Ask(ctx, fakeRequest(t, "sleep"))
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch, waitDelay/2)
	if last := evs[len(evs)-1]; last.Kind != EventError || !errors.Is(last.Err, context.Canceled) {
		t.Fatalf("final event = %+v", last)
	}
}
