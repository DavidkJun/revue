package ui

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/atotto/clipboard"

	"github.com/DavidkJun/revue/internal/diff"
	"github.com/DavidkJun/revue/internal/git"
)

// deps are the app's side effects. Tests replace them with fakes so the
// model can be driven without git, a clipboard or WezTerm.
type deps struct {
	status    func(ctx context.Context, repoPath string) (git.Status, error)
	fileDiff  func(ctx context.Context, repoPath string, fc git.FileChange) (*diff.FileDiff, error)
	patch     func(ctx context.Context, repoPath string, files []git.FileChange, maxBytes int) (string, error)
	clipboard func(text string) error
	// navigate moves focus to the WezTerm pane in direction "Left" or
	// "Right"; nil outside WezTerm.
	navigate func(ctx context.Context, direction string) error
}

func defaultDeps() deps {
	d := deps{
		status:    git.GetStatus,
		fileDiff:  git.GetFileDiff,
		patch:     git.Patch,
		clipboard: clipboard.WriteAll,
	}
	if os.Getenv("WEZTERM_PANE") != "" {
		d.navigate = weztermNavigate
	}
	return d
}

// weztermNavigate does what the user's Neovim smart-splits does at the edge
// of its windows, so ctrl+h/l crosses from revue into the neighbouring pane.
func weztermNavigate(ctx context.Context, direction string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "wezterm", "cli", "activate-pane-direction", direction)
	hideWindow(cmd)
	return cmd.Run()
}
