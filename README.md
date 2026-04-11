# ding

Wrap any long-running command and get a Telegram notification when it finishes — with exit code and duration.

```
✅ Done
make build
Exit: 0 | Duration: 2m 14s | Finished: 14:32:01
```

## Install

### curl (no Go required)

```bash
curl -fsSL https://github.com/felixschmelzer/ding/releases/latest/download/ding-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') -o /tmp/ding && chmod +x /tmp/ding && sudo mv /tmp/ding /usr/local/bin/ding
```

| Platform | Download |
|----------|----------|
| macOS Apple Silicon | `ding-darwin-arm64` |
| macOS Intel | `ding-darwin-amd64` |
| Linux x86-64 | `ding-linux-amd64` |
| Linux ARM64 | `ding-linux-arm64` |

All releases: [github.com/felixschmelzer/ding/releases](https://github.com/felixschmelzer/ding/releases)

### With Go installed

```bash
go install github.com/felixschmelzer/ding@latest
```

Requires `~/go/bin` in your `$PATH`.

## Configure

Run the interactive setup once:

```bash
ding --config
```

You'll need a Telegram bot token and your chat ID:

1. Message [@BotFather](https://t.me/BotFather) → `/newbot` → copy the token
2. Send a message to your new bot, then open `https://api.telegram.org/bot<TOKEN>/getUpdates` and look for `"id"` inside the `"chat"` object

Config is saved to `~/.config/ding/config.toml`.

## Usage

Prefix any command with `ding`:

```bash
ding make build
ding npm run test
ding ./deploy.sh
```

The command runs normally — output is passed through — and you get a Telegram message when it's done.
