package chat

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/DavidkJun/revue/internal/claude"
)

type turnState uint8

const (
	stateWaiting   turnState = iota // sent, nothing back yet
	stateThinking                   // thinking or running a tool
	stateAnswering                  // answer text is streaming
	stateDone
	stateError
	stateCancelled
)

func (s turnState) streaming() bool { return s <= stateAnswering }

type segKind uint8

const (
	segText segKind = iota
	segTool
)

// segment is a run of answer text or one tool call, in the order they
// arrived.
type segment struct {
	kind segKind
	text string

	// Rendered markdown of text[:renderedLen] at text width renderedWidth,
	// framed to the panel width.
	rendered      []string
	renderedWidth int
	renderedLen   int
	blocks        []renderedBlock
}

// turn is one question and its answer.
type turn struct {
	id       int
	question string
	chips    []string
	model    string
	segs     []*segment
	state    turnState
	result   string
	duration time.Duration
	cost     float64
	err      error

	// Laid out lines at linesWidth; nil when the turn changed since.
	lines      []string
	linesWidth int
}

// apply records one streamed event. It reports whether the event was the
// final one.
func (t *turn) apply(ev claude.Event) bool {
	t.lines = nil
	switch ev.Kind {
	case claude.EventText:
		t.state = stateAnswering
		if n := len(t.segs); n > 0 && t.segs[n-1].kind == segText {
			t.segs[n-1].text += ev.Text
		} else {
			t.segs = append(t.segs, &segment{kind: segText, text: ev.Text})
		}
	case claude.EventThinking:
		t.state = stateThinking
	case claude.EventTool:
		t.state = stateThinking
		t.segs = append(t.segs, &segment{kind: segTool, text: ev.Text})
	case claude.EventDone:
		t.state = stateDone
		t.result = ev.Result
		t.duration = ev.Duration
		t.cost = ev.CostUSD
		if t.answer() == "" && strings.TrimSpace(ev.Result) != "" {
			t.segs = append(t.segs, &segment{kind: segText, text: ev.Result})
		}
		return true
	case claude.EventError:
		t.fail(ev.Err)
		return true
	}
	return false
}

func (t *turn) fail(err error) {
	t.lines = nil
	if errors.Is(err, context.Canceled) {
		t.state = stateCancelled
		return
	}
	t.state = stateError
	t.err = err
}

// answer is the raw markdown of everything Claude wrote in this turn.
func (t *turn) answer() string {
	var parts []string
	for _, s := range t.segs {
		if s.kind == segText {
			if txt := strings.TrimSpace(s.text); txt != "" {
				parts = append(parts, txt)
			}
		}
	}
	return strings.Join(parts, "\n\n")
}
