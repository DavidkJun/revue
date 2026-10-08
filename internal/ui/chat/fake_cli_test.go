package chat

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as a fake claude CLI: the panel runs it with
// fakeEnv naming the scenario. It is caught in TestMain, before the testing
// package rejects claude's flags. Every run appends its argv and stdin to
// the file named by fakeLogEnv; runs that stay alive append a byte to the
// file named by fakeBeatEnv every beatInterval, so a test can tell whether
// a process was killed.
const (
	fakeEnv      = "REVUE_CHAT_FAKE_CLAUDE"
	fakeLogEnv   = "REVUE_CHAT_FAKE_LOG"
	fakeBeatEnv  = "REVUE_CHAT_FAKE_BEAT"
	beatInterval = 20 * time.Millisecond
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeEnv); mode != "" {
		os.Exit(fakeClaude(mode))
	}
	os.Exit(m.Run())
}

type fakeCall struct {
	Args  []string `json:"args"`
	Stdin string   `json:"stdin"`
}

func fakeClaude(mode string) int {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 1
	}
	args := os.Args[1:]
	if path := os.Getenv(fakeLogEnv); path != "" {
		line, _ := json.Marshal(fakeCall{Args: args, Stdin: string(stdin)})
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return 1
		}
		f.Write(append(line, '\n'))
		f.Close()
	}
	session := "sess-" + mode
	if i := slices.Index(args, "--resume"); i >= 0 && i+1 < len(args) {
		session = args[i+1]
	}
	cwd, _ := os.Getwd()

	emit(map[string]any{"type": "system", "subtype": "init", "session_id": session})
	switch mode {
	case "answer":
		emit(messageStart("msg_1"))
		emit(thinkingDelta("hmm"))
		emit(textDelta("Hello "))
		emit(textDelta("**world**"))
		emit(result("Hello **world**", session, 6100))
	case "tools":
		emit(messageStart("msg_1"))
		emit(textDelta("Let me look."))
		emit(assistantTool("msg_1", "Read", map[string]any{"file_path": filepath.Join(cwd, "src", "app.ts")}))
		emit(messageStart("msg_2"))
		emit(assistantTool("msg_2", "Grep", map[string]any{"pattern": "findOne"}))
		emit(messageStart("msg_3"))
		emit(textDelta("Found it."))
		emit(result("Found it.", session, 900))
	case "error":
		r := result("Prompt is too long", session, 100)
		r["is_error"] = true
		emit(r)
	case "hang":
		emit(messageStart("msg_1"))
		emit(textDelta("partial"))
		beat(time.Minute)
	case "linger":
		// claude keeps running after its result, e.g. while a hook or a
		// child it started has not finished.
		emit(messageStart("msg_1"))
		emit(textDelta("done"))
		emit(result("done", session, 100))
		beat(time.Minute)
	default:
		fmt.Fprintln(os.Stderr, "unknown fake mode", mode)
		return 2
	}
	return 0
}

// beat stays alive for d, appending to the heartbeat file while it does.
func beat(d time.Duration) {
	path := os.Getenv(fakeBeatEnv)
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(beatInterval) {
		if path == "" {
			continue
		}
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			f.Write([]byte{'.'})
			f.Close()
		}
	}
}

// alive reports whether a fake run is still beating: whether the heartbeat
// file grows within a few beats.
func alive(t *testing.T) bool {
	t.Helper()
	size := func() int64 {
		fi, err := os.Stat(os.Getenv(fakeBeatEnv))
		if err != nil {
			return 0
		}
		return fi.Size()
	}
	before := size()
	time.Sleep(10 * beatInterval)
	return size() > before
}

func emit(v any) {
	b, _ := json.Marshal(v)
	os.Stdout.Write(append(b, '\n'))
}

func streamEvent(ev map[string]any) map[string]any {
	return map[string]any{"type": "stream_event", "event": ev}
}

func messageStart(id string) map[string]any {
	return streamEvent(map[string]any{"type": "message_start", "message": map[string]any{"id": id}})
}

func textDelta(s string) map[string]any {
	return streamEvent(map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": s}})
}

func thinkingDelta(s string) map[string]any {
	return streamEvent(map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "thinking_delta", "thinking": s}})
}

func assistantTool(id, name string, input map[string]any) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{
		"id": id, "content": []any{map[string]any{"type": "tool_use", "name": name, "input": input}},
	}}
}

func result(text, session string, ms int) map[string]any {
	return map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "result": text,
		"session_id": session, "duration_ms": ms, "total_cost_usd": 0.02,
	}
}

// readCalls returns the fake runs logged so far.
func readCalls(t *testing.T) []fakeCall {
	t.Helper()
	data, err := os.ReadFile(os.Getenv(fakeLogEnv))
	if err != nil {
		t.Fatal(err)
	}
	var calls []fakeCall
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var c fakeCall
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			t.Fatal(err)
		}
		calls = append(calls, c)
	}
	return calls
}

// flag returns the value after name in args, or "" when absent.
func flag(args []string, name string) string {
	if i := slices.Index(args, name); i >= 0 && i+1 < len(args) {
		return args[i+1]
	}
	return ""
}
