// Package highlight provides syntax colouring (chroma) and the line painter
// that merges syntax colours, diff backgrounds and marks into a terminal line
// of an exact width.
package highlight

// Span styles the byte range [Start, End) of a line.
type Span struct {
	Start, End int
	FG         string // "#rrggbb"; "" = the line's default foreground
	Bold       bool
	Italic     bool
	Underline  bool
}

// Mark paints a background (and optionally a foreground) over the byte range
// [Start, End) of a line: word-level changes, search matches. Later marks
// win over earlier ones where they overlap.
type Mark struct {
	Start, End int
	BG         string // "#rrggbb"
	FG         string // "" keeps the syntax colour
	Bold       bool
}

// LineStyle controls how Render lays out one line.
type LineStyle struct {
	Width    int    // exact output width in terminal cells
	XOffset  int    // horizontal scroll in cells (cells before it are dropped)
	TabWidth int    // tab stop width; 0 means 4
	BG       string // background of the whole line, padding included; "" = terminal default
	FG       string // default foreground; "" = terminal default
}

// Highlighter colours source files. It is safe for concurrent use.
type Highlighter struct {
	style string
	pal   *palette
}

// New returns a highlighter using the chroma style with the given name
// (e.g. "tokyonight-storm"), falling back to a built-in dark style.
func New(style string) *Highlighter { return &Highlighter{style: style, pal: newPalette(style)} }

// File returns per-line spans for lines of the file called filename (used to
// pick the lexer). It returns nil when no lexer matches or the input is too
// large to highlight quickly, and gives up with nil after a few seconds of
// lexing; callers then render plain text. Spans use byte
// offsets into the corresponding line. Text in the style's base colour gets
// no span, so it takes the line's default foreground.
func (h *Highlighter) File(filename string, lines []string) [][]Span {
	if h == nil {
		return nil
	}
	pal := h.pal
	if pal == nil {
		pal = newPalette(h.style)
	}
	return highlightFile(pal, filename, lines, lexBudget)
}

// Render paints text into exactly st.Width cells: tabs expanded, control
// characters made visible, wide characters respected (a wide character cut
// by an edge becomes a space), syntax spans as foregrounds, marks as
// backgrounds, the rest of the line padded with st.BG. The result contains
// SGR sequences only (no cursor movement) and ends with a full reset.
func Render(text string, spans []Span, marks []Mark, st LineStyle) string {
	return render(text, spans, marks, st)
}

// Width returns the display width of text in cells, with tabs expanded the
// same way Render expands them.
func Width(text string, tabWidth int) int { return width(text, tabWidth) }
