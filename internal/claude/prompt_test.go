package claude

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPromptSelection(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "atlas-back-end")
	items := []ContextItem{{
		Repo:      "atlas-back-end",
		RepoPath:  repo,
		Path:      "src/app/app.module.ts",
		StartLine: 40,
		EndLine:   52,
		Selection: "~ const a = 1;\n~ const a = 2;\n+ added();\n",
		Patch:     "@@ -40,3 +40,4 @@\n-const a = 1;\n+const a = 2;\n+added();\n",
	}}
	got := BuildPrompt("  why is this async?\n", items)

	wantInOrder := []string{
		"uncommitted",
		"## atlas-back-end: src/app/app.module.ts (lines 40-52)",
		"Path: " + filepath.Join(repo, "src", "app", "app.module.ts"),
		"```\n~ const a = 1;\n~ const a = 2;\n+ added();\n```",
		"```diff\n@@ -40,3 +40,4 @@\n-const a = 1;\n+const a = 2;\n+added();\n```",
		"Question: why is this async?\n",
	}
	assertInOrder(t, got, wantInOrder)
	if !strings.HasSuffix(got, "Question: why is this async?\n") {
		t.Errorf("prompt does not end with the question:\n%s", got)
	}
	if strings.Contains(got, "truncated") {
		t.Error("small prompt mentions truncation")
	}
}

func TestBuildPromptHeadings(t *testing.T) {
	got := BuildPrompt("q", []ContextItem{
		{Repo: "front", RepoPath: `C:\r\front`, Path: "a.ts", Patch: "+x"},
		{Repo: "front", RepoPath: `C:\r\front`, Path: "b.ts", StartLine: 7, EndLine: 7, Selection: "+ y"},
		{Repo: "back", RepoPath: `C:\r\back`, Patch: "diff --git a/x b/x"},
		{Repo: "back", RepoPath: `C:\r\back`, Path: "src/app/", Patch: "diff --git a/src/app/y b/src/app/y"},
	})
	assertInOrder(t, got, []string{
		"## front: a.ts (whole file)",
		"## front: b.ts (line 7)",
		"## back: all uncommitted changes",
		"Path: " + filepath.Clean(`C:\r\back`),
		"## back: src/app/ (all changes in this directory)",
		"Path: " + filepath.Join(`C:\r\back`, "src", "app"),
	})
	if strings.Count(got, "Selected lines:") != 1 || strings.Count(got, "Patch:") != 3 {
		t.Errorf("blocks for empty selections or patches:\n%s", got)
	}
}

func TestBuildPromptFenceSurvivesBackticks(t *testing.T) {
	sel := "+ s := \"```\"\n+ ````go"
	got := BuildPrompt("q", []ContextItem{{Repo: "r", Path: "README.md", StartLine: 1, EndLine: 2, Selection: sel}})
	if !strings.Contains(got, "`````\n"+sel+"\n`````\n") {
		t.Errorf("selection not wrapped in a 5-backtick fence:\n%s", got)
	}
}

func TestBuildPromptTruncatesPatches(t *testing.T) {
	var big strings.Builder
	for i := 0; big.Len() < 400_000; i++ {
		fmt.Fprintf(&big, "+line %d of a very long generated file\n", i)
	}
	smallPatch := "@@ -1 +1 @@\n-a\n+b\n"
	items := []ContextItem{
		{Repo: "r", RepoPath: `C:\r`, Path: "big1.txt", Patch: big.String()},
		{Repo: "r", RepoPath: `C:\r`, Path: "small.txt", Patch: smallPatch},
		{Repo: "r", RepoPath: `C:\r`, Path: "big2.txt", Patch: big.String()},
		{Repo: "r", RepoPath: `C:\r`, Path: "sel.txt", StartLine: 3, EndLine: 4, Selection: "- old\n+ new"},
	}
	got := BuildPrompt("what changed?", items)

	if len(got) > maxPromptBytes {
		t.Errorf("prompt is %d bytes, limit %d", len(got), maxPromptBytes)
	}
	if len(got) < maxPromptBytes*9/10 {
		t.Errorf("prompt is only %d bytes; the budget is wasted", len(got))
	}
	if n := strings.Count(got, "The patch was truncated"); n != 2 {
		t.Errorf("%d truncation notes, want 2", n)
	}
	assertInOrder(t, got, []string{"big1.txt", "small.txt", smallPatch, "big2.txt", "sel.txt", "- old\n+ new", "Question: what changed?"})
	// Cut at line ends: every kept patch line is complete.
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "+line ") && !strings.HasSuffix(line, "generated file") {
			t.Fatalf("patch cut mid-line: %q", line)
		}
	}
}

func TestBuildPromptHugeSelection(t *testing.T) {
	sel := strings.Repeat("+ ж\n", 100_000)
	got := BuildPrompt("q", []ContextItem{{Repo: "r", Path: "x", StartLine: 1, EndLine: 100_000, Selection: sel, Patch: "+ ж"}})
	if len(got) > maxPromptBytes {
		t.Errorf("prompt is %d bytes", len(got))
	}
	if !strings.Contains(got, "The selection was truncated") {
		t.Error("missing truncation note")
	}
	if strings.Contains(got, "The patch was truncated") || !strings.Contains(got, "```diff\n+ ж\n```") {
		t.Error("the small patch should be kept whole")
	}
	if !strings.HasSuffix(got, "Question: q\n") {
		t.Error("question lost")
	}
}

func TestTruncateKeepsRunes(t *testing.T) {
	s := strings.Repeat("ж", 10) // 2 bytes each, no newline
	got, cut := truncate(s, 5)
	if !cut || got != "жж" {
		t.Errorf("truncate = %q, %v", got, cut)
	}
}

func TestFairShare(t *testing.T) {
	got := fairShare([]int{100, 10, 100, 0}, 150)
	want := []int{70, 10, 70, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fairShare = %v, want %v", got, want)
		}
	}
}

func assertInOrder(t *testing.T, s string, parts []string) {
	t.Helper()
	rest := s
	for _, p := range parts {
		i := strings.Index(rest, p)
		if i < 0 {
			t.Fatalf("missing (or out of order) %q in:\n%s", p, s)
		}
		rest = rest[i+len(p):]
	}
}

// Marking a whole directory gives many small items; when they fit as they
// are, none of them may be cut.
func TestBuildPromptManySmallItemsKeptWhole(t *testing.T) {
	var items []ContextItem
	whole := 0
	for i := 0; i < 300; i++ {
		patch := strings.Repeat(fmt.Sprintf("@@ -1,3 +1,3 @@\n-old line %d\n+new line %d\n context\n", i, i), 4)
		it := ContextItem{
			Repo:     "atlas-front-end",
			RepoPath: `C:\Users\dev\Desktop\atlas\atlas-front-end`,
			Path:     fmt.Sprintf("src/app/features/some-module/components/file-%d.component.ts", i),
			Patch:    patch,
		}
		items = append(items, it)
		whole += len(heading(it)) + len(absPath(it)) + len(patch) + 64
	}
	if whole >= maxPromptBytes {
		t.Fatalf("test items need %d bytes; they should fit in %d", whole, maxPromptBytes)
	}
	got := BuildPrompt("what changed?", items)
	if n := strings.Count(got, "was truncated"); n != 0 {
		t.Errorf("%d of %d patches truncated in a %d-byte prompt", n, len(items), len(got))
	}
	for _, it := range items {
		if !strings.Contains(got, strings.TrimRight(it.Patch, "\n")) {
			t.Fatalf("patch of %s not kept whole", it.Path)
		}
	}
}

// The notes on cut blocks must fit in the budget too, however many
// blocks are cut.
func TestBuildPromptManyCutItemsStayUnderLimit(t *testing.T) {
	for _, n := range []int{50, 200, 400} {
		var items []ContextItem
		for i := 0; i < n; i++ {
			items = append(items, ContextItem{
				Repo: "r", RepoPath: `C:\r`, Path: fmt.Sprintf("f%d.ts", i),
				StartLine: 1, EndLine: 900,
				Selection: strings.Repeat("+ selected line\n", 900),
				Patch:     strings.Repeat("+patch line\n", 900),
			})
		}
		if got := BuildPrompt("q", items); len(got) > maxPromptBytes {
			t.Errorf("%d items: prompt is %d bytes, limit %d", n, len(got), maxPromptBytes)
		}
	}
}

func TestBuildPromptRandomBudgets(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	lines := []string{"+ added ж line", "-`code` removed", "~ ```go", "context", "````", "+" + strings.Repeat("x", 300)}
	body := func(maxLines int) string {
		var b strings.Builder
		for n := rng.IntN(maxLines + 1); n > 0; n-- {
			b.WriteString(lines[rng.IntN(len(lines))])
			b.WriteString("\n")
		}
		return b.String()
	}
	for round := 0; round < 200; round++ {
		var items []ContextItem
		for n := 1 + rng.IntN(60); n > 0; n-- {
			it := ContextItem{Repo: "r", RepoPath: `C:\repo dir`, Path: fmt.Sprintf("dir/f%d.go", n), Patch: body([]int{2, 50, 5000}[rng.IntN(3)])}
			if rng.IntN(2) == 0 {
				it.StartLine, it.EndLine, it.Selection = 1, 9, body([]int{2, 50, 5000}[rng.IntN(3)])
			}
			items = append(items, it)
		}
		got := BuildPrompt("why?", items)
		if len(got) > maxPromptBytes {
			t.Fatalf("round %d: prompt is %d bytes, limit %d", round, len(got), maxPromptBytes)
		}
		if !strings.HasSuffix(got, "\nQuestion: why?\n") {
			t.Fatalf("round %d: question is not last", round)
		}
		checkFences(t, got)
	}
}

// checkFences walks the prompt like a CommonMark reader: every block opened
// after a "Selected lines:" or "Patch:" label must be closed by its own
// fence, never by a line inside it.
func checkFences(t *testing.T, prompt string) {
	t.Helper()
	ls := strings.Split(prompt, "\n")
	for i := 0; i < len(ls); i++ {
		if ls[i] != "Selected lines:" && ls[i] != "Patch:" {
			continue
		}
		open := strings.TrimRight(ls[i+1], "dif")
		if len(open) < 3 || strings.Trim(open, "`") != "" {
			t.Fatalf("line %d: no fence after the label: %q", i+1, ls[i+1])
		}
		j := i + 2
		for ; j < len(ls); j++ {
			if s := strings.TrimLeft(ls[j], " "); strings.HasPrefix(s, open) {
				break
			}
		}
		if j == len(ls) || ls[j] != open {
			t.Fatalf("line %d: block closed by %q instead of %q", j, ls[min(j, len(ls)-1)], open)
		}
		i = j
	}
}
