package sidebar

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/theme"
	"github.com/DavidkJun/revue/internal/ui/icons"
	"github.com/DavidkJun/revue/internal/ui/keys"
)

func TestTreeCompactsSingleChildChains(t *testing.T) {
	m := newModel(40, 30, repo("api", paths(
		"M src/app/core/a.ts",
		"M src/app/core/b.ts",
		"M docs/guide/intro.md",
		"M README.md",
	)...))
	equal(t, rowsOf(m), []string{
		"r:",
		"  d:docs/guide",
		"    f:intro.md",
		"  d:src/app/core",
		"    f:a.ts",
		"    f:b.ts",
		"  f:README.md",
	})
	if got := m.rows[3].path; got != "src/app/core" {
		t.Fatalf("compacted node path = %q, want the deepest directory", got)
	}
}

func TestTreeStopsCompactingAtBranches(t *testing.T) {
	m := newModel(40, 30, repo("api", paths(
		"M src/app/core/a.ts",
		"M src/app/x.ts",
		"M src/app/shared/util/y.ts",
	)...))
	equal(t, rowsOf(m), []string{
		"r:",
		"  d:src/app",
		"    d:core",
		"      f:a.ts",
		"    d:shared/util",
		"      f:y.ts",
		"    f:x.ts",
	})
}

func TestTreeOrderDirsFirstCaseInsensitive(t *testing.T) {
	m := newModel(40, 30, repo("api", paths(
		"M zeta.ts",
		"M Beta.ts",
		"M alpha.ts",
		"M lib/z.go",
		"M Lib2/a.go",
		"M assets/x.png",
	)...))
	equal(t, rowsOf(m), []string{
		"r:",
		"  d:assets",
		"    f:x.png",
		"  d:lib",
		"    f:z.go",
		"  d:Lib2",
		"    f:a.go",
		"  f:alpha.ts",
		"  f:Beta.ts",
		"  f:zeta.ts",
	})
}

func TestTreeReposKeepGivenOrder(t *testing.T) {
	m := newModel(40, 30,
		repo("zz", paths("M a.ts")...),
		repo("aa"),
	)
	equal(t, rowsOf(m), []string{"r:", "  f:a.ts", "r:"})
	if m.repos[m.rows[2].repo].Repo.Name != "aa" {
		t.Fatal("repos must stay in the given order")
	}
}

func TestTreeUnicodeNames(t *testing.T) {
	m := newModel(40, 30, repo("api", paths(
		"? docs/звіти/Звіт.md",
		"? docs/звіти/архів.md",
	)...))
	equal(t, rowsOf(m), []string{
		"r:",
		"  d:docs/звіти",
		"    f:архів.md",
		"    f:Звіт.md",
	})
}

func TestTreeFileAndDirWithSameName(t *testing.T) {
	// A file replaced by a directory of the same name (deleted + untracked).
	m := newModel(40, 30, repo("api", paths("D tool", "? tool/main.go")...))
	equal(t, rowsOf(m), []string{"r:", "  d:tool", "    f:main.go", "  f:tool"})
}

func TestFilesInDisplayOrderIgnoringCollapse(t *testing.T) {
	m := newModel(40, 30,
		repo("api", paths("M z.ts", "M src/b.ts", "M src/a.ts")...),
		repo("clean"),
		repo("web", paths("M x.tsx")...),
	)
	m.Select(0, "src/a.ts")
	act(m, keys.Collapse, 0) // to the directory
	act(m, keys.Collapse, 0) // collapse it
	want := []FileRef{{0, "src/a.ts"}, {0, "src/b.ts"}, {0, "z.ts"}, {2, "x.tsx"}}
	got := m.Files()
	if len(got) != len(want) {
		t.Fatalf("Files() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Files() = %v, want %v", got, want)
		}
	}
}

func TestSetReposDoesNotAliasCallerFiles(t *testing.T) {
	files := paths("M a.ts")
	m := newModel(40, 10, repo("api", files...))
	files[0].Path = "changed.ts"
	m.Select(0, "a.ts")
	if _, f, ok := m.Current(); !ok || f == nil || f.Path != "a.ts" {
		t.Fatalf("Current() = %v, %v", f, ok)
	}
}

func TestIconsAreOneCell(t *testing.T) {
	for _, g := range []string{iconTree, iconGit, iconBranch, iconCommit, iconFolder, iconFolderOpen, chevronOpen, chevronClosed, markGlyph} {
		if w := ansi.StringWidth(g); w != 1 {
			t.Errorf("%U is %d cells wide", []rune(g), w)
		}
	}
}

func TestFileRowShowsTheSharedIcon(t *testing.T) {
	m := newModel(40, 10, repo("api", paths("M web/App.tsx")...))
	m.Select(0, "web/App.tsx")
	want, _ := icons.For(theme.Default, "web/App.tsx")
	if row := plainLines(m.View(true))[m.cursor+1-m.offset]; !strings.Contains(row, want+" App.tsx") {
		t.Errorf("row %q lacks the icon %U", row, []rune(want))
	}
}

func TestTreeOrderIgnoresCompaction(t *testing.T) {
	// "app" sorts by its own name whether or not it is merged with its only
	// child, so a file appearing inside it does not reorder its siblings.
	want := func(app ...string) []string {
		return append(append([]string{"r:"}, app...), "  d:app-shell", "    f:y.ts", "  d:app.bak", "    f:w.ts", "    f:z.ts")
	}
	siblings := []string{"M app-shell/y.ts", "M app.bak/z.ts", "M app.bak/w.ts"}
	m := newModel(40, 20, repo("api", paths(append([]string{"M app/core/a.ts", "M app/x.ts"}, siblings...)...)...))
	equal(t, rowsOf(m), want("  d:app", "    d:core", "      f:a.ts", "    f:x.ts"))
	m.SetRepos([]Repo{repo("api", paths(append([]string{"M app/core/a.ts"}, siblings...)...)...)})
	equal(t, rowsOf(m), want("  d:app/core", "    f:a.ts"))
}

func TestFilesMatchVisibleOrder(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	parts := []string{"src", "src2", "src-old", "Src.bak", "app", "a", "b", "звіт", "日本"}
	for round := 0; round < 200; round++ {
		var specs []string
		seen := map[string]bool{}
		for range 1 + rng.IntN(12) {
			var p []string
			for range rng.IntN(4) {
				p = append(p, parts[rng.IntN(len(parts))])
			}
			p = append(p, parts[rng.IntN(len(parts))]+".ts")
			if path := strings.Join(p, "/"); !seen[path] {
				seen[path] = true
				specs = append(specs, "M "+path)
			}
		}
		m := newModel(40, 20, repo("api", paths(specs...)...), repo("web", paths("M x.ts")...))
		var visible []FileRef
		for _, n := range m.rows {
			if n.kind == fileNode {
				visible = append(visible, FileRef{n.repo, n.path})
			}
		}
		if got := m.Files(); !slices.Equal(got, visible) {
			t.Fatalf("%v:\nFiles() %v\nrows    %v", specs, got, visible)
		}
	}
}
