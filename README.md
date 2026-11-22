# yourturn

A terminal viewer for `ACTIONS.md`: the list of questions and chores your AI
agents are waiting on you for. It keeps the list on screen in a side pane,
redraws it when the file changes, and lets you scroll, open an item to read its
background, and click through its links.

```
demo actions                          open 3 · 14:56
── 🔴 urgent 1 ──────────────────────────────────────
▾ 1 Merge PR #12?
    • it ships to prod
    • https://example.com/pull/12
── 🟢 when you have time 2 ──────────────────────────
▸ 2 A or B?
▸ 3 Log in to the console and check the bill. It ha…
j/k move · enter open · o link · y copy id · a done
```

## Install

With [mise](https://mise.jdx.dev/):

```sh
mise use -g github:kitagry/yourturn
```

Or download a binary from [Releases](https://github.com/kitagry/yourturn/releases),
or `go install github.com/kitagry/yourturn@latest`.

## Usage

```sh
yourturn [flags] [file]
```

Without a file, it uses the first that exists of `./ACTIONS.md`,
`$HERDR_TASK_DIR/ACTIONS.md` and `~/tasks/ACTIONS.md` (or waits for
`./ACTIONS.md` to appear).

| Flag | |
|---|---|
| `--no-mouse` | Leave the mouse to the terminal, so a plain click opens a link (no wheel scrolling) |
| `--poll 5s` | Interval of the stat check that backs up file watching |
| `--version` | Print the version |

### Keys

| Key | |
|---|---|
| `j` / `k`, `↓` / `↑` | Next / previous item (scrolls through a long open item first) |
| `ctrl-d` / `ctrl-u`, `pgdn` / `pgup` | Half page / page |
| `g` / `G` | First / last |
| `enter`, `space` | Open / close the item |
| `l` / `h` | Open / close |
| `E` / `C` | Open all / close all |
| `a` | Show / hide done (✅) items |
| `o` | Open the item's link (pick a number when it has several) |
| `y` | Copy the item's number (e.g. `my-task#3 `) to start a reply with (OSC 52) |
| `r` | Reload |
| `?` | Help |
| `q` | Quit |
| mouse | Wheel scrolls, click selects, click again opens |

With the mouse enabled, most terminals still open a link on shift+click.

## File format

Each item is a top-level list entry whose first line says what is asked, with
background, options and links indented under it:

```markdown
# demo actions

- **🔴 1 Merge PR #12?**
  - it ships to prod
  - https://example.com/pull/12
- **🟢 2 A or B?**
  - A: keep / B: drop
- **✅ 3 Done thing**
```

- The mark sets the group: 🔴 urgent, 🟠 this week, 🟢 when you have time,
  ⏳ waiting, ✅ done. Items are grouped by their own mark (or by the `## `
  heading they are under when they have none), whatever order the file has.
- The number after the mark is the item's ID: `1`, `M2` or `<task>#3`.
- Bold, inline code, `[text](https://...)` links and bare URLs are rendered;
  links are clickable (OSC 8). Only `http(s)` links are made clickable.
- A file without any list item is shown as it is.

The file is watched (including being replaced by rename), so whatever writes it
can rebuild it from scratch.

## License

MIT
