package sidebar

import (
	"sort"
	"strings"

	"github.com/DavidkJun/revue/internal/git"
)

type nodeKind uint8

const (
	repoNode nodeKind = iota
	dirNode
	fileNode
)

// node is one row of the tree. Directory nodes may stand for a compacted
// chain of single-child directories, in which case label holds the whole
// chain ("src/app/core") and path the deepest directory.
type node struct {
	kind     nodeKind
	repo     int    // index into Model.repos
	path     string // repo-relative, forward slashes; "" for the repo node
	label    string // what the row shows
	depth    int    // 0 for repos
	parent   *node
	children []*node
	file     *git.FileChange // file nodes only
	id       string          // identity across SetRepos, see Model.id
	// merged holds the paths of the directories compacted into this one,
	// shallowest first; aliases are their identities. A directory keeps its
	// cursor and collapsed state when it is merged with its only child.
	merged  []string
	aliases []string
}

// buildRepo turns a repo's changed files into a tree rooted at a repo node.
func buildRepo(idx int, files []git.FileChange) *node {
	root := &node{kind: repoNode, repo: idx}
	dirs := map[string]*node{"": root}
	for i := range files {
		f := &files[i]
		if f.Path == "" {
			continue
		}
		dir, base := splitPath(f.Path)
		parent := ensureDir(dirs, idx, dir)
		parent.children = append(parent.children, &node{
			kind:  fileNode,
			repo:  idx,
			path:  f.Path,
			label: base,
			file:  f,
		})
	}
	compact(root)
	finish(root, nil, 0)
	return root
}

func ensureDir(dirs map[string]*node, repo int, dir string) *node {
	if n, ok := dirs[dir]; ok {
		return n
	}
	parentDir, base := splitPath(dir)
	parent := ensureDir(dirs, repo, parentDir)
	n := &node{kind: dirNode, repo: repo, path: dir, label: base}
	parent.children = append(parent.children, n)
	dirs[dir] = n
	return n
}

// compact merges every directory that holds nothing but one other directory
// into that child, so deep single-file paths take one row instead of five.
func compact(n *node) {
	for i, c := range n.children {
		if c.kind != dirNode {
			continue
		}
		for len(c.children) == 1 && c.children[0].kind == dirNode {
			only := c.children[0]
			only.label = c.label + "/" + only.label
			only.merged = append(c.merged, c.path)
			c = only
		}
		n.children[i] = c
		compact(c)
	}
}

// finish sorts children (directories first, then files, case-insensitively)
// and fills in parent links and depths.
func finish(n, parent *node, depth int) {
	n.parent = parent
	n.depth = depth
	sort.SliceStable(n.children, func(i, j int) bool {
		a, b := n.children[i], n.children[j]
		if (a.kind == dirNode) != (b.kind == dirNode) {
			return a.kind == dirNode
		}
		na, nb := sortName(a), sortName(b)
		la, lb := strings.ToLower(na), strings.ToLower(nb)
		if la != lb {
			return la < lb
		}
		return na < nb
	})
	for _, c := range n.children {
		finish(c, n, depth+1)
	}
}

// sortName is what a node sorts by: a compacted chain ("app/core") sorts by
// its first directory, so merging does not move it past siblings such as
// "app-shell" ('-' sorts before '/').
func sortName(n *node) string {
	name, _, _ := strings.Cut(n.label, "/")
	return name
}

// walk visits n and everything below it in display order.
func walk(n *node, visit func(*node)) {
	visit(n)
	for _, c := range n.children {
		walk(c, visit)
	}
}

// filesUnder lists the file nodes below n (n itself if it is a file).
func filesUnder(n *node) []*node {
	var out []*node
	walk(n, func(c *node) {
		if c.kind == fileNode {
			out = append(out, c)
		}
	})
	return out
}

func splitPath(p string) (dir, base string) {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i], p[i+1:]
	}
	return "", p
}
