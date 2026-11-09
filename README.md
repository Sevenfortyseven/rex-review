# Rex Review

A standalone tool for reviewing uncommitted Git changes in a floating [Rex](https://www.superlogical.com/rex) pane. Select lines, write comments, and send a review to the agent pane you opened it from. No plugin manager or background daemon is required.

The interface is a Go/Bubble Tea program in Rex's standard terminal block. Lua opens Rex's floating layer and preserves the originating session, tab, and pane IDs. No app patching or private block SDK is used.

The viewer keeps the terminal's default foreground and background, and uses its ANSI palette for diff colors. The comment editor also uses the terminal background. Font and terminal appearance remain controlled by Rex.

On a local Mac, the launcher reads Rex's selected light/dark built-in theme and supplies its foreground, background, cursor, and ANSI colors when creating the floating terminal. It reads the selection again each time you open the viewer. This adapter uses Rex's current macOS preference and bundled-theme formats; if the selected theme is unavailable locally (for example, a custom theme or a remote client), it falls back to Rex's terminal defaults. It does not change Rex settings.

## Install

Requires Rex API revision 11+, Go 1.24.2+ to build, and Git. The floating UI and installer have been tested on macOS.

Install Go with Homebrew if needed:

```sh
brew install go
```

Install from GitHub:

```sh
git clone https://github.com/Sevenfortyseven/rex-review.git
cd rex-review
go run ./cmd/rex-review install
```

If you already have the source directory, run the same install command from there:

```sh
go run ./cmd/rex-review install
```

The installer copies the binary and Lua action to `~/.config/rex/rex-review/`, adds a marked block to `~/.config/rex/init.lua`, checks the configuration with Rex, and reloads it. Existing config lines are preserved, and a backup is saved as `init.lua.rex-review.bak` on the first install. Running the installer again updates the installation without adding duplicate blocks. A custom `REX_CONFIG` or `XDG_CONFIG_HOME` is respected; `--config /path/to/init.lua` overrides the destination. If Rex is not running, the files are still installed; open Rex or run `rex config reload` later.

The default shortcut is **Cmd+Shift+R**; the palette action is **Rex Review: Git Changes**. Open it from a supported agent pane. Press the shortcut again to close it; saved drafts survive.

If the agent's reported folder is not a Git working tree, the viewer asks for the project folder instead of exiting. Paste the full path and press Enter. This also handles agents whose in-app `/cd` does not update their OS working directory. The chosen folder is remembered for that running agent process. Press `p` in the viewer to choose a different project; existing drafts stay with their original repository.

In the project-folder prompt, Tab completes directory paths (including `~/`, spaces, Unicode, and directory symlinks). Multiple matches expand their common prefix and appear below the input; subsequent Tab / Shift+Tab cycles through them. This is a custom input field, so shell aliases and shell completion plugins are not loaded.

Override the shortcut in `~/.config/rex/init.lua`:

```lua
rex.bind("cmd+shift+r", "rex_review")
```

For a nondefault Rex server, install with `go run ./cmd/rex-review install --server unix:///path/to/server.sock`. The default uses Rex's normal server routing.

### Migrating an older installation

If you installed a previous version through rexd, first remove that registration with `rexd remove aleksandre.rex-review`, then run the standalone install command above. If no other plugins use rexd, `rexd teardown` removes its config loader and daemon. Your saved reviews are kept.

## Use

1. Choose a file with `[` / `]`, or focus the file list with Tab. `M` is unstaged, `S` staged, and `?` untracked. A file with both staged and unstaged edits appears twice.
2. Move to a code line with the arrow keys. Press `v`, then move to select a range; mouse click/drag also selects lines. Left/right scroll long lines horizontally.
3. Press `c` to write a comment. Enter inserts a newline; **Ctrl+S** saves; Esc cancels the edit. Text is saved locally while you type, including unfinished drafts.
4. Press **Shift+C** to inspect comments. Up/down chooses a comment; `e` edits it and `d` deletes it. Shift+C returns to the diff.
5. Press `s`, then `y` or Enter to send. This pastes the combined review into the originating agent input, presses Enter, and closes the viewer. Keep that agent's input clear of unrelated drafts before sending. Esc cancels the confirmation.

`r` refreshes changes. PageUp/PageDown and Ctrl+U/Ctrl+D scroll. `g`/`G` jump to the first/last diff line. `q` closes the viewer.

Comments include the repository, relative file path, line range, diff scope, selected diff text, and comment. Deletion-only selections reference old-file lines; selections containing additions/context reference new-file lines.

## Delivery and drafts

- The target is the originating pane, rather than whichever tab is focused later. Delivery checks its tab, foreground agent PID, agent identity, and working directory again.
- Supported process names: Codex, Claude Code, Gemini, Aider, OpenCode, Cursor agent, Amp, Goose, and pi. Interpreter-hosted Claude/Codex scripts are recognized. Wrappers with other process names may need additional detection.
- A reported working state, permission prompt, question, or authentication prompt blocks delivery. Agents that do not report status can only be checked by their foreground process; status checks are best effort.
- If a commented diff changes, sending stops and keeps the draft. Refresh, delete the affected old comment, then select the current lines and re-add it. Unchanged comments remain valid.
- Paste/submission timeouts keep an uncertainty marker to prevent accidental duplicate sends. Inspect the agent input before retrying; `x` in the comments view clears that marker after you have checked it.
- Drafts are private JSON files in `~/Library/Application Support/rex-review/drafts/` on macOS (`os.UserConfigDir()` elsewhere), scoped to repository + session + pane. Successful sends clear the draft. Removing the plugin leaves drafts intact.
- This reviews uncommitted changes, not an agent's last turn or branch history. Binary files and diff headers cannot take line comments. Git output over 4 MiB is rejected instead of exhausting memory. Git external diff/textconv helpers are disabled.

## Keyboard controls

| Key | Action |
| --- | --- |
| Cmd+Shift+R | Open or close this tab's review window |
| Tab | Switch between the file list and diff |
| Up/down or `j`/`k` | Move through files, diff lines, or comments |
| `[` / `]` | Previous / next file |
| Left/right or `h`/`l` | Scroll long diff lines horizontally |
| `v` | Start or finish selecting a line range |
| `c` | Write a comment on the selected lines |
| Ctrl+S | Save the comment being edited |
| Enter while editing | Insert a newline |
| Shift+C | Switch between saved comments and the diff |
| `e` / `d` in comments | Edit / delete the selected comment |
| `s`, then `y` or Enter | Send the review to the originating agent |
| `r` | Refresh Git changes |
| `p` | Choose a project folder |
| PageUp/PageDown or Ctrl+U/Ctrl+D | Scroll the diff |
| `g` / `G` | First / last diff line |
| `?` | Show help |
| `q` or Esc | Close the viewer and keep drafts; Esc cancels an active edit or confirmation |

## Troubleshooting

- **The shortcut does nothing:** check that `init.lua` contains the marked `rex-review` block and run `rex config reload`. You can also search the Rex command palette for **Rex Review: Git Changes**. A later binding in your own config takes precedence over the installed shortcut.
- **The viewer asks for an agent pane:** open it from the pane running the agent, with that agent working inside a Git repository. A plain shell or an unrecognized agent wrapper cannot receive reviews.
- **The agent's project changed but the viewer shows another repository:** press `p` and select the current project's full path. A chosen folder overrides automatic detection for that agent process. Repository selection does not change which agent receives the comments.
- **A comment is stale:** press `r`, then Shift+C to inspect the saved comment. Delete that comment and re-add it against the current diff. Editing its text alone does not update its line snapshot.
- **Delivery is uncertain:** inspect the originating agent's input before doing anything else. If the review is pasted but unsent, submit it there. Only clear the marker with `x` in the comments view once you have checked whether it was delivered.
- **Go or Rex is not found during installation:** make sure both commands are on the shell's `PATH` before installing. Rex exposes its executable directory as `REX_BIN_DIR` inside its terminals. On macOS the installer also checks the standard Rex Beta app location.

## Develop

From the source directory:

```sh
go mod download
go test ./...
go vet ./...
go build -o bin/rex-review ./cmd/rex-review
bin/rex-review install
python3 scripts/smoke.py  # real floating UI + fake agent on a disposable server; needs cc
rex config reload
```

Run `go run ./cmd/rex-review install` after changing Go or Lua code. Lua is embedded in the executable, so reinstalling updates both. The installed copy is independent of the source checkout.

## Remove

```sh
go run ./cmd/rex-review uninstall
```

Alternatively, use `~/.config/rex/rex-review/rex-review uninstall` without Go or the source checkout. Pass the same `--config` and `--server` options if you used custom values when installing. Uninstall removes only the marked config block and installed Lua/binary files. Your other Rex settings, source checkout, config backup, and saved drafts remain on disk.

The floating-pane approach was inspired by [rexlauncher](https://github.com/tornikegomareli/rexlauncher). Rex Review installs and runs independently.
