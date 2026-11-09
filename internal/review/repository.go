package review

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func RepositoryPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "~" || strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if input == "~" {
			input = home
		} else {
			input = filepath.Join(home, input[2:])
		}
	}
	if input == "" {
		return "", fmt.Errorf("enter a project folder")
	}
	root, err := Root(input)
	if err != nil {
		return "", fmt.Errorf("not an accessible Git working tree: %q", input)
	}
	return root, nil
}

// A choice belongs to one running agent, not every tab or a future replacement.
func RepositoryHintPath(base, session, block string, pid int) string {
	id := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d", session, block, pid)))
	return filepath.Join(base, "repositories", fmt.Sprintf("%x.json", id))
}

func ReadRepositoryHint(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var root string
	if json.Unmarshal(b, &root) != nil {
		return ""
	}
	return root
}

func SaveRepositoryHint(path, repo string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(repo)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".repository-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
