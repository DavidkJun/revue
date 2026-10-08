package ui

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/DavidkJun/revue/internal/config"
	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/sidebar"
	"github.com/DavidkJun/revue/internal/ui/uimsg"
)

func TestFrameSizeAtEverySize(t *testing.T) {
	sizes := [][2]int{
		{170, 45}, {100, 30}, {80, 24}, {60, 20}, {45, 12}, {30, 8}, {24, 6},
		{20, 5}, {10, 3}, {8, 4}, {5, 2}, {3, 3}, {2, 1}, {1, 1}, {1, 0}, {0, 1}, {0, 0},
	}
	states := map[string]func(h *harness){
		"default": func(*harness) {},
		"chat":    func(h *harness) { h.keys("space", "a") },
		"help":    func(h *harness) { h.keys("?") },
		"search":  func(h *harness) { h.keys("ctrl+l", "/", "A", "u", "t", "h") },
		"nosidebar": func(h *harness) {
			h.keys("space", "e")
		},
	}
	for name, setup := range states {
		h := newHarness(t, 120, 40)
		setup(h)
		for _, s := range sizes {
			h.send(tea.WindowSizeMsg{Width: s[0], Height: s[1]})
			checkFrame(t, h.m.View().Content, s[0], s[1])
		}
		if t.Failed() {
			t.Fatalf("state %q", name)
		}
	}
}

func TestStartsOnFirstChangedFile(t *testing.T) {
	h := newHarness(t, 120, 40)
	if h.m.focused() != paneSidebar {
		t.Errorf("focus = %v, want sidebar", h.m.focused())
	}
	if r, p := h.current(); r != 0 || p == "" {
		t.Fatalf("cursor on repo %d file %q, want a file of repo 0", r, p)
	}
	files := h.m.sidebar.Files()
	if want := key(files[0].Repo, files[0].Path); h.m.shown != want {
		t.Errorf("shown = %+v, want %+v", h.m.shown, want)
	}
	if !strings.Contains(h.plain(), "auth.service.ts") {
		t.Errorf("frame does not show the first file:\n%s", h.plain())
	}
}

func TestFocusSurvivesStartupAndNarrowTerminals(t *testing.T) {
	// Bubble Tea delivers status results before the first WindowSizeMsg
	// when git is fast; the empty layout must not steal the focus.
	fg := sampleGit()
	m := newModel(`C:\work\atlas`, []git.Repo{backEnd, frontEnd}, config.Default(), theme.Default, fg.deps())
	h := &harness{t: t, m: m, git: fg, quiet: 80 * time.Millisecond}
	h.run(m.Init())
	h.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	if h.m.focused() != paneSidebar {
		t.Fatalf("focus = %v, want sidebar", h.m.focused())
	}

	// Too narrow for the tree: keys go to the diff meanwhile.
	h.send(tea.WindowSizeMsg{Width: 40, Height: 20})
	if h.m.focused() != paneDiff {
		t.Fatalf("narrow: focus = %v, want diff", h.m.focused())
	}
	h.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	if h.m.focused() != paneSidebar {
		t.Errorf("widened again: focus = %v, want sidebar", h.m.focused())
	}
}

func TestStartsOnRepoNodeWhenEverythingIsClean(t *testing.T) {
	h := newHarness(t, 120, 40, docsRepo)
	if r, p := h.current(); r != 0 || p != "" {
		t.Errorf("cursor on repo %d file %q, want the repo node", r, p)
	}
	if !strings.Contains(h.plain(), "no uncommitted changes") {
		t.Errorf("no clean message:\n%s", h.plain())
	}
}

func TestFocusMovesBetweenPanesThenToWezTerm(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("ctrl+l")
	if h.m.focused() != paneDiff {
		t.Fatalf("focus = %v after ctrl+l, want diff", h.m.focused())
	}
	h.keys("ctrl+l")
	if h.m.focused() != paneDiff || !slices.Equal(h.git.navigations(), []string{"Right"}) {
		t.Fatalf("at the right edge: focus %v, navigations %v", h.m.focused(), h.git.navigations())
	}
	h.keys("space", "a")
	if h.m.focused() != paneChat {
		t.Fatalf("focus = %v after <space>a, want chat", h.m.focused())
	}
	h.keys("ctrl+h", "ctrl+h")
	if h.m.focused() != paneSidebar {
		t.Fatalf("focus = %v after ctrl+h twice, want sidebar", h.m.focused())
	}
	h.keys("ctrl+h")
	if got := h.git.navigations(); !slices.Equal(got, []string{"Right", "Left"}) {
		t.Errorf("navigations = %v", got)
	}
}

func TestScopeDecidesWhatAKeyDoes(t *testing.T) {
	h := newHarness(t, 140, 40)
	shown := h.m.shown

	// "l" on a file in the tree opens it: focus moves to the diff.
	h.keys("l")
	if h.m.focused() != paneDiff {
		t.Fatalf("l in the sidebar: focus = %v, want diff", h.m.focused())
	}
	// In the diff "l" scrolls sideways and "j" moves the cursor; neither
	// changes the file.
	before, _ := h.m.diff.Cursor()
	h.keys("l", "j", "j")
	if h.m.focused() != paneDiff || h.m.shown != shown {
		t.Fatalf("l/j in the diff changed focus %v or file %+v", h.m.focused(), h.m.shown)
	}
	if after, _ := h.m.diff.Cursor(); after == before {
		t.Errorf("j in the diff did not move the cursor (%+v)", after)
	}
	// Back in the sidebar "j" moves to the next file.
	h.keys("ctrl+h", "j")
	if h.m.shown == shown {
		t.Errorf("j in the sidebar kept showing %+v", shown)
	}
}

func TestPendingKeysShowAndTimeOut(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("1", "2", "g")
	if !h.m.resolver.Busy() || !strings.Contains(lastLine(h.plain()), "12g") {
		t.Fatalf("pending 12g not shown: %q", lastLine(h.plain()))
	}
	stale := h.m.resolver.Seq() - 1
	h.send(pendingTimeoutMsg{seq: stale})
	if !h.m.resolver.Busy() {
		t.Fatal("a stale timeout reset the pending keys")
	}
	h.send(pendingTimeoutMsg{seq: h.m.resolver.Seq()})
	if h.m.resolver.Busy() {
		t.Fatal("the timeout did not reset the pending keys")
	}
	if strings.Contains(lastLine(h.plain()), "12g") {
		t.Errorf("statusline still shows the pending key: %q", lastLine(h.plain()))
	}

	// A completed sequence works as usual: gg goes to the top.
	h.keys("j", "j", "g", "g")
	if r, p := h.current(); r != 0 || p != "" {
		t.Errorf("gg: cursor on repo %d file %q, want the first row", r, p)
	}
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

func TestTabWalksFilesAcrossRepos(t *testing.T) {
	h := newHarness(t, 140, 40)
	files := h.m.sidebar.Files()
	if len(files) != 5 {
		t.Fatalf("sidebar lists %d files, want 5: %v", len(files), files)
	}
	var visited []string
	for range files {
		h.keys("tab")
		r, p := h.current()
		visited = append(visited, h.m.repos[r].Repo.Name+"/"+p)
	}
	var want []string
	for i := range files {
		f := files[(i+1)%len(files)]
		want = append(want, h.m.repos[f.Repo].Repo.Name+"/"+f.Path)
	}
	if !slices.Equal(visited, want) {
		t.Errorf("tab visited\n%v\nwant\n%v", visited, want)
	}
	h.keys("shift+tab")
	if r, p := h.current(); r != files[len(files)-1].Repo || p != files[len(files)-1].Path {
		t.Errorf("shift+tab from the first file: repo %d %q, want the last file", r, p)
	}
	h.keys("]", "f")
	if r, p := h.current(); r != files[0].Repo || p != files[0].Path {
		t.Errorf("]f: repo %d %q, want the first file", r, p)
	}
	if h.m.shown != key(files[0].Repo, files[0].Path) {
		t.Errorf("diff shows %+v", h.m.shown)
	}

	// From a directory between files, tab continues after the shown file.
	h.keys("k")
	if r, p := h.current(); r != 0 || p != "" {
		t.Fatalf("k: cursor on repo %d %q, want the directory above", r, p)
	}
	h.keys("tab")
	if r, p := h.current(); r != files[1].Repo || p != files[1].Path {
		t.Errorf("tab from a directory: repo %d %q, want %s", r, p, files[1].Path)
	}

	// From the repo node, tab starts at the repo's first file and shift+tab
	// goes to the file before the repo, whichever file is shown.
	h.m.sidebar.Select(0, "")
	h.keys("tab")
	if r, p := h.current(); r != files[0].Repo || p != files[0].Path {
		t.Errorf("tab from the repo node: repo %d %q, want %s", r, p, files[0].Path)
	}
	h.keys("tab")
	h.m.sidebar.Select(0, "")
	h.keys("shift+tab")
	if last := files[len(files)-1]; h.m.shown.repo != last.Repo || h.m.shown.path != last.Path {
		t.Errorf("shift+tab from the repo node: shows %+v, want %+v", h.m.shown, last)
	}
}

func TestRepoJumps(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("L")
	if r, p := h.current(); r != 1 || p == "" {
		t.Fatalf("L: repo %d %q, want a file of repo 1", r, p)
	}
	h.keys("L")
	if r, p := h.current(); r != 2 || p != "" {
		t.Fatalf("L to the clean repo: repo %d %q, want its node", r, p)
	}
	if !strings.Contains(h.plain(), "no uncommitted changes") {
		t.Errorf("clean repo message missing:\n%s", h.plain())
	}
	h.keys("]", "r")
	if r, _ := h.current(); r != 0 {
		t.Errorf("]r wraps to repo 0, got %d", r)
	}
	h.keys("H")
	if r, _ := h.current(); r != 2 {
		t.Errorf("H wraps to repo 2, got %d", r)
	}
	// Clicking a tab selects its repo.
	tabs := h.m.tabs()
	h.send(tea.MouseClickMsg{X: tabs[1].x + 2, Y: 0, Button: tea.MouseLeft})
	if r, _ := h.current(); r != 1 {
		t.Errorf("clicking tab 1 selected repo %d", r)
	}
}

func TestHelpOverlay(t *testing.T) {
	h := newHarness(t, 170, 45)
	cursor := h.m.shown
	h.keys("?")
	if !h.m.help.open || !strings.Contains(h.plain(), "Keys") || !strings.Contains(h.plain(), "toggle unified / split view") {
		t.Fatalf("help not shown:\n%s", h.plain())
	}
	if !strings.Contains(h.plain(), "j / down") {
		t.Errorf("aliases are not merged:\n%s", h.plain())
	}
	h.keys("j", "j")
	if h.m.shown != cursor {
		t.Error("j moved the tree while help was open")
	}
	h.keys("q")
	if h.m.help.open || h.quit {
		t.Fatalf("q: help open %v, quit %v", h.m.help.open, h.quit)
	}
	h.keys("?", "esc")
	if h.m.help.open {
		t.Fatal("esc did not close help")
	}
	h.keys("?", "?")
	if h.m.help.open {
		t.Fatal("? did not close help")
	}

	// Short terminals scroll the help.
	h.send(tea.WindowSizeMsg{Width: 80, Height: 20})
	h.keys("?")
	first := h.plain()
	h.keys("j", "j", "j")
	if h.m.help.scroll != 3 || h.plain() == first {
		t.Errorf("help did not scroll: scroll %d", h.m.help.scroll)
	}
	h.keys("G", "j")
	if max := h.m.helpBox(h.m.layout().bodyH).maxScroll; h.m.help.scroll != max {
		t.Errorf("G: scroll %d, want %d", h.m.help.scroll, max)
	}
}

func TestHelpGroupsCoverEveryScope(t *testing.T) {
	groups := helpGroups()
	var titles []string
	for _, g := range groups {
		titles = append(titles, g.title)
	}
	want := []string{"Global", "Movement", "Sidebar", "Diff", "Visual selection", "Claude panel"}
	if !slices.Equal(titles, want) {
		t.Fatalf("groups %v, want %v", titles, want)
	}
	found := false
	for _, e := range groups[0].entries {
		if e.help == "toggle unified / split view" {
			found = e.keys == "ctrl+shift+d / <space>d"
		}
	}
	if !found {
		t.Errorf("split toggle entry not merged as expected: %+v", groups[0].entries)
	}
}

func TestAskAboutCurrentFileLine(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("ctrl+l", "j", "j")
	loc, ok := h.m.diff.Cursor()
	if !ok {
		t.Fatal("no cursor in the diff")
	}
	msg := askResult(t, h, false)
	if len(msg.items) != 1 {
		t.Fatalf("items = %+v", msg.items)
	}
	it := msg.items[0]
	if it.Repo != "atlas-back-end" || it.RepoPath != backEnd.Path || it.Path != "src/auth/auth.service.ts" ||
		it.StartLine != loc.NewLine || it.EndLine != loc.NewLine || it.Selection != "" {
		t.Errorf("item = %+v", it)
	}
	if !strings.Contains(it.Patch, "src/auth/auth.service.ts") {
		t.Errorf("patch = %q", it.Patch)
	}
	if want := "atlas-back-end/src/auth/auth.service.ts:" + itoa(loc.NewLine); !slices.Equal(msg.chips, []string{want}) {
		t.Errorf("chips = %v, want %v", msg.chips, want)
	}

	h.keys("a")
	if !h.m.showChat || h.m.focused() != paneChat || !h.m.chat.Prompting() {
		t.Fatalf("after a: chat shown %v, focus %v, prompting %v", h.m.showChat, h.m.focused(), h.m.chat.Prompting())
	}
	if !strings.Contains(lastLine(h.plain()), "PROMPT") {
		t.Errorf("statusline: %q", lastLine(h.plain()))
	}
	// Keys now type into the prompt instead of running bindings.
	h.keys("q")
	if h.quit || !h.m.showChat {
		t.Error("q while prompting was treated as a binding")
	}
}

func TestAskAboutVisualSelection(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("ctrl+l", "j", "j", "V", "j", "j")
	sel, ok := h.m.diff.Selection()
	if !ok || !h.m.diff.Visual() {
		t.Fatal("no visual selection")
	}
	if !strings.Contains(lastLine(h.plain()), "VISUAL") {
		t.Errorf("statusline: %q", lastLine(h.plain()))
	}
	msg := askResult(t, h, false)
	if h.m.diff.Visual() {
		t.Error("visual mode was kept after building the context")
	}
	if len(msg.items) != 1 {
		t.Fatalf("items = %+v", msg.items)
	}
	it := msg.items[0]
	if it.Selection != sel.Text || it.StartLine != sel.StartLine || it.EndLine != sel.EndLine || it.Patch == "" {
		t.Errorf("item = %+v, selection %+v", it, sel)
	}
	if want := "atlas-back-end/src/auth/auth.service.ts" + lineRange(sel.StartLine, sel.EndLine); msg.chips[0] != want {
		t.Errorf("chip = %q, want %q", msg.chips[0], want)
	}
}

func TestAskAboutMarkedFiles(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("m", "tab", "tab", "tab", "m")
	marked := h.m.sidebar.Marked()
	if len(marked) != 2 {
		t.Fatalf("marked %v", marked)
	}
	if !strings.Contains(lastLine(h.plain()), "● 2") {
		t.Errorf("statusline does not count marks: %q", lastLine(h.plain()))
	}
	msg := askResult(t, h, false)
	if len(msg.items) != 2 {
		t.Fatalf("items = %+v", msg.items)
	}
	for i, it := range msg.items {
		if it.Path != marked[i].Path || it.StartLine != 0 || it.Patch == "" {
			t.Errorf("item %d = %+v, want whole file %s", i, it, marked[i].Path)
		}
	}
	if msg.items[0].Repo == msg.items[1].Repo {
		t.Errorf("marks from two repos became %s and %s", msg.items[0].Repo, msg.items[1].Repo)
	}
}

func TestAskAboutRepo(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.m.sidebar.Select(1, "")
	msg := askResult(t, h, false)
	if len(msg.items) != 1 || msg.items[0].Path != "" || msg.items[0].Repo != "atlas-front-end" {
		t.Fatalf("repo node: items = %+v", msg.items)
	}
	if got := h.git.patches[frontEnd.Path]; len(got) != 2 {
		t.Errorf("repo patch covered %d files", len(got))
	}
	if msg.chips[0] != "atlas-front-end (2 files)" {
		t.Errorf("chip = %q", msg.chips[0])
	}

	// A asks about the whole repo even from a file.
	h.m.sidebar.Select(0, "src/app.module.ts")
	msg = askResult(t, h, true)
	if len(msg.items) != 1 || msg.items[0].Repo != "atlas-back-end" || msg.items[0].Path != "" {
		t.Fatalf("A: items = %+v", msg.items)
	}
	if got := h.git.patches[backEnd.Path]; len(got) != 3 {
		t.Errorf("repo patch covered %d files", len(got))
	}
}

// askResult builds the ask context the way the a/A keys do and returns it.
func TestAskAboutDirectory(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("g", "g", "j", "j") // repo, src, src/auth
	if _, dir, ok := h.m.sidebar.CurrentDir(); !ok || dir != "src/auth" {
		t.Fatalf("cursor on %q %v, want the src/auth directory", dir, ok)
	}
	msg := askResult(t, h, false)
	if len(msg.items) != 1 || msg.items[0].Repo != "atlas-back-end" || msg.items[0].Path != "src/auth/" || msg.items[0].Patch == "" {
		t.Fatalf("items = %+v", msg.items)
	}
	var got []string
	for _, f := range h.git.patches[backEnd.Path] {
		got = append(got, f.Path)
	}
	if want := []string{"src/auth/auth.service.ts", "src/auth/refresh-token.guard.ts"}; !slices.Equal(got, want) {
		t.Errorf("patch covered %v, want %v", got, want)
	}
	if want := "atlas-back-end/src/auth/ (2 files)"; !slices.Equal(msg.chips, []string{want}) {
		t.Errorf("chips = %v, want %v", msg.chips, want)
	}
}

func askResult(t *testing.T, h *harness, repoWide bool) askMsg {
	t.Helper()
	msg, ok := h.m.ask(repoWide)().(askMsg)
	if !ok {
		t.Fatal("ask did not produce an askMsg")
	}
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	return msg
}

func itoa(n int) string { return lineRange(n, n)[1:] }

func TestQuitSemantics(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("space", "a")
	h.keys("q") // the chat is focused: q hides it
	if h.quit || h.m.showChat {
		t.Fatalf("q in chat: quit %v, chat shown %v", h.quit, h.m.showChat)
	}
	h.keys("space", "a", "ctrl+h", "q") // from the diff: q hides the panel first
	if h.quit || h.m.showChat {
		t.Fatalf("q in diff with chat: quit %v, chat shown %v", h.quit, h.m.showChat)
	}
	h.raw = nil
	h.keys("q")
	if !h.quit {
		t.Fatal("q without panels did not quit")
	}
	if len(h.raw) == 0 || !strings.Contains(h.raw[0], win32InputOff) {
		t.Errorf("win32-input-mode not turned off before quitting: %q", h.raw)
	}

	for _, k := range []string{"Q", "ctrl+c"} {
		h := newHarness(t, 140, 40)
		h.keys("space", "a", "ctrl+h")
		h.keys(k)
		if !h.quit {
			t.Errorf("%s did not quit", k)
		}
	}
}

func TestInitTurnsOnWin32InputMode(t *testing.T) {
	h := newHarness(t, 80, 24)
	if len(h.raw) == 0 || !strings.Contains(h.raw[0], win32InputOn) {
		t.Errorf("raw output at start: %q", h.raw)
	}
	if v := h.m.View(); !v.AltScreen || v.MouseMode != tea.MouseModeCellMotion || !v.ReportFocus || v.WindowTitle != "revue · atlas" {
		t.Errorf("view settings: alt %v mouse %v focus %v title %q", v.AltScreen, v.MouseMode, v.ReportFocus, v.WindowTitle)
	}
}

func TestPollingAppliesOnlyChangedStatus(t *testing.T) {
	h := newHarness(t, 140, 40)
	loads := len(h.git.loads)
	h.send(pollTickMsg{})
	if len(h.git.loads) != loads {
		t.Fatalf("an unchanged status reloaded the diff: %v", h.git.loads[loads:])
	}

	// The shown file changed on disk: same file, reloaded in place.
	st := h.git.statuses[backEnd.Path]
	st.Fingerprint = "be-2"
	st.Files = slices.Clone(st.Files)
	st.Files[1].Added = 10
	h.git.statuses[backEnd.Path] = st
	h.send(pollTickMsg{})
	if got := h.git.loads[loads:]; !slices.Equal(got, []string{backEnd.Path + "|src/auth/auth.service.ts"}) {
		t.Fatalf("reloads = %v", got)
	}

	// The shown file was committed: the cursor moves to its neighbour.
	st.Fingerprint = "be-3"
	st.Files = slices.Delete(slices.Clone(st.Files), 1, 2)
	h.git.statuses[backEnd.Path] = st
	h.send(pollTickMsg{})
	if r, p := h.current(); r != 0 || p != "src/auth/refresh-token.guard.ts" {
		t.Errorf("after commit: cursor repo %d %q", r, p)
	}
	if h.m.shown != key(0, "src/auth/refresh-token.guard.ts") {
		t.Errorf("after commit: shown %+v", h.m.shown)
	}
	if !strings.Contains(lastLine(h.plain()), "no longer changed") {
		t.Errorf("no notice: %q", lastLine(h.plain()))
	}
}

func TestPollSkipsReposStillLoading(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.m.polling[1] = true
	if cmd := h.m.loadStatus(1); cmd != nil {
		t.Error("a second status load started while one was running")
	}
	h.send(statusMsg{repo: 1, status: h.git.statuses[frontEnd.Path]})
	if h.m.polling[1] {
		t.Error("the result did not clear the in-flight flag")
	}
}

func TestLatestDiffRequestWins(t *testing.T) {
	h := newHarness(t, 140, 40)
	slow := key(0, "src/auth/refresh-token.guard.ts")
	fast := key(0, "src/app.module.ts")
	slowCmd := h.m.selectFile(slow.repo, slow.path)
	fastCmd := h.m.selectFile(fast.repo, fast.path)
	if h.m.shown == slow || h.m.shown == fast {
		t.Fatal("the shown file changed before its diff arrived")
	}
	h.send(fastCmd())
	h.send(slowCmd())
	if h.m.shown != fast {
		t.Errorf("shown %+v, want the latest request %+v", h.m.shown, fast)
	}
}

func TestSearchInput(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("ctrl+l", "/", "M", "o", "d")
	if !h.m.search.active || !strings.Contains(lastLine(h.plain()), "/Mod") || !strings.Contains(lastLine(h.plain()), "SEARCH") {
		t.Fatalf("search input: %q", lastLine(h.plain()))
	}
	h.keys("backspace", "enter")
	if h.m.search.active || h.m.search.prev != "Mo" {
		t.Fatalf("enter: active %v pattern %q", h.m.search.active, h.m.search.prev)
	}
	h.keys("/", "x", "y", "z", "esc")
	if h.m.search.active || h.m.search.prev != "Mo" {
		t.Errorf("esc did not restore the pattern: %q", h.m.search.prev)
	}
	h.keys("esc")
	if h.m.search.prev != "" {
		t.Errorf("esc in normal mode kept the pattern %q", h.m.search.prev)
	}
}

func TestIncrementalSearchStartsFromWhereTheSearchBegan(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("ctrl+l", "g", "g", "/", "r", "e", "v")
	if line, _ := h.m.diff.Position(); line != 29 {
		t.Fatalf("/rev: cursor on line %d, want 29 (revokeAll)", line)
	}
	// Vim's incsearch: "re" matches from line 1 again, not from line 29.
	h.keys("backspace")
	if line, _ := h.m.diff.Position(); line != 4 {
		t.Errorf("/re after backspace: cursor on line %d, want 4 (RefreshTokenStore)", line)
	}
	h.keys("enter")
	if line, _ := h.m.diff.Position(); line != 4 {
		t.Errorf("enter moved the cursor to line %d", line)
	}
}

func TestSearchNotFoundIsReported(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.keys("ctrl+l", "/", "q", "q", "q", "enter")
	if !h.m.notice.err || !strings.Contains(lastLine(h.plain()), "pattern not found: qqq") {
		t.Errorf("statusline: %q", lastLine(h.plain()))
	}
}

func TestToggles(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.send(keyPress("ctrl+shift+d"))
	if !h.m.diff.Split() || !strings.Contains(lastLine(h.plain()), "SPLIT") {
		t.Fatalf("ctrl+shift+d: split %v, statusline %q", h.m.diff.Split(), lastLine(h.plain()))
	}
	h.keys("space", "d")
	if h.m.diff.Split() {
		t.Fatal("<space>d did not switch back to unified")
	}
	h.keys("space", "e")
	if h.m.layout().has(paneSidebar) || h.m.focused() != paneDiff {
		t.Fatalf("<space>e: sidebar visible %v, focus %v", h.m.layout().has(paneSidebar), h.m.focused())
	}
	h.keys("space", "e")
	if !h.m.layout().has(paneSidebar) {
		t.Fatal("<space>e did not bring the sidebar back")
	}
}

func TestYankAndNotify(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.send(uimsg.Yank{Text: "const a = 1;", What: "1 line"})
	if !slices.Equal(h.git.clip, []string{"const a = 1;"}) {
		t.Errorf("clipboard got %v", h.git.clip)
	}
	if !strings.Contains(lastLine(h.plain()), "yanked 1 line") {
		t.Errorf("statusline: %q", lastLine(h.plain()))
	}
	h.send(noticeExpiredMsg{id: h.m.notice.id - 1})
	if h.m.notice.text == "" {
		t.Error("a stale expiry cleared the notice")
	}
	h.send(noticeExpiredMsg{id: h.m.notice.id})
	if h.m.notice.text != "" {
		t.Error("the notice did not expire")
	}
}

func TestMouseFocusAndScroll(t *testing.T) {
	h := newHarness(t, 140, 40)
	l := h.m.layout()
	d, _ := l.find(paneDiff)
	h.send(tea.MouseClickMsg{X: d.x + 5, Y: 10, Button: tea.MouseLeft})
	if h.m.focused() != paneDiff {
		t.Errorf("click on the diff: focus %v", h.m.focused())
	}
	s, _ := l.find(paneSidebar)
	h.send(tea.MouseClickMsg{X: s.x + 2, Y: 5, Button: tea.MouseLeft})
	if h.m.focused() != paneSidebar {
		t.Errorf("click on the sidebar: focus %v", h.m.focused())
	}
	h.send(tea.MouseWheelMsg{X: d.x + 5, Y: 10, Button: tea.MouseWheelDown})
	h.frame()
}

func TestOpenEditorRefusesDeletedFiles(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.m.sidebar.Select(1, "src/legacy/session.ts")
	h.send(uimsg.FileSelected{Repo: 1, Path: "src/legacy/session.ts"})
	h.keys("e")
	if !h.m.notice.err || !strings.Contains(h.m.notice.text, "deleted") {
		t.Errorf("notice = %+v", h.m.notice)
	}
}

func TestLayoutDropsPanesWhenNarrow(t *testing.T) {
	cases := []struct {
		w          int
		sidebar    bool
		chat       bool
		wantPanes  []pane
		wantDiffAt int
	}{
		{170, true, true, []pane{paneSidebar, paneDiff, paneChat}, 45},
		{100, true, false, []pane{paneSidebar, paneDiff}, 29},
		{70, true, true, []pane{paneSidebar, paneDiff}, 25},
		{40, true, false, []pane{paneDiff}, 0},
		{1, true, true, []pane{paneDiff}, 0},
	}
	for _, c := range cases {
		l := computeLayout(c.w, 30, c.sidebar, c.chat, false, false)
		var got []pane
		total := len(l.boxes) - 1
		for _, b := range l.boxes {
			got = append(got, b.pane)
			total += b.w
		}
		if !slices.Equal(got, c.wantPanes) || total != c.w {
			t.Errorf("width %d: panes %v (total %d), want %v", c.w, got, total, c.wantPanes)
		}
		if d, _ := l.find(paneDiff); d.x != c.wantDiffAt {
			t.Errorf("width %d: diff at %d, want %d", c.w, d.x, c.wantDiffAt)
		}
	}
	if l := computeLayout(100, 30, true, true, true, false); len(l.boxes) != 1 || l.boxes[0].pane != paneChat || l.boxes[0].w != 100 {
		t.Errorf("full chat layout %+v", l.boxes)
	}
}

func TestNearestFile(t *testing.T) {
	ref := func(r int, p string) sidebar.FileRef { return sidebar.FileRef{Repo: r, Path: p} }
	before := []sidebar.FileRef{ref(0, "a"), ref(0, "b"), ref(0, "c"), ref(1, "x")}
	cases := []struct {
		after []sidebar.FileRef
		gone  string
		want  string
	}{
		{[]sidebar.FileRef{ref(0, "a"), ref(0, "c"), ref(1, "x")}, "b", "c"},
		{[]sidebar.FileRef{ref(0, "a"), ref(0, "b"), ref(1, "x")}, "c", "b"},
		{[]sidebar.FileRef{ref(0, "z"), ref(1, "x")}, "a", "z"},
		{[]sidebar.FileRef{ref(1, "x")}, "a", ""},
	}
	for _, c := range cases {
		got, ok := nearestFile(before, c.after, key(0, c.gone))
		if c.want == "" {
			if ok {
				t.Errorf("gone %s: got %+v, want none", c.gone, got)
			}
			continue
		}
		if !ok || got.Path != c.want || got.Repo != 0 {
			t.Errorf("gone %s: got %+v, want %s", c.gone, got, c.want)
		}
	}
}

func TestStatusErrorKeepsLastGoodStatus(t *testing.T) {
	h := newHarness(t, 140, 40)
	h.send(statusMsg{repo: 0, err: errTest("fatal: index.lock exists")})
	if len(h.m.repos[0].Status.Files) != 3 || h.m.repos[0].Err == nil {
		t.Errorf("repo 0 after an error: %d files, err %v", len(h.m.repos[0].Status.Files), h.m.repos[0].Err)
	}
	if !strings.Contains(strings.Split(h.plain(), "\n")[0], "!") {
		t.Errorf("tab does not flag the error: %q", strings.Split(h.plain(), "\n")[0])
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
