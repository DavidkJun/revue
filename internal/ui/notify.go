package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

const noticeDuration = 3 * time.Second

// notice is the short message shown in the statusline.
type notice struct {
	text string
	err  bool
	id   int // bumped per message, so an older expiry tick cannot clear a newer one
}

type noticeExpiredMsg struct{ id int }

func (m *Model) notify(text string, isErr bool) tea.Cmd {
	m.notice = notice{text: text, err: isErr, id: m.notice.id + 1}
	id := m.notice.id
	return tea.Tick(noticeDuration, func(time.Time) tea.Msg { return noticeExpiredMsg{id} })
}

func (m *Model) yank(y uimsg.Yank) tea.Cmd {
	write := m.deps.clipboard
	return func() tea.Msg {
		if err := write(y.Text); err != nil {
			return uimsg.Notify{Text: "clipboard: " + err.Error(), Error: true}
		}
		return uimsg.Notify{Text: "yanked " + y.What}
	}
}
