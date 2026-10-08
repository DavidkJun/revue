package diffview

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// TestPreview writes rendered views of a real diff as ANSI text for looking
// at them as images (termshot). It only runs with REVUE_PREVIEW set to the
// output folder and REVUE_PREVIEW_REPO / REVUE_PREVIEW_FILE naming a changed
// file of a local repo.
func TestPreview(t *testing.T) {
	out, repo, path := os.Getenv("REVUE_PREVIEW"), os.Getenv("REVUE_PREVIEW_REPO"), os.Getenv("REVUE_PREVIEW_FILE")
	if out == "" || repo == "" || path == "" {
		t.Skip("REVUE_PREVIEW, REVUE_PREVIEW_REPO and REVUE_PREVIEW_FILE not all set")
	}
	fd := gitFileDiff(t, repo, path)
	if added := os.Getenv("REVUE_PREVIEW_ADDED"); added != "" {
		fd, path = addedFileDiff(t, added), filepath.Base(added)
	}
	m := newModel(t, 120, 45, 50, nil)
	run(m, m.SetFile(filepath.Base(repo), path, fd, false))
	save := func(name string, focused bool) {
		if err := os.WriteFile(filepath.Join(out, name+".ansi"), []byte(m.View(focused)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	save("unified", true)
	act(m, keys.NextChange, 1)
	save("unified-next", true)
	save("unified-unfocused", false)
	m.SetSplit(true)
	save("split", true)
	m.SetSplit(false)
	act(m, keys.VisualStart, 0)
	act(m, keys.Down, 6)
	save("visual", true)
	m.SetSplit(true)
	save("visual-split", true)
	act(m, keys.VisualEnd, 0)
	m.SetSplit(false)
	m.SetSearch("domain")
	save("search", true)
	m.SetSearch("")
	m.SetSize(100, 30)
	act(m, keys.ContextLess, 4)
	act(m, keys.Top, 0)
	save("fold", true)
	m.SetSplit(true)
	save("fold-split", true)
	m.SetSplit(false)
	m.SetSize(60, 20)
	act(m, keys.NextChange, 2)
	save("narrow", true)
	m.SetMessage("no changes in atlas-front-end")
	save("message", true)
}

// gitFileDiff reads the whole-file diff of path against HEAD (read-only).
func gitFileDiff(t *testing.T, repo, path string) *diff.FileDiff {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "--no-optional-locks", "diff", "--no-color", "--no-ext-diff", "-U1000000", "HEAD", "--", path)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	raw, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff: %v", err)
	}
	fd := &diff.FileDiff{OldPath: path, NewPath: path}
	oldNo, newNo := 0, 0
	inHunk := false
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		l := strings.TrimSuffix(sc.Text(), "\r")
		if strings.HasPrefix(l, "@@") {
			inHunk = true
			continue
		}
		if !inHunk || l == "" || l[0] == '\\' {
			continue
		}
		switch l[0] {
		case ' ':
			oldNo++
			newNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Context, OldNo: oldNo, NewNo: newNo, Text: l[1:]})
		case '-':
			oldNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Deleted, OldNo: oldNo, Text: l[1:]})
		case '+':
			newNo++
			fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Added, NewNo: newNo, Text: l[1:]})
		}
	}
	if len(fd.Lines) == 0 {
		t.Fatalf("no diff for %s in %s", path, repo)
	}
	return fd
}

// addedFileDiff shows the file at path as new, like an untracked file.
func addedFileDiff(t *testing.T, path string) *diff.FileDiff {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fd := &diff.FileDiff{NewPath: path}
	for i, l := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		fd.Lines = append(fd.Lines, diff.Line{Kind: diff.Added, NewNo: i + 1, Text: strings.TrimSuffix(l, "\r")})
	}
	return fd
}
