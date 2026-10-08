package claude

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func parseFixture(t *testing.T, name, dir string) []Event {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	p := newParser(dir)
	var evs []Event
	if err := readLines(f, func(line []byte) { evs = append(evs, p.parse(line)...) }); err != nil {
		t.Fatal(err)
	}
	checkFinal(t, evs)
	return evs
}

func parseLines(t *testing.T, dir string, lines ...map[string]any) []Event {
	t.Helper()
	p := newParser(dir)
	var evs []Event
	for _, l := range lines {
		b, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		evs = append(evs, p.parse(b)...)
	}
	return evs
}

func TestParseHaikuStream(t *testing.T) {
	evs := parseFixture(t, "haiku_stream.jsonl", ".")
	const answer = "Refactor means to restructure code without changing what it does—improving readability, maintainability, or performance while keeping the same behavior."
	if got := joinText(evs); got != answer {
		t.Errorf("text = %q", got)
	}
	if len(kinds(evs, EventThinking)) == 0 {
		t.Error("no thinking events")
	}
	if tools := kinds(evs, EventTool); len(tools) != 0 {
		t.Errorf("unexpected tools %q", tools)
	}
	done := evs[len(evs)-1]
	if done.Kind != EventDone || done.Result != answer {
		t.Fatalf("final event = %+v", done)
	}
	if done.SessionID != "6533710b-b379-4dad-a9cd-cbe7d30494ec" {
		t.Errorf("SessionID = %q", done.SessionID)
	}
	if math.Abs(done.CostUSD-0.0201663) > 1e-9 {
		t.Errorf("CostUSD = %v", done.CostUSD)
	}
	if done.Duration != 5180*time.Millisecond {
		t.Errorf("Duration = %v", done.Duration)
	}
}

func TestParseHaikuToolStream(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the fixture was captured on Windows and holds Windows paths")
	}
	evs := parseFixture(t, "haiku_tool_stream.jsonl", `C:\Users\dev\projects\demo-api`)
	if got, want := kinds(evs, EventTool), []string{"Read package.json"}; !reflect.DeepEqual(got, want) {
		t.Errorf("tools = %q, want %q", got, want)
	}
	// The tool call sits in a message without text, so no separator.
	if got, want := joinText(evs), "The name is `demo-api`."; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	done := evs[len(evs)-1]
	if done.Kind != EventDone || done.SessionID != "823e359e-69e8-47a2-bde4-eb327ef89ec4" ||
		done.Duration != 5118*time.Millisecond || math.Abs(done.CostUSD-0.0165479) > 1e-9 {
		t.Errorf("final event = %+v", done)
	}
	// The tool use comes before the answer.
	for i, ev := range evs {
		if ev.Kind == EventText {
			t.Fatalf("text at %d before the tool event", i)
		}
		if ev.Kind == EventTool {
			break
		}
	}
}

func TestParseSeparatesMessages(t *testing.T) {
	evs := parseLines(t, ".",
		messageStart("m1"),
		textDelta("first"),
		assistantText("m1", "first"),
		assistantTool("m1", "Grep", map[string]any{"pattern": "func main"}),
		messageStart("m2"),
		messageStart("m2"),
		textDelta("second"),
		assistantText("m2", "second"),
	)
	var got []string
	for _, ev := range evs {
		got = append(got, ev.Text)
	}
	if want := []string{"first", "Grep func main", "\n\n", "second"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestParseToolDescriptions(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "elsewhere", "x.go")
	evs := parseLines(t, dir,
		assistantTool("m", "Read", map[string]any{"file_path": filepath.Join(dir, "src", "app.ts")}),
		assistantTool("m", "Read", map[string]any{"file_path": outside}),
		assistantTool("m", "Read", map[string]any{"file_path": "rel/file.go"}),
		assistantTool("m", "Grep", map[string]any{"pattern": `async\s+\w+`, "path": filepath.Join(dir, "src")}),
		assistantTool("m", "Glob", map[string]any{"pattern": "**/*.ts"}),
		assistantTool("m", "Glob", map[string]any{"pattern": filepath.Join(dir, "src", "**", "*.go")}),
		assistantTool("m", "WebFetch", map[string]any{"url": "https://example.com"}),
		assistantTool("m", "Read", nil),
	)
	want := []string{
		"Read src/app.ts",
		"Read " + outside,
		"Read rel/file.go",
		`Grep async\s+\w+`,
		"Glob **/*.ts",
		"Glob src/**/*.go",
		"WebFetch",
		"Read",
	}
	if got := kinds(evs, EventTool); !reflect.DeepEqual(got, want) {
		t.Errorf("tools\n got %q\nwant %q", got, want)
	}
}

func TestParseRelativePathIgnoresCaseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive paths are a Windows thing")
	}
	evs := parseLines(t, `C:\Repos\App`,
		assistantTool("m", "Read", map[string]any{"file_path": `c:/repos/app/src/main.go`}))
	if got := kinds(evs, EventTool); !reflect.DeepEqual(got, []string{"Read src/main.go"}) {
		t.Errorf("tools = %q", got)
	}
}

func TestParseErrorResults(t *testing.T) {
	isErr := resultLine("Credit balance is too low", "s1")
	isErr["is_error"] = true

	maxTurns := resultLine("", "s2")
	maxTurns["subtype"] = "error_max_turns"
	maxTurns["is_error"] = true
	maxTurns["errors"] = []any{"Reached maximum number of turns (3)", map[string]any{"code": 7}}

	bare := resultLine("", "")
	bare["subtype"] = "error_during_execution"

	tests := []struct {
		name string
		line map[string]any
		want string
	}{
		{"is_error", isErr, "Credit balance is too low"},
		{"errors list", maxTurns, `Reached maximum number of turns (3); {"code":7}`},
		{"subtype only", bare, "error_during_execution"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evs := parseLines(t, ".", systemInit("init-session"), tt.line)
			if len(evs) != 1 || evs[0].Kind != EventError || evs[0].Err == nil {
				t.Fatalf("events = %+v", evs)
			}
			if !strings.Contains(evs[0].Err.Error(), tt.want) {
				t.Errorf("error %q does not contain %q", evs[0].Err, tt.want)
			}
			if evs[0].SessionID == "" {
				t.Error("SessionID not set")
			}
		})
	}
}

func TestParseIgnoresNoise(t *testing.T) {
	p := newParser(".")
	for _, line := range []string{
		`not json at all`,
		`{"type":"system","subtype":"status","status":"requesting"}`,
		`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed"}}`,
		`{"type":"user","message":{"role":"user","content":"plain string content"}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{"}}}`,
		`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"signature_delta","signature":"abc"}}}`,
		`{"type":"stream_event","event":{"type":"message_delta","delta":{"stop_reason":"end_turn"}}}`,
		`{"type":"brand_new_type","data":[1,2,3]}`,
		`{"type":"stream_event","event":"unexpected"}`,
	} {
		if evs := p.parse([]byte(line)); len(evs) != 0 {
			t.Errorf("%s -> %+v", line, evs)
		}
	}
}

func TestReadLines(t *testing.T) {
	huge := strings.Repeat("z", 9<<20)
	input := "one\r\n\n  \n" + huge + "\nlast-without-newline"
	var got []string
	if err := readLines(strings.NewReader(input), func(b []byte) { got = append(got, string(b)) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0] != "one" || got[1] != huge || got[2] != "last-without-newline" {
		t.Errorf("got %d lines (lengths %v)", len(got), lineLens(got))
	}
}

func lineLens(lines []string) []int {
	out := make([]int, len(lines))
	for i, l := range lines {
		out[i] = len(l)
	}
	return out
}

func TestTailBuffer(t *testing.T) {
	tb := &tailBuffer{max: 8}
	tb.Write([]byte("hello "))
	tb.Write([]byte("wor"))
	tb.Write([]byte("ld"))
	if got := tb.String(); got != "lo world" {
		t.Errorf("tail = %q", got)
	}
	tb.Write([]byte(strings.Repeat("-", 20) + "end"))
	if got := tb.String(); got != "-----end" {
		t.Errorf("tail = %q", got)
	}
	// "ї" is two bytes; a tail that starts inside it must not show half a rune.
	tb = &tailBuffer{max: 5}
	tb.Write([]byte("її abc"))
	if got := tb.String(); got != "abc" {
		t.Errorf("tail = %q", got)
	}
}

func TestParseOneSeparatorAcrossToolOnlyMessages(t *testing.T) {
	evs := parseLines(t, ".",
		messageStart("m1"),
		textDelta("Let me check."),
		assistantText("m1", "Let me check."),
		assistantTool("m1", "Read", map[string]any{"file_path": "a.go"}),
		messageStart("m2"),
		assistantTool("m2", "Grep", map[string]any{"pattern": "x"}),
		messageStart("m3"),
		thinkingDelta("hmm"),
		textDelta("Found "),
		textDelta("it."),
		assistantText("m3", "Found it."),
		resultLine("Found it.", "s"),
	)
	var got []string
	for _, ev := range evs {
		got = append(got, fmt.Sprintf("%d:%s", ev.Kind, ev.Text))
	}
	want := []string{"0:Let me check.", "2:Read a.go", "2:Grep x", "1:hmm", "0:\n\n", "0:Found ", "0:it.", "3:"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("events\n got %q\nwant %q", got, want)
	}
}
