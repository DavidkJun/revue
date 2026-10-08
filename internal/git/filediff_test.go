package git

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DavidkJun/revue/internal/diff"
)

func ctxL(o, n int, text string) diff.Line {
	return diff.Line{Kind: diff.Context, OldNo: o, NewNo: n, Text: text}
}
func delL(o int, text string) diff.Line { return diff.Line{Kind: diff.Deleted, OldNo: o, Text: text} }
func addL(n int, text string) diff.Line { return diff.Line{Kind: diff.Added, NewNo: n, Text: text} }

func (r *testRepo) fileDiff(path string) *diff.FileDiff {
	r.t.Helper()
	fd, err := GetFileDiff(testCtx(r.t), r.dir, r.file(r.status(), path))
	if err != nil {
		r.t.Fatalf("GetFileDiff(%s): %v", path, err)
	}
	return fd
}

func checkLines(t *testing.T, fd *diff.FileDiff, want ...diff.Line) {
	t.Helper()
	if fd.Binary || fd.TooLarge {
		t.Fatalf("binary %v, too large %v", fd.Binary, fd.TooLarge)
	}
	if !slices.Equal(fd.Lines, want) {
		t.Fatalf("lines:\n got %s\nwant %s", fmtLines(fd.Lines), fmtLines(want))
	}
}

func fmtLines(lines []diff.Line) string {
	var b strings.Builder
	for _, l := range lines {
		fmt.Fprintf(&b, "\n  %c %d %d %q", " +-"[l.Kind], l.OldNo, l.NewNo, l.Text)
	}
	return b.String()
}

func TestFileDiffStagedAndUnstaged(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "a\nb\nc\nd\ne\n")
	r.commitAll("init")
	r.write("f.txt", "a\nB\nc\nd\ne\n")
	r.git("add", "f.txt")
	r.write("f.txt", "a\nB\nc\ne\nf\n")
	fd := r.fileDiff("f.txt")
	if fd.OldPath != "f.txt" || fd.NewPath != "f.txt" {
		t.Errorf("paths %q -> %q", fd.OldPath, fd.NewPath)
	}
	checkLines(t, fd,
		ctxL(1, 1, "a"), delL(2, "b"), addL(2, "B"), ctxL(3, 3, "c"), delL(4, "d"), ctxL(5, 4, "e"), addL(5, "f"))
}

func TestFileDiffWholeFileContext(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	var b strings.Builder
	for i := 1; i <= 300; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	r.write("long.txt", b.String())
	r.commitAll("init")
	r.write("long.txt", strings.Replace(b.String(), "line 150\n", "line 150 changed\n", 1))
	fd := r.fileDiff("long.txt")
	if len(fd.Lines) != 301 {
		t.Fatalf("got %d lines, want the whole file (301)", len(fd.Lines))
	}
	if fd.Lines[0] != ctxL(1, 1, "line 1") || fd.Lines[300] != ctxL(300, 300, "line 300") {
		t.Errorf("ends = %v %v", fd.Lines[0], fd.Lines[300])
	}
}

func TestFileDiffRename(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("old name.txt", "1\n2\n3\n4\n5\n")
	r.commitAll("init")
	r.git("mv", "old name.txt", "new name.txt")
	r.write("new name.txt", "1\n2\n3\n4\nfive\n")
	fd := r.fileDiff("new name.txt")
	if fd.OldPath != "old name.txt" || fd.NewPath != "new name.txt" {
		t.Errorf("paths %q -> %q", fd.OldPath, fd.NewPath)
	}
	checkLines(t, fd, ctxL(1, 1, "1"), ctxL(2, 2, "2"), ctxL(3, 3, "3"), ctxL(4, 4, "4"), delL(5, "5"), addL(5, "five"))
}

func TestFileDiffPureRename(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "x\ny\n")
	r.commitAll("init")
	r.git("mv", "a.txt", "b.txt")
	checkLines(t, r.fileDiff("b.txt"), ctxL(1, 1, "x"), ctxL(2, 2, "y"))
}

func TestFileDiffRenameRewritten(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "q\nw\n")
	r.commitAll("init")
	r.git("mv", "a.txt", "b.txt")
	r.write("b.txt", "totally\ndifferent\nnow\n")
	checkLines(t, r.fileDiff("b.txt"), delL(1, "q"), delL(2, "w"), addL(1, "totally"), addL(2, "different"), addL(3, "now"))
}

func TestFileDiffCopy(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "status.renames", "copies")
	r.write("src.txt", "aaaa\nbbbb\ncccc\ndddd\n")
	r.commitAll("init")
	r.write("src.txt", "aaaa\nbbbb\ncccc\ndddd\nEEEE\n")
	r.write("copy.txt", "aaaa\nbbbb\ncccc\ndddd\nEEEE\n")
	r.git("add", "-A")
	fd := r.fileDiff("copy.txt")
	if fd.OldPath != "src.txt" {
		t.Errorf("OldPath = %q", fd.OldPath)
	}
	checkLines(t, fd, ctxL(1, 1, "aaaa"), ctxL(2, 2, "bbbb"), ctxL(3, 3, "cccc"), ctxL(4, 4, "dddd"), addL(5, "EEEE"))
}

func TestFileDiffTypeChange(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "core.symlinks", "false")
	r.write("link", "plain\n")
	r.commitAll("init")
	r.write("target-content", "target")
	blob := r.git("hash-object", "-w", filepath.Join(r.dir, "target-content"))
	r.remove("target-content")
	r.git("update-index", "--cacheinfo", "120000,"+blob+",link")
	r.write("link", "target")
	checkLines(t, r.fileDiff("link"), delL(1, "plain"), addL(1, "target"))
}

func TestFileDiffAddedDeletedUntracked(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("doomed.txt", "d1\nd2\n")
	r.commitAll("init")
	r.remove("doomed.txt")
	r.write("staged.txt", "s1\n")
	r.git("add", "staged.txt")
	r.write("staged.txt", "s1\ns2\n")
	r.write("loose.txt", "u1\nu2")

	fd := r.fileDiff("doomed.txt")
	if fd.OldPath != "doomed.txt" || fd.NewPath != "" {
		t.Errorf("deleted paths %q -> %q", fd.OldPath, fd.NewPath)
	}
	checkLines(t, fd, delL(1, "d1"), delL(2, "d2"))

	fd = r.fileDiff("staged.txt")
	if fd.OldPath != "" || fd.NewPath != "staged.txt" {
		t.Errorf("added paths %q -> %q", fd.OldPath, fd.NewPath)
	}
	checkLines(t, fd, addL(1, "s1"), addL(2, "s2"))
	checkLines(t, r.fileDiff("loose.txt"), addL(1, "u1"), addL(2, "u2"))
}

func TestFileDiffBinary(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("img.bin", "\x00\x01\x02")
	r.write("gone.bin", "\x00gone")
	r.commitAll("init")
	r.write("img.bin", "\x00\x01\x02\x03")
	r.remove("gone.bin")
	r.write("new.bin", "abc\x00def")
	for _, path := range []string{"img.bin", "gone.bin", "new.bin"} {
		if fd := r.fileDiff(path); !fd.Binary || len(fd.Lines) != 0 {
			t.Errorf("%s: binary %v, %d lines", path, fd.Binary, len(fd.Lines))
		}
	}
	if fc := r.file(r.status(), "img.bin"); !fc.Binary || fc.Added != 0 {
		t.Errorf("status of img.bin = %+v", fc)
	}
}

func TestFileDiffCRLF(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("win.txt", "one\r\ntwo\r\nthree\r\n")
	r.commitAll("init")
	r.write("win.txt", "one\r\nTWO\r\nthree\r\n")
	checkLines(t, r.fileDiff("win.txt"), ctxL(1, 1, "one"), delL(2, "two"), addL(2, "TWO"), ctxL(3, 3, "three"))
	r.write("new.txt", "a\r\nb\r\n")
	checkLines(t, r.fileDiff("new.txt"), addL(1, "a"), addL(2, "b"))
}

func TestFileDiffNoTrailingNewline(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "a\nb")
	r.write("g.txt", "x\ny\n")
	r.commitAll("init")
	r.write("f.txt", "a\nb\nc")
	r.write("g.txt", "x\ny")
	checkLines(t, r.fileDiff("f.txt"), ctxL(1, 1, "a"), delL(2, "b"), addL(2, "b"), addL(3, "c"))
	checkLines(t, r.fileDiff("g.txt"), ctxL(1, 1, "x"), delL(2, "y"), addL(2, "y"))
}

func TestFileDiffEmptyFiles(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("emptied.txt", "a\nb\n")
	r.write("filled.txt", "")
	r.commitAll("init")
	r.write("emptied.txt", "")
	r.write("filled.txt", "now\n")
	r.write("blank.txt", "")
	checkLines(t, r.fileDiff("emptied.txt"), delL(1, "a"), delL(2, "b"))
	checkLines(t, r.fileDiff("filled.txt"), addL(1, "now"))
	checkLines(t, r.fileDiff("blank.txt"))
}

func TestFileDiffModeChange(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("run.sh", "echo hi\n")
	r.commitAll("init")
	r.git("update-index", "--chmod=+x", "run.sh")
	// Where the file system has an executable bit it must agree with the index.
	if err := os.Chmod(filepath.Join(r.dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	fc := r.file(r.status(), "run.sh")
	if fc.Kind != Modified || fc.Added != 0 || fc.Deleted != 0 {
		t.Fatalf("status = %+v", fc)
	}
	checkLines(t, r.fileDiff("run.sh"), ctxL(1, 1, "echo hi"))
}

func TestFileDiffNoCommits(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("first.txt", "1\n2\n")
	r.git("add", "first.txt")
	r.write("other.txt", "o\n")
	checkLines(t, r.fileDiff("first.txt"), addL(1, "1"), addL(2, "2"))
	checkLines(t, r.fileDiff("other.txt"), addL(1, "o"))
}

func TestFileDiffConflict(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "base\nline2\n")
	r.commitAll("init")
	r.git("checkout", "-q", "-b", "other")
	r.write("f.txt", "theirs\nline2\n")
	r.commitAll("other")
	r.git("checkout", "-q", "main")
	r.write("f.txt", "ours\nline2\n")
	r.commitAll("ours")
	if _, err := r.tryGit("merge", "other"); err == nil {
		t.Fatal("merge unexpectedly succeeded")
	}
	checkLines(t, r.fileDiff("f.txt"),
		addL(1, "<<<<<<< HEAD"), ctxL(1, 2, "ours"), addL(3, "======="), addL(4, "theirs"), addL(5, ">>>>>>> other"), ctxL(2, 6, "line2"))
}

func TestFileDiffTooLarge(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	big := strings.Repeat("0123456789abcde\n", MaxFileBytes/16+1)
	r.write("shrunk.txt", big)
	r.write("grown.txt", "small\n")
	r.write("gone-big.txt", big)
	r.commitAll("init")
	r.write("shrunk.txt", "small now\n")
	r.write("grown.txt", big)
	r.remove("gone-big.txt")
	r.write("new-big.txt", big)
	for _, path := range []string{"shrunk.txt", "grown.txt", "gone-big.txt", "new-big.txt"} {
		if fd := r.fileDiff(path); !fd.TooLarge || len(fd.Lines) != 0 {
			t.Errorf("%s: too large %v, %d lines", path, fd.TooLarge, len(fd.Lines))
		}
	}
	if fc := r.file(r.status(), "new-big.txt"); fc.Added != MaxFileBytes/16 {
		t.Errorf("new-big.txt counts %d lines, want those within MaxFileBytes (%d)", fc.Added, MaxFileBytes/16)
	}
}

func TestFileDiffSpecialPaths(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("папка/файл [1].txt", "один\nдва\n")
	r.write("dir with space/a b.txt", "x\n")
	r.write("[slug]/gone.ts", "g\n")
	r.commitAll("init")
	r.write("папка/файл [1].txt", "один\nтри\n")
	r.git("mv", "dir with space/a b.txt", "dir with space/c d.txt")
	r.remove("[slug]/gone.ts")
	r.write("нове/файл.txt", "новий\n")

	checkLines(t, r.fileDiff("папка/файл [1].txt"), ctxL(1, 1, "один"), delL(2, "два"), addL(2, "три"))
	checkLines(t, r.fileDiff("dir with space/c d.txt"), ctxL(1, 1, "x"))
	checkLines(t, r.fileDiff("[slug]/gone.ts"), delL(1, "g"))
	checkLines(t, r.fileDiff("нове/файл.txt"), addL(1, "новий"))
}

// checkWholeFile checks the invariants the viewer relies on: the old side
// of the diff is exactly HEAD's version, the new side exactly the working
// tree's, and line numbers count up by one on each side from 1.
func checkWholeFile(t *testing.T, name string, fd *diff.FileDiff, oldText, newText string) {
	t.Helper()
	if want := splitLines([]byte(oldText)); !slices.Equal(fd.OldLines(), want) {
		t.Errorf("%s: old side\n got %q\nwant %q", name, fd.OldLines(), want)
	}
	if want := splitLines([]byte(newText)); !slices.Equal(fd.NewLines(), want) {
		t.Errorf("%s: new side\n got %q\nwant %q", name, fd.NewLines(), want)
	}
	oldNo, newNo := 0, 0
	for i, l := range fd.Lines {
		if l.Kind != diff.Added {
			if l.OldNo != oldNo+1 {
				t.Fatalf("%s: line %d has OldNo %d after %d%s", name, i, l.OldNo, oldNo, fmtLines(fd.Lines))
			}
			oldNo = l.OldNo
		} else if l.OldNo != 0 {
			t.Fatalf("%s: added line %d has OldNo %d", name, i, l.OldNo)
		}
		if l.Kind != diff.Deleted {
			if l.NewNo != newNo+1 {
				t.Fatalf("%s: line %d has NewNo %d after %d%s", name, i, l.NewNo, newNo, fmtLines(fd.Lines))
			}
			newNo = l.NewNo
		} else if l.NewNo != 0 {
			t.Fatalf("%s: deleted line %d has NewNo %d", name, i, l.NewNo)
		}
	}
}

// TestFileDiffRandomEdits edits files at random, with lines that look like
// diff syntax, blank lines, CRLF and missing final newlines, under the
// settings a Windows user may have (autocrlf, suppressBlankEmpty), and
// checks every diff against the two versions of the file.
func TestFileDiffRandomEdits(t *testing.T) {
	t.Parallel()
	pool := []string{
		"plain", "", " ", "--- a/x", "+++ b/x", "diff --git a/x b/x", "@@ -1,2 +1,2 @@", `\ No newline at end of file`,
		"-", "+", "\t", "same", "same", "same", "func x() {", "}", "привіт", "a\rb",
	}
	for _, cfg := range []struct {
		name  string
		crlf  bool
		extra [][2]string
	}{
		{name: "plain"},
		{name: "autocrlf", crlf: true, extra: [][2]string{{"core.autocrlf", "true"}, {"diff.suppressBlankEmpty", "true"}}},
	} {
		t.Run(cfg.name, func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewPCG(1, uint64(len(cfg.name))))
			gen := func(n int, eol string) string {
				var b strings.Builder
				for range n {
					b.WriteString(pool[rng.IntN(len(pool))])
					b.WriteString(eol)
				}
				s := b.String()
				if n > 0 && rng.IntN(4) == 0 {
					s = strings.TrimSuffix(s, eol)
				}
				return s
			}
			edit := func(s, eol string) string {
				lines := strings.SplitAfter(s, eol)
				for range 1 + rng.IntN(6) {
					i := rng.IntN(len(lines) + 1)
					switch rng.IntN(3) {
					case 0:
						lines = slices.Insert(lines, i, pool[rng.IntN(len(pool))]+eol)
					case 1:
						if i < len(lines) {
							lines = slices.Delete(lines, i, i+1)
						}
					default:
						if i < len(lines) {
							lines[i] = pool[rng.IntN(len(pool))] + eol
						}
					}
				}
				return strings.Join(lines, "")
			}
			r := newRepo(t)
			for _, kv := range cfg.extra {
				r.git("config", kv[0], kv[1])
			}
			const files = 24
			olds := make([]string, files)
			for i := range files {
				olds[i] = gen(rng.IntN(30), "\n")
				r.write(fmt.Sprintf("f%02d.txt", i), olds[i])
			}
			r.commitAll("init")
			news := make([]string, files)
			for i := range files {
				eol := "\n"
				if cfg.crlf && i%2 == 0 {
					eol = "\r\n"
					olds[i] = strings.ReplaceAll(olds[i], "\n", "\r\n")
				}
				news[i] = edit(olds[i], eol)
				r.write(fmt.Sprintf("f%02d.txt", i), news[i])
			}
			st := r.status()
			for i := range files {
				path := fmt.Sprintf("f%02d.txt", i)
				fc := FileChange{Path: path}
				for _, f := range st.Files {
					if f.Path == path {
						fc = f
					}
				}
				fd, err := GetFileDiff(testCtx(t), r.dir, fc)
				if err != nil {
					t.Fatal(err)
				}
				checkWholeFile(t, path, fd, olds[i], news[i])
			}
			checkCountsMatchDiff(t, r, st)
		})
	}
}

// TestFileDiffBothDeletedConflict: a rename/rename conflict leaves the
// original path unmerged ("DD") with no file in HEAD or on disk. There is
// nothing to show, which is not an error.
func TestFileDiffBothDeletedConflict(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "1\n2\n3\n4\n5\n")
	r.commitAll("init")
	r.git("checkout", "-q", "-b", "other")
	r.git("mv", "a.txt", "c.txt")
	r.commitAll("to c")
	r.git("checkout", "-q", "main")
	r.git("mv", "a.txt", "b.txt")
	r.commitAll("to b")
	if _, err := r.tryGit("merge", "other"); err == nil {
		t.Fatal("merge unexpectedly succeeded")
	}
	fc := r.file(r.status(), "a.txt")
	if fc.Kind != Conflicted {
		t.Fatalf("a.txt = %+v, want conflicted", fc)
	}
	checkLines(t, r.fileDiff("a.txt"))
}
