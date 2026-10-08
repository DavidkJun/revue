package icons

import (
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/DavidkJun/revue/internal/theme"
)

func TestGlyphsAreOneCell(t *testing.T) {
	all := []icon{fallback}
	for _, ic := range byName {
		all = append(all, ic)
	}
	for _, ic := range byExt {
		all = append(all, ic)
	}
	for _, ic := range all {
		if w := ansi.StringWidth(ic.glyph); w != 1 {
			t.Errorf("%U is %d cells wide", []rune(ic.glyph), w)
		}
	}
}

func TestLookup(t *testing.T) {
	for p, want := range map[string]icon{
		"src/app.ts":           byExt["ts"],
		"web/App.TSX":          byExt["tsx"],
		"package.json":         byName["package.json"],
		"config/settings.json": byExt["json"],
		"Dockerfile":           byName["dockerfile"],
		"Dockerfile.dev":       byName["dockerfile"],
		"api.dockerfile":       byName["dockerfile"],
		".env.local":           byExt["env"],
		"scripts/run.ps1":      byExt["ps1"],
		"img/logo.PNG":         byExt["png"],
		"README.md":            byName["readme.md"],
		"docs/guide.md":        byExt["md"],
		"Makefile-with-no-ext": fallback,
		"no-extension":         fallback,
	} {
		if got := lookup(p); got != want {
			t.Errorf("%s: got %U %d, want %U %d", p, []rune(got.glyph), got.hue, []rune(want.glyph), want.hue)
		}
	}
}

func TestColorsComeFromTheTheme(t *testing.T) {
	th := theme.Default
	palette := map[string]bool{th.FG: true, th.Cyan: true, th.Blue: true, th.Green: true, th.Yellow: true, th.Red: true, th.Magenta: true}
	for h := hueGrey; h <= huePurple; h++ {
		if c := h.color(th); !palette[c] {
			t.Errorf("hue %d maps to %q, not a theme colour", h, c)
		}
	}
	if g, c := For(th, "a.ts"); g != byExt["ts"].glyph || c != th.Cyan {
		t.Errorf("For(a.ts) = %U %q", []rune(g), c)
	}
}
