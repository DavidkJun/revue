package git

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"sync"
	"time"
)

// inspectWorkers bounds how many changed files are examined on disk at once.
const inspectWorkers = 8

// GetStatus reads the uncommitted changes of the repo at repoPath
// (HEAD → working tree, untracked files included individually).
func GetStatus(ctx context.Context, repoPath string) (Status, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	start := time.Now()

	type result struct {
		out []byte
		err error
	}
	numstat := make(chan result, 1)
	go func() {
		out, _, err := runAgainstHead(ctx, repoPath, 0, func(base string) []string {
			return numstatArgs(base, "-M", nil)
		})
		numstat <- result{out, err}
	}()

	raw, err := runGit(ctx, repoPath, "status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all")
	if err != nil {
		return Status{}, err
	}
	p := parsePorcelain(raw)
	disk := inspectAll(repoPath, p.files)
	res := <-numstat
	if res.err != nil {
		return Status{}, res.err
	}

	counts := parseNumstat(res.out)
	if err := recountUnpaired(ctx, repoPath, counts, p.files); err != nil {
		return Status{}, err
	}
	files := make([]FileChange, 0, len(p.files))
	tracked := make(map[string]bool)
	for _, fc := range p.files {
		if fc.Kind == Untracked {
			continue
		}
		if !fc.Submodule {
			c, ok := countsFor(counts, fc)
			if !ok && fc.Kind != Conflicted {
				continue // e.g. a staged edit undone on disk: HEAD and the working tree agree
			}
			fc.Added, fc.Deleted, fc.Binary = c.added, c.deleted, c.binary
		}
		tracked[fc.Path] = true
		files = append(files, fc)
	}
	for i, fc := range p.files {
		// "git rm --cached" leaves the file on disk, so status lists it as
		// both deleted and untracked; the deletion is the change to review.
		if fc.Kind != Untracked || tracked[fc.Path] {
			continue
		}
		fc.Added, fc.Binary = disk[i].lines, disk[i].binary
		files = append(files, fc)
	}
	slices.SortFunc(files, func(a, b FileChange) int { return cmp.Compare(a.Path, b.Path) })

	return Status{
		Branch:      p.branch,
		Head:        p.oid[:min(len(p.oid), 7)],
		Upstream:    p.upstream,
		Ahead:       p.ahead,
		Behind:      p.behind,
		Files:       files,
		Fingerprint: fingerprint(raw, p.files, disk, start, time.Now()),
	}, nil
}

// recountUnpaired replaces the counts of renames only numstat found with
// counts that keep both sides apart, the way status lists them.
func recountUnpaired(ctx context.Context, repo string, counts map[string]numstatEntry, files []FileChange) error {
	for _, paths := range pathChunks(unpairedRenames(counts, files)) {
		out, _, err := runAgainstHead(ctx, repo, 0, func(base string) []string {
			return numstatArgs(base, "--no-renames", paths)
		})
		if err != nil {
			return err
		}
		for _, p := range paths {
			delete(counts, p)
		}
		for path, c := range parseNumstat(out) {
			counts[path] = c
		}
	}
	return nil
}

// numstatArgs builds the line count query against base; renames is -M or
// --no-renames.
func numstatArgs(base, renames string, paths []string) []string {
	args := []string{"diff", "--numstat", "-z", renames, "--no-textconv", base, "--"}
	return append(args, paths...)
}

// diskState is what GetStatus learns about a changed path on disk.
type diskState struct {
	present bool
	size    int64
	mtime   int64
	lines   int // untracked files only
	binary  bool
}

// inspectAll stats every changed path and counts the lines of untracked
// files, a few files at a time.
func inspectAll(repo string, files []FileChange) []diskState {
	states := make([]diskState, len(files))
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(inspectWorkers, len(files)) {
		wg.Go(func() {
			for i := range next {
				states[i] = inspect(repo, files[i])
			}
		})
	}
	for i := range files {
		next <- i
	}
	close(next)
	wg.Wait()
	return states
}

func inspect(repo string, fc FileChange) diskState {
	full := diskPath(repo, fc.Path)
	info, err := os.Lstat(full)
	if err != nil {
		return diskState{}
	}
	st := diskState{present: true, size: info.Size(), mtime: info.ModTime().UnixNano()}
	if fc.Kind == Untracked && !fc.Submodule && !info.IsDir() {
		st.lines, st.binary = countFile(full, info)
	}
	return st
}

// fingerprint hashes the raw status, which carries HEAD's id and every
// entry's state, plus the size and mtime of each changed path on disk:
// status alone stays the same when an already-modified file is edited again.
//
// A path written between since and until (while GetStatus ran) may have been
// counted by git before the write and stat'ed after it. Such a snapshot gets
// a fingerprint nothing else shares, so the next one, which sees the settled
// file, differs from it and is not skipped as a repeat.
func fingerprint(raw []byte, files []FileChange, disk []diskState, since, until time.Time) string {
	h := sha256.New()
	h.Write(raw)
	racy := false
	for i, d := range disk {
		if d.present {
			fmt.Fprintf(h, "\x00%s\x00%d\x00%d", files[i].Path, d.size, d.mtime)
			racy = racy || (d.mtime > since.UnixNano() && d.mtime <= until.UnixNano())
		}
	}
	if racy {
		fmt.Fprintf(h, "\x00racy\x00%d", rand.Uint64())
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}
