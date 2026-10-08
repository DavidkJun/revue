// revue is a terminal reviewer for uncommitted git changes across one or
// more repositories, with vim navigation and quick questions to Claude.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DavidkJun/revue/internal/config"
	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/ui"
)

const usage = `usage: revue [path]

Review the uncommitted changes (HEAD → working tree, untracked files
included) of the git repository at path, or of every repository below it.
path defaults to the current folder.

Press ? inside revue for the keys. Config: %s
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	root := "."
	switch {
	case len(args) == 1 && (args[0] == "-h" || args[0] == "--help"):
		cfgPath, _ := config.Path()
		fmt.Printf(usage, cfgPath)
		return 0
	case len(args) == 1:
		root = args[0]
	case len(args) > 1:
		fmt.Fprintln(os.Stderr, "revue: too many arguments (see revue --help)")
		return 2
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "revue:", err)
		fmt.Fprintln(os.Stderr, "revue: continuing with the default settings")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "revue:", err)
		return 1
	}
	repos, err := git.Discover(abs, cfg.Depth)
	if err != nil {
		fmt.Fprintln(os.Stderr, "revue:", err)
		return 1
	}
	if len(repos) == 0 {
		fmt.Fprintln(os.Stderr, "revue: no git repositories in", abs)
		return 1
	}

	if err := ui.Run(abs, repos, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "revue:", err)
		return 1
	}
	return 0
}
