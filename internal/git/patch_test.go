package git

import (
	"fmt"
	"strings"
	"testing"
)

func (r *testRepo) patch(maxBytes int) string {
	r.t.Helper()
	out, err := Patch(testCtx(r.t), r.dir, r.status().Files, maxBytes)
	if err != nil {
		r.t.Fatalf("Patch: %v", err)
	}
	return out
}

func TestPatch(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("mod.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n")
	r.write("old.txt", "keep\nkeep\nkeep\n")
	r.write("dir with space/файл [1].txt", "a\n")
	r.commitAll("init")
	r.write("mod.txt", "1\n2\n3\n4\n5\n6\n7\nEIGHT\n9\n10\n11\n12\n13\n14\n15\n")
	r.git("mv", "old.txt", "new.txt")
	r.write("dir with space/файл [1].txt", "b\n")
	r.write("fresh.txt", "x\ny")
	r.write("blob.bin", "\x00\x01")
	r.write("huge.txt", strings.Repeat("z", MaxFileBytes+1))

	out := r.patch(0)
	for _, want := range []string{
		"diff --git a/mod.txt b/mod.txt\n",
		"@@ -3,11 +3,11 @@\n 3\n", // five lines of context on each side
		"-8\n+EIGHT\n",
		"rename from old.txt\nrename to new.txt\n",
		"+++ b/dir with space/файл [1].txt",
		"diff --git a/fresh.txt b/fresh.txt\nnew file mode 100644\n--- /dev/null\n+++ b/fresh.txt\n@@ -0,0 +1,2 @@\n+x\n+y\n\\ No newline at end of file\n",
		"Untracked file blob.bin: binary, content not included\n",
		"Untracked file huge.txt: larger than 4 MB, content not included\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("patch lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "patch cut") {
		t.Errorf("unlimited patch has a cut note:\n%s", out)
	}
}

func TestPatchTruncation(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	for i := range 6 {
		r.write(fmt.Sprintf("f%d.txt", i), strings.Repeat(fmt.Sprintf("line of file %d\n", i), 40))
	}
	r.commitAll("init")
	for i := range 6 {
		r.write(fmt.Sprintf("f%d.txt", i), strings.Repeat(fmt.Sprintf("LINE OF FILE %d\n", i), 40))
	}
	r.write("u1.txt", "untracked one\n")
	r.write("u2.txt", "untracked two\n")
	full := r.patch(0)

	const limit = 2000
	out := r.patch(limit)
	body, note, found := strings.Cut(out, "\n[patch cut at 2000 bytes; left out: ")
	if !found {
		t.Fatalf("no cut note in:\n%s", out)
	}
	if len(body) > limit {
		t.Errorf("body is %d bytes, over the %d limit", len(body), limit)
	}
	if !strings.HasPrefix(full, body) {
		t.Errorf("the cut patch is not a prefix of the full one")
	}
	if !strings.Contains(body, "diff --git a/f0.txt b/f0.txt") {
		t.Errorf("first file missing from the cut patch")
	}
	for _, name := range []string{"f5.txt", "u1.txt", "u2.txt"} {
		if !strings.Contains(note, name) {
			t.Errorf("note %q does not name %s", note, name)
		}
	}
	if strings.Contains(note, "f0.txt") {
		t.Errorf("note %q names a file that was included", note)
	}
	if !strings.Contains(note, "(partly)") {
		t.Errorf("note %q does not mention the file cut in the middle", note)
	}
}

func TestPatchNoCommits(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.git("add", "a.txt")
	r.write("b.txt", "b\n")
	out := r.patch(0)
	for _, want := range []string{"+++ b/a.txt\n@@ -0,0 +1 @@\n+a\n", "+++ b/b.txt\n@@ -0,0 +1,1 @@\n+b\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("patch lacks %q:\n%s", want, out)
		}
	}
}

func TestPathChunks(t *testing.T) {
	t.Parallel()
	var files []FileChange
	long := strings.Repeat("d", 1000)
	for i := range 60 {
		files = append(files, FileChange{Path: fmt.Sprintf("%s/%d.txt", long, i)})
	}
	files = append(files, FileChange{Path: "new.txt", OldPath: "old.txt", Kind: Renamed})
	chunks := pathChunks(files)
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks, want the paths spread over several", len(chunks))
	}
	total := 0
	for _, c := range chunks {
		chars := 0
		for _, p := range c {
			chars += len(p) + 3
		}
		if chars > maxPathChars {
			t.Errorf("chunk of %d chars", chars)
		}
		total += len(c)
	}
	if total != 62 {
		t.Errorf("chunks hold %d paths, want 62", total)
	}
	last := chunks[len(chunks)-1]
	if last[len(last)-2] != "new.txt" || last[len(last)-1] != "old.txt" {
		t.Errorf("rename split apart: %v", last)
	}
}
