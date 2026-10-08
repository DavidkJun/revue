package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// waitDelay bounds how long Wait keeps the pipes open after claude exits,
// in case a leftover grandchild still holds them.
const waitDelay = 3 * time.Second

// Ask starts claude for req and streams its events. The channel is closed
// after the final EventDone or EventError. Cancelling ctx kills the process
// (and its children) and yields EventError with context.Canceled.
func Ask(ctx context.Context, req Request) (<-chan Event, error) {
	bin := req.Binary
	if bin == "" {
		bin = "claude"
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}

	args := buildArgs(req)
	if err := checkBatchArgs(path, args); err != nil {
		return nil, err
	}
	cmd := exec.Command(path, args...)
	cmd.Dir = req.Dir
	cmd.Stdin = strings.NewReader(req.Prompt)
	// A pipe written by exec's own copier (instead of StdoutPipe) lets Wait
	// and WaitDelay decide when output ends, so a grandchild that keeps the
	// handle open cannot hang the reader forever.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	stderr := &tailBuffer{max: stderrTail}
	cmd.Stderr = stderr
	cmd.WaitDelay = waitDelay
	hideConsole(cmd)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	tree := attachTree(cmd.Process)

	ch := make(chan Event, 64)
	s := &session{
		ctx:     ctx,
		cmd:     cmd,
		tree:    tree,
		stdout:  pr,
		stdoutW: pw,
		stderr:  stderr,
		parser:  newParser(req.Dir),
		out:     ch,
	}
	go s.run()
	return ch, nil
}

// questionSettings turns hooks off for review questions. They can only read,
// so the user's hooks have nothing to guard, and plugin hooks (superpowers)
// would add context to every question.
const questionSettings = `{"disableAllHooks":true}`

// settingsArg is the --settings value: a file holding questionSettings,
// because a .cmd shim cannot pass the quotes of inline JSON. If the file
// cannot be written the JSON goes inline, which a real claude.exe accepts.
var settingsArg = sync.OnceValue(func() string {
	path := filepath.Join(os.TempDir(), "revue-claude-settings.json")
	if err := os.WriteFile(path, []byte(questionSettings), 0o644); err != nil {
		return questionSettings
	}
	return path
})

// buildArgs returns claude's argv after the program name. The prompt goes
// to stdin, so nothing positional follows the flags. --tools and --add-dir
// take variadic values in the CLI: --tools is always followed by another
// flag, and --add-dir uses the = form so it never swallows what comes next.
// --strict-mcp-config without --mcp-config starts no MCP servers: with only
// Read, Grep and Glob allowed their tools are unusable, yet their definitions
// made up about 75k of the ~90k prompt tokens of every question.
func buildArgs(req Request) []string {
	sp := req.SystemPrompt
	if sp == "" {
		sp = DefaultSystemPrompt
	}
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--tools", "Read,Grep,Glob",
		"--permission-mode", "dontAsk",
		"--settings", settingsArg(),
		"--strict-mcp-config",
		"--append-system-prompt", sp,
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}
	if req.SessionID != "" {
		args = append(args, "--resume", req.SessionID)
	}
	for _, dir := range req.AddDirs {
		args = append(args, "--add-dir="+dir)
	}
	return append(args, req.ExtraArgs...)
}

type session struct {
	ctx     context.Context
	cmd     *exec.Cmd
	tree    *procTree
	stdout  *io.PipeReader
	stdoutW *io.PipeWriter // closed once Wait returns, which ends the read loop
	stderr  *tailBuffer
	parser  *parser
	out     chan Event
	closed  bool
}

func (s *session) run() {
	stop := context.AfterFunc(s.ctx, s.tree.kill)
	waited := make(chan error, 1)
	go func() {
		err := s.cmd.Wait()
		s.stdoutW.Close()
		waited <- err
	}()

	// The reader never waits for the caller (see eventQueue). Output after
	// the result line is drained but ignored; the final event goes out as
	// soon as it is known so the UI does not wait for shutdown.
	q := newEventQueue()
	go func() {
		q.close(readLines(s.stdout, func(line []byte) { q.push(s.parser.parse(line)...) }))
	}()
	var readErr error
	for {
		ev, ok, err := q.next()
		if !ok {
			readErr = err
			break
		}
		if ev.Kind == EventDone || ev.Kind == EventError {
			s.finish(ev)
			break
		}
		s.send(ev)
	}
	waitErr := <-waited
	stop()
	s.tree.release()
	if s.closed {
		return
	}
	s.finish(s.exitEvent(waitErr, readErr))
}

func (s *session) exitEvent(waitErr, readErr error) Event {
	ev := Event{Kind: EventError, SessionID: s.parser.sessionID}
	if err := s.ctx.Err(); err != nil {
		ev.Err = fmt.Errorf("claude: %w", err)
		return ev
	}
	err := errors.Join(waitErr, readErr)
	msg := "claude exited without a result"
	if err != nil {
		msg += " (" + err.Error() + ")"
	}
	if tail := s.stderr.String(); tail != "" {
		msg += ":\n" + tail
	}
	ev.Err = errors.New(msg)
	return ev
}

// send delivers a streamed event unless the caller has cancelled.
func (s *session) send(ev Event) {
	if s.ctx.Err() != nil {
		return
	}
	select {
	case s.out <- ev:
	case <-s.ctx.Done():
	}
}

// finish delivers the final event and closes the channel. After
// cancellation the caller may have stopped reading, so instead of blocking
// it discards the oldest unread events to make room; the channel is
// buffered and this goroutine is its only sender, so that always succeeds.
func (s *session) finish(ev Event) {
	s.closed = true
	defer close(s.out)
	select {
	case s.out <- ev:
		return
	case <-s.ctx.Done():
	}
	for {
		select {
		case s.out <- ev:
			return
		default:
		}
		select {
		case <-s.out:
		default:
		}
	}
}
