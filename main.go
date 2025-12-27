package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const configFile = ".quickstream.json"

var (
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0455DD")).MarginBottom(1)
	selectedItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F50404")).Bold(true)
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA")).MarginTop(1)
	urlBoxStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#04D575")).Padding(1).MarginBottom(1)
	presetBoxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#D50404")).Padding(1)
	selectedURLStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#04F575")).Bold(true)
	normalURLStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#AAAAAA"))
)

type Config struct {
	URLs    []string `json:"urls"`
	Presets []string `json:"string"`
}

type model struct {
	config         Config
	urlIndex       int // Selected URL index
	presetIndex    int // Selected preset index
	selectedURL    int // Confirmed selected URL (-1 if not selected)
	selectedPreset int // Confirmed selected preset (-1 if not selected)
	showAddURL     bool
	showAddPreset  bool
	urlInput       textinput.Model
	presetInput    textinput.Model
	streaming      bool
	streamCmd      *exec.Cmd
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil

	case tea.KeyMsg:
		// Handle form inputs
		if m.showAddURL {
			return m.updateAddURL(msg)
		}
		if m.showAddPreset {
			return m.updateAddPreset(msg)
		}

		switch msg.String() {
		case "ctrl+c", "q":
			if m.streaming && m.streamCmd != nil {
				_ = m.streamCmd.Process.Kill()
			}
			return m, tea.Quit

		case "w", "W":
			// Move up in URL list
			if len(m.config.URLs) > 0 {
				m.urlIndex--
				if m.urlIndex < 0 {
					m.urlIndex = len(m.config.URLs) - 1
				}
				m.selectedURL = -1 // Reset selection when moving
			}
			return m, nil

		case "s", "S":
			// Move down in URL list
			if len(m.config.URLs) > 0 {
				m.urlIndex++
				if m.urlIndex >= len(m.config.URLs) {
					m.urlIndex = 0
				}
				m.selectedURL = -1 // Reset selection when moving
			}
			return m, nil

		case "up":
			// Move up in preset list
			if len(m.config.Presets) > 0 {
				m.presetIndex--
				if m.presetIndex < 0 {
					m.presetIndex = len(m.config.Presets) - 1
				}
				m.selectedPreset = -1 // Reset selection when moving
			}
			return m, nil

		case "down":
			// Move down in preset list
			if len(m.config.Presets) > 0 {
				m.presetIndex++
				if m.presetIndex >= len(m.config.Presets) {
					m.presetIndex = 0
				}
				m.selectedPreset = -1 // Reset selection when moving
			}
			return m, nil

		case "enter":
			// Select URL and/or start streaming
			if len(m.config.URLs) > 0 && m.urlIndex >= 0 && m.urlIndex < len(m.config.URLs) {
				m.selectedURL = m.urlIndex
			}
			if len(m.config.Presets) > 0 && m.presetIndex >= 0 && m.presetIndex < len(m.config.Presets) {
				m.selectedPreset = m.presetIndex
			}
			// If both selected, start streaming
			if m.selectedURL >= 0 && m.selectedPreset >= 0 {
				return m.startStreaming()
			}
			return m, nil

		case "shift+a", "shift+A", "A":
			// Delete URL (Shift+A or capital A)
			if len(m.config.URLs) > 0 && m.urlIndex >= 0 && m.urlIndex < len(m.config.URLs) {
				m.config.URLs = append(m.config.URLs[:m.urlIndex], m.config.URLs[m.urlIndex+1:]...)
				_ = saveConfig(m.config)
				// Adjust urlIndex if needed
				if m.urlIndex >= len(m.config.URLs) {
					m.urlIndex = len(m.config.URLs) - 1
				}
				if m.urlIndex < 0 && len(m.config.URLs) > 0 {
					m.urlIndex = 0
				}
				m.selectedURL = -1
			}
			return m, nil

		case "a":
			// Add URL (lowercase a only)
			m.showAddURL = true
			m.urlInput = textinput.New()
			m.urlInput.Placeholder = "rtmp://example.com/live/stream"
			m.urlInput.Focus()
			m.urlInput.CharLimit = 200
			m.urlInput.Width = 100
			return m, nil

		case "shift+p", "shift+P", "P":
			// Delete preset (Shift+P or capital P)
			if len(m.config.Presets) > 0 && m.presetIndex >= 0 && m.presetIndex < len(m.config.Presets) {
				m.config.Presets = append(m.config.Presets[:m.presetIndex], m.config.Presets[m.presetIndex+1:]...)
				_ = saveConfig(m.config)
				// Adjust presetIndex if needed
				if m.presetIndex >= len(m.config.Presets) {
					m.presetIndex = len(m.config.Presets) - 1
				}
				if m.presetIndex < 0 && len(m.config.Presets) > 0 {
					m.presetIndex = 0
				}
				m.selectedPreset = -1
			}
			return m, nil

		case "p":
			// Add preset (lowercase p only)
			m.showAddPreset = true
			m.presetInput = textinput.New()
			m.presetInput.Placeholder = "-f v4l2 -framerate 25 -video_size 1920x1080 -i /dev/video0 -f alsa -i plughw:2,0 libx264 aac -preset veryfast -maxrate 1M -bufsize 2M -pix_fmt yuv420p -b:a 96k -ar 44100"
			m.presetInput.Focus()
			m.presetInput.CharLimit = 1000
			m.presetInput.Width = 500
			return m, nil
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) updateAddURL(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			url := strings.TrimSpace(m.urlInput.Value())
			if url != "" {
				m.config.URLs = append(m.config.URLs, url)
				_ = saveConfig(m.config)
				m.urlIndex = len(m.config.URLs) - 1
				m.showAddURL = false
			}
			return m, nil
		case "esc":
			m.showAddURL = false
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.urlInput, cmd = m.urlInput.Update(msg)
	return m, cmd
}

func (m model) updateAddPreset(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			preset := strings.TrimSpace(m.presetInput.Value())
			if preset != "" {
				m.config.Presets = append(m.config.Presets, preset)
				_ = saveConfig(m.config)
				m.presetIndex = len(m.config.Presets) - 1
				m.showAddPreset = false
			} else if len(m.config.Presets) == 0 {
				m.config.Presets = append(m.config.Presets, "-f v4l2 -framerate 25 -video_size 1920x1080 -i /dev/video0 -f alsa -i plughw:2,0 libx264 aac -preset veryfast -maxrate 1M -bufsize 2M -pix_fmt yuv420p -b:a 96k -ar 44100")
				_ = saveConfig(m.config)
				m.presetIndex = len(m.config.Presets) - 1
				m.showAddPreset = false
			}
			return m, nil
		case "esc":
			m.showAddPreset = false
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.presetInput, cmd = m.presetInput.Update(msg)
	return m, cmd
}

func (m model) startStreaming() (tea.Model, tea.Cmd) {
	if m.streamCmd != nil && m.streamCmd.Process != nil {
		_ = m.streamCmd.Process.Kill() // force kill
		_ = m.streamCmd.Wait()         // reap the process
		m.streamCmd = nil
	}

	url := m.config.URLs[m.selectedURL]
	preset := m.config.Presets[m.selectedPreset]
	presetArgs := strings.Fields(preset)
	// Build ffmpeg command
	args := []string{}
	args = append(args, presetArgs...)
	args = append(args, "-f", "flv", url)

	m.streaming = true
	m.selectedURL = -1
	m.selectedPreset = -1

	cmd := exec.Command("ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Detach process
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	// Start in background
	if err := cmd.Start(); err != nil {
		// handle error
		return m, nil
	}

	m.streamCmd = cmd

	return m, nil
}

func (m model) View() string {
	if m.showAddURL {
		return fmt.Sprintf(
			"\n%s\n\n%s\n\n%s",
			titleStyle.Render("Add stream URL"),
			m.urlInput.View(),
			helpStyle.Render("Press Enter to save, Esc to cancel"),
		)
	}

	if m.showAddPreset {
		return fmt.Sprintf(
			"\n%s\n\n%s\n\n%s",
			titleStyle.Render("Add stream preset"),
			m.presetInput.View(),
			helpStyle.Render("Press Enter to save, Esc to cancel"),
		)
	}

	// Render URL list
	var urlView strings.Builder
	urlView.WriteString("Stream URLs\n")
	if len(m.config.URLs) == 0 {
		urlView.WriteString("-No urls-\n")
	} else {
		for i, url := range m.config.URLs {
			if i == m.urlIndex {
				urlView.WriteString(selectedURLStyle.Render(fmt.Sprintf("► %s", url)))
			} else {
				urlView.WriteString(normalURLStyle.Render(fmt.Sprintf("  %s", url)))
			}
			urlView.WriteString("\n")
		}
	}
	urlBox := urlBoxStyle.Render(urlView.String())

	// Render preset list
	var presetView strings.Builder
	presetView.WriteString("Presets\n")
	if len(m.config.Presets) == 0 {
		presetView.WriteString("-No presets-\n")
	} else {
		for i, preset := range m.config.Presets {
			if i == m.presetIndex {
				presetView.WriteString(selectedItemStyle.Render(fmt.Sprintf("► %s", preset)))
			} else {
				presetView.WriteString(normalURLStyle.Render(fmt.Sprintf("  %s", preset)))
			}
			presetView.WriteString("\n")
		}
	}
	presetBox := presetBoxStyle.Render(presetView.String())

	helpText := helpStyle.Render("w/s: url • ↑/↓: preset • enter: start • a/p: add • shift+a/p: delete • q: quit")
	return fmt.Sprintf(
		"%s\n%s\n%s\n%s",
		titleStyle.Render("Quick Stream"),
		urlBox,
		presetBox,
		helpText,
	)
}

func main() {
	config := loadConfig()

	urlIndex := 0
	if len(config.URLs) == 0 {
		urlIndex = -1
	}

	presetIndex := 0
	if len(config.Presets) == 0 {
		presetIndex = -1
	}

	m := model{
		config:         config,
		urlIndex:       urlIndex,
		presetIndex:    presetIndex,
		selectedURL:    -1,
		selectedPreset: -1,
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}

func loadConfig() Config {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	configPath := filepath.Join(homeDir, configFile)

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Return empty config if file doesn't exist
		return Config{
			URLs:    []string{},
			Presets: []string{},
		}
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing config: %v\n", err)
		return Config{}
	}

	return config
}

func saveConfig(config Config) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	configPath := filepath.Join(homeDir, configFile)

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0o644)
}
