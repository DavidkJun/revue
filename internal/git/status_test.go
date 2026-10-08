package git

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestStatusKinds(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.git("config", "core.symlinks", "false")
	r.git("config", "status.renames", "copies")
	r.write("mod.txt", "one\ntwo\nthree\n")
	r.write("old.txt", "keep\nkeep\nkeep\nkeep\n")
	r.write("gone.txt", "a\nb\n")
	r.write("staged-gone.txt", "x\n")
	r.write("mod-gone.txt", "y\n")
	r.write("src.txt", "aaaa\nbbbb\ncccc\ndddd\n")
	r.write("link", "plain\n")
	r.write("reverted.txt", "same\n")
	r.write("untracked-now.txt", "u\n")
	r.commitAll("init")

	r.write("mod.txt", "one\nTWO\nthree\nfour\n") // .M
	r.git("mv", "old.txt", "new.txt")             // R.
	r.remove("gone.txt")                          // .D
	r.git("rm", "-q", "staged-gone.txt")          // D.
	r.write("mod-gone.txt", "y2\n")               // MD
	r.git("add", "mod-gone.txt")
	r.remove("mod-gone.txt")
	r.write("added.txt", "n1\n") // AM
	r.git("add", "added.txt")
	r.write("added.txt", "n1\nn2\n")
	r.write("ita.txt", "i\n") // .A
	r.git("add", "-N", "ita.txt")
	r.write("phantom.txt", "p\n") // AD: nothing to review
	r.git("add", "phantom.txt")
	r.remove("phantom.txt")
	r.write("reverted.txt", "changed\n") // MM back to HEAD: nothing to review
	r.git("add", "reverted.txt")
	r.write("reverted.txt", "same\n")
	r.git("rm", "-q", "--cached", "untracked-now.txt")   // D. plus ? for the same path
	r.write("src.txt", "aaaa\nbbbb\ncccc\ndddd\nEEEE\n") // C. from a modified source
	r.write("copy.txt", "aaaa\nbbbb\ncccc\ndddd\nEEEE\n")
	r.git("add", "src.txt", "copy.txt")
	blob := r.git("hash-object", "-w", filepath.Join(r.dir, "mod.txt"))
	r.git("update-index", "--cacheinfo", "120000,"+blob+",link") // T.
	r.write("link", "one\nTWO\nthree\nfour\n")
	r.write("fresh.txt", "f1\nf2\nf3") // untracked, no trailing newline

	st := r.status()
	type want struct {
		kind             Kind
		old              string
		staged, unstaged bool
		added, deleted   int
	}
	wants := map[string]want{
		"added.txt":         {Added, "", true, true, 2, 0},
		"copy.txt":          {Copied, "src.txt", true, false, 5, 0},
		"fresh.txt":         {Untracked, "", false, true, 3, 0},
		"gone.txt":          {Deleted, "", false, true, 0, 2},
		"ita.txt":           {Added, "", false, true, 1, 0},
		"link":              {TypeChanged, "", true, false, 4, 1},
		"mod-gone.txt":      {Deleted, "", true, true, 0, 1},
		"mod.txt":           {Modified, "", false, true, 2, 1},
		"new.txt":           {Renamed, "old.txt", true, false, 0, 0},
		"src.txt":           {Modified, "", true, false, 1, 0},
		"staged-gone.txt":   {Deleted, "", true, false, 0, 1},
		"untracked-now.txt": {Deleted, "", true, false, 0, 1},
	}
	var paths []string
	for _, fc := range st.Files {
		paths = append(paths, fc.Path)
	}
	wantPaths := make([]string, 0, len(wants))
	for p := range wants {
		wantPaths = append(wantPaths, p)
	}
	slices.Sort(wantPaths)
	if !slices.Equal(paths, wantPaths) {
		t.Fatalf("paths:\n got %v\nwant %v", paths, wantPaths)
	}
	for _, fc := range st.Files {
		w := wants[fc.Path]
		got := want{fc.Kind, fc.OldPath, fc.Staged, fc.Unstaged, fc.Added, fc.Deleted}
		if got != w {
			t.Errorf("%s: got %+v, want %+v", fc.Path, got, w)
		}
	}
	if st.Branch != "main" || len(st.Head) != 7 || st.Upstream != "" || st.Ahead != 0 || st.Behind != 0 {
		t.Errorf("branch info = %q %q %q +%d -%d", st.Branch, st.Head, st.Upstream, st.Ahead, st.Behind)
	}
}

func TestStatusRenameThenDelete(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "1\n2\n3\n")
	r.commitAll("init")
	r.git("mv", "a.txt", "b.txt")
	r.remove("b.txt") // RD
	st := r.status()
	if len(st.Files) != 1 {
		t.Fatalf("files = %+v", st.Files)
	}
	fc := st.Files[0]
	if fc.Path != "a.txt" || fc.Kind != Deleted || fc.OldPath != "" || fc.Deleted != 3 {
		t.Fatalf("got %+v, want a.txt deleted", fc)
	}
}

func TestStatusRenameRewritten(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "q\nw\ne\nr\n")
	r.commitAll("init")
	r.git("mv", "a.txt", "b.txt")
	r.write("b.txt", "totally\ndifferent\ncontent\nhere\nnow\n")
	fc := r.file(r.status(), "b.txt")
	if fc.Kind != Renamed || fc.OldPath != "a.txt" || fc.Added != 5 || fc.Deleted != 4 {
		t.Fatalf("got %+v, want rename a.txt -> b.txt +5 -4", fc)
	}
}

func TestStatusConflict(t *testing.T) {
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
	fc := r.file(r.status(), "f.txt")
	if fc.Kind != Conflicted || !fc.Staged || !fc.Unstaged || fc.Added != 4 || fc.Deleted != 0 {
		t.Fatalf("got %+v", fc)
	}
}

func TestStatusNoCommits(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("staged.txt", "a\nb\n")
	r.git("add", "staged.txt")
	r.write("loose.txt", "c\n")
	st := r.status()
	if st.Head != "" || st.Branch != "main" {
		t.Errorf("head %q branch %q", st.Head, st.Branch)
	}
	if fc := r.file(st, "staged.txt"); fc.Kind != Added || fc.Added != 2 || !fc.Staged {
		t.Errorf("staged.txt = %+v", fc)
	}
	if fc := r.file(st, "loose.txt"); fc.Kind != Untracked || fc.Added != 1 {
		t.Errorf("loose.txt = %+v", fc)
	}
}

func TestStatusDetached(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.commitAll("init")
	oid := r.git("rev-parse", "HEAD")
	r.git("checkout", "-q", "--detach")
	st := r.status()
	if st.Branch != "" || st.Head != oid[:7] || len(st.Files) != 0 {
		t.Fatalf("got branch %q head %q files %v, want detached at %s", st.Branch, st.Head, st.Files, oid[:7])
	}
}

func TestStatusAheadBehind(t *testing.T) {
	t.Parallel()
	origin := newRepo(t)
	origin.write("a.txt", "a\n")
	origin.commitAll("init")
	clone := &testRepo{t: t, dir: filepath.Join(t.TempDir(), "clone")}
	origin.git("clone", "-q", origin.dir, clone.dir)

	clone.write("b.txt", "b\n")
	clone.commitAll("mine 1")
	clone.write("c.txt", "c\n")
	clone.commitAll("mine 2")
	origin.write("d.txt", "d\n")
	origin.commitAll("theirs")
	clone.git("fetch", "-q")

	st := clone.status()
	if st.Branch != "main" || st.Upstream != "origin/main" || st.Ahead != 2 || st.Behind != 1 {
		t.Fatalf("got %q %q +%d -%d, want main origin/main +2 -1", st.Branch, st.Upstream, st.Ahead, st.Behind)
	}
}

func TestStatusUntrackedContent(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("keep.txt", "k\n")
	r.commitAll("init")
	r.write("bin.dat", "ab\x00cd\n")
	r.write("empty.txt", "")
	r.write("crlf.txt", "a\r\nb\r\n")
	r.write("deep/er/file.txt", "1\n2\n")
	st := r.status()
	cases := map[string]struct {
		added  int
		binary bool
	}{
		"bin.dat":          {0, true},
		"empty.txt":        {0, false},
		"crlf.txt":         {2, false},
		"deep/er/file.txt": {2, false},
	}
	for path, w := range cases {
		fc := r.file(st, path)
		if fc.Kind != Untracked || fc.Added != w.added || fc.Binary != w.binary || fc.Staged || !fc.Unstaged {
			t.Errorf("%s = %+v", path, fc)
		}
	}
}

func TestStatusSpecialPaths(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("папка/файл.txt", "один\n")
	r.write("dir with space/f [1].txt", "x\n")
	r.write("[id]/page.tsx", "p\n")
	r.commitAll("init")
	r.write("папка/файл.txt", "один\nдва\n")
	r.write("dir with space/f [1].txt", "x\ny\n")
	r.write("[id]/page.tsx", "q\n")
	r.write("нове/new file.txt", "n\n")
	st := r.status()
	for _, path := range []string{"папка/файл.txt", "dir with space/f [1].txt"} {
		if fc := r.file(st, path); fc.Kind != Modified || fc.Added != 1 || fc.Deleted != 0 {
			t.Errorf("%s = %+v", path, fc)
		}
	}
	if fc := r.file(st, "[id]/page.tsx"); fc.Kind != Modified || fc.Added != 1 || fc.Deleted != 1 {
		t.Errorf("[id]/page.tsx = %+v", fc)
	}
	if fc := r.file(st, "нове/new file.txt"); fc.Kind != Untracked || fc.Added != 1 {
		t.Errorf("untracked = %+v", fc)
	}
}

func TestStatusSubmoduleAndNestedRepo(t *testing.T) {
	t.Parallel()
	lib := newRepo(t)
	lib.write("lib.txt", "1\n")
	lib.commitAll("lib")

	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.commitAll("init")
	r.git("submodule", "add", "-q", lib.dir, "sub")
	r.commitAll("add sub")
	sub := &testRepo{t: t, dir: filepath.Join(r.dir, "sub")}
	sub.write("lib.txt", "2\n")
	sub.commitAll("bump")
	initRepo(t, filepath.Join(r.dir, "nested"))

	st := r.status()
	if fc := r.file(st, "sub"); fc.Kind != Modified || !fc.Submodule || fc.Added != 0 || fc.Deleted != 0 {
		t.Errorf("sub = %+v", fc)
	}
	if fc := r.file(st, "nested"); fc.Kind != Untracked || !fc.Submodule {
		t.Errorf("nested = %+v", fc)
	}

	fd, err := GetFileDiff(testCtx(t), r.dir, r.file(st, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if len(fd.Lines) != 2 || !strings.HasPrefix(fd.Lines[0].Text, "Subproject commit ") {
		t.Errorf("submodule diff = %+v", fd.Lines)
	}
	fd, err = GetFileDiff(testCtx(t), r.dir, r.file(st, "nested"))
	if err != nil || len(fd.Lines) != 0 {
		t.Errorf("nested repo diff = %+v, %v", fd, err)
	}
}

func TestFingerprint(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.write("b.txt", "b\n")
	r.commitAll("init")
	r.write("a.txt", "a2\n")
	fp1 := r.status().Fingerprint
	if fp1 == "" || r.status().Fingerprint != fp1 {
		t.Fatal("fingerprint is not stable while nothing changes")
	}

	r.write("a.txt", "a2\nmore\n") // already modified, edited again
	fp2 := r.status().Fingerprint
	if fp2 == fp1 {
		t.Fatal("fingerprint did not change after re-editing a modified file")
	}

	r.write("a.txt", "a3\nmore\n") // same size: only the mtime moves
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(filepath.Join(r.dir, "a.txt"), later, later); err != nil {
		t.Fatal(err)
	}
	fp3 := r.status().Fingerprint
	if fp3 == fp2 {
		t.Fatal("fingerprint did not change after a same-size edit")
	}

	r.git("add", "a.txt") // only the index changes
	if r.status().Fingerprint == fp3 {
		t.Fatal("fingerprint did not change after staging")
	}
	r.write("u.txt", "u\n")
	fp4 := r.status().Fingerprint
	r.write("u.txt", "u\nv\n")
	if r.status().Fingerprint == fp4 {
		t.Fatal("fingerprint did not change after editing an untracked file")
	}
}

func TestStatusNotARepo(t *testing.T) {
	t.Parallel()
	if _, err := GetStatus(testCtx(t), t.TempDir()); err == nil {
		t.Fatal("want an error outside a repository")
	}
}

// TestTiming measures GetStatus, GetFileDiff and Patch on a real repository
// named by REVUE_TIMING_REPO (all read-only); it is skipped otherwise.
func TestTiming(t *testing.T) {
	repo := os.Getenv("REVUE_TIMING_REPO")
	if repo == "" {
		t.Skip("REVUE_TIMING_REPO not set")
	}
	// The real repo should see the user's configuration (fscache, etc.).
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	os.Unsetenv("GIT_CONFIG_GLOBAL")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "")
	os.Unsetenv("GIT_CONFIG_NOSYSTEM")
	var st Status
	for i := range 3 {
		start := time.Now()
		var err error
		if st, err = GetStatus(testCtx(t), repo); err != nil {
			t.Fatal(err)
		}
		t.Logf("GetStatus run %d: %v, branch %q, %d files", i+1, time.Since(start), st.Branch, len(st.Files))
	}
	for _, fc := range st.Files {
		start := time.Now()
		fd, err := GetFileDiff(testCtx(t), repo, fc)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("GetFileDiff %s: %v, %d lines", fc.Path, time.Since(start), len(fd.Lines))
	}
	start := time.Now()
	p, err := Patch(testCtx(t), repo, st.Files, 200<<10)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Patch: %v, %d bytes", time.Since(start), len(p))
}

// checkCountsMatchDiff checks that the sidebar's +/- counts of every entry
// agree with the lines GetFileDiff shows for it. Copies are left out: their
// counts are against nothing (numstat -M) while the diff is against the
// source.
func checkCountsMatchDiff(t *testing.T, r *testRepo, st Status) {
	t.Helper()
	for _, fc := range st.Files {
		if fc.Binary || fc.Submodule || fc.Kind == Copied {
			continue
		}
		fd, err := GetFileDiff(testCtx(t), r.dir, fc)
		if err != nil {
			t.Fatalf("GetFileDiff(%s): %v", fc.Path, err)
		}
		if fd.Binary || fd.TooLarge {
			continue
		}
		if added, deleted := fd.Stats(); added != fc.Added || deleted != fc.Deleted {
			t.Errorf("%s (%s): status says +%d -%d, diff shows +%d -%d", fc.Path, fc.Kind.Letter(), fc.Added, fc.Deleted, added, deleted)
		}
	}
}

// TestStatusRenameStatusDoesNotSee covers renames that git diff HEAD -M
// pairs up but git status does not report as one: rename detection turned
// off for status, a staged deletion plus a staged file that only resembles
// the deleted one in the working tree, and a rename made on disk and
// registered with "git add -N". The deleted file must still be listed, and
// the new one counted as all added, the way GetFileDiff shows them.
func TestStatusRenameStatusDoesNotSee(t *testing.T) {
	t.Parallel()
	check := func(t *testing.T, r *testRepo, newAdded, oldDeleted int) {
		t.Helper()
		st := r.status()
		if fc := r.file(st, "a.txt"); fc.Kind != Deleted || fc.Deleted != oldDeleted || fc.Added != 0 {
			t.Errorf("a.txt = %+v, want deleted -%d", fc, oldDeleted)
		}
		if fc := r.file(st, "b.txt"); fc.Kind != Added || fc.Added != newAdded || fc.Deleted != 0 || fc.OldPath != "" {
			t.Errorf("b.txt = %+v, want added +%d", fc, newAdded)
		}
		checkCountsMatchDiff(t, r, st)
	}
	t.Run("status.renames=false", func(t *testing.T) {
		t.Parallel()
		r := newRepo(t)
		r.write("a.txt", "1\n2\n3\n4\n5\n")
		r.commitAll("init")
		r.git("config", "status.renames", "false")
		r.git("mv", "a.txt", "b.txt")
		r.write("b.txt", "1\n2\n3\n4\n5\n6\n")
		check(t, r, 6, 5)
	})
	t.Run("similar only in the working tree", func(t *testing.T) {
		t.Parallel()
		r := newRepo(t)
		r.write("a.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
		r.commitAll("init")
		r.git("rm", "-q", "a.txt")
		r.write("b.txt", "zzz\n")
		r.git("add", "b.txt")
		r.write("b.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n")
		check(t, r, 9, 8)
	})
	t.Run("intent to add", func(t *testing.T) {
		t.Parallel()
		r := newRepo(t)
		r.write("a.txt", "1\n2\n3\n4\n")
		r.commitAll("init")
		r.git("rm", "-q", "--cached", "a.txt")
		if err := os.Rename(filepath.Join(r.dir, "a.txt"), filepath.Join(r.dir, "b.txt")); err != nil {
			t.Fatal(err)
		}
		r.git("add", "-N", "b.txt")
		check(t, r, 4, 4)
	})
}

// TestStatusRemovedThenIntentToAdd: "git rm --cached" followed by
// "git add -N" gives "DA". HEAD and the working tree both have the file, so
// the change is a modification, not a new file.
func TestStatusRemovedThenIntentToAdd(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "1\n2\n3\n")
	r.commitAll("init")
	r.git("rm", "-q", "--cached", "a.txt")
	r.write("a.txt", "1\n2\nX\n")
	r.git("add", "-N", "a.txt")
	st := r.status()
	if fc := r.file(st, "a.txt"); fc.Kind != Modified || fc.Added != 1 || fc.Deleted != 1 {
		t.Errorf("a.txt = %+v, want modified +1 -1", fc)
	}
	checkCountsMatchDiff(t, r, st)
}

// TestFingerprintRacingEdit: a file rewritten while GetStatus runs can be
// counted by git before the rewrite and stat'ed after it. That snapshot must
// not share its fingerprint with the next one, which counts the new
// content, or a poller keeps the stale counts. One fingerprint seen with two
// different counts is that bug.
func TestFingerprintRacingEdit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "base\n")
	r.commitAll("init")
	full := filepath.Join(r.dir, "f.txt")
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		for i := 1; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			// A write can lose to git holding the file open; the next
			// one goes through.
			_ = os.WriteFile(full, []byte(strings.Repeat("x\n", 1+i%7)), 0o644)
			time.Sleep(time.Duration(i*37%120) * time.Millisecond)
		}
	}()
	defer func() {
		close(done)
		<-stopped
	}()
	seen := make(map[string]int)
	for end := time.Now().Add(4 * time.Second); time.Now().Before(end); {
		st, err := GetStatus(testCtx(t), r.dir)
		if err != nil {
			// git can fail on a file that changes size under it ("mmap
			// failed"); such a poll yields no snapshot to compare.
			t.Logf("GetStatus: %v", err)
			continue
		}
		if len(st.Files) != 1 {
			continue
		}
		n := st.Files[0].Added
		if prev, ok := seen[st.Fingerprint]; ok && prev != n {
			t.Errorf("fingerprint %s came with +%d and with +%d", st.Fingerprint, prev, n)
		}
		seen[st.Fingerprint] = n
	}
}

// TestFingerprintRacy pins down the rule TestFingerprintRacingEdit relies on.
func TestFingerprintRacy(t *testing.T) {
	t.Parallel()
	since := time.Now()
	until := since.Add(100 * time.Millisecond)
	raw := []byte("1 .M N... 100644 100644 100644 a a f.txt\x00")
	files := []FileChange{{Path: "f.txt"}}
	at := func(mtime time.Time) string {
		return fingerprint(raw, files, []diskState{{present: true, size: 3, mtime: mtime.UnixNano()}}, since, until)
	}
	for name, mtime := range map[string]time.Time{
		"written before":        since.Add(-time.Second),
		"written as it started": since,
		"future (clock skew)":   until.Add(time.Hour),
	} {
		if at(mtime) != at(mtime) {
			t.Errorf("%s: fingerprint not stable", name)
		}
	}
	if at(since.Add(time.Millisecond)) == at(since.Add(time.Millisecond)) {
		t.Error("written during the run: fingerprint repeats")
	}
}

// TestStatusRecordLikePaths: with -z, a rename's original path is a record
// of its own; names that look like porcelain records or headers must not be
// read as one.
func TestStatusRecordLikePaths(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("# branch.oid x", "1\n2\n3\n")
	r.write("2 R. N... y", "4\n5\n6\n")
	r.commitAll("init")
	r.git("mv", "# branch.oid x", "1 .M N... renamed")
	r.git("mv", "2 R. N... y", "u UU z")
	r.write("u UU z", "4\n5\n6\n7\n")
	r.write("# branch.head new", "n\n")
	st := r.status()
	if fc := r.file(st, "1 .M N... renamed"); fc.Kind != Renamed || fc.OldPath != "# branch.oid x" || fc.Added != 0 || fc.Deleted != 0 {
		t.Errorf("first rename = %+v", fc)
	}
	if fc := r.file(st, "u UU z"); fc.Kind != Renamed || fc.OldPath != "2 R. N... y" || fc.Added != 1 || fc.Deleted != 0 {
		t.Errorf("second rename = %+v", fc)
	}
	if fc := r.file(st, "# branch.head new"); fc.Kind != Untracked || fc.Added != 1 {
		t.Errorf("untracked = %+v", fc)
	}
	if len(st.Files) != 3 || st.Branch != "main" {
		t.Errorf("files %+v, branch %q", st.Files, st.Branch)
	}
	checkCountsMatchDiff(t, r, st)
}
