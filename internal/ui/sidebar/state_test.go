package sidebar

import (
	"testing"

	"github.com/DavidkJun/revue/internal/git"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

func TestReposWithChangesStartExpanded(t *testing.T) {
	m := newModel(40, 20,
		repo("api", paths("M a.ts")...),
		repo("clean"),
		errRepo("broken"),
	)
	equal(t, rowsOf(m), []string{"r:", "  f:a.ts", "r:", "r:"})
	if m.isExpanded(m.roots[1]) || m.isExpanded(m.roots[2]) {
		t.Fatal("repos without changes must start collapsed")
	}
}

func TestExpansionSurvivesSetRepos(t *testing.T) {
	files := paths("M src/a.ts", "M lib/b.ts")
	m := newModel(40, 20, repo("api", files...), repo("web", paths("M x.ts")...))
	m.Select(0, "src/a.ts")
	act(m, keys.Collapse, 0) // to src
	act(m, keys.Collapse, 0) // collapse src
	m.Select(1, "")
	act(m, keys.Collapse, 0) // collapse web

	m.SetRepos([]Repo{
		repo("api", append(files, fc("M src/c.ts", 1, 0))...),
		repo("web", paths("M x.ts", "M y.ts")...),
	})
	equal(t, rowsOf(m), []string{"r:", "  d:lib", "    f:b.ts", "  d:src", "r:"})
}

func TestExpansionKeyedByRepoIdentityNotIndex(t *testing.T) {
	api, web := repo("api", paths("M a.ts")...), repo("web", paths("M b.ts")...)
	m := newModel(40, 20, api, web)
	m.Select(1, "")
	act(m, keys.Collapse, 0)

	m.SetRepos([]Repo{web, api})
	equal(t, rowsOf(m), []string{"r:", "r:", "  f:a.ts"})
	if m.repos[m.rows[0].repo].Repo.Name != "web" || curLabel(m) != "r:" || m.current().repo != 0 {
		t.Fatalf("cursor should follow web to index 0, got repo %d", m.current().repo)
	}
}

func TestCollapsedRepoReopensDefaultAfterGoingClean(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M a.ts")...))
	act(m, keys.Collapse, 0)
	act(m, keys.Expand, 0)
	m.SetRepos([]Repo{repo("api")})
	m.SetRepos([]Repo{repo("api", paths("M a.ts")...)})
	equal(t, rowsOf(m), []string{"r:", "  f:a.ts"})
}

func TestCursorStaysOnSameFileWhenRowsAppearAbove(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M b.ts", "M c.ts")...))
	m.Select(0, "c.ts")
	m.SetRepos([]Repo{repo("api", paths("M a.ts", "M aa.ts", "M b.ts", "M c.ts")...)})
	if curLabel(m) != "f:c.ts" {
		t.Fatalf("cursor on %s, want f:c.ts", curLabel(m))
	}
}

func TestCursorMovesToNextFileWhenItsFileDisappears(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M a.ts", "M b.ts", "M c.ts")...))
	m.Select(0, "b.ts")
	m.SetRepos([]Repo{repo("api", paths("M a.ts", "M c.ts")...)})
	if curLabel(m) != "f:c.ts" {
		t.Fatalf("cursor on %s, want the next file f:c.ts", curLabel(m))
	}
}

func TestCursorMovesBackWhenLastFileDisappears(t *testing.T) {
	m := newModel(40, 20,
		repo("api", paths("M a.ts", "M b.ts")...),
		repo("web", paths("M x.ts")...),
	)
	m.Select(0, "b.ts")
	m.SetRepos([]Repo{repo("api", paths("M a.ts")...), repo("web", paths("M x.ts")...)})
	if curLabel(m) != "f:a.ts" || m.current().repo != 0 {
		t.Fatalf("cursor on %s in repo %d, want f:a.ts in the same repo", curLabel(m), m.current().repo)
	}
}

func TestCursorFallsBackToRepoWhenRepoGoesClean(t *testing.T) {
	m := newModel(40, 20,
		repo("api", paths("M src/a.ts")...),
		repo("web", paths("M x.ts")...),
	)
	m.Select(0, "src/a.ts")
	m.SetRepos([]Repo{repo("api"), repo("web", paths("M x.ts")...)})
	if curLabel(m) != "r:" || m.current().repo != 0 {
		t.Fatalf("cursor on %s in repo %d, want the api repo node", curLabel(m), m.current().repo)
	}
}

func TestCursorFollowsDirectoryThroughCompaction(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M src/app/a.ts", "M src/b.ts")...))
	m.Select(0, "src/app/a.ts")
	act(m, keys.Collapse, 0) // to src/app
	if curLabel(m) != "d:src/app" {
		t.Fatalf("cursor on %s", curLabel(m))
	}
	// src/b.ts goes away: src and app compact into one "src/app" node.
	m.SetRepos([]Repo{repo("api", paths("M src/app/a.ts")...)})
	if curLabel(m) != "d:src/app" {
		t.Fatalf("cursor on %s, want d:src/app", curLabel(m))
	}
	equal(t, rowsOf(m), []string{"r:", "  d:src/app", "    f:a.ts"})
}

func TestCursorWhenRepoRemoved(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M a.ts")...), repo("web", paths("M x.ts")...))
	m.Select(1, "x.ts")
	m.SetRepos([]Repo{repo("api", paths("M a.ts")...)})
	if curLabel(m) != "f:a.ts" {
		t.Fatalf("cursor on %s, want the nearest surviving row", curLabel(m))
	}
	m.SetRepos(nil)
	if _, _, ok := m.Current(); ok {
		t.Fatal("Current must report !ok without repos")
	}
	m.SetRepos([]Repo{repo("api", paths("M a.ts")...)})
	if curLabel(m) != "r:" {
		t.Fatalf("cursor on %s after repos came back", curLabel(m))
	}
}

func TestMarksDroppedWhenFileNoLongerChanged(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M a.ts", "M b.ts")...))
	act(m, keys.Mark, 0) // repo: marks both
	if len(m.Marked()) != 2 {
		t.Fatalf("Marked() = %v", m.Marked())
	}
	m.SetRepos([]Repo{repo("api", paths("M b.ts", "M c.ts")...)})
	got := m.Marked()
	if len(got) != 1 || got[0] != (FileRef{0, "b.ts"}) {
		t.Fatalf("Marked() = %v, want only b.ts", got)
	}
}

func TestDuplicateRepoIdentitiesStayDistinct(t *testing.T) {
	a := Repo{Status: statusOf(paths("M a.ts")), Loaded: true}
	b := Repo{Status: statusOf(paths("M b.ts")), Loaded: true}
	m := newModel(40, 20, a, b)
	m.Select(1, "b.ts")
	if curLabel(m) != "f:b.ts" || m.current().repo != 1 {
		t.Fatalf("cursor on %s in repo %d", curLabel(m), m.current().repo)
	}
}

func TestCollapsedRepoStaysCollapsedThroughCleanAndBack(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M a.ts")...))
	act(m, keys.Collapse, 0)
	m.SetRepos([]Repo{repo("api")})
	m.SetRepos([]Repo{repo("api", paths("M a.ts", "M b.ts")...)})
	equal(t, rowsOf(m), []string{"r:"})
}

func TestSetReposKeepsWheelScrolledView(t *testing.T) {
	m := longTree(100, 21)
	m.Scroll(50)
	top := m.rows[m.offset].path

	// A poll that only changes line counts must not pull the view back to
	// the cursor the user scrolled away from.
	var files []git.FileChange
	for i := 0; i < 100; i++ {
		files = append(files, fc("M f"+pad3(i)+".ts", 2, 1))
	}
	m.SetRepos([]Repo{repo("api", files...)})
	if m.offset != 50 || m.cursor != 0 {
		t.Fatalf("offset %d cursor %d, want the view left at 50 and the cursor at 0", m.offset, m.cursor)
	}

	// Rows appearing above keep the same row at the top of the pane.
	m.SetRepos([]Repo{repo("api", append(paths("M a1.ts", "M a2.ts", "M a3.ts"), files...)...)})
	if got := m.rows[m.offset].path; got != top || m.cursor != 0 {
		t.Fatalf("top row %s cursor %d, want %s at the top and the cursor at 0", got, m.cursor, top)
	}
}

func TestCollapsedDirSurvivesCompaction(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M src/app/a.ts", "M src/b.ts", "M z.ts")...))
	m.Select(0, "src/b.ts")
	act(m, keys.Collapse, 0) // to src
	act(m, keys.Collapse, 0) // collapse src

	// src/b.ts goes: src now holds only app and compacts into "src/app".
	m.SetRepos([]Repo{repo("api", paths("M src/app/a.ts", "M z.ts")...)})
	equal(t, rowsOf(m), []string{"r:", "  d:src/app", "  f:z.ts"})
	if curLabel(m) != "d:src/app" {
		t.Fatalf("cursor on %s, want the compacted d:src/app", curLabel(m))
	}

	// Expanding the compacted node clears the choice made on src.
	act(m, keys.Expand, 0)
	m.SetRepos([]Repo{repo("api", paths("M src/app/a.ts", "M src/b.ts", "M z.ts")...)})
	equal(t, rowsOf(m), []string{"r:", "  d:src", "    d:app", "      f:a.ts", "    f:b.ts", "  f:z.ts"})
}

func TestMarkFollowsRename(t *testing.T) {
	m := newModel(40, 20, repo("api", paths("M a.ts", "M c.ts")...))
	m.Select(0, "a.ts")
	act(m, keys.Mark, 0)

	rn := fc("R b.ts", 1, 1)
	rn.OldPath = "a.ts"
	m.SetRepos([]Repo{repo("api", rn, fc("M c.ts", 1, 0))})
	got := m.Marked()
	if len(got) != 1 || got[0] != (FileRef{0, "b.ts"}) {
		t.Fatalf("Marked() = %v, want the mark carried to the new name b.ts", got)
	}
}
