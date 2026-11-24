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
m mouse on · j/k move · enter open · o link · y cop…
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
| `--mouse` / `--no-mouse` | Capture the mouse at start, or not (default captured unless `YOURTURN_MOUSE=0`; `m` toggles) |
| `--poll 5s` | Interval of the stat check that backs up file watching |
| `--reply-cmd CMD` | Command that sends a reply (`R`); default `$YOURTURN_REPLY_CMD`. See [Replying](#replying) |
| `--reply-status FILE` | File that reports what became of sent replies; default `$YOURTURN_REPLY_STATUS`. See [Reply status](#reply-status) |
| `--reply-delay 5s` | How long a reply can be undone (`u`) before it is sent |
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
| `/` | Search the shown items, their first line and body, ignoring case (`esc` cancels) |
| `n` / `N` | Next / previous match, wrapping around |
| `esc` | Clear the search highlight |
| `o` | Open the item's link (pick a number when it has several) |
| `y` | Copy the item's number (e.g. `my-task#3 `) to start a reply with (OSC 52) |
| `R` | Reply to the item (needs `--reply-cmd`) |
| `u` | Undo the reply that is about to be sent |
| `r` | Reload |
| `m` | Capture the mouse / give it back to the terminal |
| `?` | Help |
| `q` | Quit |
| mouse | When captured: wheel scrolls, click selects, click again opens |

`/` moves to the first item from the selected one that contains the text and
highlights every match; an item that matches only in its body is opened. Done
items hidden by `a` are not searched. The highlight stays across reloads until
`esc`.

The mouse is captured by default: the wheel scrolls and a click selects an
item (click it again to open it). To select and copy text or click a link in
the terminal, press `m` to give the mouse back, and `m` again to capture it;
the footer shows which (`m mouse on` / `m mouse off`). Set `YOURTURN_MOUSE=0`
to start with it released. While captured, many terminals still select text
on shift+drag.

## Replying

yourturn does not know where replies go; a command you give it does. With
`--reply-cmd` (or `YOURTURN_REPLY_CMD`), `R` asks for a reply to the selected
item, e.g. `A`, `OK` or a sentence. The reply waits for `--reply-delay` (5s),
during which `u` takes it back, and then the command is run with `sh -c`:

| | |
|---|---|
| `YOURTURN_ID` | The item's ID, e.g. `my-task#3` |
| `YOURTURN_TEXT` | What you typed |
| `YOURTURN_MESSAGE` | `<ID> <text>`, also given on stdin |
| `YOURTURN_FILE` | The absolute path of the ACTIONS.md |

A command may print a line `receipt: <key>` on stdout to name the reply; see
[Reply status](#reply-status). Other output is ignored.

The reply is only passed as data, never put into the command line. A command
that exits non-zero marks the reply failed with its last line of stderr, and
`R` offers the text again. Sent replies are marked under their item (`↳ sent
"A" 15:20`) and the marks are kept in `$XDG_STATE_HOME/yourturn/sent.json`
(default `~/.local/state`) for 14 days.

```sh
# e.g. append replies to a file another tool reads
yourturn --reply-cmd 'cat >> ~/replies.txt'
```

### Reply status

`↳ sent "A" 15:20` only says the command ran. When whatever carries the reply
on can report back, yourturn shows how far it got:

```
↳ sent "A" 15:20 · relayed 15:32 · acked 15:40
```

For that, the reply command prints `receipt: <key>` (any key without spaces,
unique per reply; the last such line counts), and the carrier appends events
to the file given by `--reply-status` (or `YOURTURN_REPLY_STATUS`), one line
each, tab separated:

```
<key>	relayed	2026-11-24T06:32:00Z
<key>	acked	2026-11-24T06:40:00Z
```

The time is RFC 3339. `relayed` and `acked` are the states yourturn knows; lines
with other states or that are broken are skipped. The file is watched, and the
receipt is kept with the sent mark in `sent.json`, so it survives a restart.
A reply is *done* when its item is checked off (✅) or gone from the file: the
mark is then removed, which is how you can tell it from one still on its way.
Without a receipt or a status file the mark looks as it always did.

### herdr

[`contrib/herdr/yourturn-reply.sh`](contrib/herdr/yourturn-reply.sh) sends
replies to a [herdr](https://github.com/herdrdev/herdr)-task setup, where
the master's `~/tasks/ACTIONS.md` uses `<task>#<n>` IDs and each task's
`~/tasks/<task>/ACTIONS.md` uses `<n>`. It is included in the release archives:

```sh
export YOURTURN_REPLY_CMD="$(dirname "$(mise which yourturn)")/contrib/herdr/yourturn-reply.sh"
```

`YOURTURN_HERDR_MODE=inbox` (default) appends the reply to `~/tasks/.inbox` for
the master to pass on; `coordinator` sends it straight to the task's coordinator
with `task-ask.sh --no-wait` and copies it to the inbox.

Each reply gets a key like `r-20261124T103200-a1b2`. The script ends the inbox
line (and the message to the coordinator) with ` (reply r-20261124T103200-a1b2)`,
records `sent` for it in `~/tasks/.reply-status` and prints
`receipt: r-20261124T103200-a1b2`, so the carriers can add `relayed` and `acked`
to that file. To show them, run yourturn with
`--reply-status ~/tasks/.reply-status` (or set `YOURTURN_REPLY_STATUS`). The
tasks root is `$HERDR_TASKS_ROOT` (default `~/tasks`).

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
- The number after the mark is the item's ID: a number (`1`, `M2`, `A266`,
  `CI-9`, `A266c`) or `<task>#<number>` (`alpha#3`, `ci-maintenance#CI-9`).
  A number is digits, optionally after upper-case letters and an optional `-`,
  and optionally followed by one lower-case branch letter (`A266b`, `3a`). It
  must be followed by a space.
- Bold, inline code, `[text](https://...)` links and bare URLs are rendered;
  links are clickable (OSC 8). Only `http(s)` links are made clickable.
- A file without any list item is shown as it is.

The file is watched (including being replaced by rename), so whatever writes it
can rebuild it from scratch.

## License

MIT
