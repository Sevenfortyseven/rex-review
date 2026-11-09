package ui

import (
	"os/exec"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestRepositoryPickerRecoversFromNonGitFolder(t *testing.T) {
	m := newRepositoryPicker(t.TempDir())
	value, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = value.(repositoryPicker)
	if cmd == nil {
		t.Fatal("directory was not checked")
	}
	value, _ = m.Update(cmd())
	m = value.(repositoryPicker)
	if m.repo != "" || m.status == "" || m.checking {
		t.Fatal("invalid folder should remain editable")
	}
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("%s: %v", out, err)
	}
	m.input.SetValue(repo)
	value, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = value.(repositoryPicker)
	value, cmd = m.Update(cmd())
	m = value.(repositoryPicker)
	if m.repo == "" || cmd == nil {
		t.Fatal("valid repository did not open")
	}
}

func TestProjectSwitchPreservesDraft(t *testing.T) {
	m := key(key(testModel(t), "c"), "Saved for original project")
	value, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = value.(Model)
	value, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	m = value.(Model)
	if !m.ChooseRepository || cmd == nil || len(m.Draft.Comments) != 1 {
		t.Fatal("project switch lost draft or did not request picker")
	}
}
