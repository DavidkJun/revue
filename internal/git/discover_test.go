package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func repoNames(repos []Repo) []string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return names
}

func TestDiscoverParentFolder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, dir := range []string{
		"b", "a", "node_modules/pkg", ".hidden/repo", "group/e", "group/deep/g", "a/nested",
	} {
		initRepo(t, filepath.Join(root, filepath.FromSlash(dir)))
	}
	// A linked worktree or submodule has a ".git" file instead of a folder.
	wt := filepath.Join(root, "wt")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: ../a/.git/worktrees/wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "plain", "folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	linkTo(t, filepath.Join(root, "group"), filepath.Join(root, "link"))

	repos, err := Discover(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "b", "group/e", "wt"}
	if got := repoNames(repos); !slices.Equal(got, want) {
		t.Fatalf("depth 2: got %v, want %v", got, want)
	}
	if want := filepath.Join(root, "group", "e"); repos[2].Path != want {
		t.Errorf("Path = %q, want %q", repos[2].Path, want)
	}

	repos, err = Discover(root, 3)
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"a", "b", "group/deep/g", "group/e", "wt"}
	if got := repoNames(repos); !slices.Equal(got, want) {
		t.Fatalf("depth 3: got %v, want %v", got, want)
	}

	repos, err = Discover(root, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 0 {
		t.Fatalf("depth 0: got %v, want none", repoNames(repos))
	}
}

// linkTo makes link point at target: a junction on Windows (no privilege
// needed), a symlink elsewhere.
func linkTo(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			t.Fatalf("mklink /J: %v\n%s", err, out)
		}
		return
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverInsideRepo(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	sub := filepath.Join(r.dir, "src", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	initRepo(t, filepath.Join(r.dir, "vendor", "inner")) // must not be listed
	for _, root := range []string{r.dir, sub} {
		repos, err := Discover(root, 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(repos) != 1 || repos[0].Name != "repo" {
			t.Fatalf("Discover(%s) = %+v, want only repo", root, repos)
		}
		if !sameDir(t, repos[0].Path, r.dir) {
			t.Errorf("Path = %q, want %q", repos[0].Path, r.dir)
		}
	}
}

func TestDiscoverMissingRoot(t *testing.T) {
	t.Parallel()
	if _, err := Discover(filepath.Join(t.TempDir(), "nope"), 2); err == nil {
		t.Fatal("want an error for a missing root")
	}
}

func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ia, err := os.Stat(a)
	if err != nil {
		t.Fatal(err)
	}
	ib, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(ia, ib)
}

// TestDiscoverLinkLoop: a link back to the root must not be followed, or a
// deep scan would list the same repos again and again under longer names.
func TestDiscoverLinkLoop(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	initRepo(t, filepath.Join(root, "sub", "r"))
	linkTo(t, root, filepath.Join(root, "sub", "loop"))
	repos, err := Discover(root, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := repoNames(repos); !slices.Equal(got, []string{"sub/r"}) {
		t.Fatalf("got %v, want only sub/r", got)
	}
}
