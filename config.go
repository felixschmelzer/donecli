package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Config holds the Telegram credentials and notification settings.
type Config struct {
	BotToken       string `json:"bot_token"`
	ChatID         string `json:"chat_id"`
	NotifyInterval int    `json:"notify_interval"` // minutes between "still running" pings; 0 = disabled
	ShowSummary    bool   `json:"show_summary"`    // print a summary to the terminal when the command finishes
	TrackHistory   bool   `json:"track_history"`   // save run records to ~/.local/share/ding/history.json
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ding", "config.json"), nil
}

func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.BotToken == "" || cfg.ChatID == "" {
		return nil, fmt.Errorf("config is incomplete")
	}
	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ── Bubble Tea TUI ────────────────────────────────────────────────────────────

type configState int

const (
	stateEditing   configState = iota
	stateSubmitted             //nolint:deadcode
	stateCancelled
)

const totalFocusable = 5 // 3 text inputs + 2 checkboxes

type configModel struct {
	inputs       [3]textinput.Model
	showSummary  bool
	trackHistory bool
	focused      int // 0-2: text inputs, 3: summary checkbox, 4: history checkbox
	state        configState
}

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	hintStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	boxStyle   = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			Padding(1, 2)
)

func initialConfigModel() configModel {
	existing, _ := loadConfig()

	token := textinput.New()
	token.Placeholder = "123456789:ABCDefGhIJKlmNoPQRsTUVwxyZ"
	token.Prompt = "Bot Token : "
	token.CharLimit = 200
	token.SetWidth(50)

	chat := textinput.New()
	chat.Placeholder = "123456789"
	chat.Prompt = "Chat ID   : "
	chat.CharLimit = 30
	chat.SetWidth(50)

	interval := textinput.New()
	interval.Placeholder = "0"
	interval.Prompt = "Interval  : "
	interval.CharLimit = 5
	interval.SetWidth(50)

	showSummary := false
	trackHistory := false
	if existing != nil {
		token.SetValue(existing.BotToken)
		chat.SetValue(existing.ChatID)
		if existing.NotifyInterval > 0 {
			interval.SetValue(strconv.Itoa(existing.NotifyInterval))
		}
		showSummary = existing.ShowSummary
		trackHistory = existing.TrackHistory
	}
	token.Focus()

	return configModel{
		inputs:       [3]textinput.Model{token, chat, interval},
		showSummary:  showSummary,
		trackHistory: trackHistory,
		focused:      0,
		state:        stateEditing,
	}
}

func (m configModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m configModel) moveFocus(dir int) (configModel, tea.Cmd) {
	next := m.focused + dir
	if next < 0 || next >= totalFocusable {
		return m, nil
	}
	if m.focused < len(m.inputs) {
		m.inputs[m.focused].Blur()
	}
	m.focused = next
	if m.focused < len(m.inputs) {
		return m, m.inputs[m.focused].Focus()
	}
	return m, nil
}

func (m configModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.state = stateCancelled
			return m, tea.Quit
		case "enter":
			if m.focused == totalFocusable-1 {
				m.state = stateSubmitted
				return m, tea.Quit
			}
			return m.moveFocus(1)
		case "tab", "down":
			return m.moveFocus(1)
		case "shift+tab", "up":
			return m.moveFocus(-1)
		case "space":
			switch m.focused {
			case 3:
				m.showSummary = !m.showSummary
				return m, nil
			case 4:
				m.trackHistory = !m.trackHistory
				return m, nil
			}
		}
	}

	// Delegate keypresses to the focused text input
	if m.focused < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m configModel) View() tea.View {
	telegramHeader := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Telegram"),
		labelStyle.Render("1. Message @BotFather on Telegram → /newbot → copy the token"),
		labelStyle.Render("2. Message your bot, then open:"),
		labelStyle.Render("   https://api.telegram.org/bot<TOKEN>/getUpdates"),
		labelStyle.Render(`   Find "id" inside the "chat" object — that's your Chat ID.`),
	)
	telegramForm := lipgloss.JoinVertical(lipgloss.Left,
		m.inputs[0].View(),
		m.inputs[1].View(),
	)

	notificationsHeader := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Notifications"),
		labelStyle.Render("Periodically notify you while a command is still running."),
	)
	notificationsForm := lipgloss.JoinVertical(lipgloss.Left,
		m.inputs[2].View()+hintStyle.Render("  minutes (0 = off)"),
	)

	summaryMark := "[ ] "
	if m.showSummary {
		summaryMark = "[x] "
	}
	summaryStyle := labelStyle
	if m.focused == 3 {
		summaryStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	}

	historyMark := "[ ] "
	if m.trackHistory {
		historyMark = "[x] "
	}
	historyStyle := labelStyle
	if m.focused == 4 {
		historyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	}

	terminalHeader := lipgloss.JoinVertical(lipgloss.Left,
		titleStyle.Render("Terminal"),
		labelStyle.Render("Options for output shown directly in your terminal."),
	)
	terminalForm := lipgloss.JoinVertical(lipgloss.Left,
		summaryStyle.Render(summaryMark+"Show summary when command finishes"),
		historyStyle.Render(historyMark+"Track command history  (view with: ding --history)"),
	)

	body := lipgloss.JoinVertical(lipgloss.Left,
		telegramHeader,
		boxStyle.Render(telegramForm),
		"",
		notificationsHeader,
		boxStyle.Render(notificationsForm),
		"",
		terminalHeader,
		boxStyle.Render(terminalForm),
		"",
		hintStyle.Render("↑↓ / tab: navigate   space: toggle   enter: next / save   esc: cancel"),
	)
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

func runConfig() error {
	p := tea.NewProgram(initialConfigModel())
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	m, ok := finalModel.(configModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}

	if m.state == stateCancelled {
		return nil
	}

	token := strings.TrimSpace(m.inputs[0].Value())
	chatID := strings.TrimSpace(m.inputs[1].Value())
	if token == "" || chatID == "" {
		return fmt.Errorf("bot token and chat ID are required")
	}

	intervalStr := strings.TrimSpace(m.inputs[2].Value())
	notifyInterval := 0
	if intervalStr != "" && intervalStr != "0" {
		v, err := strconv.Atoi(intervalStr)
		if err != nil || v < 0 {
			return fmt.Errorf("interval must be a non-negative number of minutes")
		}
		notifyInterval = v
	}

	if err := saveConfig(&Config{BotToken: token, ChatID: chatID, NotifyInterval: notifyInterval, ShowSummary: m.showSummary, TrackHistory: m.trackHistory}); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	path, _ := configPath()
	fmt.Printf("Config saved to %s\n", path)
	return nil
}
