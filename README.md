# done-msg

Wrap any long-running command and get a Telegram notification when it finishes — with exit code and duration.

```
✅ Done
make build
Exit: 0 | Duration: 2m 14s | Finished: 14:32:01
```

## Install

```bash
go install github.com/felixschmelzer/done-msg@latest
```

Requires `~/go/bin` in your `$PATH`.

## Configure

Run the interactive setup once:

```bash
done-msg --config
```

You'll need a Telegram bot token and your chat ID:

1. Message [@BotFather](https://t.me/BotFather) → `/newbot` → copy the token
2. Send a message to your new bot, then open `https://api.telegram.org/bot<TOKEN>/getUpdates` and look for `"id"` inside the `"chat"` object

Config is saved to `~/.config/done-msg/config.toml`.

## Usage

Prefix any command with `donecli`:

```bash
done-msg make build
done-msg npm run test
done-msg ./deploy.sh
```

The command runs normally — output is passed through — and you get a Telegram message when it's done.
