// Package uimsg holds the messages UI components send to the app. Components
// never import the app; they return commands producing these.
package uimsg

// FileSelected: the sidebar cursor landed on a file, which the diff pane
// should show.
type FileSelected struct {
	Repo int    // index into the app's repo list
	Path string // repo-relative, forward slashes
}

// Yank asks the app to put Text on the clipboard. What describes it for the
// notification, e.g. "3 lines" or "src/app.ts:42".
type Yank struct {
	Text string
	What string
}

// Notify shows a short message in the statusline for a few seconds.
type Notify struct {
	Text  string
	Error bool
}
