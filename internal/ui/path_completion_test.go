package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func folders(t *testing.T, base string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.Mkdir(filepath.Join(base, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
}
func TestDirectoryCompletionLiteralPaths(t *testing.T) {
	dir := t.TempDir()
	folders(t, dir, "project one", "project two", "ქართული", ".hidden")
	if err := os.WriteFile(filepath.Join(dir, "project-file"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "project one"), filepath.Join(dir, "project-link")); err != nil {
		t.Fatal(err)
	}
	matches, err := directoryMatches(dir + "/project")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{dir + "/project one/", dir + "/project two/", dir + "/project-link/"}
	if !reflect.DeepEqual(matches, want) {
		t.Fatalf("got %q want %q", matches, want)
	}
	if prefix := commonDirectoryPrefix(matches); prefix != dir+"/project" {
		t.Fatalf("bad prefix %q", prefix)
	}
	matches, err = directoryMatches(dir + "/ქარ")
	if err != nil || len(matches) != 1 || matches[0] != dir+"/ქართული/" {
		t.Fatalf("Unicode: %q %v", matches, err)
	}
	matches, _ = directoryMatches(dir + "/")
	for _, match := range matches {
		if strings.Contains(match, ".hidden") {
			t.Fatal("hidden folder completed without a dot")
		}
	}
	matches, _ = directoryMatches(dir + "/.")
	if len(matches) != 1 || matches[0] != dir+"/.hidden/" {
		t.Fatal("explicit hidden completion failed")
	}
}
func TestHomeAndCommonUnicodePrefix(t *testing.T) {
	home := t.TempDir()
	folders(t, home, "Development")
	t.Setenv("HOME", home)
	matches, err := directoryMatches("~/Dev")
	if err != nil || !reflect.DeepEqual(matches, []string{"~/Development/"}) {
		t.Fatalf("home completion: %q %v", matches, err)
	}
	if got := commonDirectoryPrefix([]string{"/ქა/", "/ქბ/"}); got != "/ქ" {
		t.Fatalf("prefix split a Unicode rune: %q", got)
	}
}
func complete(t *testing.T, m repositoryPicker, key tea.KeyType) repositoryPicker {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: key})
	m = next.(repositoryPicker)
	if cmd != nil {
		switch msg := cmd().(type) {
		case directoryCompleted:
			next, _ = m.Update(msg)
			m = next.(repositoryPicker)
		}
	}
	return m
}
func TestPickerTabCompletesAndCycles(t *testing.T) {
	dir := t.TempDir()
	folders(t, dir, "repo-a", "repo-b")
	m := newRepositoryPicker(dir + "/rep")
	m = complete(t, m, tea.KeyTab)
	if m.input.Value() != dir+"/repo-" || len(m.matches) != 2 {
		t.Fatalf("common completion: %q", m.input.Value())
	}
	if !strings.Contains(m.View(), "repo-a/") || !strings.Contains(m.View(), "repo-b/") {
		t.Fatal("choices not shown")
	}
	m = complete(t, m, tea.KeyTab)
	if m.input.Value() != dir+"/repo-a/" {
		t.Fatal("first choice missing")
	}
	m = complete(t, m, tea.KeyTab)
	if m.input.Value() != dir+"/repo-b/" {
		t.Fatal("next choice missing")
	}
	m = complete(t, m, tea.KeyShiftTab)
	if m.input.Value() != dir+"/repo-a/" {
		t.Fatal("reverse cycle missing")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m = next.(repositoryPicker)
	if len(m.matches) != 0 {
		t.Fatal("editing retained stale completion choices")
	}
}
func TestCompletionErrorLeavesInputEditable(t *testing.T) {
	m := newRepositoryPicker(t.TempDir() + "/missing/path")
	m = complete(t, m, tea.KeyTab)
	if m.completing || m.status == "" {
		t.Fatal("completion did not recover from unreadable folder")
	}
	next, _ := m.Update(directoryCompleted{value: "older value", matches: []string{"wrong/"}})
	if next.(repositoryPicker).input.Value() != m.input.Value() {
		t.Fatal("stale completion replaced input")
	}
}
