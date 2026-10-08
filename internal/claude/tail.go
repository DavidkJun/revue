package claude

import (
	"strings"
	"unicode/utf8"
)

// stderrTail is how much of claude's stderr is kept for error messages.
const stderrTail = 2 << 10

// tailBuffer is an io.Writer that keeps only the last max bytes written.
// exec's copier is the only writer, and it is read after Wait returns.
type tailBuffer struct {
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= t.max {
		t.buf = append(t.buf[:0], p[len(p)-t.max:]...)
		return n, nil
	}
	if over := len(t.buf) + len(p) - t.max; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	t.buf = append(t.buf, p...)
	return n, nil
}

// String returns the kept bytes, dropping a rune cut in half at the start.
func (t *tailBuffer) String() string {
	b := t.buf
	for len(b) > 0 && !utf8.RuneStart(b[0]) {
		b = b[1:]
	}
	return strings.TrimSpace(string(b))
}
