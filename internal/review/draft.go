package review

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Comment struct {
	ID      int64  `json:"id"`
	Path    string `json:"path"`
	Scope   string `json:"scope"`
	Hash    string `json:"hash"`
	Side    string `json:"side"`
	Start   int    `json:"start"`
	End     int    `json:"end"`
	Snippet string `json:"snippet"`
	Text    string `json:"text"`
}

type Draft struct {
	Repo     string    `json:"repo"`
	Session  string    `json:"session"`
	Block    string    `json:"block"`
	Comments []Comment `json:"comments"`
	// A paste succeeded but submission failed: retain it without allowing a duplicate send.
	Pasted bool `json:"pasted"`
}

func NewComment(f File, a, b int, text string) (Comment, error) {
	if a > b {
		a, b = b, a
	}
	if a < 0 || b >= len(f.Lines) {
		return Comment{}, fmt.Errorf("select code lines first")
	}
	start, end, side := 0, 0, "new"
	// Any selected additions/context use new-file coordinates; deletion-only uses old.
	for _, l := range f.Lines[a : b+1] {
		if l.New > 0 {
			if start == 0 {
				start = l.New
			}
			end = l.New
		}
	}
	if start == 0 {
		side = "old"
		for _, l := range f.Lines[a : b+1] {
			if l.Old > 0 {
				if start == 0 {
					start = l.Old
				}
				end = l.Old
			}
		}
	}
	if start == 0 {
		return Comment{}, fmt.Errorf("select code lines, rather than a diff header")
	}
	if strings.TrimSpace(text) == "" {
		return Comment{}, fmt.Errorf("write a comment first")
	}
	var snippet []string
	for _, l := range f.Lines[a : b+1] {
		snippet = append(snippet, l.Text)
	}
	return Comment{ID: time.Now().UnixNano(), Path: f.Path, Scope: f.Scope, Hash: f.Hash, Side: side, Start: start, End: end, Snippet: strings.Join(snippet, "\n"), Text: strings.TrimSpace(text)}, nil
}

func (c Comment) Location() string {
	ref := fmt.Sprintf("%s:%d", c.Path, c.Start)
	if c.End != c.Start {
		ref += fmt.Sprintf("-%d", c.End)
	}
	return fmt.Sprintf("%s (%s, %s-file lines)", ref, c.Scope, c.Side)
}

func (d Draft) Message() string {
	var s strings.Builder
	fmt.Fprintf(&s, "Please address these review comments on the uncommitted Git changes in %q. Line references use the indicated old or new side of the diff.\n", d.Repo)
	for i, c := range d.Comments {
		// JSON quoting preserves paths and snippets containing Markdown fences/control chars.
		fmt.Fprintf(&s, "\n%d. %s\nSelected diff: %q\nComment: %s\n", i+1, c.Location(), c.Snippet, c.Text)
	}
	return s.String()
}

func DraftPath(base, repo, session, block string) string {
	id := sha256.Sum256([]byte(repo + "\x00" + session + "\x00" + block))
	return filepath.Join(base, fmt.Sprintf("%x.json", id))
}

func ReadDraft(path string, expected Draft) (Draft, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return expected, nil
	}
	if err != nil {
		return expected, err
	}
	var d Draft
	if err = json.Unmarshal(b, &d); err != nil {
		return expected, fmt.Errorf("read saved draft: %w", err)
	}
	if d.Repo != expected.Repo || d.Session != expected.Session || d.Block != expected.Block {
		return expected, fmt.Errorf("draft destination does not match")
	}
	return d, nil
}

func SaveDraft(path string, d Draft) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".draft-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
