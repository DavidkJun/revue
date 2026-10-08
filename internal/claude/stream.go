package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// streamLine is the subset of a stream-json line the reviewer cares about.
// Nested payloads stay raw so lines of ignored types (tool results can be
// megabytes of file content) are never decoded field by field.
type streamLine struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	SessionID string          `json:"session_id"`
	Event     json.RawMessage `json:"event"`
	Message   json.RawMessage `json:"message"`

	IsError      bool              `json:"is_error"`
	Result       string            `json:"result"`
	TotalCostUSD float64           `json:"total_cost_usd"`
	DurationMS   float64           `json:"duration_ms"`
	Errors       []json.RawMessage `json:"errors"`
}

type apiEvent struct {
	Type    string `json:"type"`
	Message struct {
		ID string `json:"id"`
	} `json:"message"`
	ContentBlock struct {
		Type     string `json:"type"`
		Thinking string `json:"thinking"`
	} `json:"content_block"`
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		Thinking string `json:"thinking"`
	} `json:"delta"`
}

type apiMessage struct {
	ID      string `json:"id"`
	Content []struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
}

// parser turns stream-json lines into events. It is not safe for concurrent
// use.
type parser struct {
	dir       string // absolute working directory; tool paths are shown relative to it
	sessionID string
	messageID string
	wroteText bool
	separate  bool // a new message started after text; separate it from the next text
}

func newParser(dir string) *parser {
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return &parser{dir: dir}
}

// parse returns the events for one line. A final event (EventDone or
// EventError) is always the last one returned. Lines that are not JSON or
// are of no interest yield nothing.
func (p *parser) parse(line []byte) []Event {
	var l streamLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nil
	}
	if l.SessionID != "" {
		p.sessionID = l.SessionID
	}
	switch l.Type {
	case "stream_event":
		var ev apiEvent
		if json.Unmarshal(l.Event, &ev) != nil {
			return nil
		}
		return p.streamEvent(ev)
	case "assistant":
		var msg apiMessage
		if json.Unmarshal(l.Message, &msg) != nil {
			return nil
		}
		return p.assistant(msg)
	case "result":
		return []Event{p.result(l)}
	}
	return nil
}

func (p *parser) streamEvent(ev apiEvent) []Event {
	switch ev.Type {
	case "message_start":
		p.startMessage(ev.Message.ID)
	case "content_block_start":
		if t := ev.ContentBlock.Type; t == "thinking" || t == "redacted_thinking" {
			return []Event{{Kind: EventThinking, Text: ev.ContentBlock.Thinking}}
		}
	case "content_block_delta":
		switch ev.Delta.Type {
		case "text_delta":
			return p.text(ev.Delta.Text)
		case "thinking_delta":
			return []Event{{Kind: EventThinking, Text: ev.Delta.Thinking}}
		}
	}
	return nil
}

// assistant reports tool calls only: the CLI repeats every finished content
// block as an assistant message, and the text already arrived as deltas.
func (p *parser) assistant(msg apiMessage) []Event {
	p.startMessage(msg.ID)
	var out []Event
	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			out = append(out, Event{Kind: EventTool, Text: p.describeTool(block.Name, block.Input)})
		}
	}
	return out
}

func (p *parser) startMessage(id string) {
	if id == "" || id == p.messageID {
		return
	}
	p.messageID = id
	if p.wroteText {
		p.separate = true
	}
}

func (p *parser) text(s string) []Event {
	if s == "" {
		return nil
	}
	p.wroteText = true
	if p.separate {
		p.separate = false
		return []Event{{Kind: EventText, Text: "\n\n"}, {Kind: EventText, Text: s}}
	}
	return []Event{{Kind: EventText, Text: s}}
}

func (p *parser) result(l streamLine) Event {
	ev := Event{
		SessionID: p.sessionID,
		CostUSD:   l.TotalCostUSD,
		Duration:  time.Duration(l.DurationMS * float64(time.Millisecond)),
	}
	if l.IsError || l.Subtype != "success" {
		ev.Kind = EventError
		ev.Err = resultError(l)
		return ev
	}
	ev.Kind = EventDone
	ev.Result = l.Result
	return ev
}

func resultError(l streamLine) error {
	if msg := strings.TrimSpace(l.Result); msg != "" {
		return errors.New("claude: " + msg)
	}
	var msgs []string
	for _, raw := range l.Errors {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			s = string(raw)
		}
		if s = strings.TrimSpace(s); s != "" {
			msgs = append(msgs, s)
		}
	}
	if len(msgs) > 0 {
		return errors.New("claude: " + strings.Join(msgs, "; "))
	}
	if l.Subtype != "" {
		return errors.New("claude: request failed (" + l.Subtype + ")")
	}
	return errors.New("claude: request failed")
}

func (p *parser) describeTool(name string, input json.RawMessage) string {
	var in struct {
		FilePath string `json:"file_path"`
		Pattern  string `json:"pattern"`
	}
	_ = json.Unmarshal(input, &in)
	arg := ""
	switch name {
	case "Read":
		arg = p.rel(in.FilePath)
	case "Grep":
		arg = in.Pattern
	case "Glob":
		arg = p.rel(in.Pattern)
	}
	if arg == "" {
		return name
	}
	return name + " " + arg
}

// rel shows an absolute path inside the working directory relative to it,
// with forward slashes like the rest of the UI; anything else is unchanged.
func (p *parser) rel(path string) string {
	if path == "" || p.dir == "" || !filepath.IsAbs(path) {
		return path
	}
	r, err := filepath.Rel(p.dir, path)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(r)
}

// readLines calls fn for every non-empty line of r. Lines may be arbitrarily
// long (a single assistant message can carry megabytes), so it cannot use a
// bufio.Scanner with its fixed token limit. fn must not retain the slice.
func readLines(r io.Reader, fn func([]byte)) error {
	br := bufio.NewReaderSize(r, 64<<10)
	var buf []byte
	for {
		chunk, err := br.ReadSlice('\n')
		buf = append(buf, chunk...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if line := bytes.TrimSpace(buf); len(line) > 0 {
			fn(line)
		}
		buf = buf[:0]
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}
