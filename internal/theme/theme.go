// Package theme holds the colours of the viewer. They follow the user's
// WezTerm "Tokyo Night neon" palette and the soft diff backgrounds from their
// Neovim config, so code stays readable on top of the coloured lines.
package theme

// Theme is a set of "#rrggbb" colours plus the names of the syntax and
// markdown styles. The terminal background is never painted; panes are
// transparent like the user's Neovim.
type Theme struct {
	FG     string
	Muted  string // secondary text, borders of unfocused panes
	Subtle string // fold markers, line numbers
	Accent string // focused borders, active tab, prompt marker

	Red     string
	Green   string
	Yellow  string
	Blue    string
	Magenta string
	Cyan    string

	AddBG     string // added line
	AddWordBG string
	DelBG     string // deleted line
	DelWordBG string
	ModBG     string // new version of a modified line
	ModOldBG  string // old version of a modified line
	ModWordBG string // changed words inside a modified line

	CursorLineBG string
	VisualBG     string
	SearchBG     string
	SelectedBG   string // selected sidebar entry / active tab
	BarBG        string // statusline and tab bar background
	FoldBG       string

	SyntaxStyle   string // chroma style
	MarkdownStyle string // glamour style
}

// Default is the theme used unless the config overrides it.
var Default = Theme{
	FG:     "#d5dbff",
	Muted:  "#565f89",
	Subtle: "#3b4261",
	Accent: "#5ec8ff",

	Red:     "#ff5c7a",
	Green:   "#5ef2a0",
	Yellow:  "#ffd95c",
	Blue:    "#5ec8ff",
	Magenta: "#c792ff",
	Cyan:    "#66e6ff",

	AddBG:     "#12261c",
	AddWordBG: "#1f4d2b",
	DelBG:     "#2a181b",
	DelWordBG: "#63262b",
	ModBG:     "#2b2716",
	ModOldBG:  "#221f13",
	ModWordBG: "#5a4d1c",

	CursorLineBG: "#2a2f45",
	VisualBG:     "#33467c",
	SearchBG:     "#7a5c12",
	SelectedBG:   "#2f3550",
	BarBG:        "#1f2335",
	FoldBG:       "#1c1f2e",

	SyntaxStyle:   "vscode-dark-plus", // the user's Neovim colorscheme (vscode.nvim)
	MarkdownStyle: "tokyo-night",
}
