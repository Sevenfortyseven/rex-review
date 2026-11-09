package review

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Line struct {
	Text string `json:"text"`
	Old  int    `json:"old"`
	New  int    `json:"new"`
	Kind byte   `json:"kind"`
}

type File struct {
	Path  string `json:"path"`
	Scope string `json:"scope"`
	Hash  string `json:"hash"`
	Lines []Line `json:"lines"`
}

func (f File) ID() string { return f.Scope + "\x00" + f.Path }

// Bound memory while git is writing, including binary/untracked or minified files.
type limitedBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	const limit = 4 << 20
	if b.Len()+len(p) > limit {
		b.exceeded = true
		return 0, fmt.Errorf("diff exceeds 4 MiB")
	}
	return b.Buffer.Write(p)
}

func git(dir string, diff bool, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	base := []string{"--literal-pathspecs", "-C", dir}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	var out limitedBuffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()
	if out.exceeded {
		return "", fmt.Errorf("Git output exceeds 4 MiB; narrow the changes before reviewing")
	}
	if err != nil {
		if e, ok := err.(*exec.ExitError); !(diff && ok && e.ExitCode() == 1) {
			return "", fmt.Errorf("git: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
	}
	return out.String(), nil
}

func Root(dir string) (string, error) {
	s, err := git(dir, false, "rev-parse", "--show-toplevel")
	return strings.TrimSuffix(s, "\n"), err
}

func Load(repo string) ([]File, error) {
	var files []File
	for _, scope := range []string{"unstaged", "staged", "untracked"} {
		args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-renames", "--name-only", "-z"}
		if scope == "staged" {
			args = append(args, "--cached")
		}
		if scope == "untracked" {
			args = []string{"ls-files", "--others", "--exclude-standard", "-z"}
		}
		out, err := git(repo, false, args...)
		if err != nil {
			return nil, err
		}
		for _, path := range strings.Split(out, "\x00") {
			if path == "" {
				continue
			}
			f, err := LoadFile(repo, path, scope)
			if err != nil {
				return nil, err
			}
			files = append(files, f)
		}
	}
	return files, nil
}

func LoadFile(repo, path, scope string) (File, error) {
	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--unified=3"}
	if scope == "staged" {
		args = append(args, "--cached")
	}
	if scope == "untracked" {
		args = append(args, "--no-index", "--", "/dev/null", filepath.Join(repo, path))
	} else {
		args = append(args, "--", path)
	}
	out, err := git(repo, scope == "untracked", args...)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	return File{Path: path, Scope: scope, Hash: fmt.Sprintf("%x", sha256.Sum256([]byte(out))), Lines: ParseDiff(out)}, nil
}

var hunk = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

func ParseDiff(s string) []Line {
	var lines []Line
	old, current, inside := 0, 0, false
	for _, text := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		if m := hunk.FindStringSubmatch(text); m != nil {
			old, _ = strconv.Atoi(m[1])
			current, _ = strconv.Atoi(m[2])
			inside = true
			lines = append(lines, Line{Text: text, Kind: '@'})
			continue
		}
		l := Line{Text: text, Kind: 'h'}
		if inside && text != "" {
			l.Kind = text[0]
			switch l.Kind {
			case '+':
				l.New = current
				current++
			case '-':
				l.Old = old
				old++
			case ' ':
				l.Old = old
				l.New = current
				old++
				current++
			}
		}
		lines = append(lines, l)
	}
	return lines
}
