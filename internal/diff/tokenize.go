package diff

import (
	"unicode"
	"unicode/utf8"

	"github.com/rivo/uniseg"
)

type tokenKind uint8

const (
	tokWord  tokenKind = iota // run of letters, digits and underscores
	tokSpace                  // run of whitespace
	tokOther                  // one character of anything else
)

// token is the byte range [start, end) of one word-diff token.
type token struct {
	start, end int
	kind       tokenKind
}

// tokenize splits s into word runs, whitespace runs and single other
// characters. It steps over grapheme clusters rather than runes so that a
// combining accent or an emoji modifier always stays in the same token as the
// character it modifies, and changed ranges never split a visible character.
func tokenize(s string) []token {
	toks := make([]token, 0, len(s)/4+1)
	for i := 0; i < len(s); {
		n, kind := nextCluster(s, i)
		if last := len(toks) - 1; last >= 0 && kind != tokOther && toks[last].kind == kind {
			toks[last].end = i + n
		} else {
			toks = append(toks, token{start: i, end: i + n, kind: kind})
		}
		i += n
	}
	return toks
}

// nextCluster returns the byte length and class of the grapheme cluster at
// s[i]. An ASCII byte followed by another ASCII byte (or the end) is always a
// cluster of its own, which keeps plain code off the slower Unicode path; the
// only exception, CR LF, is whitespace either way.
func nextCluster(s string, i int) (int, tokenKind) {
	c := s[i]
	if c < utf8.RuneSelf && (i+1 == len(s) || s[i+1] < utf8.RuneSelf) {
		return 1, classifyASCII(c)
	}
	cluster, _, _, _ := uniseg.FirstGraphemeClusterInString(s[i:], -1)
	if len(cluster) == 0 {
		return 1, tokOther
	}
	r, size := utf8.DecodeRuneInString(cluster)
	kind := classifyRune(r)
	// A mark on a space or NBSP is how a standalone accent is written: the
	// cluster is visible, so it must count as content, not whitespace.
	if kind == tokSpace && size < len(cluster) {
		kind = tokOther
	}
	return len(cluster), kind
}

func classifyASCII(c byte) tokenKind {
	switch {
	case c == '_', '0' <= c && c <= '9', 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z':
		return tokWord
	case c == ' ', '\t' <= c && c <= '\r':
		return tokSpace
	default:
		return tokOther
	}
}

func classifyRune(r rune) tokenKind {
	switch {
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return tokWord
	case unicode.IsSpace(r):
		return tokSpace
	default:
		return tokOther
	}
}

// interner gives equal token texts equal ids so comparisons are integer
// compares. weight[id] is the token's non-whitespace byte count, which is
// what similarity is measured in.
type interner struct {
	ids    map[string]int32
	weight []int32
}

func newInterner() *interner {
	return &interner{ids: make(map[string]int32)}
}

func (in *interner) id(text string, kind tokenKind) int32 {
	if id, ok := in.ids[text]; ok {
		return id
	}
	id := int32(len(in.weight))
	in.ids[text] = id
	w := int32(len(text))
	if kind == tokSpace {
		w = 0
	}
	in.weight = append(in.weight, w)
	return id
}

// tokLine is one line prepared for comparison.
type tokLine struct {
	text  string
	toks  []token
	ids   []int32 // parallel to toks
	nonWS int     // bytes outside whitespace tokens
}

func (in *interner) line(s string) tokLine {
	toks := tokenize(s)
	ids := make([]int32, len(toks))
	nonWS := 0
	for k, t := range toks {
		ids[k] = in.id(s[t.start:t.end], t.kind)
		if t.kind != tokSpace {
			nonWS += t.end - t.start
		}
	}
	return tokLine{text: s, toks: toks, ids: ids, nonWS: nonWS}
}
