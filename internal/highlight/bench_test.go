package highlight

import (
	"strings"
	"testing"
)

// benchLine is a typical 120-character line of NestJS code.
const benchLine = "\t\tconst user = await this.usersRepository.findOne({ where: { id: userId, deletedAt: IsNull() }, relations: ['roles'] });"

// benchSpans colours the first 20 words and punctuation runs of line, as a
// style that colours identifiers too would. File's own spans for benchLine
// are fewer (plain identifiers get none), which would flatter the benchmark.
func benchSpans(line string) []Span {
	colours := []string{"#bb9af7", "#7aa2f7", "#e0af68", "#9ece6a"}
	var spans []Span
	for i := 0; i < len(line) && len(spans) < 20; {
		if line[i] == ' ' || line[i] == '\t' {
			i++
			continue
		}
		word := isWordByte(line[i])
		j := i
		for j < len(line) && line[j] != ' ' && isWordByte(line[j]) == word {
			j++
		}
		n := len(spans)
		spans = append(spans, Span{Start: i, End: j, FG: colours[n%len(colours)], Bold: !word})
		i = j
	}
	return spans
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

func benchInput(b *testing.B) ([]Span, []Mark, LineStyle) {
	b.Helper()
	spans := benchSpans(benchLine)
	at := strings.Index(benchLine, "findOne")
	marks := []Mark{
		{Start: at, End: at + 7, BG: "#1f4d2b"},
		{Start: at + 20, End: at + 40, BG: "#1f4d2b"},
		{Start: at + 30, End: at + 35, BG: "#5a4d1c", FG: "#ffd95c", Bold: true},
	}
	return spans, marks, LineStyle{Width: 140, BG: "#12261c", FG: "#d5dbff"}
}

func BenchmarkRender(b *testing.B) {
	spans, marks, st := benchInput(b)
	b.Logf("%d bytes, %d spans, %d marks", len(benchLine), len(spans), len(marks))
	b.ReportAllocs()
	for b.Loop() {
		Render(benchLine, spans, marks, st)
	}
}

func BenchmarkRenderScrolled(b *testing.B) {
	spans, marks, st := benchInput(b)
	st.XOffset, st.Width = 40, 60
	b.ReportAllocs()
	for b.Loop() {
		Render(benchLine, spans, marks, st)
	}
}

func BenchmarkRenderCyrillic(b *testing.B) {
	line := "\t// Повертає користувача за ідентифікатором або кидає помилку, якщо його не знайдено 中文 \U0001F600"
	st := LineStyle{Width: 120, FG: "#d5dbff"}
	b.ReportAllocs()
	for b.Loop() {
		Render(line, nil, nil, st)
	}
}

func BenchmarkWidth(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		Width(benchLine, 4)
	}
}

func BenchmarkFile(b *testing.B) {
	var lines []string
	for len(lines) < 1000 {
		lines = append(lines, tsSample...)
		lines = append(lines, benchLine)
	}
	lines = lines[:1000]
	h := New("")
	b.ReportAllocs()
	for b.Loop() {
		h.File("users.service.ts", lines)
	}
}
