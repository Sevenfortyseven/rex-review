package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"rex-review/internal/review"
)

type repositoryPicker struct {
	input           textinput.Model
	repo            string
	status          string
	width           int
	height          int
	checking        bool
	completing      bool
	matches         []string
	completionValue string
	completionIndex int
}
type directoryCompleted struct {
	value   string
	matches []string
	err     error
}
type repositoryChecked struct {
	root string
	err  error
}

func newRepositoryPicker(initial string) repositoryPicker {
	in := textinput.New()
	in.PlaceholderStyle = muted
	in.CompletionStyle = muted
	in.PromptStyle = accent
	in.Placeholder = "/path/to/your/project"
	in.SetValue(initial)
	in.CursorEnd()
	in.Focus()
	in.CharLimit = 4096
	return repositoryPicker{input: in, width: 80, height: 24}
}
func (m repositoryPicker) Init() tea.Cmd { return textinput.Blink }
func (m repositoryPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(8, msg.Width-8)
		return m, nil
	case repositoryChecked:
		m.checking = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.repo = msg.root
		return m, tea.Quit
	case directoryCompleted:
		m.completing = false
		if m.checking || m.input.Value() != msg.value {
			return m, nil
		}
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.matches = msg.matches
		m.completionIndex = -1
		if len(msg.matches) == 0 {
			m.status = "No matching folders"
			return m, nil
		}
		completed := commonDirectoryPrefix(msg.matches)
		m.input.SetValue(completed)
		m.input.CursorEnd()
		m.completionValue = completed
		if len(msg.matches) == 1 {
			m.matches = nil
			m.status = "Folder completed · Tab to browse inside · Enter to open"
		} else {
			m.status = fmt.Sprintf("%d folders · Tab / Shift+Tab to cycle", len(msg.matches))
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			if m.checking || m.completing {
				return m, nil
			}
			if len(m.matches) > 1 && m.input.Value() == m.completionValue {
				step := 1
				if msg.String() == "shift+tab" {
					step = -1
				}
				if m.completionIndex < 0 && step < 0 {
					m.completionIndex = 0
				}
				m.completionIndex = (m.completionIndex + step + len(m.matches)) % len(m.matches)
				m.completionValue = m.matches[m.completionIndex]
				m.input.SetValue(m.completionValue)
				m.input.CursorEnd()
				m.status = fmt.Sprintf("Folder %d of %d · Tab / Shift+Tab cycles · type to continue", m.completionIndex+1, len(m.matches))
				return m, nil
			}
			m.completing = true
			value := m.input.Value()
			m.status = "Completing folder…"
			return m, func() tea.Msg {
				matches, err := directoryMatches(value)
				return directoryCompleted{value, matches, err}
			}
		case "enter":
			if m.checking {
				return m, nil
			}
			m.checking = true
			value := m.input.Value()
			m.status = "Checking project folder…"
			return m, func() tea.Msg { root, err := review.RepositoryPath(value); return repositoryChecked{root, err} }
		}
	}
	if m.checking {
		return m, nil
	}
	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.matches = nil
		m.status = ""
	}
	return m, cmd
}
func (m repositoryPicker) View() string {
	width := max(10, m.width-4)
	var lines []string
	content := []string{"Rex Review · Choose project folder", "", "The agent's reported folder may differ from its current project.", "Enter the Git project folder you want to review:", "", m.input.View()}
	if len(m.matches) > 1 {
		count := min(5, max(0, m.height-12))
		start := max(0, m.completionIndex-count+1)
		for i := start; i < min(len(m.matches), start+count); i++ {
			mark := "  "
			if i == m.completionIndex {
				mark = "› "
			}
			content = append(content, mark+clean(filepath.Base(strings.TrimSuffix(m.matches[i], "/")))+"/")
		}
	}
	content = append(content, "", "Tab complete · Shift+Tab previous · Enter open · Ctrl+U clear · Esc cancel", "", "The choice is remembered for this running agent. Press p in the viewer to change it.", "", clean(m.status))
	for _, line := range content {
		lines = append(lines, fit(line, width))
	}
	return strings.Join(lines, "\n")
}

// SelectRepository is separate from the diff viewer so an invalid launch directory
// can be corrected in-place without sending anything to the originating agent.
func SelectRepository(initial string) (string, error) {
	result, err := tea.NewProgram(newRepositoryPicker(initial), tea.WithAltScreen()).Run()
	if err != nil {
		return "", err
	}
	m, ok := result.(repositoryPicker)
	if !ok {
		return "", fmt.Errorf("unexpected project picker result")
	}
	return m.repo, nil
}
