package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMain isolates every git run (the tests' and the package's own) from
// the user's global and system configuration.
func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "revue-git-config")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(dir)
	cfg := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	os.Setenv("GIT_CONFIG_GLOBAL", cfg)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// A repository somewhere above the temp folder must not swallow the
	// test repos.
	os.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	return m.Run()
}

type testRepo struct {
	t   *testing.T
	dir string
}

// newRepo creates an empty repository with branch main in a fresh temp dir.
func newRepo(t *testing.T) *testRepo {
	t.Helper()
	return initRepo(t, filepath.Join(t.TempDir(), "repo"))
}

func initRepo(t *testing.T, dir string) *testRepo {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	r := &testRepo{t: t, dir: dir}
	r.git("init", "-q", "-b", "main")
	r.git("config", "core.autocrlf", "false")
	return r
}

// git runs a git command in the repo and returns its trimmed output.
func (r *testRepo) git(args ...string) string {
	r.t.Helper()
	out, err := r.tryGit(args...)
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func (r *testRepo) tryGit(args ...string) (string, error) {
	full := append([]string{
		"-C", r.dir,
		"-c", "user.name=revue test", "-c", "user.email=revue@example.com",
		"-c", "commit.gpgsign=false", "-c", "protocol.file.allow=always",
	}, args...)
	out, err := exec.Command("git", full...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (r *testRepo) write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *testRepo) remove(path string) {
	r.t.Helper()
	if err := os.Remove(filepath.Join(r.dir, filepath.FromSlash(path))); err != nil {
		r.t.Fatal(err)
	}
}

func (r *testRepo) commitAll(msg string) {
	r.t.Helper()
	r.git("add", "-A")
	r.git("commit", "-q", "-m", msg)
}

func (r *testRepo) status() Status {
	r.t.Helper()
	st, err := GetStatus(testCtx(r.t), r.dir)
	if err != nil {
		r.t.Fatalf("GetStatus: %v", err)
	}
	return st
}

// file returns the status entry for path.
func (r *testRepo) file(st Status, path string) FileChange {
	r.t.Helper()
	for _, fc := range st.Files {
		if fc.Path == path {
			return fc
		}
	}
	r.t.Fatalf("no status entry for %q in %+v", path, st.Files)
	return FileChange{}
}

func testCtx(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	return ctx
}
