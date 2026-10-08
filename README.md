# revue

A terminal app for reviewing uncommitted git changes, across every repo in a
folder, with Vim keys and quick questions to Claude.

Point it at a folder that holds several repos (say `api/` and `web/` side by
side) and you get one file tree for all of them, a diff pane, and a Claude panel.

- Unified diff with deleted lines red, added lines green, and changed lines
  yellow with the changed words highlighted; `Ctrl+Shift+D` switches to a
  side-by-side view.
- Generous context (50 lines around each change); longer unchanged stretches
  fold away and open with Vim's fold keys.
- Vim navigation everywhere: counts, `gg`/`G`, `]c`/`[c`, `/` search, visual
  selection.
- Select lines and press `a` to ask Claude about them; the answer streams into
  a side panel.
- Refreshes on its own when files change (e.g. while an agent edits them), and
  opens the file in your editor at the cursor line with `e`.

It only reads your repos: no staging, no commits, no index writes.

## Requirements

- [Go](https://go.dev/dl/) 1.27 or newer, to build it
- git
- A true-colour terminal and a [Nerd Font](https://www.nerdfonts.com/) for the
  icons. Developed on Windows 11 in WezTerm; it also builds for macOS and Linux,
  which are less tested.
- Optional: [Claude Code](https://docs.claude.com/en/docs/claude-code) (`claude`
  on your PATH, logged in), for the Claude panel
- Optional: Neovim, or another editor set in the config, for `e`

## Install

```sh
go install github.com/DavidkJun/revue@latest
```

That puts `revue` in Go's bin folder (`go env GOPATH` + `/bin`), which the Go
installer usually adds to your PATH. Or build from a clone:

```sh
git clone https://github.com/DavidkJun/revue.git
cd revue
go build -o revue .    # revue.exe on Windows
```

## Use

```sh
revue [path]
```

Inside a repo it reviews that repo; in a folder above repos it finds every repo
up to two levels down. It reviews everything uncommitted: HEAD against the
working tree, staged and unstaged together, untracked files included.

| Keys | |
|---|---|
| `j` `k` `gg` `G` `Ctrl+d` `Ctrl+u` | move (counts work: `10j`) |
| `Enter` / `l` | open the file under the cursor in the diff |
| `Ctrl+h` `Ctrl+l` | move between the tree, the diff and the Claude panel |
| `H` `L` | previous / next repo |
| `Tab` `Shift+Tab` | next / previous changed file, across repos |
| `]c` `[c` | next / previous change |
| `Ctrl+Shift+D` or `Space d` | unified ⇄ split view |
| `zo` `zR` `zM`, `+` `-` | open a fold, show everything, fold again, more / less context |
| `v`, `y`, `yy`, `yp` | select lines, yank them, yank a line, yank `repo/path:line` |
| `/` `n` `N` | search |
| `a` | ask Claude about the selection, the marked files (`m` in the tree) or the current file |
| `A` | ask Claude about all changes of the repo |
| `Space a` / `Space e` | show / hide the Claude panel / the file tree |
| `e` | open the file in your editor at the cursor line |
| `?` | every key |
| `q` / `Q` | close the panel or quit / quit now |

## Claude

Questions go to your own `claude` CLI in headless mode, so they use your
existing Claude login. Claude may only read files (Read, Grep, Glob), and the
session starts without hooks or MCP servers, which keeps a quick question at a
few seconds. Follow-ups continue the same conversation; in the panel `n` starts
a new one and `m` switches between haiku, sonnet and opus.

## Config

Optional, at `~/.config/revue/config.toml`:

```toml
context = 50        # lines of context around changes
depth = 2           # how deep to look for repos below the folder
editor = "nvim"     # opened as: editor +LINE FILE

[claude]
model = "sonnet"
effort = "medium"
extra_args = []     # passed to claude as is
```

## Notes

- Telling `Ctrl+Shift+D` apart from `Ctrl+D` needs the terminal's help. On
  Windows revue turns on the console's win32-input-mode. On macOS and Linux it
  should work in terminals that speak the kitty keyboard protocol (kitty,
  Ghostty, or WezTerm with `enable_kitty_keyboard = true`), but that is
  untested. `Space d` works everywhere.
- In WezTerm, `Ctrl+h` / `Ctrl+l` at the left or right edge move to the
  neighbouring WezTerm pane, the way smart-splits setups do in Neovim.
- Navigation keys also work on Cyrillic keyboard layouts (`о` acts as `j`).

## Development

```sh
go test ./...
```
