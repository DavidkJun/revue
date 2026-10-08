// Package icons picks the Nerd Font icon and colour of a file, so the file
// tree and the diff pane show the same one. Glyphs and hues follow
// mini.icons, which the user's LazyVim takes its file icons from.
package icons

import (
	"path"
	"strings"

	"github.com/DavidkJun/revue/internal/theme"
)

// For returns the icon of the file at p and its colour in th. Every glyph is
// one cell wide in a "Nerd Font Mono" font.
func For(th theme.Theme, p string) (glyph, color string) {
	ic := lookup(p)
	return ic.glyph, ic.hue.color(th)
}

// hue names a mini.icons highlight group.
type hue uint8

const (
	hueGrey hue = iota
	hueAzure
	hueBlue
	hueCyan
	hueGreen
	hueYellow
	hueOrange
	hueRed
	huePurple
)

type icon struct {
	glyph string
	hue   hue
}

// Glyphs are written as escapes: the private-use code points are invisible
// (and easily lost) in most editors.

// byName is matched on the whole file name first, so package.json does not
// get the generic JSON icon.
var byName = map[string]icon{
	"package.json":        {"\U0000e718", hueGreen},
	"tsconfig.json":       {"\U0000e628", hueAzure},
	"tsconfig.build.json": {"\U0000e628", hueAzure},
	"yarn.lock":           {"\U0000e6a7", hueBlue},
	".yarnrc.yml":         {"\U0000e6a7", hueBlue},
	".prettierrc":         {"\U0000e6b4", huePurple},
	".prettierignore":     {"\U0000e6b4", hueOrange},
	".eslintrc.js":        {"\U000f0c7a", hueYellow},
	"eslint.config.js":    {"\U000f0c7a", hueYellow},
	"eslint.config.mjs":   {"\U000f0c7a", hueYellow},
	".node-version":       {"\U0000e718", hueGreen},
	".nvmrc":              {"\U0000e718", hueGreen},
	".gitignore":          {"\U000f02a2", huePurple},
	".gitattributes":      {"\U000f02a2", hueYellow},
	".gitkeep":            {"\U000f02a2", hueRed},
	".dockerignore":       {"\U000f0868", hueOrange},
	"dockerfile":          {"\U000f0868", hueBlue},
	"docker-compose.yml":  {"\U000f0868", hueYellow},
	"docker-compose.yaml": {"\U000f0868", hueYellow},
	"compose.yml":         {"\U000f0868", hueYellow},
	"compose.yaml":        {"\U000f0868", hueYellow},
	"makefile":            {"\U000f1064", hueGrey},
	"license":             {"\U0000e60a", hueCyan},
	"readme.md":           {"\U0000f4ed", hueYellow},
	".editorconfig":       {"\U0000e652", hueGrey},
	"go.mod":              {"\U000f07d3", hueAzure},
	"go.sum":              {"\U000f07d3", hueCyan},
}

var byExt = map[string]icon{
	"ts":      {"\U000f06e6", hueAzure},
	"mts":     {"\U000f06e6", hueAzure},
	"cts":     {"\U000f06e6", hueAzure},
	"tsx":     {"\U0000e7ba", hueBlue},
	"js":      {"\U000f031e", hueYellow},
	"mjs":     {"\U000f031e", hueYellow},
	"cjs":     {"\U000f031e", hueYellow},
	"jsx":     {"\U0000e625", hueAzure},
	"json":    {"\U000f0626", hueYellow},
	"jsonc":   {"\U000f0626", hueYellow},
	"go":      {"\U000f07d3", hueAzure},
	"md":      {"\U000f0354", hueGrey},
	"mdx":     {"\U000f0354", hueGrey},
	"css":     {"\U000f031c", hueAzure},
	"scss":    {"\U000f07ec", hueRed},
	"sass":    {"\U000f07ec", hueRed},
	"less":    {"\U000f031c", huePurple},
	"html":    {"\U000f031d", hueOrange},
	"htm":     {"\U000f031d", hueOrange},
	"yml":     {"\U0000e6a8", huePurple},
	"yaml":    {"\U0000e6a8", huePurple},
	"toml":    {"\U0000e6b2", hueOrange},
	"py":      {"\U000f0320", hueYellow},
	"sh":      {"\U0000e691", hueGrey},
	"bash":    {"\U0000e691", hueGreen},
	"zsh":     {"\U0000e691", hueGreen},
	"ps1":     {"\U000f0a0a", hueBlue},
	"psm1":    {"\U000f0a0a", hueBlue},
	"bat":     {"\U000f0bc2", hueGreen},
	"cmd":     {"\U000f0bc2", hueGreen},
	"sql":     {"\U000f01bc", hueGrey},
	"prisma":  {"\U0000e684", hueBlue},
	"graphql": {"\U000f0877", hueRed},
	"gql":     {"\U000f0877", hueRed},
	"xml":     {"\U000f05c0", hueOrange},
	"svg":     {"\U000f0721", hueYellow},
	"vue":     {"\U000f0844", hueGreen},
	"lua":     {"\U000f08b1", hueAzure},
	"rs":      {"\U000f1617", hueOrange},
	"java":    {"\U000f0b37", hueOrange},
	"txt":     {"\U000f09aa", hueYellow},
	"csv":     {"\U0000e64a", hueGreen},
	"ini":     {"\U000f0bc2", hueAzure},
	"conf":    {"\U000f0493", hueGrey},
	"env":     {"\U0000eb52", hueYellow},
	"png":     {"\U000f0e2d", huePurple},
	"jpg":     {"\U000f0225", hueOrange},
	"jpeg":    {"\U000f0225", hueOrange},
	"gif":     {"\U000f0d78", hueAzure},
	"bmp":     {"\U000f021f", hueGreen},
	"webp":    {"\U000f021f", hueBlue},
	"tif":     {"\U000f021f", hueYellow},
	"tiff":    {"\U000f021f", hueYellow},
	"zip":     {"\U000f05c4", hueAzure},
	"lock":    {"\U000f033e", hueGrey},
}

var fallback = icon{"\U000f0214", hueGrey}

func lookup(p string) icon {
	name := strings.ToLower(path.Base(p))
	if ic, ok := byName[name]; ok {
		return ic
	}
	if strings.HasPrefix(name, ".env") {
		return byExt["env"]
	}
	if strings.HasPrefix(name, "dockerfile") || strings.HasSuffix(name, ".dockerfile") {
		return byName["dockerfile"]
	}
	if i := strings.LastIndexByte(name, '.'); i >= 0 {
		if ic, ok := byExt[name[i+1:]]; ok {
			return ic
		}
	}
	return fallback
}

// color maps a mini.icons hue onto the theme the way TokyoNight does.
func (h hue) color(th theme.Theme) string {
	switch h {
	case hueAzure, hueCyan:
		return th.Cyan
	case hueBlue:
		return th.Blue
	case hueGreen:
		return th.Green
	case hueYellow, hueOrange:
		return th.Yellow
	case hueRed:
		return th.Red
	case huePurple:
		return th.Magenta
	}
	return th.FG
}
