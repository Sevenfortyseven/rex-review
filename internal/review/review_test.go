package review

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func command(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s: %v", args, out, err)
	}
}
func write(t *testing.T, repo, path, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, path), []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func fixture(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	command(t, d, "init", "-q")
	command(t, d, "config", "user.name", "Fixture")
	command(t, d, "config", "user.email", "fixture@example.test")
	write(t, d, "main.txt", "one\ntwo\nthree\n")
	command(t, d, "add", ".")
	command(t, d, "commit", "-qm", "baseline")
	return d
}

func TestLoadSeparatesIndexWorktreeAndUntracked(t *testing.T) {
	d := fixture(t)
	write(t, d, "main.txt", "one\nstaged\nthree\n")
	command(t, d, "add", "main.txt")
	write(t, d, "main.txt", "one\nunstaged\nthree\n")
	path := "new file\twith\nnewline.txt"
	write(t, d, path, "brand new\n")
	files, err := Load(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("got %d: %+v", len(files), files)
	}
	if files[0].Scope != "unstaged" || files[1].Scope != "staged" || files[2].Path != path {
		t.Fatalf("unexpected entries: %+v", files)
	}
	for _, f := range files {
		if len(f.Lines) == 0 || f.Hash == "" {
			t.Fatalf("empty diff: %+v", f)
		}
	}
	if !strings.Contains(joinLines(files[0]), "-staged") || !strings.Contains(joinLines(files[0]), "+unstaged") {
		t.Fatal("worktree should compare against index")
	}
	if !strings.Contains(joinLines(files[1]), "-two") || !strings.Contains(joinLines(files[1]), "+staged") {
		t.Fatal("index should compare against HEAD")
	}
}
func joinLines(f File) string {
	var s []string
	for _, l := range f.Lines {
		s = append(s, l.Text)
	}
	return strings.Join(s, "\n")
}

func TestUnbornRepoAndBinary(t *testing.T) {
	d := t.TempDir()
	command(t, d, "init", "-q")
	write(t, d, "binary.dat", "\x00\x01\x02")
	write(t, d, "new.txt", "new\n")
	command(t, d, "add", "new.txt")
	files, err := Load(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %+v", files)
	}
	if !strings.Contains(joinLines(files[1]), "Binary files") {
		t.Fatal("binary should have a nonselectable notice")
	}
}

func TestDeletedLinesHaveOldCoordinates(t *testing.T) {
	d := fixture(t)
	write(t, d, "main.txt", "one\nthree\n")
	files, err := Load(d)
	if err != nil {
		t.Fatal(err)
	}
	f := files[0]
	index := -1
	for i, l := range f.Lines {
		if l.Kind == '-' {
			index = i
		}
	}
	c, err := NewComment(f, index, index, "Why remove this?")
	if err != nil {
		t.Fatal(err)
	}
	if c.Side != "old" || c.Start != 2 || c.End != 2 {
		t.Fatalf("wrong location %+v", c)
	}
	if _, err = NewComment(f, 0, 0, "header"); err == nil {
		t.Fatal("header should not accept a line comment")
	}
}

func TestMixedRangeAndStaleSnapshot(t *testing.T) {
	d := fixture(t)
	write(t, d, "main.txt", "one\nreplacement\nthree\n")
	f, err := LoadFile(d, "main.txt", "unstaged")
	if err != nil {
		t.Fatal(err)
	}
	a, b := -1, -1
	for i, l := range f.Lines {
		if l.Kind == '-' {
			a = i
		}
		if l.Kind == '+' {
			b = i
		}
	}
	c, err := NewComment(f, b, a, "Review replacement")
	if err != nil {
		t.Fatal(err)
	}
	if c.Side != "new" || c.Start != 2 || c.End != 2 || !strings.Contains(c.Snippet, "-two") {
		t.Fatalf("wrong mixed range %+v", c)
	}
	write(t, d, "main.txt", "one\nlater change\nthree\n")
	after, _ := LoadFile(d, "main.txt", "unstaged")
	if after.Hash == c.Hash {
		t.Fatal("changed diff kept its fingerprint")
	}
}

func TestLiteralPathAndDeletedFile(t *testing.T) {
	d := fixture(t)
	write(t, d, "[abc]*.txt", "literal\n")
	command(t, d, "add", ".")
	command(t, d, "commit", "-qm", "literal")
	write(t, d, "[abc]*.txt", "changed\n")
	if err := os.Remove(filepath.Join(d, "main.txt")); err != nil {
		t.Fatal(err)
	}
	files, err := Load(d)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %+v", files)
	}
	for _, f := range files {
		if len(f.Lines) < 4 {
			t.Fatalf("missing diff for %q", f.Path)
		}
	}
}

func TestDraftRoundTripAndDestinationIsolation(t *testing.T) {
	d := Draft{Repo: "/repo", Session: "s", Block: "b", Comments: []Comment{{Path: "main.txt", Start: 2, End: 4, Side: "old", Scope: "staged", Text: "Keep this", Snippet: "-code"}}}
	path := DraftPath(t.TempDir(), d.Repo, d.Session, d.Block)
	if err := SaveDraft(path, d); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDraft(path, Draft{Repo: d.Repo, Session: d.Session, Block: d.Block})
	if err != nil {
		t.Fatal(err)
	}
	if got.Comments[0].Text != "Keep this" {
		t.Fatalf("lost comment %+v", got)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0600 {
		t.Fatal("draft is not private")
	}
	if _, err = ReadDraft(path, Draft{Repo: "/repo", Session: "s", Block: "other"}); err == nil {
		t.Fatal("accepted another destination")
	}
	msg := got.Message()
	if !strings.Contains(msg, "main.txt:2-4 (staged, old-file lines)") || !strings.Contains(msg, "Keep this") {
		t.Fatalf("bad message %q", msg)
	}
}
