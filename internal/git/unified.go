package git

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/DavidkJun/revue/internal/diff"
)

// section is one "diff --git" block of git diff output.
type section struct {
	title            string // what follows "diff --git "
	oldPath, newPath string // "" for /dev/null or when the header does not say
	deleted          bool   // "deleted file mode": the file is gone on the new side
	binary           bool
	hunks            bool
	lines            []diff.Line
}

// parseUnified parses git diff output into sections. Hunk bodies are
// consumed by the line counts of their "@@" header, so content that merely
// looks like a header ("--- x", "diff --git") is never taken for one.
func parseUnified(out []byte) []section {
	var secs []section
	var oldNo, newNo, oldLeft, newLeft int
	for len(out) > 0 {
		var raw []byte
		if i := bytes.IndexByte(out, '\n'); i >= 0 {
			raw, out = out[:i], out[i+1:]
		} else {
			raw, out = out, nil
		}

		if oldLeft > 0 || newLeft > 0 {
			sec := &secs[len(secs)-1]
			switch {
			// diff.suppressBlankEmpty prints an empty context line as "".
			case (len(raw) == 0 || raw[0] == ' ') && oldLeft > 0 && newLeft > 0:
				sec.lines = append(sec.lines, diff.Line{Kind: diff.Context, OldNo: oldNo, NewNo: newNo, Text: lineText(raw)})
				oldNo, newNo, oldLeft, newLeft = oldNo+1, newNo+1, oldLeft-1, newLeft-1
				continue
			case len(raw) > 0 && raw[0] == '-' && oldLeft > 0:
				sec.lines = append(sec.lines, diff.Line{Kind: diff.Deleted, OldNo: oldNo, Text: lineText(raw)})
				oldNo, oldLeft = oldNo+1, oldLeft-1
				continue
			case len(raw) > 0 && raw[0] == '+' && newLeft > 0:
				sec.lines = append(sec.lines, diff.Line{Kind: diff.Added, NewNo: newNo, Text: lineText(raw)})
				newNo, newLeft = newNo+1, newLeft-1
				continue
			case len(raw) > 0 && raw[0] == '\\':
				continue
			}
			// The hunk ended early; read on as headers.
			oldLeft, newLeft = 0, 0
		}

		line := string(raw)
		switch {
		case strings.HasPrefix(line, "diff --git "):
			secs = append(secs, section{title: line[len("diff --git "):]})
		case len(secs) == 0:
		case strings.HasPrefix(line, "@@ "):
			if oStart, oCount, nStart, nCount, ok := parseHunkHeader(line); ok {
				oldNo, oldLeft, newNo, newLeft = oStart, oCount, nStart, nCount
				secs[len(secs)-1].hunks = true
			}
		default:
			secs[len(secs)-1].header(line)
		}
	}
	return secs
}

// lineText strips the one-character prefix and a CRLF file's "\r".
func lineText(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	return string(bytes.TrimSuffix(raw[1:], []byte{'\r'}))
}

func (s *section) header(line string) {
	switch {
	case strings.HasPrefix(line, "--- "):
		s.oldPath = headerPath(line[4:], "a/")
	case strings.HasPrefix(line, "+++ "):
		s.newPath = headerPath(line[4:], "b/")
	case strings.HasPrefix(line, "rename from "):
		s.oldPath = unquotePath(line[len("rename from "):])
	case strings.HasPrefix(line, "rename to "):
		s.newPath = unquotePath(line[len("rename to "):])
	case strings.HasPrefix(line, "copy from "):
		s.oldPath = unquotePath(line[len("copy from "):])
	case strings.HasPrefix(line, "copy to "):
		s.newPath = unquotePath(line[len("copy to "):])
	case strings.HasPrefix(line, "deleted file mode "):
		s.deleted = true
	case strings.HasPrefix(line, "Binary files "), line == "GIT binary patch":
		s.binary = true
	}
}

// name is the path the section is about, for messages.
func (s section) name() string {
	switch {
	case s.newPath != "":
		return s.newPath
	case s.oldPath != "":
		return s.oldPath
	}
	// A binary or mode-only change names its path only in "a/<p> b/<p>".
	if n := len(s.title); n > 5 && (n-5)%2 == 0 {
		p := s.title[2 : 2+(n-5)/2]
		if s.title == "a/"+p+" b/"+p {
			return p
		}
	}
	return s.title
}

// headerPath reads the path of a "---" or "+++" line. git appends a tab
// when the name contains a space and C-quotes names with special bytes.
func headerPath(s, prefix string) string {
	s = strings.TrimSuffix(s, "\t")
	if s == "/dev/null" {
		return ""
	}
	return strings.TrimPrefix(unquotePath(s), prefix)
}

func unquotePath(s string) string {
	if strings.HasPrefix(s, `"`) {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
	}
	return s
}

// parseHunkHeader reads "@@ -os[,oc] +ns[,nc] @@"; a missing count is 1.
func parseHunkHeader(line string) (oldStart, oldCount, newStart, newCount int, ok bool) {
	rest, found := strings.CutPrefix(line, "@@ -")
	if !found {
		return
	}
	ranges, _, found := strings.Cut(rest, " @@")
	if !found {
		return
	}
	o, n, found := strings.Cut(ranges, " +")
	if !found {
		return
	}
	oldStart, oldCount, okOld := parseRange(o)
	newStart, newCount, okNew := parseRange(n)
	return oldStart, oldCount, newStart, newCount, okOld && okNew
}

func parseRange(s string) (start, count int, ok bool) {
	a, b, hasCount := strings.Cut(s, ",")
	start, err := strconv.Atoi(a)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	count = 1
	if hasCount {
		if count, err = strconv.Atoi(b); err != nil || count < 0 {
			return 0, 0, false
		}
	}
	return start, count, true
}
