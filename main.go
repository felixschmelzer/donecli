package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var version = "dev"

const usage = `ding — run a command and get a Telegram notification when it finishes

Usage:
  ding <command> [args...]

Options:
  -c, --config   Open the interactive setup
  -v, --version  Print version
  -h, --help     Show this help

Examples:
  ding make build
  ding npm run test
  ding ./deploy.sh
  ding -h	# opens help
  ding -c	# opens config ui`

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

	exitCode, duration, err := runCommand(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "done: %v\n", err)
		os.Exit(1)
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
