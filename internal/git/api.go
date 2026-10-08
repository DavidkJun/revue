// Package git reads repository state for the viewer: discovery of repos
// under a folder, per-repo status, and whole-file diffs. It never writes to a
// repository (no optional locks, no index refresh).
//
// The entry points are Discover (discover.go), GetStatus (status.go),
// GetFileDiff (filediff.go) and Patch (patch.go).
package git

// Repo is one discovered repository.
type Repo struct {
	Name string // path relative to the scan root, forward slashes; the folder name when the root is itself in a repo
	Path string // absolute working-tree root, OS-native separators
}

// Kind is the kind of change a file has relative to HEAD.
type Kind uint8

const (
	Modified Kind = iota
	Added         // new file in the index
	Deleted
	Renamed
	Copied
	Untracked
	TypeChanged
	Conflicted
)

// Letter is the one-letter status shown in the sidebar.
func (k Kind) Letter() string {
	letters := [...]string{"M", "A", "D", "R", "C", "?", "T", "U"}
	if int(k) >= len(letters) {
		return " "
	}
	return letters[k]
}

// FileChange is one changed file of a repo.
type FileChange struct {
	Path     string // repo-relative, forward slashes; the new path for renames
	OldPath  string // previous path for renames/copies, "" otherwise
	Kind     Kind
	Staged   bool // the index differs from HEAD
	Unstaged bool // the working tree differs from the index (always true for untracked)
	Added    int  // lines added vs HEAD; 0 when Binary
	Deleted  int  // lines deleted vs HEAD; 0 when Binary
	Binary   bool
	// Submodule marks a submodule, or with Kind Untracked a nested repository
	// git does not look into. Neither has lines of its own, so Added and
	// Deleted stay 0.
	Submodule bool
}

// Status is a snapshot of one repo's uncommitted changes.
type Status struct {
	Branch   string // current branch, "" when detached
	Head     string // short commit hash, "" for a repo without commits
	Upstream string
	Ahead    int
	Behind   int
	Files    []FileChange // sorted by Path
	// Fingerprint changes whenever the set of changes or the content of any
	// changed file changes, so polling can skip work when nothing moved. A
	// snapshot taken while a changed file was being written gets a
	// fingerprint of its own, so the next poll is never skipped as a repeat.
	Fingerprint string
}

// MaxFileBytes is the largest file GetFileDiff renders; bigger files come
// back with TooLarge set.
const MaxFileBytes = 4 << 20
