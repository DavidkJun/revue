package sidebar

// Nerd Font glyphs of the tree itself, written as escapes because the
// private-use code points are invisible (and easily lost) in most editors.
// Every one is a single cell in the user's "Nerd Font Mono" font. File icons
// come from the icons package, shared with the diff pane.
const (
	iconTree       = "\U000f0645" // md-file_tree
	iconGit        = "\U000f02a2" // md-git
	iconBranch     = "\U0000e725" // dev-git_branch
	iconCommit     = "\U0000f417" // oct-git_commit
	iconFolder     = "\U0000f07b" // fa-folder
	iconFolderOpen = "\U0000f07c" // fa-folder_open

	chevronOpen   = "▾"
	chevronClosed = "▸"
	markGlyph     = "●"
)
