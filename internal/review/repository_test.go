package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositorySelectionAndHints(t *testing.T) {
	repo := fixture(t)
	root, err := RepositoryPath(repo)
	if err != nil {
		t.Fatal(err)
	}
	if root == "" {
		t.Fatal("empty root")
	}
	for _, input := range []string{"", t.TempDir(), filepath.Join(repo, "missing")} {
		if _, err := RepositoryPath(input); err == nil {
			t.Fatalf("accepted nonrepo %q", input)
		}
	}
	base := t.TempDir()
	path := RepositoryHintPath(base, "s", "b", 42)
	if err := SaveRepositoryHint(path, root); err != nil {
		t.Fatal(err)
	}
	if ReadRepositoryHint(path) != root {
		t.Fatal("lost project choice")
	}
	for _, other := range []string{RepositoryHintPath(base, "s", "other", 42), RepositoryHintPath(base, "s", "b", 43)} {
		if ReadRepositoryHint(other) != "" {
			t.Fatal("project choice leaked to another agent")
		}
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatal("project choice should be private")
	}
}
