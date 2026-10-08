package highlight

import (
	"strings"
	"unicode/utf8"
)

// splitter cuts a token stream into per-line spans. It matches every token
// against the original text instead of trusting token lengths: chroma lexes
// runes, so invalid UTF-8 comes back as U+FFFD and would shift every offset
// after it.
type splitter struct {
	text      string
	pos       int // byte offset in text
	lineStart int // byte offset where the current line starts
	spans     []Span
	ends      []int // ends[i] is len(spans) once line i is done
}

// add consumes one token, reporting false when it does not match the text.
func (s *splitter) add(v string, ts tokenStyle) bool {
	for len(v) > 0 && s.pos < len(s.text) {
		seg, rest, nl := strings.Cut(v, "\n")
		start := s.pos
		n, ok := s.match(seg)
		if !ok {
			return false
		}
		s.pos += n
		s.emit(start, s.pos, ts)
		if !nl || s.pos == len(s.text) {
			// A newline past the end is one a lexer appended to the text.
			return true
		}
		if s.text[s.pos] != '\n' {
			return false
		}
		s.pos++
		s.ends = append(s.ends, len(s.spans))
		s.lineStart = s.pos
		v = rest
	}
	return true
}

// match returns how many bytes of text at pos seg covers.
func (s *splitter) match(seg string) (int, bool) {
	rest := s.text[s.pos:]
	if strings.HasPrefix(rest, seg) {
		return len(seg), true
	}
	n := 0
	for len(seg) > 0 && n < len(rest) {
		rv, sn := utf8.DecodeRuneInString(seg)
		rt, tn := utf8.DecodeRuneInString(rest[n:])
		if rv != rt {
			return 0, false
		}
		seg = seg[sn:]
		n += tn
	}
	return n, true
}

func (s *splitter) emit(start, end int, ts tokenStyle) {
	if start == end || ts == (tokenStyle{}) {
		return
	}
	if !ts.underline && blank(s.text[start:end]) {
		return
	}
	sp := Span{
		Start:     start - s.lineStart,
		End:       end - s.lineStart,
		FG:        ts.fg,
		Bold:      ts.bold,
		Italic:    ts.italic,
		Underline: ts.underline,
	}
	lineFirst := 0
	if len(s.ends) > 0 {
		lineFirst = s.ends[len(s.ends)-1]
	}
	// Neighbouring tokens of different types often share a colour; one span
	// is cheaper to paint than two.
	if n := len(s.spans); n > lineFirst {
		last := &s.spans[n-1]
		if last.End == sp.Start && last.FG == sp.FG && last.Bold == sp.Bold &&
			last.Italic == sp.Italic && last.Underline == sp.Underline {
			last.End = sp.End
			return
		}
	}
	s.spans = append(s.spans, sp)
}

func (s *splitter) result(lines int) [][]Span {
	out := make([][]Span, lines)
	from := 0
	for i := range out {
		to := len(s.spans)
		if i < len(s.ends) {
			to = s.ends[i]
		}
		if to > from {
			out[i] = s.spans[from:to:to]
		}
		from = to
	}
	return out
}

func blank(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\t' {
			return false
		}
	}
	return true
}
