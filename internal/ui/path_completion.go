package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Complete directory names directly rather than invoking a shell. Spaces,
// Unicode and shell metacharacters stay literal in this input field.
func directoryMatches(value string) ([]string, error) {
	if value == "~" {
		return []string{"~/"}, nil
	}
	path := value
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(home, path[2:])
		if strings.HasSuffix(value, "/") {
			path += string(filepath.Separator)
		}
	}
	dir, prefix := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	displayDir, _ := filepath.Split(value)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot list project folders: %w", err)
	}
	var matches []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || strings.HasPrefix(name, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		isDir := entry.IsDir()
		if !isDir && entry.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
				isDir = info.IsDir()
			}
		}
		if isDir {
			matches = append(matches, displayDir+name+string(filepath.Separator))
		}
	}
	return matches, nil
}

func commonDirectoryPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := values[0]
	for _, value := range values[1:] {
		for !strings.HasPrefix(value, prefix) {
			_, size := utf8.DecodeLastRuneInString(prefix)
			prefix = prefix[:len(prefix)-size]
		}
	}
	return prefix
}
