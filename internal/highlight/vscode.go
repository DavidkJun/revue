package highlight

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

// VSCodeDarkPlus is the name of VS Code's Dark+ palette as the user's Neovim
// shows it (vscode.nvim). Chroma ships no such style, so it is registered
// here; Refine supplies the distinctions a regex lexer cannot make on its
// own (function vs variable vs type, pink control-flow keywords).
const VSCodeDarkPlus = "vscode-dark-plus"

func init() {
	styles.Register(chroma.MustNewStyle(VSCodeDarkPlus, chroma.StyleEntries{
		chroma.Background: "bg:#1e1e1e",
		chroma.Text:       "#d4d4d4",
		chroma.Error:      "#d4d4d4",

		chroma.Comment:        "italic #6a9955",
		chroma.CommentPreproc: "noitalic #c586c0",

		chroma.Keyword:          "#569cd6",
		chroma.KeywordNamespace: "#c586c0", // also the bucket Refine moves control flow into
		chroma.KeywordType:      "#4ec9b0",

		chroma.Name:              "#9cdcfe",
		chroma.NameAttribute:     "#9cdcfe",
		chroma.NameBuiltin:       "#4ec9b0",
		chroma.NameBuiltinPseudo: "#569cd6",
		chroma.NameClass:         "#4ec9b0",
		chroma.NameConstant:      "#4fc1ff",
		chroma.NameDecorator:     "#dcdcaa",
		chroma.NameEntity:        "#569cd6",
		chroma.NameException:     "#4ec9b0",
		chroma.NameFunction:      "#dcdcaa",
		chroma.NameLabel:         "#c8c8c8",
		chroma.NameNamespace:     "#4ec9b0",
		chroma.NameTag:           "#569cd6",

		chroma.LiteralString:      "#ce9178",
		chroma.LiteralStringRegex: "#d16969",
		chroma.LiteralNumber:      "#b5cea8",

		chroma.Operator:     "#d4d4d4",
		chroma.OperatorWord: "#569cd6",
		chroma.Punctuation:  "#d4d4d4",

		chroma.GenericHeading:    "bold #569cd6",
		chroma.GenericSubheading: "bold #569cd6",
		chroma.GenericEmph:       "italic",
		chroma.GenericStrong:     "bold",
		chroma.GenericDeleted:    "#ce9178",
		chroma.GenericInserted:   "#b5cea8",
	}))
}
