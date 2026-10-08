package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as a fake claude CLI: Ask re-executes it with
// fakeEnv naming the scenario. It has to be caught in TestMain, before the
// testing package rejects claude's flags.
const (
	fakeEnv    = "REVUE_FAKE_CLAUDE"
	fakePIDEnv = "REVUE_FAKE_CLAUDE_PIDFILE"

	burstDeltas = 80
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeEnv); mode != "" {
		os.Exit(fakeClaude(mode))
	}
	os.Exit(m.Run())
}

func fakeClaude(mode string) int {
	cwd, _ := os.Getwd()
	switch mode {
	case "args":
		stdin, err := io.ReadAll(os.Stdin)
		if err != nil {
			return 1
		}
		report, _ := json.Marshal(map[string]any{"args": os.Args[1:], "stdin": string(stdin), "cwd": cwd})
		emit(systemInit("sess-args"))
		emit(resultLine(string(report), "sess-args"))
	case "stream":
		emit(systemInit("sess-stream"))
		emit(messageStart("msg_1"))
		emit(streamEvent(map[string]any{"type": "content_block_start", "content_block": map[string]any{"type": "thinking", "thinking": ""}}))
		emit(thinkingDelta("hmm"))
		emit(textDelta("Let me "))
		emit(textDelta("look."))
		emit(assistantText("msg_1", "Let me look."))
		emit(assistantTool("msg_1", "Read", map[string]any{"file_path": filepath.Join(cwd, "src", "app.ts")}))
		emit(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": "tool output"}})
		emit(map[string]any{"type": "rate_limit_event", "rate_limit_info": map[string]any{"status": "allowed"}})
		emit(messageStart("msg_2"))
		emit(textDelta("It is fine."))
		emit(assistantText("msg_2", "It is fine."))
		r := resultLine("It is fine.", "sess-stream")
		r["total_cost_usd"] = 0.5
		r["duration_ms"] = 1234
		emit(r)
	case "error":
		emit(systemInit("sess-err"))
		r := resultLine("Prompt is too long", "sess-err")
		r["is_error"] = true
		emit(r)
	case "noresult":
		emit(systemInit("sess-noresult"))
		emit(textDelta("partial"))
		fmt.Fprint(os.Stderr, "BEGIN"+strings.Repeat("x", 5000)+"\nfatal: boom\n")
		return 3
	case "huge":
		emit(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": strings.Repeat("u", 12<<20)}})
		emit(textDelta(strings.Repeat("a", 6<<20)))
		emit(resultLine("done", "sess-huge"))
	case "flood":
		for i := 0; i < 500; i++ {
			emit(textDelta("x"))
		}
		time.Sleep(time.Minute)
	case "sleeptree":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), fakeEnv+"=sleep")
		// Inherit stdout like a real subprocess could, so a surviving
		// grandchild would keep the stream open.
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			return 1
		}
		if err := os.WriteFile(os.Getenv(fakePIDEnv), []byte(fmt.Sprint(child.Process.Pid)), 0o644); err != nil {
			return 1
		}
		emit(textDelta("ready"))
		time.Sleep(time.Minute)
	case "burst":
		// Enough deltas to fill Ask's channel while the reader is away, then
		// the last delta and the result in separate writes, so the result is
		// still in the OS pipe when this process exits.
		for i := 0; i < burstDeltas; i++ {
			emit(textDelta("x"))
		}
		time.Sleep(300 * time.Millisecond)
		emit(textDelta("x"))
		time.Sleep(100 * time.Millisecond)
		emit(resultLine(strings.Repeat("x", burstDeltas+1), "sess-burst"))
	case "drip":
		if err := os.WriteFile(os.Getenv(fakePIDEnv), []byte(fmt.Sprint(os.Getpid())), 0o644); err != nil {
			return 1
		}
		for {
			emit(textDelta("x"))
			time.Sleep(5 * time.Millisecond)
		}
	case "linger":
		// A slow shutdown after the result (hooks, MCP servers).
		emit(textDelta("answer"))
		emit(resultLine("answer", "sess-linger"))
		time.Sleep(time.Minute)
	case "stderrflood":
		chunk := strings.Repeat("e", 1<<10) + "\n"
		for i := 0; i < 4<<10; i++ {
			fmt.Fprint(os.Stderr, chunk)
		}
		emit(textDelta("ok"))
		emit(resultLine("ok", "sess-stderr"))
	case "sleep":
		time.Sleep(time.Minute)
	default:
		fmt.Fprintln(os.Stderr, "unknown fake mode", mode)
		return 2
	}
	return 0
}

func emit(v any) {
	b, _ := json.Marshal(v)
	os.Stdout.Write(append(b, '\n'))
}

func systemInit(session string) map[string]any {
	return map[string]any{"type": "system", "subtype": "init", "session_id": session, "tools": []string{"Glob", "Grep", "Read"}}
}

func streamEvent(ev map[string]any) map[string]any {
	return map[string]any{"type": "stream_event", "event": ev, "parent_tool_use_id": nil}
}

func messageStart(id string) map[string]any {
	return streamEvent(map[string]any{"type": "message_start", "message": map[string]any{"id": id, "role": "assistant", "content": []any{}}})
}

func textDelta(s string) map[string]any {
	return streamEvent(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": s}})
}

func thinkingDelta(s string) map[string]any {
	return streamEvent(map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "thinking_delta", "thinking": s}})
}

func assistantText(id, text string) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{
		"id": id, "role": "assistant", "content": []any{map[string]any{"type": "text", "text": text}},
	}}
}

func assistantTool(id, name string, input map[string]any) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{
		"id": id, "role": "assistant", "content": []any{map[string]any{"type": "tool_use", "id": "toolu_" + name, "name": name, "input": input}},
	}}
}

func resultLine(result, session string) map[string]any {
	return map[string]any{
		"type": "result", "subtype": "success", "is_error": false, "result": result,
		"session_id": session, "duration_ms": 100, "total_cost_usd": 0.01,
	}
}
