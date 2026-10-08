package sidebar

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

// TestPreview writes rendered views to $REVUE_SIDEBAR_PREVIEW so they can be
// turned into screenshots and looked at. It reads the user's live repos when
// $REVUE_SIDEBAR_REPOS points at a folder; nothing is ever written there.
func TestPreview(t *testing.T) {
	dir := os.Getenv("REVUE_SIDEBAR_PREVIEW")
	if dir == "" {
		t.Skip("set REVUE_SIDEBAR_PREVIEW to a directory to write previews")
	}
	write := func(name string, m *Model, focused bool) {
		if err := os.WriteFile(filepath.Join(dir, name+".ansi"), []byte(m.View(focused)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sample := sampleRepos()
	m := newModel(36, 24, sample...)
	m.Select(0, "src/modules/auth/auth.service.ts")
	act(m, keys.Mark, 0)
	m.Select(0, "src/modules/auth/strategies/jwt.strategy.ts")
	act(m, keys.Mark, 0)
	m.Select(0, "src/modules/users/users.controller.ts")
	write("sample-36x24", m, true)
	write("sample-36x24-unfocused", m, false)
	m.SetSize(28, 24)
	write("sample-28x24", m, true)

	m.SetSize(36, 12)
	m.Select(0, "src/modules/users/users.controller.ts")
	act(m, keys.Collapse, 0)
	act(m, keys.Collapse, 0)
	m.Select(1, "")
	write("sample-36x12-collapsed", m, true)

	root := os.Getenv("REVUE_SIDEBAR_REPOS")
	if root == "" {
		return
	}
	found, err := git.Discover(root, 2)
	if err != nil {
		t.Fatal(err)
	}
	var repos []Repo
	for _, r := range found {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		st, err := git.GetStatus(ctx, r.Path)
		cancel()
		repos = append(repos, Repo{Repo: r, Status: st, Err: err, Loaded: true})
	}
	live := newModel(36, 40, repos...)
	if files := live.Files(); len(files) > 2 {
		live.Select(files[2].Repo, files[2].Path)
		act(live, keys.Mark, 0)
	}
	write("live-36x40", live, true)
	live.SetSize(28, 20)
	write("live-28x20", live, true)
}

func sampleRepos() []Repo {
	back := repo("atlas-back-end",
		fc("M src/modules/auth/auth.service.ts", 12, 3),
		fc("A src/modules/auth/strategies/jwt.strategy.ts", 40, 0),
		fc("M src/modules/users/users.controller.ts", 4, 1),
		fc("M src/modules/users/users.service.ts", 27, 18),
		fc("D src/modules/users/dto/legacy-user-profile-response.dto.ts", 0, 64),
		fc("? src/modules/notifications/templates/звіт-про-зміни.md", 15, 0),
		fc("M package.json", 1, 1),
		fc("M .env.example", 2, 0),
		fc("M docker-compose.yml", 3, 3),
	)
	back.Status.Branch = "feature/ABC-1234-refresh-tokens"
	back.Status.Ahead, back.Status.Behind = 1, 2
	rn := fc("R src/app.module.ts", 2, 1)
	rn.OldPath = "src/main.module.ts"
	back.Status.Files = append(back.Status.Files, rn)
	bin := fc("A assets/logo.png", 0, 0)
	bin.Binary = true
	back.Status.Files = append(back.Status.Files, bin)

	front := repo("atlas-front-end")
	front.Status.Branch = "develop"
	infra := errRepo("atlas-infra")
	docs := repo("atlas-docs")
	docs.Loaded = false
	return []Repo{back, front, infra, docs}
}
