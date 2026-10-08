package chat

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/claude"
)

// models is the cycle of keys.ChatModel.
var models = [...]string{"haiku", "sonnet", "opus"}

// nextModel follows models; a configured full model id ("claude-opus-…",
// "sonnet[1m]") counts as its family.
func nextModel(cur string) string {
	cur = strings.ToLower(cur)
	for i, name := range models {
		if strings.Contains(cur, name) {
			return models[(i+1)%len(models)]
		}
	}
	return models[0]
}

// send asks question as a new turn. Context items go into the prompt; a
// follow-up without new items is sent as is, since the resumed session
// already holds the earlier context. A running answer is cancelled first:
// its session id is not known yet, so the conversation continues from the
// last finished turn. Context whose answer never finished is not in that
// session, so a follow-up carries it again.
func (m *Model) send(question string, items []claude.ContextItem, chips []string) tea.Cmd {
	m.Cancel()
	if len(items) == 0 {
		items = m.unanswered
	}
	m.unanswered = items
	prompt := question
	if len(items) > 0 {
		prompt = claude.BuildPrompt(question, items)
	}
	t := m.begin(question, chips)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	req := claude.Request{
		Prompt:    prompt,
		Dir:       m.opts.Dir,
		AddDirs:   m.opts.AddDirs,
		SessionID: m.session,
		Model:     m.model,
		Effort:    m.opts.Effort,
		ExtraArgs: m.opts.ExtraArgs,
		Binary:    m.opts.Binary,
	}
	return tea.Batch(ask(ctx, t.id, req), m.startTick())
}

// begin adds a streaming turn and makes it the active one.
func (m *Model) begin(question string, chips []string) *turn {
	m.nextID++
	t := &turn{id: m.nextID, question: question, chips: chips, model: m.model}
	m.turns = append(m.turns, t)
	m.active = t
	m.follow = true
	m.dirty = true
	return t
}
