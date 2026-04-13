# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
go build                # Build the binary
go run .                # Run without building
go vet ./...            # Vet for common issues
go install github.com/felixschmelzer/ding@latest  # Install binary
```

No test files exist in this project.

## Architecture

`ding` is a single-package Go application (5 files) that wraps any shell command, runs it in a PTY, and sends a Telegram notification when it finishes.

**Entry point & flow (`main.go`):**
- Parses CLI args; `--config` launches TUI setup, `--history` opens the history browser, `--version` prints the version, `--completions <shell>` outputs a shell completion script
- Embeds shell completion scripts (`completions/_ding`, `completions/ding.bash`, `completions/ding.fish`) via `//go:embed`
- Starts an optional goroutine for periodic "still running" Telegram pings (configurable interval)
- Calls `runCommand()` from `runner.go`, then if `TrackHistory` is enabled saves the run via `appendRun()`, then sends the final Telegram notification
- Optionally prints a terminal summary if `ShowSummary` is set in config

**Key files:**
- `runner.go` — Runs the command in a PTY (`github.com/creack/pty`), mirrors I/O and terminal resize signals; falls back to normal exec if PTY unavailable
- `config.go` — Bubble Tea TUI for interactive setup; loads/saves JSON config at `~/.config/ding/config.json`
- `history.go` — `RunRecord` model, JSON storage at `~/.local/share/ding/history.json`, and Bubble Tea history browser TUI
- `telegram.go` — Sends HTTP POST to Telegram Bot API; formats success/failure/running messages using HTML parse mode
- `completions/_ding` — zsh completion: delegates to the wrapped command's completion via `_normal`
- `completions/ding.bash` — bash completion: adjusts `COMP_WORDS`/`COMP_CWORD` and calls the wrapped command's registered completion function
- `completions/ding.fish` — fish completion: uses `__fish_complete_subcommand` to delegate

**Config struct** (`config.go`):
```go
type Config struct {
    BotToken       string `json:"bot_token"`
    ChatID         string `json:"chat_id"`
    NotifyInterval int    `json:"notify_interval"` // minutes; 0 = disabled
    ShowSummary    bool   `json:"show_summary"`
    TrackHistory   bool   `json:"track_history"`   // save runs to history
}
```
Config is stored at `~/.config/ding/config.json`.

**RunRecord struct** (`history.go`):
```go
type RunRecord struct {
    ID        string    `json:"id"`         // UnixNano timestamp string
    StartTime time.Time `json:"start_time"`
    EndTime   time.Time `json:"end_time"`
    Command   string    `json:"command"`
    ExitCode  int       `json:"exit_code"`
    WorkDir   string    `json:"work_dir"`
}
```
History is stored at `~/.local/share/ding/history.json` (XDG data dir, separate from config).

**History TUI (`ding --history` / `ding -H`):**
- Sortable table: `[1-5]` keys sort by Start, Duration, Exit, Command, Folder (toggle asc/desc)
- Live search: `[/]` filters by command or folder as you type
- Navigation: `↑↓`, `jk`, `g/G` (top/bottom)
- `[d]` delete selected run with confirmation, `[D]` clear all runs with confirmation
- `[q]`/`esc` to quit

## CI / Releases

Releases are fully automated via two GitHub Actions workflows:

- **`.github/workflows/release.yml`** — fires on push to `main`; runs semantic-release (creates a version tag + updates `CHANGELOG.md` for `fix:`/`feat:` commits), then conditionally runs goreleaser to cross-compile binaries for linux/darwin × amd64/arm64 and publish a GitHub Release. `chore:`, `refactor:`, etc. produce no release.

Config files: `.goreleaser.yaml`, `.releaserc.json`.

GoReleaser also publishes a Homebrew cask to the tap repo `felixschmelzer/homebrew-tap` (secret: `TAP_GITHUB_TOKEN`). Users install via `brew tap felixschmelzer/tap && brew install ding`.

The `version` variable in `main.go` is injected at build time via `-X main.version={{.Version}}`.

## Notes

- Telegram messages use HTML parse mode (`<code>`, `<b>` tags)
- The README mentions `.toml` config extension but the actual implementation uses `.json`

## Claude Behavior

- Keep this CLAUDE.md up to date after making changes to the codebase (new commands, architectural changes, config changes, etc.)
