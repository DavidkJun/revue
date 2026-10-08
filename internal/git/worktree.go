package git

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/DavidkJun/revue/internal/diff"
)

// binaryProbe is how much of a file git looks at for a NUL byte when it
// decides whether the file is binary.
const binaryProbe = 8000

func isBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), binaryProbe)], 0) >= 0
}

// lineCount counts lines the way git does: a last line without a newline
// still counts, an empty file has none.
func lineCount(data []byte) int {
	n := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// splitLines splits file content into lineCount(data) lines, without "\n"
// or "\r\n".
func splitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// wholeFile turns file content into diff lines that are all of one kind.
func wholeFile(data []byte, kind diff.LineKind) []diff.Line {
	texts := splitLines(data)
	lines := make([]diff.Line, len(texts))
	for i, text := range texts {
		l := diff.Line{Kind: kind, Text: text}
		if kind != diff.Added {
			l.OldNo = i + 1
		}
		if kind != diff.Deleted {
			l.NewNo = i + 1
		}
		lines[i] = l
	}
	return lines
}

func diskPath(repo, path string) string {
	return filepath.Join(repo, filepath.FromSlash(path))
}

// readWorktree reads a working-tree file unless it is bigger than
// MaxFileBytes. A symlink reads as its target, which is what git stores.
func readWorktree(repo, path string) (data []byte, tooLarge bool, err error) {
	full := diskPath(repo, path)
	info, err := os.Lstat(full)
	if err != nil {
		return nil, false, err
	}
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, err := os.Readlink(full)
		return []byte(filepath.ToSlash(target)), false, err
	case info.IsDir():
		return nil, false, fmt.Errorf("%s is a directory", path)
	case info.Size() > MaxFileBytes:
		return nil, true, nil
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	// One byte over the limit tells a file that grew since Lstat.
	data, err = io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return nil, false, err
	}
	if len(data) > MaxFileBytes {
		return nil, true, nil
	}
	return data, false, nil
}

// countFile counts the lines of an untracked file for the sidebar. It reads
// at most MaxFileBytes, so for a bigger file the count is a lower bound.
func countFile(full string, info fs.FileInfo) (lines int, binary bool) {
	if info.Mode()&fs.ModeSymlink != 0 {
		return 1, false // git counts a symlink as its one-line target
	}
	f, err := os.Open(full)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	data, _ := io.ReadAll(io.LimitReader(f, MaxFileBytes))
	if isBinary(data) {
		return 0, true
	}
	return lineCount(data), false
}
