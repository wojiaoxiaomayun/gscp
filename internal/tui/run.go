package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gscp/internal/config"
	"gscp/internal/deploy"
	"gscp/internal/runconfig"
)

var (
	appStyle           = lipgloss.NewStyle().Padding(1, 2)
	titleStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB")).Background(lipgloss.Color("#1F3A5F")).Padding(0, 1)
	subtitleStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	panelStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Background(lipgloss.Color("#111827")).Padding(1, 2)
	selectedEnvStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111827")).Background(lipgloss.Color("#F59E0B")).Padding(0, 1)
	envStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Padding(0, 1)
	metaStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	labelStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	valueStyle         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB"))
	statusRunningStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111827")).Background(lipgloss.Color("#34D399")).Padding(0, 1)
	statusDoneStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111827")).Background(lipgloss.Color("#A3E635")).Padding(0, 1)
	statusErrorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB")).Background(lipgloss.Color("#DC2626")).Padding(0, 1)
	statusIdleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#111827")).Background(lipgloss.Color("#CBD5E1")).Padding(0, 1)
	progressTrackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#374151"))
	totalFillStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B"))
	fileFillStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#60A5FA"))
	logTitleStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F9FAFB"))
	logLineStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB"))
	hintStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	errorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#FCA5A5")).Bold(true)
	commandStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#FDE68A")).Bold(true)
	failureBoxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#DC2626")).Foreground(lipgloss.Color("#FCA5A5")).Padding(0, 1)
)

type runFinishedMsg struct{ err error }
type runEventMsg struct{ event deploy.Event }

type runModel struct {
	envKeys        []string
	targets        map[string]runconfig.Target
	servers        map[string]config.Server
	workingDir     string
	autoExit       bool
	selected       int
	chosenEnv      string
	phase          string
	status         string
	errorText      string
	currentCommand string
	failedCommand  string
	done           bool
	quitting       bool
	started        bool
	events         chan tea.Msg
	totalBytes     int64
	written        int64
	totalFiles     int
	fileIndex      int
	fileName       string
	fileSize       int64
	fileDone       int64
	speed          float64
	eta            time.Duration
	logs           []string
}

func Run(envKeys []string, targets map[string]runconfig.Target, servers map[string]config.Server, explicitEnv string, workingDir string, autoExit bool) error {
	model := runModel{envKeys: envKeys, targets: targets, servers: servers, workingDir: workingDir, events: make(chan tea.Msg, 128), phase: "select", status: "Choose an environment", autoExit: autoExit}
	if explicitEnv != "" {
		model.chosenEnv = explicitEnv
		model.phase = "running"
		model.status = "Preparing deployment"
	}
	program := tea.NewProgram(model)
	finalModel, err := program.Run()
	if err != nil {
		return err
	}
	result, ok := finalModel.(runModel)
	if !ok {
		return nil
	}
	if result.errorText != "" && !result.done {
		return errors.New(result.errorText)
	}
	return nil
}

func (m runModel) Init() tea.Cmd {
	if m.phase == "running" {
		return m.startRunCmd()
	}
	return nil
}

func (m runModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.phase {
		case "select":
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			case "up", "k":
				if m.selected > 0 {
					m.selected--
				}
			case "down", "j":
				if m.selected < len(m.envKeys)-1 {
					m.selected++
				}
			case "enter":
				m.chosenEnv = m.envKeys[m.selected]
				m.phase = "running"
				m.status = "Preparing deployment"
				cmd := m.startRunCmd()
				return m, cmd
			}
		case "running":
			switch msg.String() {
			case "ctrl+c", "q":
				m.quitting = true
				return m, tea.Quit
			}
		case "done", "error":
			switch msg.String() {
			case "enter", "q", "ctrl+c":
				return m, tea.Quit
			}
		}
	case runEventMsg:
		m.applyEvent(msg.event)
		return m, waitForEvent(m.events)
	case runFinishedMsg:
		if msg.err != nil {
			m.phase = "error"
			m.status = "Deployment failed"
			m.errorText = msg.err.Error()
			if m.currentCommand != "" {
				m.failedCommand = m.currentCommand
			}
			m.logs = append(m.logs, "ERROR: "+msg.err.Error())
			if m.autoExit {
				return m, tea.Quit
			}
			return m, nil
		}
		m.phase = "done"
		m.done = true
		m.status = "Deployment finished"
		m.currentCommand = ""
		m.logs = append(m.logs, "Deployment finished")
		if m.autoExit {
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

func (m runModel) View() string {
	if m.quitting {
		return "\n"
	}
	switch m.phase {
	case "select":
		return m.selectView()
	case "running", "done", "error":
		return m.progressView()
	default:
		return ""
	}
}

func (m *runModel) startRunCmd() tea.Cmd {
	if m.started {
		return waitForEvent(m.events)
	}
	m.started = true
	envKey := m.chosenEnv
	target := m.targets[envKey]
	server := m.servers[target.ActiveAlias]
	workingDir := m.workingDir
	ch := m.events
	go func() {
		runner := deploy.Runner{Notify: func(event deploy.Event) { ch <- runEventMsg{event: event} }}
		err := runner.Run(server, workingDir, deploy.Plan{EnvKey: envKey, LocalPath: target.LocalPath, ToPath: target.ToPath, Commands: target.Commands})
		ch <- runFinishedMsg{err: err}
		close(ch)
	}()
	return waitForEvent(ch)
}

func waitForEvent(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

func (m *runModel) applyEvent(event deploy.Event) {
	switch event.Type {
	case deploy.EventStatus:
		m.status = event.Message
		m.logs = append(m.logs, event.Message)
	case deploy.EventUploadProgress:
		m.totalBytes = event.TotalBytes
		m.written = event.WrittenBytes
		m.totalFiles = event.TotalFiles
		m.fileIndex = event.CurrentFileIndex
		m.fileName = event.CurrentFile
		m.fileSize = event.CurrentFileSize
		m.fileDone = event.CurrentFileDone
		m.speed = event.SpeedBytes
		m.eta = event.ETA
		m.status = "Uploading"
	case deploy.EventUploadDone:
		m.status = event.Message
		m.logs = append(m.logs, event.Message)
	case deploy.EventCommandStart:
		m.currentCommand = event.Command
		m.status = event.Message
	case deploy.EventCommandOutput:
		trimmed := strings.TrimSpace(event.Output)
		if trimmed != "" {
			for _, line := range strings.Split(trimmed, "\n") {
				m.logs = append(m.logs, line)
			}
		}
	case deploy.EventCommandDone:
		m.status = event.Message
		if strings.HasPrefix(event.Message, "<<< done ") {
			m.currentCommand = ""
		} else {
			m.currentCommand = ""
			m.logs = append(m.logs, event.Message)
		}
	}
	if len(m.logs) > 10 {
		m.logs = m.logs[len(m.logs)-10:]
	}
}

func (m runModel) selectView() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(" gscp deploy "))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Choose an environment to upload and execute"))
	b.WriteString("\n\n")
	for i, envKey := range m.envKeys {
		target := m.targets[envKey]
		server := m.servers[target.ActiveAlias]
		line := fmt.Sprintf("%s  %s -> %s  %s", envKey, target.LocalPath, target.ToPath, server.Host)
		if i == m.selected {
			b.WriteString(selectedEnvStyle.Render("> " + line))
		} else {
			b.WriteString(envStyle.Render("  " + line))
		}
		b.WriteString("\n")
		b.WriteString(metaStyle.Render(fmt.Sprintf("     alias: %s  commands: %d", target.ActiveAlias, len(target.Commands))))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("up/down to move, enter to confirm, q to quit"))
	return appStyle.Render(panelStyle.Render(b.String()))
}

func (m runModel) progressView() string {
	target := m.targets[m.chosenEnv]
	server := m.servers[target.ActiveAlias]
	var b strings.Builder
	b.WriteString(titleStyle.Render(" gscp deploy "))
	b.WriteString("\n")
	b.WriteString(renderStatusBadge(m.phase, m.status))
	b.WriteString("\n\n")
	b.WriteString(renderFact("Env", m.chosenEnv))
	b.WriteString("   ")
	b.WriteString(renderFact("Server", fmt.Sprintf("%s (%s)", target.ActiveAlias, server.Host)))
	b.WriteString("   ")
	b.WriteString(renderFact("Target", target.ToPath))
	b.WriteString("\n")
	if m.currentCommand != "" {
		b.WriteString("\n")
		b.WriteString(renderFact("Running", m.currentCommand))
	}
	if m.failedCommand != "" {
		b.WriteString("\n\n")
		b.WriteString(failureBoxStyle.Render("Failed command: " + m.failedCommand))
	}
	b.WriteString("\n\n")
	b.WriteString(renderMeter("Total", progressRatio(m.written, m.totalBytes), humanSize(m.written)+"/"+humanSize(m.totalBytes), totalFillStyle))
	b.WriteString("\n")
	b.WriteString(renderMeter(fmt.Sprintf("File %d/%d", m.fileIndex, m.totalFiles), progressRatio(m.fileDone, m.fileSize), fmt.Sprintf("%s  %s/%s", filepathBase(m.fileName), humanSize(m.fileDone), humanSize(m.fileSize)), fileFillStyle))
	b.WriteString("\n\n")
	b.WriteString(renderFact("Speed", humanSize(int64(m.speed))+"/s"))
	b.WriteString("   ")
	b.WriteString(renderFact("ETA", formatDuration(m.eta)))
	b.WriteString("\n\n")
	b.WriteString(logTitleStyle.Render("Recent logs"))
	b.WriteString("\n")
	for _, line := range m.logs {
		style := logLineStyle
		if strings.HasPrefix(line, ">>>") || strings.HasPrefix(line, "<<<") {
			style = commandStyle
		}
		if strings.HasPrefix(line, "ERROR:") {
			style = errorStyle
		}
		b.WriteString(style.Render("- " + line))
		b.WriteString("\n")
	}
	if m.phase == "running" {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("q to quit"))
	}
	if m.phase == "done" && !m.autoExit {
		b.WriteString("\n")
		b.WriteString(hintStyle.Render("press enter or q to exit"))
	}
	if m.phase == "error" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("Error: " + m.errorText))
		if !m.autoExit {
			b.WriteString("\n")
			b.WriteString(hintStyle.Render("press enter or q to exit"))
		}
	}
	return appStyle.Render(panelStyle.Render(b.String()))
}

func bar(ratio float64, width int, fill lipgloss.Style) string {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	if filled > width {
		filled = width
	}
	return "[" + fill.Render(strings.Repeat("=", filled)) + progressTrackStyle.Render(strings.Repeat("-", width-filled)) + "]"
}

func progressRatio(done, total int64) float64 {
	if total <= 0 {
		if done > 0 {
			return 1
		}
		return 0
	}
	return float64(done) / float64(total)
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	value := float64(size)
	units := []string{"KB", "MB", "GB", "TB"}
	idx := -1
	for value >= unit && idx < len(units)-1 {
		value /= unit
		idx++
	}
	return fmt.Sprintf("%.2f %s", value, units[idx])
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "--"
	}
	seconds := int(d.Seconds() + 0.5)
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	secs := seconds % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

func filepathBase(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

func renderStatusBadge(phase, status string) string {
	switch phase {
	case "running":
		return statusRunningStyle.Render(" " + status + " ")
	case "done":
		return statusDoneStyle.Render(" " + status + " ")
	case "error":
		return statusErrorStyle.Render(" " + status + " ")
	default:
		return statusIdleStyle.Render(" " + status + " ")
	}
}

func renderFact(label, value string) string {
	return labelStyle.Render(label+": ") + valueStyle.Render(value)
}

func renderMeter(label string, ratio float64, meta string, fill lipgloss.Style) string {
	return fmt.Sprintf("%s  %s  %6.2f%%  %s", valueStyle.Render(label), bar(ratio, 30, fill), ratio*100, metaStyle.Render(meta))
}
