// Package keys turns key presses into actions the way Vim does: counts
// ("5j"), multi-key sequences ("gg", "]c", "<space>a"), and per-pane scopes.
// The help overlay is generated from the same binding table, so it can never
// drift from what the keys actually do.
package keys

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// Scope is where a binding applies. The app passes the active scopes to
// Resolver.Feed, most specific first.
type Scope uint8

const (
	Global  Scope = iota
	Nav           // movement shared by every pane
	Sidebar       // file tree focused
	Diff          // diff pane focused (normal mode)
	Visual        // diff pane in visual mode
	Chat          // Claude panel focused
)

func (s Scope) String() string {
	return [...]string{"Global", "Movement", "Sidebar", "Diff", "Visual selection", "Claude panel"}[s]
}

// Action is what a key sequence asks for.
type Action string

const (
	FocusLeft     Action = "focus-left"
	FocusRight    Action = "focus-right"
	PrevRepo      Action = "prev-repo"
	NextRepo      Action = "next-repo"
	PrevFile      Action = "prev-file"
	NextFile      Action = "next-file"
	ToggleSplit   Action = "toggle-split"
	Ask           Action = "ask"
	AskRepo       Action = "ask-repo"
	ToggleChat    Action = "toggle-chat"
	ToggleSidebar Action = "toggle-sidebar"
	OpenEditor    Action = "open-editor"
	Refresh       Action = "refresh"
	Help          Action = "help"
	Quit          Action = "quit"      // close the topmost overlay/panel, else quit
	QuitAll       Action = "quit-all"  // quit immediately
	Interrupt     Action = "interrupt" // ctrl+c: cancel a running answer, else quit
	Escape        Action = "escape"

	Down         Action = "down"
	Up           Action = "up"
	Top          Action = "top"    // with a count: go to that line
	Bottom       Action = "bottom" // with a count: go to that line
	HalfDown     Action = "half-down"
	HalfUp       Action = "half-up"
	PageDown     Action = "page-down"
	PageUp       Action = "page-up"
	ScrollDown   Action = "scroll-down"
	ScrollUp     Action = "scroll-up"
	CenterCursor Action = "center-cursor"
	CursorTop    Action = "cursor-top"
	CursorBottom Action = "cursor-bottom"

	Expand     Action = "expand"
	Collapse   Action = "collapse"
	Mark       Action = "mark"
	ClearMarks Action = "clear-marks"

	NextChange   Action = "next-change"
	PrevChange   Action = "prev-change"
	ScrollLeft   Action = "scroll-left"
	ScrollRight  Action = "scroll-right"
	ScrollHome   Action = "scroll-home"
	FoldOpen     Action = "fold-open"
	FoldClose    Action = "fold-close"
	FoldToggle   Action = "fold-toggle"
	FoldOpenAll  Action = "fold-open-all"
	FoldCloseAll Action = "fold-close-all"
	ContextMore  Action = "context-more"
	ContextLess  Action = "context-less"
	VisualStart  Action = "visual-start"
	VisualEnd    Action = "visual-end"
	Yank         Action = "yank" // visual selection
	YankLine     Action = "yank-line"
	YankRef      Action = "yank-ref"
	Search       Action = "search"
	SearchNext   Action = "search-next"
	SearchPrev   Action = "search-prev"
	ChatFollowUp Action = "chat-follow-up"
	ChatNew      Action = "chat-new"
	ChatModel    Action = "chat-model"
	ChatYank     Action = "chat-yank"
	ChatFull     Action = "chat-full"
	ChatHide     Action = "chat-hide"
	ChatCancel   Action = "chat-cancel"
)

// Binding maps a key sequence (key names separated by spaces, e.g. "g g",
// "space a", "ctrl+shift+d") to an action within a scope.
type Binding struct {
	Keys   string
	Action Action
	Scope  Scope
	Help   string
}

// Bindings is the whole keymap. Aliases are separate entries with the same
// action, scope and help text; the help overlay merges them.
var Bindings = []Binding{
	{"ctrl+h", FocusLeft, Global, "focus pane on the left (then the WezTerm pane)"},
	{"ctrl+l", FocusRight, Global, "focus pane on the right (then the WezTerm pane)"},
	{"H", PrevRepo, Global, "previous repo"},
	{"[ r", PrevRepo, Global, "previous repo"},
	{"L", NextRepo, Global, "next repo"},
	{"] r", NextRepo, Global, "next repo"},
	{"tab", NextFile, Global, "next changed file (into the next repo at the end)"},
	{"] f", NextFile, Global, "next changed file (into the next repo at the end)"},
	{"shift+tab", PrevFile, Global, "previous changed file"},
	{"[ f", PrevFile, Global, "previous changed file"},
	{"ctrl+shift+d", ToggleSplit, Global, "toggle unified / split view"},
	{"space d", ToggleSplit, Global, "toggle unified / split view"},
	{"a", Ask, Global, "ask Claude (selection > marked files > current file)"},
	{"A", AskRepo, Global, "ask Claude about all changes of the repo"},
	{"space a", ToggleChat, Global, "show / hide the Claude panel"},
	{"space e", ToggleSidebar, Global, "show / hide the file tree"},
	{"e", OpenEditor, Global, "open the file in the editor at the cursor line"},
	{"g f", OpenEditor, Global, "open the file in the editor at the cursor line"},
	{"R", Refresh, Global, "refresh now"},
	{"?", Help, Global, "show this help"},
	{"q", Quit, Global, "close panel / quit"},
	{"Q", QuitAll, Global, "quit"},
	{"ctrl+c", Interrupt, Global, "cancel Claude's answer / quit"},
	{"esc", Escape, Global, "cancel pending keys, clear search"},

	{"j", Down, Nav, "down"},
	{"down", Down, Nav, "down"},
	{"k", Up, Nav, "up"},
	{"up", Up, Nav, "up"},
	{"g g", Top, Nav, "first line (N gg: line N)"},
	{"home", Top, Nav, "first line (N gg: line N)"},
	{"G", Bottom, Nav, "last line (N G: line N)"},
	{"end", Bottom, Nav, "last line (N G: line N)"},
	{"ctrl+d", HalfDown, Nav, "half page down"},
	{"ctrl+u", HalfUp, Nav, "half page up"},
	{"ctrl+f", PageDown, Nav, "page down"},
	{"pgdown", PageDown, Nav, "page down"},
	{"ctrl+b", PageUp, Nav, "page up"},
	{"pgup", PageUp, Nav, "page up"},
	{"ctrl+e", ScrollDown, Nav, "scroll down one line"},
	{"ctrl+y", ScrollUp, Nav, "scroll up one line"},
	{"z z", CenterCursor, Nav, "cursor line to the middle"},
	{"z t", CursorTop, Nav, "cursor line to the top"},
	{"z b", CursorBottom, Nav, "cursor line to the bottom"},

	{"l", Expand, Sidebar, "expand / open file"},
	{"enter", Expand, Sidebar, "expand / open file"},
	{"right", Expand, Sidebar, "expand / open file"},
	{"h", Collapse, Sidebar, "collapse / go to parent"},
	{"left", Collapse, Sidebar, "collapse / go to parent"},
	{"m", Mark, Sidebar, "mark file(s) as Claude context"},
	{"M", ClearMarks, Sidebar, "clear marks"},

	{"] c", NextChange, Diff, "next change"},
	{"] h", NextChange, Diff, "next change"},
	{"[ c", PrevChange, Diff, "previous change"},
	{"[ h", PrevChange, Diff, "previous change"},
	{"h", ScrollLeft, Diff, "scroll left"},
	{"left", ScrollLeft, Diff, "scroll left"},
	{"l", ScrollRight, Diff, "scroll right"},
	{"right", ScrollRight, Diff, "scroll right"},
	{"0", ScrollHome, Diff, "scroll to column 0"},
	{"z o", FoldOpen, Diff, "open fold"},
	{"enter", FoldOpen, Diff, "open fold"},
	{"z c", FoldClose, Diff, "close fold"},
	{"z a", FoldToggle, Diff, "toggle fold"},
	{"z R", FoldOpenAll, Diff, "open all folds (whole file)"},
	{"z M", FoldCloseAll, Diff, "close all folds"},
	{"+", ContextMore, Diff, "more context (+10 lines)"},
	{"=", ContextMore, Diff, "more context (+10 lines)"},
	{"-", ContextLess, Diff, "less context (-10 lines)"},
	{"v", VisualStart, Diff, "visual line selection"},
	{"V", VisualStart, Diff, "visual line selection"},
	{"y y", YankLine, Diff, "yank line"},
	{"y p", YankRef, Diff, "yank path:line"},
	{"/", Search, Diff, "search"},
	{"n", SearchNext, Diff, "next match"},
	{"N", SearchPrev, Diff, "previous match"},

	{"y", Yank, Visual, "yank selection"},
	{"v", VisualEnd, Visual, "end selection"},
	{"V", VisualEnd, Visual, "end selection"},
	{"esc", VisualEnd, Visual, "end selection"},

	{"a", ChatFollowUp, Chat, "follow-up question"},
	{"i", ChatFollowUp, Chat, "follow-up question"},
	{"n", ChatNew, Chat, "new conversation"},
	{"m", ChatModel, Chat, "cycle model (haiku / sonnet / opus)"},
	{"y", ChatYank, Chat, "yank last answer"},
	{"f", ChatFull, Chat, "toggle full-width panel"},
	{"q", ChatHide, Chat, "hide the panel"},
	{"esc", ChatCancel, Chat, "cancel the running answer"},
}

// Result is what one key press resolved to.
type Result struct {
	Action  Action // "" unless a binding completed
	Count   int    // the typed count, 0 when none was typed
	Pending bool   // the key started or continued a sequence
	Unbound bool   // the key matched nothing; any pending sequence was dropped
}

// Resolver accumulates counts and pending sequences between key presses.
type Resolver struct {
	pending []string
	count   int
	seq     int
}

// Feed resolves one key name (see Name) against the active scopes, most
// specific first; Global is always tried last. The first scope that knows the
// sequence decides it: an exact match fires, a prefix waits for more keys.
func (r *Resolver) Feed(key string, scopes ...Scope) Result {
	if len(r.pending) == 0 && isCountDigit(key, r.count) {
		r.count = r.count*10 + int(key[0]-'0')
		r.seq++
		return Result{Pending: true}
	}

	seq := strings.Join(append(append([]string{}, r.pending...), key), " ")
	for _, sc := range withGlobal(scopes) {
		exact, prefix := lookup(seq, sc)
		if exact != "" {
			count := r.count
			r.Reset()
			return Result{Action: exact, Count: count}
		}
		if prefix {
			r.pending = append(r.pending, key)
			r.seq++
			return Result{Pending: true}
		}
	}
	r.Reset()
	return Result{Unbound: true}
}

// Reset drops any pending count or sequence.
func (r *Resolver) Reset() {
	if len(r.pending) > 0 || r.count > 0 {
		r.seq++
	}
	r.pending = r.pending[:0]
	r.count = 0
}

// Seq changes every time the pending state changes; a timeout tick carrying
// an old Seq must not reset newer input.
func (r *Resolver) Seq() int { return r.seq }

// Busy reports whether a count or sequence is pending.
func (r *Resolver) Busy() bool { return len(r.pending) > 0 || r.count > 0 }

// Pending renders the pending input like Vim's showcmd, e.g. "2g".
func (r *Resolver) Pending() string {
	var b strings.Builder
	if r.count > 0 {
		b.WriteString(itoa(r.count))
	}
	for _, k := range r.pending {
		if k == "space" {
			k = "␣"
		}
		b.WriteString(k)
	}
	return b.String()
}

func isCountDigit(key string, count int) bool {
	if len(key) != 1 || key[0] < '0' || key[0] > '9' {
		return false
	}
	return key[0] != '0' || count > 0
}

func withGlobal(scopes []Scope) []Scope {
	for _, s := range scopes {
		if s == Global {
			return scopes
		}
	}
	return append(append([]Scope{}, scopes...), Global)
}

func lookup(seq string, sc Scope) (exact Action, prefix bool) {
	for _, b := range Bindings {
		if b.Scope != sc {
			continue
		}
		if b.Keys == seq {
			exact = b.Action
		} else if strings.HasPrefix(b.Keys, seq+" ") {
			prefix = true
		}
	}
	return exact, prefix
}

func itoa(n int) string {
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// Name returns the key name used in Bindings for a key press. Letters typed
// on a non-Latin layout (Ukrainian, Russian) map to the Latin key at the same
// position, so Vim navigation keeps working right after typing a question in
// Ukrainian. Text input (prompts, search) must use the key's Text instead.
func Name(k tea.KeyPressMsg) string {
	s := k.String()
	r, size := utf8.DecodeRuneInString(k.Text)
	if size == 0 || size != len(k.Text) || r < utf8.RuneSelf || !unicode.IsLetter(r) {
		return s
	}
	base := k.BaseCode
	if base == 0 || base >= utf8.RuneSelf || !unicode.IsLetter(base) {
		mapped, ok := cyrillicQwerty[unicode.ToLower(r)]
		if !ok {
			return s
		}
		base = mapped
	}
	if unicode.IsUpper(r) {
		return string(unicode.ToUpper(base))
	}
	return string(unicode.ToLower(base))
}

// cyrillicQwerty maps Ukrainian/Russian ЙЦУКЕН letters to the QWERTY key in
// the same position, for terminals that do not report the base key.
var cyrillicQwerty = map[rune]rune{
	'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p',
	'ф': 'a', 'і': 's', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l',
	'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm',
}
