// Package claude asks the Claude Code CLI questions about code under review
// and streams the answer back as events. It runs the user's own `claude`
// binary in headless mode, so it uses their subscription login.
package claude

import "time"

// Request is one question to Claude.
type Request struct {
	Prompt       string   // full prompt, sent on stdin (see BuildPrompt)
	Dir          string   // working directory for the claude process
	AddDirs      []string // extra directories Claude may read (--add-dir)
	SessionID    string   // resume this conversation when non-empty (--resume)
	Model        string   // --model; "" = CLI default
	Effort       string   // --effort; "" = CLI default
	SystemPrompt string   // --append-system-prompt; "" = DefaultSystemPrompt
	ExtraArgs    []string // appended verbatim
	Binary       string   // executable; "" = "claude"
}

// EventKind classifies streamed events.
type EventKind uint8

const (
	EventText     EventKind = iota // Text is the next chunk of the answer
	EventThinking                  // the model is thinking; Text may hold a chunk
	EventTool                      // Text describes a tool call, e.g. "Read src/app.ts"
	EventDone                      // final event: Result, SessionID, Cost, Duration set
	EventError                     // final event: Err set (SessionID set if known)
)

// Event is one streamed update.
type Event struct {
	Kind      EventKind
	Text      string
	Result    string // full answer text (EventDone)
	SessionID string
	CostUSD   float64
	Duration  time.Duration
	Err       error
}

// DefaultSystemPrompt is appended to Claude Code's system prompt for review
// questions.
const DefaultSystemPrompt = `You are answering quick questions from a developer who is reviewing uncommitted git changes in a terminal diff viewer. Lead with the answer, keep it short, use markdown. Reference code as path:line. You may read files in the repositories for context, but never modify anything.`

// ContextItem is one piece of code the question is about.
type ContextItem struct {
	Repo      string // repo display name
	RepoPath  string // absolute repo root
	Path      string // repo-relative file path, forward slashes; a directory ends with "/" and covers the changes below it
	StartLine int    // new-file line range of a selection; 0 = whole file
	EndLine   int
	Selection string // selected lines as shown (with -/+/~ markers), "" for whole-file items
	Patch     string // unified diff of the file (or of the whole repo for repo-wide questions)
}
