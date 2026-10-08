package diffview

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// search is the active pattern and its matches in the current view. The
// matches are recomputed lazily when the file or the view changes.
type search struct {
	pattern string
	valid   bool
	matches []match // sorted by line, side, start
}

// match is one occurrence in the text of a view line. side tells the halves
// of a split row apart: 0 is the old side (and context), 1 the new side.
type match struct {
	line       int32
	side       uint8
	start, end int32
}

func (m *Model) setSearch(pattern string) int {
	m.search = search{pattern: pattern}
	if pattern == "" || !m.active() {
		return 0
	}
	m.fresh = false
	ms := m.matches()
	if len(ms) == 0 {
		return 0
	}
	from := m.cursorLine()
	k := sort.Search(len(ms), func(k int) bool { return int(ms[k].line) >= from })
	if k == len(ms) {
		k = 0
	}
	m.reveal(m.cur(), int(ms[k].line))
	return len(ms)
}

func (m *Model) matches() []match {
	s := &m.search
	if !s.valid {
		s.matches = m.findAll(s.pattern)
		s.valid = true
	}
	return s.matches
}

func (m *Model) findAll(pattern string) []match {
	if pattern == "" || !m.active() {
		return nil
	}
	fold := !hasUpper(pattern)
	if fold {
		pattern = strings.ToLower(pattern)
	}
	var out []match
	add := func(line int, side uint8, text string) {
		find(text, pattern, fold, func(s, e int) {
			out = append(out, match{line: int32(line), side: side, start: int32(s), end: int32(e)})
		})
	}
	f := m.f
	if !m.split {
		for i, r := range f.rows {
			add(i, 0, r.Text)
		}
		return out
	}
	for l, sr := range f.split {
		if sr.Left >= 0 {
			add(l, 0, f.rows[sr.Left].Text)
		}
		if sr.Right >= 0 && sr.Right != sr.Left {
			add(l, 1, f.rows[sr.Right].Text)
		}
	}
	return out
}

// find reports every non-overlapping occurrence of pattern in text as a byte
// range of text. With fold the pattern is already lower case and the text
// is compared case-insensitively.
func find(text, pattern string, fold bool, emit func(start, end int)) {
	if !fold {
		for i := 0; ; {
			k := strings.Index(text[i:], pattern)
			if k < 0 {
				return
			}
			emit(i+k, i+k+len(pattern))
			i += k + len(pattern)
		}
	}
	if isASCII(text) {
		find(strings.ToLower(text), pattern, false, emit)
		return
	}
	// Lower-casing can change the byte length of non-ASCII text, so compare
	// rune by rune to keep offsets into the original.
	for i := 0; i < len(text); {
		if n, ok := prefixFold(text[i:], pattern); ok {
			emit(i, i+n)
			i += n
			continue
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		i += size
	}
}

// prefixFold reports whether s starts with the lower-case pattern p, case
// ignored, and how many bytes of s that prefix takes.
func prefixFold(s, p string) (int, bool) {
	n := 0
	for p != "" {
		if s == "" {
			return 0, false
		}
		a, sa := utf8.DecodeRuneInString(s)
		b, sb := utf8.DecodeRuneInString(p)
		if a != b && unicode.ToLower(a) != b {
			return 0, false
		}
		s, p, n = s[sa:], p[sb:], n+sa
	}
	return n, n > 0
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func hasUpper(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

// searchJump moves to the count-th next (or previous) line with a match,
// wrapping around the file like Vim's wrapscan.
func (m *Model) searchJump(count int, forward bool) tea.Cmd {
	if m.search.pattern == "" {
		return notify("no previous search", true)
	}
	ms := m.matches()
	if len(ms) == 0 {
		return notify("pattern not found: "+m.search.pattern, true)
	}
	p := m.cur()
	it := p.folds.Items()[p.cursor]
	start, _ := span(it)
	// On a closed fold, matches hidden at its first line count as ahead.
	pos := start
	if it.Fold >= 0 && forward {
		pos = start - 1
	}
	wrapped := false
	for range count {
		var k int
		if forward {
			k = sort.Search(len(ms), func(k int) bool { return int(ms[k].line) > pos })
			if k == len(ms) {
				k, wrapped = 0, true
			}
		} else {
			k = sort.Search(len(ms), func(k int) bool { return int(ms[k].line) >= pos }) - 1
			if k < 0 {
				k, wrapped = len(ms)-1, true
			}
		}
		pos = int(ms[k].line)
	}
	m.reveal(p, pos)
	if wrapped {
		return notify("search wrapped", false)
	}
	return nil
}

// lineMatches returns the matches on line l of the current view.
func (m *Model) lineMatches(l int) []match {
	if m.search.pattern == "" {
		return nil
	}
	ms := m.matches()
	k := sort.Search(len(ms), func(k int) bool { return int(ms[k].line) >= l })
	e := k
	for e < len(ms) && int(ms[e].line) == l {
		e++
	}
	return ms[k:e]
}
