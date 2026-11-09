package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"rex-review/internal/review"
	"rex-review/internal/rex"
)

func testModel(t *testing.T) Model {
	t.Helper()
	d := review.Draft{Repo: "/repo", Session: "s", Block: "b"}
	m := New(rex.Client{}, rex.Process{Name: "codex"}, "w", filepath.Join(t.TempDir(), "draft.json"), d)
	m.files = []review.File{{Path: "example.swift", Scope: "unstaged", Hash: "snapshot", Lines: []review.Line{{Text: "+let value = 1", Kind: '+', New: 7}}}}
	return m
}
func key(m Model, s string) Model {
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return v.(Model)
}
func TestEditorAutosavesAndCancelRestores(t *testing.T) {
	m := key(testModel(t), "c")
	if m.editing < 0 {
		t.Fatal("editor did not open")
	}
	m = key(m, "Check this line")
	d, err := review.ReadDraft(m.DraftPath, review.Draft{Repo: "/repo", Session: "s", Block: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Comments) != 1 || d.Comments[0].Text != "Check this line" {
		t.Fatalf("draft not saved: %+v", d)
	}
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = v.(Model)
	if len(m.Draft.Comments) != 0 || m.editing != -1 {
		t.Fatal("cancel did not remove new draft")
	}
}
func TestSendRequiresConfirmationAndRejectsUncertainDelivery(t *testing.T) {
	m := testModel(t)
	c, _ := review.NewComment(m.files[0], 0, 0, "Fix this")
	m.Draft.Comments = []review.Comment{c}
	m = key(m, "s")
	if !m.confirm || m.busy {
		t.Fatal("send did not wait for confirmation")
	}
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = v.(Model)
	m.Draft.Pasted = true
	m = key(m, "s")
	if m.confirm {
		t.Fatal("uncertain delivery could be submitted twice")
	}
}
func TestStaleReviewNeverTouchesAgent(t *testing.T) {
	m := testModel(t)
	m.Draft.Repo = t.TempDir()
	m.Draft.Comments = []review.Comment{{Path: "missing.swift", Scope: "unstaged", Hash: "old", Text: "comment"}}
	msg := m.send()().(sent)
	if msg.err == nil || msg.success {
		t.Fatal("missing diff reached agent")
	}
	if msg.draft.Pasted {
		t.Fatal("failed validation marked as pasted")
	}
	if _, err := os.Stat(m.DraftPath); !os.IsNotExist(err) {
		t.Fatal("failed validation mutated draft")
	}
}
func TestViewFitsAndHidesTerminalControlCodes(t *testing.T) {
	m := testModel(t)
	m.files[0].Path = "bad\x1b[2J.swift"
	m.w = 120
	m.h = 35
	if strings.Contains(m.View(), "bad\x1b[2J") {
		t.Fatal("file path could execute terminal commands")
	}
	m.w = 40
	if !strings.Contains(m.View(), "Resize") {
		t.Fatal("small window not handled")
	}
}

func breakDraftSave(t *testing.T, m *Model) {
	t.Helper()
	m.DraftPath = filepath.Join(t.TempDir(), "existing-directory")
	if err := os.Mkdir(m.DraftPath, 0700); err != nil {
		t.Fatal(err)
	}
}

func TestFailedCancelKeepsEditorAndText(t *testing.T) {
	m := key(key(testModel(t), "c"), "Keep my draft")
	breakDraftSave(t, &m)
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = v.(Model)
	if len(m.Draft.Comments) != 1 || m.editing != 0 || m.Draft.Comments[0].Text != "Keep my draft" {
		t.Fatalf("failed save corrupted editor state: %+v", m.Draft)
	}
	if !strings.Contains(m.status, "Could not save draft") {
		t.Fatal("save error hidden")
	}
	// Further typing used to index a deleted comment and panic.
	m = key(m, "!")
	if m.Draft.Comments[0].Text != "Keep my draft!" {
		t.Fatal("editor could not recover")
	}
}

func TestFailedDeleteAndMarkerClearKeepDraft(t *testing.T) {
	m := key(key(testModel(t), "c"), "Keep this comment")
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	m = v.(Model)
	m.comments = true
	m.Draft.Pasted = true
	breakDraftSave(t, &m)
	m = key(m, "d")
	if len(m.Draft.Comments) != 1 || !strings.Contains(m.status, "Could not save draft") {
		t.Fatal("failed deletion lost comment or error")
	}
	m = key(m, "x")
	if !m.Draft.Pasted || !strings.Contains(m.status, "Could not save draft") {
		t.Fatal("failed marker save allowed duplicate delivery")
	}
}

func TestHelpFitsSmallPaneAndScrolls(t *testing.T) {
	m := testModel(t)
	m.w = 60
	m.h = 16
	m = key(m, "?")
	if lipgloss.Height(m.View()) > m.h {
		t.Fatal("help overflows pane")
	}
	if strings.Contains(m.View(), "Mouse:") {
		t.Fatal("test expects mouse instructions below initial viewport")
	}
	v, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = v.(Model)
	if !strings.Contains(m.View(), "Mouse:") {
		t.Fatal("cannot scroll to remaining help")
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if lipgloss.Width(line) > m.w {
			t.Fatal("help wraps beyond pane width")
		}
	}
	v, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m = v.(Model)
	if lipgloss.Height(m.View()) > m.h {
		t.Fatal("resize overflowed help")
	}
}
