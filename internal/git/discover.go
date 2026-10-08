package git

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// discoverTimeout bounds the one git call Discover makes; it has no context
// of its own.
const discoverTimeout = 10 * time.Second

// Discover returns the repositories to review. If root is inside a git
// repository, that repository alone is returned. Otherwise the folders below
// root are scanned up to maxDepth levels (skipping node_modules and hidden
// folders, never descending into a found repo), sorted by Name.
func Discover(root string, maxDepth int) ([]Repo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", abs)
	}

	top, err := toplevel(abs)
	if err != nil {
		return nil, err
	}
	if top != "" {
		return []Repo{{Name: filepath.Base(top), Path: top}}, nil
	}

	var repos []Repo
	var walk func(dir string, depth int) error
	walk = func(dir string, depth int) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			// IsDir is false for symlinks and for junctions (name-surrogate
			// reparse points), so links are never followed.
			name := e.Name()
			if !e.IsDir() || name == "node_modules" || strings.HasPrefix(name, ".") {
				continue
			}
			path := filepath.Join(dir, name)
			if isRepo(path) {
				rel, err := filepath.Rel(abs, path)
				if err != nil {
					continue
				}
				repos = append(repos, Repo{Name: filepath.ToSlash(rel), Path: path})
				continue
			}
			if depth < maxDepth {
				_ = walk(path, depth+1) // an unreadable subfolder is just skipped
			}
		}
		return nil
	}
	if maxDepth > 0 {
		if err := walk(abs, 1); err != nil {
			return nil, err
		}
	}
	slices.SortFunc(repos, func(a, b Repo) int { return cmp.Compare(a.Name, b.Name) })
	return repos, nil
}

// toplevel returns the root of the work tree containing dir, or "" when dir
// is not inside one.
func toplevel(dir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), discoverTimeout)
	defer cancel()
	out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	top := strings.TrimSpace(string(out))
	if top == "" {
		return "", nil
	}
	return filepath.FromSlash(top), nil
}

// isRepo reports whether dir is the root of a work tree. ".git" is a file
// for linked worktrees and submodules.
func isRepo(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}
