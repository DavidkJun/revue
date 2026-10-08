package git

import (
	"context"
	"fmt"
	"strings"
)

const (
	// patchContext is the number of context lines in a Patch.
	patchContext = 5
	// maxPathChars keeps one git command line well below Windows' limit of
	// 32767 characters; longer path lists are split over several runs.
	maxPathChars = 24000
	// maxNamedLeftOut bounds the list of left-out files in the cut note.
	maxNamedLeftOut = 20
)

// piece is the patch text of one file (one git diff section).
type piece struct {
	name string
	text string
}

// Patch returns a compact unified diff (a few lines of context) of the given
// changes, untracked files rendered as new-file diffs, for sending to Claude.
// Output is cut at maxBytes with a note saying what was left out; the note
// comes after the cut. maxBytes <= 0 means no limit.
func Patch(ctx context.Context, repoPath string, files []FileChange, maxBytes int) (string, error) {
	var tracked, untracked []FileChange
	for _, fc := range files {
		if fc.Kind == Untracked {
			untracked = append(untracked, fc)
		} else {
			tracked = append(tracked, fc)
		}
	}

	var pieces []piece
	gitCut := false
	if len(tracked) > 0 {
		out, cut, err := trackedPatch(ctx, repoPath, tracked, maxBytes)
		if err != nil {
			return "", err
		}
		pieces, gitCut = splitPatch(out), cut
	}
	size := 0
	for _, p := range pieces {
		size += len(p.text)
	}
	for _, fc := range untracked {
		if maxBytes > 0 && size > maxBytes {
			break // it would be left out anyway
		}
		p := piece{name: fc.Path, text: newFilePatch(repoPath, fc)}
		pieces = append(pieces, p)
		size += len(p.text)
	}
	return assemble(pieces, files, gitCut, maxBytes), nil
}

// trackedPatch runs git diff for the tracked changes, reading no more than
// maxBytes+1 bytes of output in total.
func trackedPatch(ctx context.Context, repo string, files []FileChange, maxBytes int) ([]byte, bool, error) {
	var out []byte
	for _, paths := range pathChunks(files) {
		limit := 0
		if maxBytes > 0 {
			if len(out) > maxBytes {
				return out, true, nil
			}
			limit = maxBytes + 1 - len(out)
		}
		chunk, cut, err := runAgainstHead(ctx, repo, limit, func(base string) []string {
			args := []string{
				"diff", fmt.Sprintf("-U%d", patchContext), "-M", "--no-color", "--no-ext-diff", "--no-textconv",
				"--src-prefix=a/", "--dst-prefix=b/", base, "--",
			}
			return append(args, paths...)
		})
		if err != nil {
			return nil, false, err
		}
		out = append(out, chunk...)
		if cut {
			return out, true, nil
		}
	}
	return out, false, nil
}

// pathChunks splits the pathspecs of files into command-line sized groups,
// keeping a rename's two paths together so git can still pair them.
func pathChunks(files []FileChange) [][]string {
	var chunks [][]string
	var cur []string
	chars := 0
	for _, fc := range files {
		paths := []string{fc.Path}
		if fc.OldPath != "" {
			paths = append(paths, fc.OldPath)
		}
		n := 0
		for _, p := range paths {
			n += len(p) + 3 // quotes and a separator on the command line
		}
		if len(cur) > 0 && chars+n > maxPathChars {
			chunks = append(chunks, cur)
			cur, chars = nil, 0
		}
		cur = append(cur, paths...)
		chars += n
	}
	if len(cur) > 0 {
		chunks = append(chunks, cur)
	}
	return chunks
}

// splitPatch splits git diff output into one piece per "diff --git" section.
// Content lines always carry a prefix, so a line starting with "diff --git "
// is always a header.
func splitPatch(out []byte) []piece {
	var pieces []piece
	s := string(out)
	for len(s) > 0 {
		end := len(s)
		if i := strings.Index(s[1:], "\ndiff --git "); i >= 0 {
			end = i + 2
		}
		text := s[:end]
		name := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
		if secs := parseUnified([]byte(text)); len(secs) > 0 {
			name = secs[0].name()
		}
		pieces = append(pieces, piece{name: name, text: text})
		s = s[end:]
	}
	return pieces
}

// newFilePatch renders an untracked file as a new-file diff, or as a single
// line when its content cannot be shown.
func newFilePatch(repo string, fc FileChange) string {
	if fc.Submodule {
		return fmt.Sprintf("Untracked directory %s/: a nested git repository, content not included\n", fc.Path)
	}
	data, large, err := readWorktree(repo, fc.Path)
	switch {
	case err != nil:
		return fmt.Sprintf("Untracked file %s: could not be read (%v)\n", fc.Path, err)
	case large:
		return fmt.Sprintf("Untracked file %s: larger than %d MB, content not included\n", fc.Path, MaxFileBytes>>20)
	case isBinary(data):
		return fmt.Sprintf("Untracked file %s: binary, content not included\n", fc.Path)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n", fc.Path, fc.Path, fc.Path)
	lines := splitLines(data)
	if len(lines) == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, "@@ -0,0 +1,%d @@\n", len(lines))
	for _, l := range lines {
		b.WriteString("+")
		b.WriteString(l)
		b.WriteString("\n")
	}
	if data[len(data)-1] != '\n' {
		b.WriteString("\\ No newline at end of file\n")
	}
	return b.String()
}

// assemble joins the pieces up to maxBytes, cutting the first piece that
// does not fit at a line boundary, and notes which files did not make it.
func assemble(pieces []piece, files []FileChange, gitCut bool, maxBytes int) string {
	var b strings.Builder
	included := make(map[string]bool)
	seen := make(map[string]bool)
	partial := ""
	cut := false
	for _, p := range pieces {
		seen[p.name] = true
		if cut {
			continue
		}
		if maxBytes <= 0 || b.Len()+len(p.text) <= maxBytes {
			b.WriteString(p.text)
			included[p.name] = true
			continue
		}
		cut = true
		room := maxBytes - b.Len()
		if i := strings.LastIndexByte(p.text[:room], '\n'); i >= 0 {
			b.WriteString(p.text[:i+1])
			partial = p.name
		} else if included[p.name] {
			partial = p.name
		}
	}
	if !cut {
		return b.String()
	}

	var left []string
	if partial != "" {
		left = append(left, partial+" (partly)")
	}
	for _, fc := range files {
		if fc.Path == partial || included[fc.Path] {
			continue
		}
		// A tracked file git printed nothing for has no changes to show,
		// unless git's output was cut before reaching it.
		if fc.Kind != Untracked && !seen[fc.Path] && !gitCut {
			continue
		}
		left = append(left, fc.Path)
	}
	if more := len(left) - maxNamedLeftOut; more > 0 {
		left = append(left[:maxNamedLeftOut], fmt.Sprintf("and %d more", more))
	}
	fmt.Fprintf(&b, "\n[patch cut at %d bytes", maxBytes)
	if len(left) > 0 {
		fmt.Fprintf(&b, "; left out: %s", strings.Join(left, ", "))
	}
	b.WriteString("]\n")
	return b.String()
}
