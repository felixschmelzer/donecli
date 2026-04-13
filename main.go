package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var version = "dev"

//go:embed completions/_ding
var zshCompletion string

//go:embed completions/ding.bash
var bashCompletion string

//go:embed completions/ding.fish
var fishCompletion string

const usage = `ding — run a command and get a Telegram notification when it finishes

Usage:
  ding <command> [args...]

Options:
  -c, --config                  Open the interactive setup
  -H, --history                 Browse command run history
  -v, --version                 Print version
  -h, --help                    Show this help
  --completions install         Auto-install completions for the current shell
  --completions <shell>         Output completion script (bash, zsh, fish)

Examples:
  ding make build
  ding npm run test
  ding ./deploy.sh
  ding -h                       # opens help
  ding -c                       # opens config ui
  ding --history                # browse run history
  ding --completions install    # install shell completions automatically`

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		os.Exit(0)
	}

	switch os.Args[1] {
	case "-h", "--help":
		fmt.Println(usage)
		os.Exit(0)
	case "-v", "--version":
		fmt.Println("ding", version)
		os.Exit(0)
	case "-c", "--config":
		if err := runConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "ding: %v\n", err)
			os.Exit(1)
		}
		return
	case "-H", "--history":
		if err := runHistory(); err != nil {
			fmt.Fprintf(os.Stderr, "ding: %v\n", err)
			os.Exit(1)
		}
		return
	case "--completions":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "ding: --completions requires an argument: install, bash, zsh, or fish")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "install":
			if err := installCompletions(); err != nil {
				fmt.Fprintf(os.Stderr, "ding: %v\n", err)
				os.Exit(1)
			}
		case "zsh":
			fmt.Print(zshCompletion)
		case "bash":
			fmt.Print(bashCompletion)
		case "fish":
			fmt.Print(fishCompletion)
		default:
			fmt.Fprintf(os.Stderr, "ding: unknown shell %q (supported: install, bash, zsh, fish)\n", os.Args[2])
			os.Exit(1)
		}
		return
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "ding: no config found, run 'ding -c' to set up")
		os.Exit(1)
	}

	args := os.Args[1:]

	// Start periodic "still running" notifications if configured.
	cmdStart := time.Now()
	if cfg.NotifyInterval > 0 {
		stopNotify := make(chan struct{})
		defer close(stopNotify)
		go func() {
			ticker := time.NewTicker(time.Duration(cfg.NotifyInterval) * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					msg := buildRunningMessage(
						strings.Join(args, " "),
						formatDuration(time.Since(cmdStart)),
					)
					_ = sendTelegram(cfg, msg)
				case <-stopNotify:
					return
				}
			}
		}()
	}

	// Save a partial record before running so in-progress runs appear in history.
	var runID, runWd string
	if cfg.TrackHistory {
		runWd, _ = os.Getwd()
		runID = fmt.Sprintf("%d", cmdStart.UnixNano())
		if saveErr := appendRun(RunRecord{
			ID:        runID,
			StartTime: cmdStart,
			Command:   strings.Join(args, " "),
			WorkDir:   runWd,
			// EndTime is zero — signals "in progress" to the history TUI
		}); saveErr != nil {
			fmt.Fprintf(os.Stderr, "ding: failed to save run history: %v\n", saveErr)
		}
	}

	exitCode, duration, err := runCommand(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "done: %v\n", err)
		os.Exit(1)
	}

	if cfg.TrackHistory && runID != "" {
		if saveErr := updateRun(RunRecord{
			ID:        runID,
			StartTime: cmdStart,
			EndTime:   cmdStart.Add(duration),
			Command:   strings.Join(args, " "),
			ExitCode:  exitCode,
			WorkDir:   runWd,
		}); saveErr != nil {
			fmt.Fprintf(os.Stderr, "ding: failed to update run history: %v\n", saveErr)
		}
	}

	cmd := strings.Join(args, " ")
	dur := formatDuration(duration)
	finished := time.Now().Format("15:04:05")

	if cfg.ShowSummary {
		icon, status := "✅", "Done"
		if exitCode != 0 {
			icon, status = "❌", "Failed"
		}
		fmt.Printf("\n%s %s\n%s\nExit: %d | Duration: %s | Finished: %s\n",
			icon, status, cmd, exitCode, dur, finished)
	}

	msg := buildMessage(cmd, exitCode, dur, finished)
	if err := sendTelegram(cfg, msg); err != nil {
		fmt.Fprintf(os.Stderr, "ding: notification failed: %v\n", err)
	}

	os.Exit(exitCode)
}

func installCompletions() error {
	shell := filepath.Base(os.Getenv("SHELL"))

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not find home directory: %w", err)
	}

	switch shell {
	case "zsh":
		dir := filepath.Join(home, ".zsh", "completions")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		dest := filepath.Join(dir, "_ding")
		if err := os.WriteFile(dest, []byte(zshCompletion), 0644); err != nil {
			return err
		}
		fmt.Printf("Installed zsh completion to %s\n", dest)

		zshrc := filepath.Join(home, ".zshrc")
		if !fileContains(zshrc, ".zsh/completions") {
			if err := appendToFile(zshrc, "\n# ding shell completions\nfpath=(~/.zsh/completions $fpath)\n"); err != nil {
				fmt.Println("Add this to your ~/.zshrc:")
				fmt.Println("  fpath=(~/.zsh/completions $fpath)")
			} else {
				fmt.Println("Updated ~/.zshrc with fpath entry.")
			}
		}
		if !fileContains(zshrc, "compinit") {
			if err := appendToFile(zshrc, "autoload -Uz compinit && compinit\n"); err != nil {
				fmt.Println("  autoload -Uz compinit && compinit")
			} else {
				fmt.Println("Updated ~/.zshrc with compinit.")
			}
		}
		fmt.Println("Restart your shell or run: source ~/.zshrc")

	case "bash":
		dir := filepath.Join(home, ".bash_completion.d")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		dest := filepath.Join(dir, "ding")
		if err := os.WriteFile(dest, []byte(bashCompletion), 0644); err != nil {
			return err
		}
		fmt.Printf("Installed bash completion to %s\n", dest)

		sourceLine := "for f in ~/.bash_completion.d/*; do source \"$f\"; done"
		bashrc := filepath.Join(home, ".bashrc")
		if !fileContains(bashrc, ".bash_completion.d") {
			if err := appendToFile(bashrc, "\n# ding shell completions\n"+sourceLine+"\n"); err != nil {
				fmt.Println("Add this to your ~/.bashrc:")
				fmt.Println(" ", sourceLine)
			} else {
				fmt.Println("Updated ~/.bashrc.")
			}
		}
		fmt.Println("Restart your shell or run: source ~/.bashrc")

	case "fish":
		dir := filepath.Join(home, ".config", "fish", "completions")
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		dest := filepath.Join(dir, "ding.fish")
		if err := os.WriteFile(dest, []byte(fishCompletion), 0644); err != nil {
			return err
		}
		fmt.Printf("Installed fish completion to %s\n", dest)
		fmt.Println("Active immediately in new fish sessions.")

	default:
		return fmt.Errorf("unsupported shell %q — run 'ding --completions <bash|zsh|fish>' to print the script manually", shell)
	}
	return nil
}

func fileContains(path, substr string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substr)
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
