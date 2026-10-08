package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/config"
	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/theme"
)

// fakeGit serves canned statuses, diffs and patches, and records the side
// effects the app asks for.
type fakeGit struct {
	mu       sync.Mutex
	statuses map[string]git.Status       // by repo path
	diffs    map[string]*diff.FileDiff   // by repo path + "|" + file path
	patches  map[string][]git.FileChange // by repo path, the last request
	clip     []string
	nav      []string
	loads    []string                 // repo path + "|" + file path, in request order
	delay    map[string]time.Duration // per diff key
}

func (f *fakeGit) deps() deps {
	return deps{
		status: func(_ context.Context, repoPath string) (git.Status, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			st, ok := f.statuses[repoPath]
			if !ok {
				return git.Status{}, fmt.Errorf("no such repo %s", repoPath)
			}
			return st, nil
		},
		fileDiff: func(_ context.Context, repoPath string, fc git.FileChange) (*diff.FileDiff, error) {
			k := repoPath + "|" + fc.Path
			f.mu.Lock()
			f.loads = append(f.loads, k)
			fd, d := f.diffs[k], f.delay[k]
			f.mu.Unlock()
			if d > 0 {
				time.Sleep(d)
			}
			if fd == nil {
				fd = script(fc.Path, " line one", "-old", "+new", " line four")
			}
			return fd, nil
		},
		patch: func(_ context.Context, repoPath string, files []git.FileChange, maxBytes int) (string, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.patches == nil {
				f.patches = map[string][]git.FileChange{}
			}
			f.patches[repoPath] = files
			var b strings.Builder
			for _, fc := range files {
				fmt.Fprintf(&b, "diff --git a/%s b/%s\n", fc.Path, fc.Path)
			}
			return b.String(), nil
		},
		clipboard: func(text string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.clip = append(f.clip, text)
			return nil
		},
		navigate: func(_ context.Context, dir string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.nav = append(f.nav, dir)
			return nil
		},
	}
}

func (f *fakeGit) navigations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.nav...)
}

// script builds a file diff from lines prefixed with " ", "-" or "+".
func script(path string, lines ...string) *diff.FileDiff {
	fd := &diff.FileDiff{OldPath: path, NewPath: path}
	oldNo, newNo := 0, 0
	for _, l := range lines {
		text := l[1:]
		switch l[0] {
		case '-':
			oldNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Deleted, OldNo: oldNo, Text: text})
		case '+':
			newNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Added, NewNo: newNo, Text: text})
		default:
			oldNo++
			newNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Context, OldNo: oldNo, NewNo: newNo, Text: text})
		}
	}
	return fd
}

var (
	backEnd  = git.Repo{Name: "atlas-back-end", Path: `C:\work\atlas\atlas-back-end`}
	frontEnd = git.Repo{Name: "atlas-front-end", Path: `C:\work\atlas\atlas-front-end`}
	docsRepo = git.Repo{Name: "atlas-docs", Path: `C:\work\atlas\atlas-docs`}
)

func change(kind git.Kind, path string, added, deleted int) git.FileChange {
	return git.FileChange{Path: path, Kind: kind, Added: added, Deleted: deleted, Unstaged: true}
}

// sampleGit has two repos with changes and a clean one. The first file in
// tree order (directories first) is src/auth/auth.service.ts.
func sampleGit() *fakeGit {
	return &fakeGit{
		statuses: map[string]git.Status{
			backEnd.Path: {
				Branch: "feature/auth-refresh", Head: "4f1c2ab", Upstream: "origin/feature/auth-refresh", Ahead: 2,
				Files: []git.FileChange{
					change(git.Modified, "src/app.module.ts", 3, 1),
					change(git.Modified, "src/auth/auth.service.ts", 9, 4),
					change(git.Untracked, "src/auth/refresh-token.guard.ts", 32, 0),
				},
				Fingerprint: "be-1",
			},
			frontEnd.Path: {
				Branch: "main", Head: "9ab0c11",
				Files: []git.FileChange{
					change(git.Modified, "src/api/client.ts", 7, 3),
					change(git.Deleted, "src/legacy/session.ts", 0, 41),
				},
				Fingerprint: "fe-1",
			},
			docsRepo.Path: {Branch: "main", Head: "77aa001", Fingerprint: "docs-1"},
		},
		diffs: map[string]*diff.FileDiff{
			backEnd.Path + "|src/auth/auth.service.ts": script("src/auth/auth.service.ts",
				" import { Injectable, UnauthorizedException } from '@nestjs/common';",
				" import { JwtService } from '@nestjs/jwt';",
				"-import { UsersService } from '../users/users.service';",
				"+import { UsersService } from '../modules/users/users.service';",
				"+import { RefreshTokenStore } from './refresh-token.store';",
				" ",
				" @Injectable()",
				" export class AuthService {",
				"   constructor(",
				"     private readonly users: UsersService,",
				"     private readonly jwt: JwtService,",
				"+    private readonly refreshTokens: RefreshTokenStore,",
				"   ) {}",
				" ",
				"   async validate(email: string, password: string) {",
				"     const user = await this.users.findByEmail(email);",
				"-    if (!user || user.password !== password) {",
				"+    if (!user || !(await user.checkPassword(password))) {",
				"       throw new UnauthorizedException();",
				"     }",
				"     return user;",
				"   }",
				" ",
				"   async login(user: User) {",
				"-    const token = this.jwt.sign({ sub: user.id });",
				"-    return { token };",
				"+    const access = this.jwt.sign({ sub: user.id }, { expiresIn: '15m' });",
				"+    const refresh = await this.refreshTokens.issue(user.id);",
				"+    return { access, refresh };",
				"   }",
				" ",
				"-  logout() {}",
				"+  async logout(userId: string) {",
				"+    await this.refreshTokens.revokeAll(userId);",
				"+  }",
				" }",
			),
			backEnd.Path + "|src/app.module.ts": script("src/app.module.ts",
				" import { Module } from '@nestjs/common';",
				" import { ConfigModule } from '@nestjs/config';",
				"-import { AuthModule } from './auth/auth.module';",
				"+import { AuthModule } from './modules/auth/auth.module';",
				"+import { UsersModule } from './modules/users/users.module';",
				" ",
				" @Module({",
				"   imports: [",
				"     ConfigModule.forRoot({ isGlobal: true }),",
				"     AuthModule,",
				"+    UsersModule,",
				"   ],",
				" })",
				" export class AppModule {}",
			),
		},
	}
}

// harness drives a Model the way Bubble Tea would, running commands and
// feeding their messages back. Commands that do not finish quickly (ticks)
// are dropped, so timeouts are tested by sending their messages directly.
type harness struct {
	t     *testing.T
	m     *Model
	git   *fakeGit
	raw   []string
	quit  bool
	quiet time.Duration // how long run waits for more messages
}

func newHarness(t *testing.T, w, h int, repos ...git.Repo) *harness {
	t.Helper()
	if len(repos) == 0 {
		repos = []git.Repo{backEnd, frontEnd, docsRepo}
	}
	fg := sampleGit()
	return startHarness(t, w, h, `C:\work\atlas`, repos, fg, fg.deps(), 80*time.Millisecond)
}

func startHarness(t *testing.T, w, h int, root string, repos []git.Repo, fg *fakeGit, d deps, quiet time.Duration) *harness {
	t.Helper()
	cfg := config.Default()
	cfg.Editor = "nvim"
	hs := &harness{t: t, git: fg, quiet: quiet, m: newModel(root, repos, cfg, theme.Default, d)}
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	hs.run(hs.m.Init())
	return hs
}

// run executes cmd and everything it leads to until nothing more arrives
// for a short while.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	msgs := make(chan tea.Msg, 256)
	var start func(tea.Cmd)
	start = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			msg := c()
			if seq, ok := sequence(msg); ok {
				// Sequenced commands deliver in order, like Bubble Tea.
				for _, sc := range seq {
					if sc == nil {
						continue
					}
					msgs <- sc()
				}
				return
			}
			msgs <- msg
		}()
	}
	start(cmd)
	for steps := 0; steps < 2000; steps++ {
		select {
		case msg := <-msgs:
			switch msg := msg.(type) {
			case nil:
			case tea.BatchMsg:
				for _, c := range msg {
					start(c)
				}
			case tea.QuitMsg:
				h.quit = true
			case tea.RawMsg:
				h.raw = append(h.raw, fmt.Sprint(msg.Msg))
			default:
				_, next := h.m.Update(msg)
				start(next)
			}
		case <-time.After(h.quiet):
			return
		}
	}
	h.t.Fatal("commands did not settle")
}

// sequence unwraps tea.Sequence's unexported message type.
func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return nil, false
	}
	if _, ok := msg.(tea.BatchMsg); ok {
		return nil, false
	}
	out := make([]tea.Cmd, v.Len())
	for i := range out {
		out[i] = v.Index(i).Interface().(tea.Cmd)
	}
	return out, true
}

// send delivers one message and runs what it leads to.
func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	_, cmd := h.m.Update(msg)
	h.run(cmd)
}

// keys presses keys by their binding names ("j", "ctrl+l", "space", "?").
func (h *harness) keys(names ...string) {
	h.t.Helper()
	for _, n := range names {
		h.send(keyPress(n))
	}
}

func keyPress(name string) tea.KeyPressMsg {
	special := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "space": tea.KeySpace,
		"backspace": tea.KeyBackspace, "up": tea.KeyUp, "down": tea.KeyDown,
		"left": tea.KeyLeft, "right": tea.KeyRight,
	}
	var k tea.KeyPressMsg
	rest := name
	for {
		switch {
		case strings.HasPrefix(rest, "ctrl+") && len(rest) > len("ctrl+"):
			k.Mod |= tea.ModCtrl
			rest = rest[len("ctrl+"):]
			continue
		case strings.HasPrefix(rest, "shift+") && len(rest) > len("shift+"):
			k.Mod |= tea.ModShift
			rest = rest[len("shift+"):]
			continue
		}
		break
	}
	if code, ok := special[rest]; ok {
		k.Code = code
		if code == tea.KeySpace && k.Mod == 0 {
			k.Text = " "
		}
		return k
	}
	r := []rune(rest)[0]
	k.Code = r
	if k.Mod&tea.ModCtrl == 0 {
		k.Text = rest
	}
	return k
}

// frame renders the view and checks its size.
func (h *harness) frame() string {
	h.t.Helper()
	f := h.m.View().Content
	checkFrame(h.t, f, h.m.width, h.m.height)
	return f
}

func checkFrame(t *testing.T, f string, w, h int) {
	t.Helper()
	got := strings.Split(f, "\n")
	if h == 0 {
		if f != "" {
			t.Fatalf("0-height frame is %q", f)
		}
		return
	}
	if len(got) != h {
		t.Fatalf("%dx%d: frame has %d lines", w, h, len(got))
	}
	for i, l := range got {
		if n := ansi.StringWidth(l); n != w {
			t.Fatalf("%dx%d: line %d is %d cells wide: %q", w, h, i, n, ansi.Strip(l))
		}
	}
}

// plain is the frame without styling.
func (h *harness) plain() string { return ansi.Strip(h.frame()) }

// dumpFrame writes the frame with its ANSI styling where termshot can read
// it, when $REVUE_FRAME_DIR is set.
func dumpFrame(t *testing.T, name, frame string) {
	t.Helper()
	dir := os.Getenv("REVUE_FRAME_DIR")
	if dir == "" {
		return
	}
	path := filepath.Join(dir, name+".ansi")
	if err := os.WriteFile(path, []byte(frame+"\x1b[0m\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("frame written to %s", path)
}

func (h *harness) current() (int, string) {
	r, f, ok := h.m.sidebar.Current()
	if !ok || f == nil {
		return r, ""
	}
	return r, f.Path
}
