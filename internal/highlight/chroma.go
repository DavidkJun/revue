package highlight

import (
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

const (
	defaultStyle = VSCodeDarkPlus

	// Beyond these, tokenising takes long enough to be felt, and a file that
	// big is rarely worth colouring.
	maxFileBytes = 1 << 20
	maxFileLines = 20000
	maxLineBytes = 10000

	// Some rules rescan the rest of the text on every attempt (a markdown
	// link looking for its ']'), so input inside the size limits can still
	// take minutes; past this, the file is shown plain.
	lexBudget = 3 * time.Second
)

// tokenStyle is a chroma style entry reduced to what a Span carries.
type tokenStyle struct {
	fg                      string
	bold, italic, underline bool
}

// palette maps token types to span styles for one chroma style. It is
// written only while it is built, so File can share it between goroutines.
type palette struct {
	style *chroma.Style
	base  chroma.Colour
	types map[chroma.TokenType]tokenStyle
}

func newPalette(name string) *palette {
	s := lookupStyle(name)
	p := &palette{
		style: s,
		base:  s.Get(chroma.Text).Colour,
		types: make(map[chroma.TokenType]tokenStyle, len(chroma.StandardTypes)),
	}
	for tt := range chroma.StandardTypes {
		p.types[tt] = p.compute(tt)
	}
	return p
}

func lookupStyle(name string) *chroma.Style {
	if name == "" {
		name = defaultStyle
	}
	if s, ok := styles.Registry[strings.ToLower(name)]; ok {
		return s
	}
	if s, ok := styles.Registry[defaultStyle]; ok {
		return s
	}
	return styles.Fallback
}

func (p *palette) of(tt chroma.TokenType) tokenStyle {
	if ts, ok := p.types[tt]; ok {
		return ts
	}
	return p.compute(tt)
}

func (p *palette) compute(tt chroma.TokenType) tokenStyle {
	e := p.style.Get(tt)
	ts := tokenStyle{
		bold:      e.Bold == chroma.Yes,
		italic:    e.Italic == chroma.Yes,
		underline: e.Underline == chroma.Yes,
	}
	// Every entry inherits the style's base text colour; leaving it out lets
	// plain code take the line's own foreground.
	if e.Colour.IsSet() && e.Colour != p.base {
		ts.fg = e.Colour.String()
	}
	return ts
}

func lexerFor(filename string) chroma.Lexer {
	l := lexers.Match(filename)
	if l == nil || l.Config().Name == "plaintext" {
		return nil
	}
	return l
}

// deadlineLexer ends the token stream once its deadline passes. It goes
// under Coalesce, which can pull thousands of tokens into one.
type deadlineLexer struct {
	chroma.Lexer
	deadline time.Time
	expired  *bool
}

func (d deadlineLexer) Tokenise(options *chroma.TokeniseOptions, text string) (chroma.Iterator, error) {
	it, err := d.Lexer.Tokenise(options, text)
	if err != nil {
		return nil, err
	}
	return func() chroma.Token {
		// The iterator lexes lazily, so this bounds the work done, not just
		// the wait; reading the clock is cheap next to lexing a token.
		if time.Now().After(d.deadline) {
			*d.expired = true
			return chroma.EOF
		}
		return it()
	}, nil
}

func highlightFile(pal *palette, filename string, lines []string, budget time.Duration) (out [][]Span) {
	if len(lines) == 0 || len(lines) > maxFileLines {
		return nil
	}
	size := len(lines) - 1
	for _, l := range lines {
		// A newline inside a line would shift every later line's spans.
		if len(l) > maxLineBytes || strings.IndexByte(l, '\n') >= 0 {
			return nil
		}
		size += len(l)
	}
	if size > maxFileBytes {
		return nil
	}
	lexer := lexerFor(filename)
	if lexer == nil {
		return nil
	}
	defer func() {
		// Chroma panics on some malformed input (lexer mutators, unknown
		// states); plain text is a better answer than a crash.
		if recover() != nil {
			out = nil
		}
	}()

	// Lines come without the file's final newline, and rules that match up to
	// a line end (markdown headings, JSX line comments) would miss the last
	// line without one.
	text := strings.Join(lines, "\n") + "\n"
	lang := lexer.Config().Name
	expired := false
	lexer = chroma.Coalesce(deadlineLexer{Lexer: lexer, deadline: time.Now().Add(budget), expired: &expired})
	// EnsureLF would rewrite \r and shift every offset after it.
	it, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root"}, text)
	if err != nil {
		return nil
	}
	// Refine needs the tokens around each one, so the whole file is lexed
	// before any line is split.
	var toks []chroma.Token
	for tok := it(); tok != chroma.EOF; tok = it() {
		toks = append(toks, tok)
	}
	if expired {
		return nil
	}
	toks = Refine(lang, toks)
	sp := splitter{text: text, ends: make([]int, 0, len(lines))}
	for _, tok := range toks {
		if !sp.add(tok.Value, pal.of(tok.Type)) {
			return nil
		}
	}
	return sp.result(len(lines))
}
