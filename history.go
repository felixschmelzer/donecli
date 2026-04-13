package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
)

// ── Data model ─────────────────────────────────────────────────────────────────

// RunRecord holds metadata for a single ding-wrapped command execution.
// EndTime is zero while the command is still running.
type RunRecord struct {
	ID        string    `json:"id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Command   string    `json:"command"`
	ExitCode  int       `json:"exit_code"`
	WorkDir   string    `json:"work_dir"`
}

func (r RunRecord) runDuration() time.Duration {
	if r.EndTime.IsZero() {
		return time.Since(r.StartTime)
	}
	return r.EndTime.Sub(r.StartTime)
}

func (r RunRecord) running() bool {
	return r.EndTime.IsZero()
}

// ── Storage ────────────────────────────────────────────────────────────────────

func historyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "ding", "history.json"), nil
}

func loadHistory() ([]RunRecord, error) {
	path, err := historyPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []RunRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var records []RunRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func saveHistory(records []RunRecord) error {
	path, err := historyPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func appendRun(rec RunRecord) error {
	records, err := loadHistory()
	if err != nil {
		return err
	}
	records = append(records, rec)
	return saveHistory(records)
}

// updateRun replaces the record with the matching ID in-place.
func updateRun(rec RunRecord) error {
	records, err := loadHistory()
	if err != nil {
		return err
	}
	for i, r := range records {
		if r.ID == rec.ID {
			records[i] = rec
			return saveHistory(records)
		}
	}
	return nil
}

// ── History TUI ────────────────────────────────────────────────────────────────

type histSortCol int

const (
	histSortTime histSortCol = iota
	histSortDuration
	histSortExit
	histSortCommand
	histSortFolder
)

type histConfirm int

const (
	histConfirmNone histConfirm = iota
	histConfirmOne
	histConfirmAll
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type histModel struct {
	all       []RunRecord
	visible   []RunRecord
	cursor    int
	offset    int
	search    textinput.Model
	searching bool
	sortBy    histSortCol
	sortAsc   bool
	confirm   histConfirm
	status    string
	height    int
	width     int
}

func newHistModel() (histModel, error) {
	records, err := loadHistory()
	if err != nil {
		return histModel{}, err
	}

	si := textinput.New()
	si.Placeholder = "filter commands and folders..."
	si.Prompt = "  Search ▸ "
	si.CharLimit = 200
	si.SetWidth(60)

	m := histModel{
		all:     records,
		search:  si,
		sortBy:  histSortTime,
		sortAsc: false, // newest first by default
		height:  24,
		width:   120,
	}
	m.visible = m.filterRecords()
	return m, nil
}

func (m histModel) hasRunning() bool {
	for _, r := range m.all {
		if r.running() {
			return true
		}
	}
	return false
}

func (m histModel) Init() tea.Cmd {
	if m.hasRunning() {
		return tickCmd()
	}
	return nil
}

func (m histModel) filterRecords() []RunRecord {
	out := make([]RunRecord, len(m.all))
	copy(out, m.all)

	sort.SliceStable(out, func(i, j int) bool {
		var less bool
		switch m.sortBy {
		case histSortTime:
			less = out[i].StartTime.Before(out[j].StartTime)
		case histSortDuration:
			less = out[i].runDuration() < out[j].runDuration()
		case histSortExit:
			// In-progress runs sort last
			ri, rj := out[i].running(), out[j].running()
			if ri != rj {
				less = rj // in-progress is "greater"
			} else {
				less = out[i].ExitCode < out[j].ExitCode
			}
		case histSortCommand:
			less = strings.ToLower(out[i].Command) < strings.ToLower(out[j].Command)
		case histSortFolder:
			less = strings.ToLower(out[i].WorkDir) < strings.ToLower(out[j].WorkDir)
		}
		if m.sortAsc {
			return less
		}
		return !less
	})

	q := strings.ToLower(strings.TrimSpace(m.search.Value()))
	if q == "" {
		return out
	}
	var filtered []RunRecord
	for _, r := range out {
		if strings.Contains(strings.ToLower(r.Command), q) ||
			strings.Contains(strings.ToLower(r.WorkDir), q) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

func (m histModel) withSort(col histSortCol) histModel {
	if m.sortBy == col {
		m.sortAsc = !m.sortAsc
	} else {
		m.sortBy = col
		m.sortAsc = col != histSortTime
	}
	m.visible = m.filterRecords()
	m.cursor = 0
	m.offset = 0
	return m
}

func (m histModel) tableRows() int {
	// reserved: title(1) + divider(1) + search(1) + blank(1) + colhdr(1) + divider(1) + blank(1) + footer(1) + scroll(1)
	h := m.height - 9
	if h < 1 {
		h = 1
	}
	return h
}

func (m histModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		// Reload from disk so in-progress and newly finished runs stay current.
		if records, err := loadHistory(); err == nil {
			m.all = records
			m.visible = m.filterRecords()
		}
		if m.hasRunning() {
			return m, tickCmd()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		return m, nil

	case tea.KeyPressMsg:
		if m.confirm != histConfirmNone {
			switch msg.String() {
			case "y", "Y":
				return m.execConfirm()
			default:
				m.confirm = histConfirmNone
				m.status = "Cancelled."
			}
			return m, nil
		}

		if m.searching {
			switch msg.String() {
			case "esc", "enter":
				m.searching = false
				m.search.Blur()
				return m, nil
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				m.cursor = 0
				m.offset = 0
				m.visible = m.filterRecords()
				return m, cmd
			}
		}

		tableH := m.tableRows()
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.searching = true
			m.status = ""
			return m, m.search.Focus()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}
		case "down", "j":
			if m.cursor < len(m.visible)-1 {
				m.cursor++
				if m.cursor >= m.offset+tableH {
					m.offset = m.cursor - tableH + 1
				}
			}
		case "g":
			m.cursor = 0
			m.offset = 0
		case "G":
			m.cursor = len(m.visible) - 1
			if m.cursor < 0 {
				m.cursor = 0
			}
			if m.cursor >= tableH {
				m.offset = m.cursor - tableH + 1
			}
		case "1":
			m = m.withSort(histSortTime)
		case "2":
			m = m.withSort(histSortDuration)
		case "3":
			m = m.withSort(histSortExit)
		case "4":
			m = m.withSort(histSortCommand)
		case "5":
			m = m.withSort(histSortFolder)
		case "y":
			if len(m.visible) > 0 && m.cursor < len(m.visible) {
				if err := clipboard.WriteAll(m.visible[m.cursor].Command); err != nil {
					m.status = "Clipboard error: " + err.Error()
				} else {
					m.status = "Copied to clipboard."
				}
			}
		case "d":
			if len(m.visible) > 0 {
				m.confirm = histConfirmOne
				m.status = ""
			}
		case "D":
			if len(m.all) > 0 {
				m.confirm = histConfirmAll
				m.status = ""
			}
		}
	}
	return m, nil
}

func (m histModel) execConfirm() (tea.Model, tea.Cmd) {
	switch m.confirm {
	case histConfirmOne:
		if m.cursor < len(m.visible) {
			id := m.visible[m.cursor].ID
			newAll := make([]RunRecord, 0, len(m.all)-1)
			for _, r := range m.all {
				if r.ID != id {
					newAll = append(newAll, r)
				}
			}
			m.all = newAll
			if err := saveHistory(m.all); err != nil {
				m.status = "Error: " + err.Error()
			} else {
				m.status = "Run deleted."
			}
			m.visible = m.filterRecords()
			if m.cursor >= len(m.visible) && m.cursor > 0 {
				m.cursor--
			}
		}
	case histConfirmAll:
		m.all = []RunRecord{}
		if err := saveHistory(m.all); err != nil {
			m.status = "Error: " + err.Error()
		} else {
			m.status = "All runs deleted."
		}
		m.visible = m.filterRecords()
		m.cursor = 0
		m.offset = 0
	}
	m.confirm = histConfirmNone
	return m, nil
}

// ── Styles ─────────────────────────────────────────────────────────────────────

var (
	hTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	hColStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	hSelectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("15")).Bold(true)
	hOkStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	hErrStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	hRunningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	hDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	hWarnStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
	hStatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
)

// runepad pads or truncates s to exactly n rune-columns.
func runepad(s string, n int) string {
	rs := []rune(s)
	if len(rs) >= n {
		return string(rs[:n])
	}
	return s + strings.Repeat(" ", n-len(rs))
}

// runetrunc truncates s to at most n rune-columns, adding "…" if truncated.
func runetrunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	if n <= 1 {
		return string(rs[:n])
	}
	return string(rs[:n-1]) + "…"
}

func homeShortenPath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func (m histModel) renderColHeader(col histSortCol, key, label string, width int) string {
	indicator := "  "
	if m.sortBy == col {
		if m.sortAsc {
			indicator = " ▲"
		} else {
			indicator = " ▼"
		}
	}
	text := "[" + key + "] " + label + indicator
	styled := hColStyle.Render(text)
	rs := []rune(text)
	if len(rs) < width {
		styled += strings.Repeat(" ", width-len(rs))
	}
	return styled
}

func (m histModel) View() tea.View {
	w := m.width
	if w < 80 {
		w = 80
	}

	// Column widths must accommodate both data values and header text.
	// Header text lengths (including "[N] " prefix and "  " indicator):
	//   START:    "[1] START  "    = 11 → startW=19 gives width 21, fine
	//   DURATION: "[2] DURATION  " = 14 → durW must make durW+gapW ≥ 14, so durW=12
	//   EXIT:     "[3] EXIT  "     = 10 → exitW must make exitW+gapW ≥ 10, so exitW=8
	//   COMMAND:  "[4] COMMAND  "  = 13 → cmdW+gapW ≥ 13, handled by min below
	//   FOLDER:   "[5] FOLDER  "   = 12 → folderW ≥ 12, handled by min below
	const (
		cursorW = 2
		startW  = 19
		durW    = 12
		exitW   = 8
		gapW    = 2
	)
	remaining := w - cursorW - startW - durW - exitW - gapW*4
	cmdW := remaining * 55 / 100
	folderW := remaining - cmdW
	if cmdW < 20 {
		cmdW = 20
	}
	if folderW < 15 {
		folderW = 15
	}

	var sb strings.Builder

	// Title bar
	countStr := hDimStyle.Render(fmt.Sprintf("  (%d runs)", len(m.all)))
	sb.WriteString(hTitleStyle.Render("ding history") + countStr + "\n")
	sb.WriteString(hDimStyle.Render(strings.Repeat("─", w)) + "\n")

	// Search input (always visible)
	sb.WriteString(m.search.View() + "\n")
	sb.WriteString("\n")

	// Column headers — each header width matches its data column + gap exactly.
	sb.WriteString(strings.Repeat(" ", cursorW))
	sb.WriteString(m.renderColHeader(histSortTime, "1", "START", startW+gapW))
	sb.WriteString(m.renderColHeader(histSortDuration, "2", "DURATION", durW+gapW))
	sb.WriteString(m.renderColHeader(histSortExit, "3", "EXIT", exitW+gapW))
	sb.WriteString(m.renderColHeader(histSortCommand, "4", "COMMAND", cmdW+gapW))
	sb.WriteString(m.renderColHeader(histSortFolder, "5", "FOLDER", folderW))
	sb.WriteString("\n")
	sb.WriteString(hDimStyle.Render(strings.Repeat("─", w)) + "\n")

	// Table rows
	tableH := m.tableRows()
	if len(m.visible) == 0 {
		if m.search.Value() != "" {
			sb.WriteString("\n" + hDimStyle.Render("  No runs match your search.") + "\n")
		} else {
			sb.WriteString("\n" + hDimStyle.Render("  No runs recorded yet. Enable history tracking in ding -c") + "\n")
		}
	} else {
		end := m.offset + tableH
		if end > len(m.visible) {
			end = len(m.visible)
		}
		for i, r := range m.visible[m.offset:end] {
			idx := m.offset + i
			sel := idx == m.cursor

			startStr := runepad(r.StartTime.Format("2006-01-02 15:04:05"), startW)
			cmdStr := runepad(runetrunc(r.Command, cmdW), cmdW)
			folderStr := runetrunc(homeShortenPath(r.WorkDir), folderW)

			var durStr, exitStr string
			if r.running() {
				durStr = runepad(formatDuration(r.runDuration()), durW)
				exitStr = runepad("…", exitW)
			} else {
				durStr = runepad(formatDuration(r.runDuration()), durW)
				exitStr = runepad(fmt.Sprintf("%d", r.ExitCode), exitW)
			}

			if sel {
				row := "▶ " + startStr + "  " + durStr + "  " + exitStr + "  " + cmdStr + "  " + folderStr
				sb.WriteString(hSelectedStyle.Render(row) + "\n")
			} else if r.running() {
				sb.WriteString("  " + startStr + "  " +
					hRunningStyle.Render(durStr) + "  " +
					hRunningStyle.Render(exitStr) + "  " +
					cmdStr + "  " + folderStr + "\n")
			} else {
				var exitRendered string
				if r.ExitCode == 0 {
					exitRendered = hOkStyle.Render(exitStr)
				} else {
					exitRendered = hErrStyle.Render(exitStr)
				}
				sb.WriteString("  " + startStr + "  " + durStr + "  " + exitRendered + "  " + cmdStr + "  " + folderStr + "\n")
			}
		}

		// Scroll position indicator
		if len(m.visible) > tableH {
			end := m.offset + tableH
			if end > len(m.visible) {
				end = len(m.visible)
			}
			sb.WriteString(hDimStyle.Render(fmt.Sprintf("  %d–%d of %d", m.offset+1, end, len(m.visible))) + "\n")
		}
	}

	sb.WriteString("\n")

	// Footer: confirm prompt, status message, or key hints
	switch m.confirm {
	case histConfirmOne:
		if m.cursor < len(m.visible) {
			cmd := runetrunc(m.visible[m.cursor].Command, 40)
			sb.WriteString(hWarnStyle.Render(fmt.Sprintf("  Delete %q? [y/N] ", cmd)))
		}
	case histConfirmAll:
		sb.WriteString(hWarnStyle.Render(fmt.Sprintf("  Delete all %d runs? This cannot be undone. [y/N] ", len(m.all))))
	default:
		if m.status != "" {
			sb.WriteString(hStatusStyle.Render("  " + m.status))
		} else {
			hints := "  [1-5] sort  [/] search  [↑↓/jk] move  [g/G] top/bottom  [y] copy  [d] delete  [D] clear all  [q] quit"
			sb.WriteString(hDimStyle.Render(hints))
		}
	}

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

func runHistory() error {
	m, err := newHistModel()
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}
	p := tea.NewProgram(m)
	_, err = p.Run()
	return err
}
