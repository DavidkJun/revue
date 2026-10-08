package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// emptyTree is git's well-known empty tree; it stands in for HEAD in a
// repository without commits.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// errOutputCut stops os/exec from copying more output once a cappedBuffer is
// full.
var errOutputCut = errors.New("git output limit reached")

// command builds a read-only git invocation in repo. Optional locks are off
// so polling never contends with the user's own git, pathspecs are literal so
// names like "a[1].ts" match only themselves, and paths come out unescaped.
// git diff rewrites .git/index (taking index.lock) when it meets a file that
// was touched but not changed, and --no-optional-locks does not stop that;
// diff.autoRefreshIndex=false does.
func command(ctx context.Context, repo string, args []string) *exec.Cmd {
	full := append([]string{
		"-C", repo, "--no-optional-locks", "--literal-pathspecs",
		"-c", "core.quotepath=false", "-c", "diff.autoRefreshIndex=false",
	}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	// In a partial clone git diff would otherwise fetch missing blobs, which
	// means network access and writes to .git/objects.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_NO_LAZY_FETCH=1")
	// A grandchild that inherited the pipes must not keep Wait blocked after
	// git itself was killed.
	cmd.WaitDelay = time.Second
	hideWindow(cmd)
	return cmd
}

// runGit runs git in repo and returns its standard output.
func runGit(ctx context.Context, repo string, args ...string) ([]byte, error) {
	out, _, err := runGitCapped(ctx, repo, 0, args...)
	return out, err
}

// runGitCapped is runGit keeping at most limit bytes of output (no limit when
// limit <= 0). When more arrives git is stopped, cut is true and the output
// read so far is returned without an error.
func runGitCapped(ctx context.Context, repo string, limit int, args ...string) (out []byte, cut bool, err error) {
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	stdout := &cappedBuffer{limit: limit, onFull: stop}
	var stderr bytes.Buffer
	cmd := command(runCtx, repo, args)
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	switch {
	case ctx.Err() != nil:
		return nil, false, ctx.Err()
	case stdout.full:
		return stdout.buf.Bytes(), true, nil
	case err != nil:
		return nil, false, gitError(args, err, stderr.String())
	}
	return stdout.buf.Bytes(), false, nil
}

// runAgainstHead runs a git command that compares with HEAD, built by args
// for a given base. In a repository without commits it compares with the
// empty tree instead.
func runAgainstHead(ctx context.Context, repo string, limit int, args func(base string) []string) ([]byte, bool, error) {
	out, cut, err := runGitCapped(ctx, repo, limit, args("HEAD")...)
	if err == nil || ctx.Err() != nil || hasHead(ctx, repo) {
		return out, cut, err
	}
	return runGitCapped(ctx, repo, limit, args(emptyTree)...)
}

// hasHead reports whether HEAD points at a commit.
func hasHead(ctx context.Context, repo string) bool {
	_, err := runGit(ctx, repo, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	return err == nil
}

// blobSize returns the size of an object such as "HEAD:path".
func blobSize(ctx context.Context, repo, object string) (int64, error) {
	out, err := runGit(ctx, repo, "cat-file", "-s", object)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
}

// gitError keeps git's own explanation, which is usually the last line of
// stderr ("fatal: ..."), while still wrapping the exec error.
func gitError(args []string, err error, stderr string) error {
	name := "git"
	if len(args) > 0 {
		name += " " + args[0]
	}
	msg := ""
	for _, line := range strings.Split(stderr, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			msg = line
		}
	}
	if msg == "" {
		return fmt.Errorf("%s: %w", name, err)
	}
	return fmt.Errorf("%s: %s (%w)", name, msg, err)
}

// cappedBuffer keeps the first limit bytes written to it (everything when
// limit <= 0) and calls onFull once when more arrive.
type cappedBuffer struct {
	buf    bytes.Buffer
	limit  int
	full   bool
	onFull func()
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		return b.buf.Write(p)
	}
	room := b.limit - b.buf.Len()
	if len(p) <= room {
		return b.buf.Write(p)
	}
	b.buf.Write(p[:room])
	if !b.full {
		b.full = true
		b.onFull()
	}
	return room, errOutputCut
}
