package highlight

import "strings"

// writeSGR writes one SGR sequence that turns style from into style to. With
// reset it starts from a full reset instead, so the line never inherits
// whatever the terminal state was before it.
func writeSGR(b *strings.Builder, from, to cellStyle, reset bool) {
	b.WriteString("\x1b[")
	p := params{b: b}
	if reset {
		p.add("0")
		from = cellStyle{}
	}
	if to.bold != from.bold {
		p.add(pick(to.bold, "1", "22"))
	}
	if to.italic != from.italic {
		p.add(pick(to.italic, "3", "23"))
	}
	if to.underline != from.underline {
		p.add(pick(to.underline, "4", "24"))
	}
	if to.fg != from.fg {
		p.sep()
		writeColour(b, to.fg, "38", "39")
	}
	if to.bg != from.bg {
		p.sep()
		writeColour(b, to.bg, "48", "49")
	}
	b.WriteByte('m')
}

// params separates SGR parameters with ';'.
type params struct {
	b *strings.Builder
	n int
}

func (p *params) sep() {
	if p.n > 0 {
		p.b.WriteByte(';')
	}
	p.n++
}

func (p *params) add(s string) {
	p.sep()
	p.b.WriteString(s)
}

func pick(on bool, yes, no string) string {
	if on {
		return yes
	}
	return no
}

// writeColour writes the 24-bit parameters for hex ("#rrggbb"), or the
// terminal-default parameter when hex is empty or malformed: an unusable
// colour must never paint something the theme did not ask for.
func writeColour(b *strings.Builder, hex, truecolor, fallback string) {
	r, g, bl, ok := parseHex(hex)
	if !ok {
		b.WriteString(fallback)
		return
	}
	b.WriteString(truecolor)
	b.WriteString(";2;")
	writeUint8(b, r)
	b.WriteByte(';')
	writeUint8(b, g)
	b.WriteByte(';')
	writeUint8(b, bl)
}

func parseHex(s string) (r, g, b byte, ok bool) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0, false
	}
	var v [3]byte
	for i := range v {
		hi, ok1 := hexDigit(s[1+2*i])
		lo, ok2 := hexDigit(s[2+2*i])
		if !ok1 || !ok2 {
			return 0, 0, 0, false
		}
		v[i] = hi<<4 | lo
	}
	return v[0], v[1], v[2], true
}

func hexDigit(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func writeUint8(b *strings.Builder, v byte) {
	if v >= 100 {
		b.WriteByte('0' + v/100)
	}
	if v >= 10 {
		b.WriteByte('0' + v/10%10)
	}
	b.WriteByte('0' + v%10)
}
