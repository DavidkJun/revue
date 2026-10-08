// Package chat is the Claude panel: a conversation about the code under
// review, with a prompt input, streamed markdown answers and tool activity.
package chat

import (
	"context"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/DavidkJun/revue/internal/claude"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

// Options configures the panel.
type Options struct {
	Theme     theme.Theme
	Dir       string   // working directory for claude (the launch folder)
	AddDirs   []string // repo roots outside Dir that Claude may read
	Model     string   // initial model ("sonnet"); cycled with keys.ChatModel
	Effort    string
	ExtraArgs []string
	Binary    string // claude executable; "" = "claude" (tests use a fake)
}

// Model is the panel. It is used through a pointer and mutated in place.
type Model struct {
	opts          Options
	th            theme.Theme
	md            *markdown
	width, height int
	model         string // model of the next question
	full          bool

	turns      []*turn
	session    string               // claude session of the conversation, for --resume
	unanswered []claude.ContextItem // context sent with no answer finished since
	nextID     int
	active     *turn // the streaming turn
	cancel     context.CancelFunc
	linger     time.Duration // how long claude may run on after its answer

	input         textarea.Model
	prompting     bool
	items         []claude.ContextItem // context of the question being typed
	chips         []string
	readClipboard func() (string, error)

	offset int  // first visible conversation line
	follow bool // keep the bottom in view as the answer grows

	frame      int // spinner frame
	ticking    bool
	nextRender time.Time // earliest re-render of the streaming answer

	// The laid out conversation; rebuilt when dirty or the width changed.
	lines      []string
	starts     []int // first line of each turn in lines
	linesWidth int
	dirty      bool
}

// New creates an empty panel.
func New(opts Options) *Model {
	return &Model{
		opts:          opts,
		th:            opts.Theme,
		md:            newMarkdown(opts.Theme),
		model:         opts.Model,
		linger:        lingerGrace,
		input:         newInput(opts.Theme),
		readClipboard: clipboard.ReadAll,
		follow:        true,
		dirty:         true,
	}
}

// SetSize sets the panel size in cells, header line included.
func (m *Model) SetSize(width, height int) {
	if width == m.width && height == m.height {
		return
	}
	m.width, m.height = max(width, 0), max(height, 0)
	// SetWidth re-fits the input height, so the cap has to be set first.
	m.input.MaxHeight = max(1, min(maxInputLines, m.height/3))
	m.input.SetWidth(max(m.width-2, 3))
	m.dirty = true
}

// StartPrompt opens the prompt input for a question about items; chips are
// short labels of the context shown above the input (e.g.
// "atlas-back-end/src/app.ts:40-52"). With no items it is a follow-up in the
// current conversation. The question is sent when the user presses enter.
func (m *Model) StartPrompt(items []claude.ContextItem, chips []string) tea.Cmd {
	m.items, m.chips = items, chips
	m.input.Placeholder = m.placeholder(len(items) > 0)
	m.prompting = true
	m.follow = true
	return m.input.Focus()
}

// Prompting reports whether the prompt input is active; the app then sends
// every key press and paste to Update instead of resolving bindings.
func (m *Model) Prompting() bool { return m.prompting }

// Busy reports whether an answer is streaming.
func (m *Model) Busy() bool { return m.active != nil }

// Update handles key presses and pastes while prompting (ctrl+v comes back
// as a tea.PasteMsg), streamed claude events and the panel's own ticks. The
// app should pass messages it does not know: events and ticks have
// unexported types.
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case eventsMsg:
		return m.handleEvents(msg)
	case tickMsg:
		return m.handleTick()
	}
	if m.prompting {
		return m.updateInput(msg)
	}
	return nil
}

// HandleAction applies a movement (keys.Nav) or panel (keys.Chat) action.
// keys.ChatYank returns a command producing uimsg.Yank.
func (m *Model) HandleAction(a keys.Action, count int) tea.Cmd {
	n := max(count, 1)
	page := max(m.bodyHeight(), 1)
	switch a {
	case keys.Down, keys.ScrollDown:
		m.Scroll(n)
	case keys.Up, keys.ScrollUp:
		m.Scroll(-n)
	case keys.HalfDown:
		m.Scroll(n * max(page/2, 1))
	case keys.HalfUp:
		m.Scroll(-n * max(page/2, 1))
	case keys.PageDown:
		m.Scroll(n * page)
	case keys.PageUp:
		m.Scroll(-n * page)
	case keys.Top:
		m.jumpTo(max(count-1, 0))
	case keys.Bottom:
		if count > 0 {
			m.jumpTo(count - 1)
		} else {
			m.follow = true
		}
	case keys.ChatFollowUp:
		return m.StartPrompt(nil, nil)
	case keys.ChatNew:
		m.Cancel()
		m.turns, m.session, m.unanswered = nil, "", nil
		m.offset, m.follow, m.dirty = 0, true, true
	case keys.ChatModel:
		m.model = nextModel(m.model)
	case keys.ChatYank:
		return m.yank()
	case keys.ChatFull:
		m.full = !m.full
	case keys.ChatCancel:
		m.Cancel()
	}
	return nil
}

// Cancel stops the running answer, if any.
func (m *Model) Cancel() {
	if m.active == nil {
		return
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.active.fail(context.Canceled)
	m.finish()
}

// View renders exactly width x height cells; the first line is the header
// (model, state).
func (m *Model) View(focused bool) string { return m.view(focused) }

// ModelName is the model the next question will use.
func (m *Model) ModelName() string { return m.model }

// Full reports whether the panel wants the full width.
func (m *Model) Full() bool { return m.full }

// Empty reports whether the conversation has no turns yet.
func (m *Model) Empty() bool { return len(m.turns) == 0 }

// Scroll moves the conversation by n lines (mouse wheel).
func (m *Model) Scroll(n int) {
	start := m.offset
	if m.follow {
		start = m.maxOffset()
	}
	m.scrollTo(start + n)
}

func (m *Model) yank() tea.Cmd {
	for i := len(m.turns) - 1; i >= 0; i-- {
		if text := m.turns[i].answer(); text != "" {
			return func() tea.Msg { return uimsg.Yank{Text: text, What: "Claude's answer"} }
		}
	}
	return func() tea.Msg { return uimsg.Notify{Text: "No answer to yank yet"} }
}
