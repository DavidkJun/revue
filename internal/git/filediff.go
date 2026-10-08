package git

import (
	"cmp"
	"context"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strconv"

	"github.com/DavidkJun/revue/internal/diff"
)

// maxDiffOutput bounds git diff output for one file: both sides within
// MaxFileBytes plus a prefix byte per line and the headers.
const maxDiffOutput = 4*MaxFileBytes + 64<<10

// GetFileDiff returns the whole-file diff of fc between HEAD and the working
// tree (staged and unstaged changes combined): every line of the file, with
// unchanged lines as Context. Untracked files are all Added, deleted files all
// Deleted. Binary or oversized files come back with Binary/TooLarge set.
func GetFileDiff(ctx context.Context, repoPath string, fc FileChange) (*diff.FileDiff, error) {
	switch {
	case fc.Submodule && fc.Kind == Untracked:
		// A nested repository has no content of its own here.
		return &diff.FileDiff{NewPath: fc.Path}, nil
	case fc.Submodule:
		return gitFileDiff(ctx, repoPath, fc)
	case fc.Kind == Untracked || fc.Kind == Added:
		return newFileDiff(repoPath, fc.Path)
	case fc.Kind == Deleted:
		return deletedFileDiff(ctx, repoPath, fc.Path)
	}
	return gitFileDiff(ctx, repoPath, fc)
}

// newFileDiff shows a file HEAD does not have, straight from disk.
func newFileDiff(repo, path string) (*diff.FileDiff, error) {
	fd := &diff.FileDiff{NewPath: path}
	data, tooLarge, err := readWorktree(repo, path)
	if err != nil {
		return nil, err
	}
	switch {
	case tooLarge:
		fd.TooLarge = true
	case isBinary(data):
		fd.Binary = true
	default:
		fd.Lines = wholeFile(data, diff.Added)
	}
	return fd, nil
}

// deletedFileDiff shows HEAD's version of a file that is gone.
func deletedFileDiff(ctx context.Context, repo, path string) (*diff.FileDiff, error) {
	fd := &diff.FileDiff{OldPath: path}
	var data []byte
	large, err := withHeadSizeCheck(ctx, repo, path, func(ctx context.Context) (err error) {
		data, _, err = runGitCapped(ctx, repo, MaxFileBytes+1, "cat-file", "blob", "HEAD:"+path)
		return err
	})
	switch {
	case err != nil:
		return nil, err
	case large || len(data) > MaxFileBytes:
		fd.TooLarge = true
	case isBinary(data):
		fd.Binary = true
	default:
		fd.Lines = wholeFile(data, diff.Deleted)
	}
	return fd, nil
}

// gitFileDiff asks git for a diff with more context than the file has lines,
// which yields the whole file as one hunk.
func gitFileDiff(ctx context.Context, repo string, fc FileChange) (*diff.FileDiff, error) {
	fd := &diff.FileDiff{OldPath: fc.Path, NewPath: fc.Path}
	switch {
	case fc.Kind == Added:
		fd.OldPath = ""
	case fc.Kind == Deleted:
		fd.NewPath = ""
	case fc.OldPath != "":
		fd.OldPath = fc.OldPath
	}
	if !fc.Submodule {
		if info, err := os.Lstat(diskPath(repo, fc.Path)); err == nil && info.Size() > MaxFileBytes {
			fd.TooLarge = true
			return fd, nil
		}
	}

	var out []byte
	var cut bool
	run := func(ctx context.Context) (err error) {
		out, cut, err = runAgainstHead(ctx, repo, maxDiffOutput, func(base string) []string {
			return fileDiffArgs(base, fc)
		})
		return err
	}
	var large bool
	var err error
	if fc.Submodule {
		err = run(ctx) // a submodule's content is a commit id: nothing to measure
	} else {
		large, err = withHeadSizeCheck(ctx, repo, cmp.Or(fc.OldPath, fc.Path), run)
	}
	if err != nil {
		return nil, err
	}
	// cut means the file grew past the limit after it was measured.
	if large || cut {
		fd.TooLarge = true
		return fd, nil
	}

	secs := pickSections(parseUnified(out), fc)
	hunks := false
	for _, s := range secs {
		if s.binary {
			fd.Binary = true
			return fd, nil
		}
		hunks = hunks || s.hunks
	}
	if !hunks {
		return unchangedFileDiff(repo, fc, fd)
	}
	for _, s := range secs {
		fd.Lines = append(fd.Lines, s.lines...)
	}
	return fd, nil
}

func fileDiffArgs(base string, fc FileChange) []string {
	args := []string{
		"diff", "--no-color", "--no-ext-diff", "--no-textconv",
		// Parsing relies on the standard prefixes whatever diff.noprefix or
		// diff.mnemonicPrefix say.
		"--src-prefix=a/", "--dst-prefix=b/",
		// A file within MaxFileBytes cannot have more lines than this.
		"--unified=" + strconv.Itoa(MaxFileBytes+1),
	}
	if fc.Kind == Copied {
		// An unmodified copy source is only considered when looking harder.
		args = append(args, "-C", "--find-copies-harder")
	} else {
		args = append(args, "-M")
	}
	args = append(args, base, "--")
	if fc.OldPath != "" {
		args = append(args, fc.OldPath)
	}
	return append(args, fc.Path)
}

// withHeadSizeCheck runs fetch while git measures HEAD's version of path;
// when that is bigger than MaxFileBytes fetch is cancelled and tooLarge is
// true. Running both at once saves a process start of latency per file.
func withHeadSizeCheck(ctx context.Context, repo, path string, fetch func(context.Context) error) (tooLarge bool, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	large := make(chan bool, 1)
	go func() {
		// Without a HEAD version (both sides of a conflict added the file,
		// or there are no commits) there is nothing to measure.
		size, err := blobSize(ctx, repo, "HEAD:"+path)
		over := err == nil && size > MaxFileBytes
		if over {
			cancel()
		}
		large <- over
	}()
	err = fetch(ctx)
	if <-large {
		return true, nil
	}
	return false, err
}

// pickSections keeps the sections of git's output that belong to fc. git
// splits a type change, or a rename it does not recognise once the working
// tree is compared, into a deletion and an addition; a copy diff also lists
// the copy's source. The deletion comes first so old lines precede new ones.
func pickSections(secs []section, fc FileChange) []section {
	if len(secs) <= 1 {
		return secs
	}
	var picked []section
	for _, s := range secs {
		ownDeletion := s.deleted && s.oldPath != "" && (s.oldPath == fc.Path || s.oldPath == fc.OldPath)
		if s.newPath == fc.Path || ownDeletion {
			picked = append(picked, s)
		}
	}
	if len(picked) == 0 {
		return secs
	}
	slices.SortStableFunc(picked, func(a, b section) int {
		switch {
		case a.deleted == b.deleted:
			return 0
		case a.deleted:
			return -1
		}
		return 1
	})
	return picked
}

// unchangedFileDiff handles a change without hunks (a mode change, a pure
// rename): the whole file, unchanged.
func unchangedFileDiff(repo string, fc FileChange, fd *diff.FileDiff) (*diff.FileDiff, error) {
	if fc.Submodule {
		return fd, nil
	}
	data, large, err := readWorktree(repo, fc.Path)
	if errors.Is(err, fs.ErrNotExist) {
		// Without hunks the file is missing on both sides, as for a path
		// both sides of a merge deleted: there is nothing to show.
		return fd, nil
	}
	if err != nil {
		return nil, err
	}
	switch {
	case large:
		fd.TooLarge = true
	case isBinary(data):
		fd.Binary = true
	default:
		fd.Lines = wholeFile(data, diff.Context)
	}
	return fd, nil
}
