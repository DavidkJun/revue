package chat

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/claude"
)

// tickInterval paces the spinner and the re-rendering of a streaming
// answer: markdown is rendered at most once per tick, never per token.
const tickInterval = 80 * time.Millisecond

// maxBatch bounds how many already-buffered events one message carries, so
// a fast stream costs a few updates per frame instead of one per token.
const maxBatch = 256

// eventsMsg carries events of the answer to turn. The command reading the
// next ones is issued again until a final event arrives.
type eventsMsg struct {
	turn   int
	ch     <-chan claude.Event
	events []claude.Event
	closed bool // the channel closed (after the events, if any)
}

type tickMsg struct{}

var errStreamEnded = errors.New("claude: the answer ended without a result")

// ask starts claude off the UI goroutine (spawning a process takes a
// moment) and delivers its first events.
func ask(ctx context.Context, id int, req claude.Request) tea.Cmd {
	return func() tea.Msg {
		ch, err := claude.Ask(ctx, req)
		if err != nil {
			return eventsMsg{turn: id, events: []claude.Event{{Kind: claude.EventError, Err: err}}}
		}
		return readEvents(id, ch)()
	}
}

// readEvents waits for the next event, then takes whatever else is already
// buffered.
func readEvents(id int, ch <-chan claude.Event) tea.Cmd {
	return func() tea.Msg {
		msg := eventsMsg{turn: id, ch: ch}
		ev, ok := <-ch
		if !ok {
			msg.closed = true
			return msg
		}
		msg.events = append(msg.events, ev)
		for len(msg.events) < maxBatch && !final(ev) {
			select {
			case ev, ok = <-ch:
				if !ok {
					msg.closed = true
					return msg
				}
				msg.events = append(msg.events, ev)
			default:
				return msg
			}
		}
		return msg
	}
}

func final(ev claude.Event) bool {
	return ev.Kind == claude.EventDone || ev.Kind == claude.EventError
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// handleEvents applies events of the running turn; events of cancelled or
// replaced turns are dropped (their process is already being killed).
func (m *Model) handleEvents(msg eventsMsg) tea.Cmd {
	t := m.active
	if t == nil || t.id != msg.turn {
		return nil
	}
	m.dirty = true
	for _, ev := range msg.events {
		if t.apply(ev) {
			if ev.Kind == claude.EventDone && ev.SessionID != "" {
				m.session, m.unanswered = ev.SessionID, nil
			}
			m.finish()
			return nil
		}
	}
	if msg.closed {
		t.fail(errStreamEnded)
		m.finish()
		return nil
	}
	return readEvents(t.id, msg.ch)
}

func (m *Model) handleTick() tea.Cmd {
	if m.active == nil {
		m.ticking = false
		return nil
	}
	m.frame++
	m.active.lines = nil
	m.dirty = true
	if now := time.Now(); now.After(m.nextRender) {
		for _, s := range m.active.segs {
			if s.kind == segText && s.renderedLen != len(s.text) {
				m.renderSegment(s)
			}
		}
		// A long answer can take tens of milliseconds to render; skipping
		// ticks keeps that under a third of the UI goroutine's time.
		m.nextRender = now.Add(2 * time.Since(now))
	}
	return tick()
}

// startTick starts the tick loop unless one is already running; the loop
// stops by itself on the first tick after the answer ends.
func (m *Model) startTick() tea.Cmd {
	if m.ticking {
		return nil
	}
	m.ticking = true
	return tick()
}

// lingerGrace is how long claude may run on after its final event. It is
// still shutting down then (saving the session, running SessionEnd hooks;
// about a second when measured), and cancelling kills it, which could
// break --resume. One that never exits is killed after the grace, so it
// cannot pile up with every question for the rest of the session.
const lingerGrace = time.Minute

// finish ends the running turn; its process is killed after the grace.
func (m *Model) finish() {
	if m.cancel != nil {
		time.AfterFunc(m.linger, m.cancel)
	}
	m.active, m.cancel = nil, nil
	m.dirty = true
}
