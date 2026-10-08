package git

import (
	"strconv"
	"strings"
)

// porcelain is the parsed output of
// git status --porcelain=v2 -z --branch --untracked-files=all.
type porcelain struct {
	oid      string // "" before the first commit
	branch   string // "" when detached
	upstream string
	ahead    int
	behind   int
	files    []FileChange // in git's order; tracked entries first
}

func parsePorcelain(out []byte) porcelain {
	var p porcelain
	recs := strings.Split(string(out), "\x00")
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if len(rec) < 2 {
			continue
		}
		switch rec[0] {
		case '#':
			p.header(rec[2:])
		case '1':
			if f := strings.SplitN(rec, " ", 9); len(f) == 9 {
				p.add(ordinaryChange(f[1], f[2], f[8]))
			}
		case '2':
			// The original path is the next NUL-separated record, even when
			// this one is malformed.
			if i+1 >= len(recs) {
				continue
			}
			i++
			if f := strings.SplitN(rec, " ", 10); len(f) == 10 {
				p.add(renameChange(f[1], f[2], f[9], recs[i]))
			}
		case 'u':
			if f := strings.SplitN(rec, " ", 11); len(f) == 11 {
				p.add(conflictChange(f[1], f[2], f[10]))
			}
		case '?':
			p.files = append(p.files, untrackedChange(rec[2:]))
		}
	}
	return p
}

func (p *porcelain) header(h string) {
	key, val, _ := strings.Cut(h, " ")
	switch key {
	case "branch.oid":
		if val != "(initial)" {
			p.oid = val
		}
	case "branch.head":
		if val != "(detached)" {
			p.branch = val
		}
	case "branch.upstream":
		p.upstream = val
	case "branch.ab":
		ahead, behind, _ := strings.Cut(val, " ")
		p.ahead, _ = strconv.Atoi(strings.TrimPrefix(ahead, "+"))
		p.behind, _ = strconv.Atoi(strings.TrimPrefix(behind, "-"))
	}
}

func (p *porcelain) add(fc FileChange, ok bool) {
	if ok {
		p.files = append(p.files, fc)
	}
}

// ordinaryChange describes a "1" entry as a HEAD → working tree change. X is
// HEAD vs index and Y index vs working tree, so the combined change is
// decided by whether the file exists in HEAD and on disk. ok is false when it
// exists in neither.
func ordinaryChange(xy, sub, path string) (fc FileChange, ok bool) {
	if len(xy) != 2 {
		return fc, false
	}
	x, y := xy[0], xy[1]
	fc = FileChange{Path: path, Staged: x != '.', Unstaged: y != '.', Submodule: isSubmodule(sub)}
	switch {
	case x == 'A' && y == 'D':
		return fc, false
	case x == 'D' && y == 'A':
		// Removed from the index and re-added with "git add -N": HEAD and
		// the working tree both have the file.
		fc.Kind = Modified
	case x == 'A' || y == 'A': // y == 'A' is an intent-to-add entry
		fc.Kind = Added
	case x == 'D' || y == 'D':
		fc.Kind = Deleted
	case x == 'T' || y == 'T':
		fc.Kind = TypeChanged
	default:
		fc.Kind = Modified
	}
	return fc, true
}

// renameChange describes a "2" entry (rename or copy in the index or, for
// intent-to-add files, in the working tree).
func renameChange(xy, sub, path, orig string) (fc FileChange, ok bool) {
	if len(xy) != 2 {
		return fc, false
	}
	x, y := xy[0], xy[1]
	fc = FileChange{Path: path, Staged: x != '.', Unstaged: y != '.', Submodule: isSubmodule(sub)}
	switch {
	case y == 'D' && x == 'C':
		// The copy is gone from disk again; its source is reported on its own.
		return fc, false
	case y == 'D':
		// Renamed in the index, then deleted from disk: seen from HEAD the
		// original file is simply gone.
		fc.Path, fc.Kind = orig, Deleted
	case x == 'C' || y == 'C':
		fc.Kind, fc.OldPath = Copied, orig
	default:
		fc.Kind, fc.OldPath = Renamed, orig
	}
	return fc, true
}

// conflictChange describes a "u" (unmerged) entry.
func conflictChange(xy, sub, path string) (fc FileChange, ok bool) {
	if len(xy) != 2 {
		return fc, false
	}
	return FileChange{
		Path:      path,
		Kind:      Conflicted,
		Staged:    xy[0] != '.',
		Unstaged:  xy[1] != '.',
		Submodule: isSubmodule(sub),
	}, true
}

// untrackedChange describes a "?" entry. git lists a nested repository as
// its folder with a trailing slash instead of looking inside it.
func untrackedChange(path string) FileChange {
	fc := FileChange{Path: path, Kind: Untracked, Unstaged: true}
	if trimmed, nested := strings.CutSuffix(path, "/"); nested {
		fc.Path, fc.Submodule = trimmed, true
	}
	return fc
}

func isSubmodule(sub string) bool { return strings.HasPrefix(sub, "S") }
