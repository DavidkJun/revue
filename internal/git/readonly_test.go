package git

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestReadOnlyIndex checks that polling leaves .git/index alone even when a
// tracked file is stat-dirty (touched, content unchanged), which is what an
// editor or formatter that rewrites a file with the same bytes leaves
// behind. git diff refreshes the index in that case unless told not to, and
// --no-optional-locks does not stop it.
func TestReadOnlyIndex(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("touched.txt", "same\n")
	r.write("mod.txt", "a\n")
	r.commitAll("init")
	r.write("mod.txt", "b\n")
	r.write("new.txt", "n\n")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(r.dir, "touched.txt"), past, past); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(r.dir, ".git", "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}

	st := r.status()
	for _, fc := range st.Files {
		if _, err := GetFileDiff(testCtx(t), r.dir, fc); err != nil {
			t.Fatal(err)
		}
	}
	files := append(st.Files, FileChange{Path: "touched.txt"})
	if _, err := Patch(testCtx(t), r.dir, files, 0); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal(".git/index was rewritten")
	}
}
