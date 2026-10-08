package git

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
)

// numstatEntry is one file of git diff --numstat output.
type numstatEntry struct {
	added, deleted int
	binary         bool
	from           string // rename source, "" otherwise
}

// parseNumstat parses git diff --numstat -z output keyed by (new) path. A
// plain record is "added\tdeleted\tpath\0"; a rename leaves the path empty
// and follows with "old\0new\0". Binary files count as "-\t-".
func parseNumstat(out []byte) map[string]numstatEntry {
	counts := make(map[string]numstatEntry)
	recs := strings.Split(string(out), "\x00")
	for i := 0; i < len(recs); i++ {
		f := strings.SplitN(recs[i], "\t", 3)
		if len(f) != 3 {
			continue
		}
		var c numstatEntry
		if f[0] == "-" && f[1] == "-" {
			c.binary = true
		} else {
			c.added, _ = strconv.Atoi(f[0])
			c.deleted, _ = strconv.Atoi(f[1])
		}
		path := f[2]
		if path == "" {
			if i+2 >= len(recs) {
				break
			}
			c.from, path = recs[i+1], recs[i+2]
			i += 2
		}
		if prev, seen := counts[path]; seen {
			c.added += prev.added
			c.deleted += prev.deleted
			c.binary = c.binary || prev.binary
		}
		counts[path] = c
	}
	return counts
}

// unpairedRenames lists the renames numstat found that status does not
// report. numstat compares HEAD with the working tree, status HEAD with the
// index (and may have rename detection off), so numstat can pair a deletion
// with an addition that status lists separately; their counts are then only
// valid for a rename nobody shows.
func unpairedRenames(counts map[string]numstatEntry, files []FileChange) []FileChange {
	renamed := make(map[[2]string]bool)
	for _, fc := range files {
		if fc.Kind == Renamed {
			renamed[[2]string{fc.OldPath, fc.Path}] = true
		}
	}
	var unpaired []FileChange
	for path, c := range counts {
		if c.from != "" && !renamed[[2]string{c.from, path}] {
			unpaired = append(unpaired, FileChange{Path: path, OldPath: c.from, Kind: Renamed})
		}
	}
	slices.SortFunc(unpaired, func(a, b FileChange) int { return cmp.Compare(a.Path, b.Path) })
	return unpaired
}

// countsFor finds the line counts of a tracked change; ok is false when git
// reports no difference between HEAD and the working tree for it.
func countsFor(counts map[string]numstatEntry, fc FileChange) (c numstatEntry, ok bool) {
	c, ok = counts[fc.Path]
	if fc.Kind != Renamed || c.from == fc.OldPath {
		return c, ok
	}
	// status pairs a rename in the index, numstat compares HEAD with the
	// working tree: a renamed file rewritten on disk shows up there as a
	// deletion plus an addition, and both halves belong to this change.
	if old, found := counts[fc.OldPath]; found && old.from == "" {
		c.added += old.added
		c.deleted += old.deleted
		c.binary = c.binary || old.binary
		ok = true
	}
	return c, ok
}
