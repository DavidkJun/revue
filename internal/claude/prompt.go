package claude

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// maxPromptBytes keeps prompts small enough to send quickly and leave the
// model room. Small blocks are kept whole; in practice the patches are what
// gets cut, since selections are short.
const maxPromptBytes = 150_000

// The budget is planned without rendering: these over-estimate the bytes
// around each item (heading and path lines), around each fenced block
// (label, fence lines) and of the note added to a block that is cut.
// Only cut blocks are charged a note, so many small items that fit whole
// are never cut to make room for notes they do not get.
const (
	itemReserve  = 32
	blockReserve = 48
	noteReserve  = 160
)

const promptPreamble = "I'm reviewing uncommitted git changes (HEAD → working tree). " +
	"In \"Selected lines\" each line starts with a marker: \"-\" deleted, \"+\" added, " +
	"\"~\" modified (the old version directly above the new one), none for unchanged context."

// BuildPrompt formats the question and its context into the prompt sent to
// Claude: file paths (absolute and repo-relative), line ranges, the selected
// lines, and the patches, followed by the question.
func BuildPrompt(question string, items []ContextItem) string {
	sels := make([]string, len(items))
	patches := make([]string, len(items))
	fixed := len(promptPreamble) + len(question) + 64
	for i, it := range items {
		sels[i] = strings.TrimRight(it.Selection, "\n")
		patches[i] = strings.TrimRight(it.Patch, "\n")
		fixed += len(heading(it)) + len(absPath(it)) + itemReserve
		for _, body := range []string{sels[i], patches[i]} {
			if body != "" {
				fixed += blockReserve + 2*len(fence(body))
			}
		}
	}

	// Reserving notes can cut more blocks, which need notes too; the count
	// only grows, so this settles.
	bodies := append(sizes(sels), sizes(patches)...)
	budget := fairShare(bodies, max(maxPromptBytes-fixed, 0))
	for notes := 0; ; {
		n := countCut(bodies, budget)
		if n <= notes {
			break
		}
		notes = n
		budget = fairShare(bodies, max(maxPromptBytes-fixed-notes*noteReserve, 0))
	}
	selBudget, patchBudget := budget[:len(items)], budget[len(items):]

	var b strings.Builder
	b.WriteString(promptPreamble)
	b.WriteString("\n")
	for i, it := range items {
		fmt.Fprintf(&b, "\n## %s\n\nPath: %s\n", heading(it), absPath(it))
		writeBlock(&b, "Selected lines", "selection", "", sels[i], selBudget[i])
		writeBlock(&b, "Patch", "patch", "diff", patches[i], patchBudget[i])
	}
	b.WriteString("\nQuestion: ")
	b.WriteString(strings.TrimSpace(question))
	b.WriteString("\n")
	return b.String()
}

func heading(it ContextItem) string {
	name := it.Path
	if name == "" {
		name = "all uncommitted changes"
	}
	if it.Repo != "" {
		name = it.Repo + ": " + name
	}
	switch {
	case it.Path == "":
		return name
	case strings.HasSuffix(it.Path, "/"):
		return name + " (all changes in this directory)"
	case it.StartLine <= 0:
		return name + " (whole file)"
	case it.EndLine <= it.StartLine:
		return name + " (line " + strconv.Itoa(it.StartLine) + ")"
	default:
		return fmt.Sprintf("%s (lines %d-%d)", name, it.StartLine, it.EndLine)
	}
}

func absPath(it ContextItem) string {
	if it.RepoPath == "" {
		return filepath.FromSlash(it.Path)
	}
	return filepath.Join(it.RepoPath, filepath.FromSlash(it.Path))
}

func writeBlock(b *strings.Builder, label, what, lang, body string, budget int) {
	if body == "" {
		return
	}
	kept, cut := truncate(body, budget)
	f := fence(body)
	fmt.Fprintf(b, "\n%s:\n%s%s\n", label, f, lang)
	if kept != "" {
		b.WriteString(kept)
		b.WriteString("\n")
	}
	b.WriteString(f)
	b.WriteString("\n")
	if cut {
		fmt.Fprintf(b, "(The %s was truncated to its first %d of %d lines to keep this prompt short; read the file for the rest.)\n",
			what, lineCount(kept), lineCount(body))
	}
}

// fence returns a backtick fence longer than any backtick run in body, so
// nothing inside can close it.
func fence(body string) string {
	longest, run := 0, 0
	for i := 0; i < len(body); i++ {
		if body[i] == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", max(3, longest+1))
}

// truncate cuts s to at most budget bytes, at a line end when possible and
// never inside a rune.
func truncate(s string, budget int) (string, bool) {
	if len(s) <= budget {
		return s, false
	}
	if budget <= 0 {
		return "", true
	}
	cut := s[:budget]
	if nl := strings.LastIndexByte(cut, '\n'); nl >= 0 {
		return cut[:nl], true
	}
	for len(cut) > 0 && !utf8.RuneStart(s[len(cut)]) {
		cut = cut[:len(cut)-1]
	}
	return cut, true
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func sizes(bodies []string) []int {
	out := make([]int, len(bodies))
	for i, s := range bodies {
		out[i] = len(s)
	}
	return out
}

func countCut(sizes, budget []int) int {
	n := 0
	for i, s := range sizes {
		if budget[i] < s {
			n++
		}
	}
	return n
}

// fairShare splits total across the sizes so small ones are kept whole and
// the large ones share what is left equally.
func fairShare(sizes []int, total int) []int {
	order := make([]int, len(sizes))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return sizes[order[a]] < sizes[order[b]] })
	out := make([]int, len(sizes))
	left := total
	for n, i := range order {
		out[i] = min(sizes[i], left/(len(order)-n))
		left -= out[i]
	}
	return out
}
