package chat

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/claude"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// TestPreview writes the panel in typical states as ANSI files into
// $REVUE_CHAT_PREVIEW, for rendering to PNG with termshot.
func TestPreview(t *testing.T) {
	dir := os.Getenv("REVUE_CHAT_PREVIEW")
	if dir == "" {
		t.Skip("set REVUE_CHAT_PREVIEW to a directory to write previews")
	}
	const w, h = 60, 40
	write := func(name string, m *Model, focused bool) {
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(m.View(focused)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	newPanel := func() *Model {
		m := New(Options{Theme: theme.Default, Model: "sonnet"})
		m.SetSize(w, h)
		return m
	}

	m := newPanel()
	write("empty", m, false)

	m = newPanel()
	m.StartPrompt([]claude.ContextItem{{Path: "src/app.ts"}}, []string{
		"atlas-back-end/src/modules/users/user.service.ts:40-52",
		"atlas-back-end/src/app.module.ts",
	})
	for _, r := range "Чому тут потрібен await?" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	for _, r := range "And is the transaction safe" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	write("prompting", m, true)

	m = newPanel()
	m.begin("why is findOne awaited here?", []string{"atlas-back-end/src/modules/users/user.service.ts:40-52"})
	feed(m, claude.Event{Kind: claude.EventThinking})
	feed(m, claude.Event{Kind: claude.EventText, Text: "Let me check the repository first."})
	feed(m, claude.Event{Kind: claude.EventTool, Text: "Read atlas-back-end/src/modules/users/user.repository.ts"})
	feed(m, claude.Event{Kind: claude.EventTool, Text: "Grep findOne"})
	feed(m, claude.Event{Kind: claude.EventText, Text: "\n\n`findOne` returns a **Promise**, so without `await` the guard at"})
	m.frame = 3
	write("streaming", m, true)

	m = newPanel()
	m.begin("why is findOne awaited here?", []string{"atlas-back-end/src/modules/users/user.service.ts:40-52"})
	feed(m, claude.Event{Kind: claude.EventTool, Text: "Read atlas-back-end/src/modules/users/user.repository.ts"})
	feed(m, claude.Event{Kind: claude.EventText, Text: previewAnswer})
	feed(m, claude.Event{Kind: claude.EventDone, SessionID: "s1", Duration: 6100 * time.Millisecond})
	m.model = "opus"
	m.begin("can it return undefined?", nil)
	feed(m, claude.Event{Kind: claude.EventText, Text: "No — TypeORM's `findOne` resolves to `null` when nothing matches, never `undefined`."})
	feed(m, claude.Event{Kind: claude.EventDone, SessionID: "s1", Duration: 3200 * time.Millisecond})
	write("answer", m, true)
	m.HandleAction(keys.Top, 0)
	write("answer-top", m, true)
	m.SetSize(38, h)
	write("answer-narrow", m, true)
	m.SetSize(w, h)
	m.StartPrompt(nil, nil)
	for _, r := range "and what if the user was soft-deleted?" {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	write("followup", m, true)
	m.SetSize(w, 14)
	write("followup-short", m, true)

	m = newPanel()
	m.begin("what does this change break?", []string{"atlas-front-end/src/app/routes.tsx:12-30"})
	feed(m, claude.Event{Kind: claude.EventThinking})
	write("thinking", m, true)

	m = newPanel()
	m.begin("explain the whole diff", []string{"atlas-front-end (all changes)"})
	feed(m, claude.Event{Kind: claude.EventError, Err: errors.New("claude: Prompt is too long")})
	m.begin("and this one?", nil)
	m.Cancel()
	write("error", m, false)
}

func feed(m *Model, ev claude.Event) {
	m.Update(eventsMsg{turn: m.active.id, events: []claude.Event{ev}})
}

const previewAnswer = "`findOne` returns a **Promise**; without `await` the guard on line 44 compares a pending promise, which is always truthy.\n\n" +
	"- the handler runs inside a transaction\n" +
	"- a rejected promise would surface as an unhandled rejection\n\n" +
	"```ts\n" +
	"async function load(id: string): Promise<User | null> {\n" +
	"  const user = await this.repo.findOne({ where: { id } });\n" +
	"  if (!user) throw new NotFoundException(`user ${id}`);\n" +
	"  return user;\n" +
	"}\n" +
	"```\n\n" +
	"See `user.repository.ts:18` for the query."
