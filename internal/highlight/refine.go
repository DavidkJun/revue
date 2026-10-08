package highlight

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2"
)

// controlWords are the keywords Dark+ colours pink instead of blue: control
// flow and module wiring, in any language.
var controlWords = wordSet("import", "from", "export", "return", "if", "else", "elif", "for", "while",
	"do", "switch", "case", "default", "break", "continue", "try", "catch", "except", "finally",
	"throw", "raise", "of", "yield", "goto", "select", "fallthrough", "defer")

// jsFamily are the chroma lexers whose names Refine tells apart: they tag
// every TypeScript/JavaScript identifier alike (Name.Other).
var jsFamily = wordSet("TypeScript", "JavaScript", "react")

// contextualWords are JS/TS keywords that are plain names in some places:
// get(...) is a call, Array.from a method.
var contextualWords = wordSet("get", "set", "of", "from", "as", "type", "declare", "readonly",
	"abstract", "module", "namespace", "is", "keyof", "infer")

// arrowScan bounds how far assignsFunction looks for the "=>" of an arrow
// function, so a long parenthesised expression cannot cost more.
const arrowScan = 400

// Refine reclassifies the tokens of a file lexed by the chroma lexer named
// lang to approximate how the user's Neovim (tree-sitter with vscode.nvim)
// colours code: control-flow keywords pink, and in JS/TS names split into
// functions (called or assigned an arrow function), properties (after a dot),
// types (Capitalised), constants (ALL_CAPS) and variables. It may reuse toks.
func Refine(lang string, toks []chroma.Token) []chroma.Token {
	js := jsFamily[lang]
	if js {
		toks = splitNames(toks)
	}
	for i := range toks {
		t := &toks[i]
		switch {
		case lang == "JSON" && t.Type == chroma.NameTag:
			t.Type = chroma.NameProperty
		case t.Type.InCategory(chroma.Keyword) && t.Type != chroma.KeywordType && t.Type != chroma.KeywordConstant:
			t.Type = keywordType(toks, i, js)
		case js && t.Type == chroma.NameTag && upperFirst(strings.TrimSpace(t.Value)):
			t.Type = chroma.NameClass
		case js && (t.Type == chroma.NameOther || t.Type == chroma.Name):
			t.Type = nameType(toks, i)
		}
	}
	return toks
}

// splitNames cuts name tokens that carry more than a name, which chroma's
// TypeScript lexer produces for getters ("domain() {"), into the name and
// punctuation, so the "(" after it can be seen.
func splitNames(toks []chroma.Token) []chroma.Token {
	var out []chroma.Token
	for i, t := range toks {
		cut := identLen(t.Value)
		if (t.Type != chroma.NameOther && t.Type != chroma.Name) || cut == 0 || cut == len(t.Value) {
			if out != nil {
				out = append(out, t)
			}
			continue
		}
		if out == nil {
			out = append(make([]chroma.Token, 0, len(toks)+8), toks[:i]...)
		}
		out = append(out,
			chroma.Token{Type: t.Type, Value: t.Value[:cut]},
			chroma.Token{Type: chroma.Punctuation, Value: t.Value[cut:]})
	}
	if out == nil {
		return toks
	}
	return out
}

// identLen is the length of the identifier at the start of s.
func identLen(s string) int {
	for i, r := range s {
		if r != '_' && r != '$' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return i
		}
	}
	return len(s)
}

func keywordType(toks []chroma.Token, i int, js bool) chroma.TokenType {
	word := strings.TrimSpace(toks[i].Value)
	switch {
	case js && strings.HasPrefix(word, "@"):
		return chroma.NameDecorator
	case js && contextualWords[word] && (afterDot(toks, i) || calledNext(toks, i)):
		return nameType(toks, i)
	case controlWords[word]:
		return chroma.KeywordNamespace
	}
	return toks[i].Type
}

func nameType(toks []chroma.Token, i int) chroma.TokenType {
	word := strings.TrimSpace(toks[i].Value)
	switch {
	case word == "constructor":
		return chroma.Keyword
	case afterDot(toks, i):
		if calledNext(toks, i) {
			return chroma.NameFunction
		}
		return chroma.NameProperty
	case allCaps(word):
		return chroma.NameConstant
	case upperFirst(word):
		return chroma.NameClass
	case calledNext(toks, i) || assignsFunction(toks, i):
		return chroma.NameFunction
	}
	return chroma.NameOther
}

// significant reports whether a token takes part in the grammar: not
// whitespace and not a comment.
func significant(t chroma.Token) bool {
	return strings.TrimSpace(t.Value) != "" && !t.Type.InCategory(chroma.Comment)
}

// neighbour returns the trimmed value of the n-th significant token after
// (step 1) or before (step -1) index i.
func neighbour(toks []chroma.Token, i, step, n int) (string, int) {
	for j := i + step; j >= 0 && j < len(toks); j += step {
		if significant(toks[j]) {
			if n--; n == 0 {
				return strings.TrimSpace(toks[j].Value), j
			}
		}
	}
	return "", -1
}

// afterDot: a member access (obj.name, obj?.name), but not a spread (...name).
func afterDot(toks []chroma.Token, i int) bool {
	prev, _ := neighbour(toks, i, -1, 1)
	return strings.HasSuffix(prev, ".") && !strings.HasSuffix(prev, "..")
}

func calledNext(toks []chroma.Token, i int) bool {
	next, _ := neighbour(toks, i, 1, 1)
	return strings.HasPrefix(next, "(")
}

// assignsFunction recognises "name = (args) => ...", "name = arg => ...",
// "name = async ..." and "name = function ...": Neovim colours such a name
// as a function.
func assignsFunction(toks []chroma.Token, i int) bool {
	if eq, _ := neighbour(toks, i, 1, 1); eq != "=" {
		return false
	}
	value, at := neighbour(toks, i, 1, 2)
	switch {
	case value == "async" || value == "function":
		return true
	case strings.HasPrefix(value, "("):
		return arrowAfterParens(toks, at)
	case value != "":
		arrow, _ := neighbour(toks, i, 1, 3)
		return arrow == "=>"
	}
	return false
}

// arrowAfterParens walks from the token opening a parameter list to its
// matching ")" and reports whether "=>" follows before the statement ends,
// allowing for a return type annotation in between.
func arrowAfterParens(toks []chroma.Token, open int) bool {
	depth := 0
	for j := open; j < len(toks) && j < open+arrowScan; j++ {
		t := toks[j]
		if t.Type.InCategory(chroma.LiteralString) || t.Type.InCategory(chroma.Comment) {
			continue
		}
		for _, r := range t.Value {
			switch r {
			case '(', '[', '{':
				depth++
			case ')', ']', '}':
				depth--
			}
		}
		if depth > 0 {
			continue
		}
		for k, n := j, 0; n < 8; n++ {
			next, at := neighbour(toks, k, 1, 1)
			if next == "=>" {
				return true
			}
			if at < 0 || strings.ContainsAny(next, ";{}=") {
				return false
			}
			k = at
		}
		return false
	}
	return false
}

func allCaps(word string) bool {
	letters := 0
	for _, r := range word {
		switch {
		case unicode.IsUpper(r):
			letters++
		case r == '_' || unicode.IsDigit(r):
		default:
			return false
		}
	}
	return letters > 0 && utf8.RuneCountInString(word) >= 2
}

func upperFirst(word string) bool {
	r, _ := utf8.DecodeRuneInString(word)
	return unicode.IsUpper(r)
}

func wordSet(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}
