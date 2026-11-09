// Package install connects the standalone application to Rex's configuration.
package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"rex-review/internal/rex"
	rexconfig "rex-review/rex"
)

const begin = "-- BEGIN rex-review (managed)"
const end = "-- END rex-review (managed)"

func DefaultConfig() (string, error) {
	if path := os.Getenv("REX_CONFIG"); path != "" {
		return filepath.Abs(path)
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "rex", "init.lua"), nil
}

func configTarget(path string) (string, error) {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return path, nil
	}
	if err != nil {
		return "", err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return filepath.EvalSymlinks(path)
	}
	return path, nil
}

func withoutBlock(text string) (string, error) {
	a, b := strings.Index(text, begin), strings.Index(text, end)
	if a < 0 && b < 0 {
		return text, nil
	}
	if a < 0 || b < a || strings.Count(text, begin) != 1 || strings.Count(text, end) != 1 {
		return "", fmt.Errorf("Rex config has an incomplete or duplicate rex-review block")
	}
	finish := b + len(end)
	if finish < len(text) && text[finish] == '\n' {
		finish++
	}
	return text[:a] + text[finish:], nil
}

func configured(text, dir, server string) (string, error) {
	clean, err := withoutBlock(text)
	if err != nil {
		return "", err
	}
	block := fmt.Sprintf("%s\ndo\n  local setup = dofile(%s)\n  setup{ bin = %s, server = %s }\nend\n%s\n", begin, strconv.Quote(filepath.Join(dir, "config.lua")), strconv.Quote(filepath.Join(dir, "rex-review")), strconv.Quote(server), end)
	// Load before user bindings so their later shortcut overrides keep working.
	return block + clean, nil
}

type fileState struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

func read(path string) (fileState, error) {
	s := fileState{path: path, mode: 0600}
	st, err := os.Stat(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if !st.Mode().IsRegular() {
		return s, fmt.Errorf("refusing to replace nonregular file %s", path)
	}
	s.mode = st.Mode().Perm()
	s.data, err = os.ReadFile(path)
	s.exists = true
	return s, err
}
func write(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".rex-review-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func restore(s fileState) error {
	if s.exists {
		return write(s.path, s.data, s.mode)
	}
	err := os.Remove(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func check(client rex.Client, configPath, text string) error {
	if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(configPath), ".rex-review-check-*.lua")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if _, err = client.Run("", "config", "check", f.Name()); err != nil {
		return fmt.Errorf("Rex rejected the config; existing init.lua was kept: %w", err)
	}
	return nil
}

func Install(configPath, server, executable string, client rex.Client) (err error) {
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return err
	}
	declaredPath := configPath
	dir := filepath.Join(filepath.Dir(configPath), "rex-review")
	configPath, err = configTarget(configPath)
	if err != nil {
		return err
	}
	config, err := read(configPath)
	if err != nil {
		return err
	}
	text, err := configured(string(config.data), dir, server)
	if err != nil {
		return err
	}
	binary, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	var changed []fileState
	defer func() {
		if err != nil {
			for i := len(changed) - 1; i >= 0; i-- {
				if e := restore(changed[i]); e != nil {
					err = fmt.Errorf("%w; restore %s: %v", err, changed[i].path, e)
				}
			}
		}
	}()
	for _, asset := range []struct {
		name string
		data []byte
		mode os.FileMode
	}{{"rex-review", binary, 0755}, {"config.lua", rexconfig.Lua, 0600}} {
		path := filepath.Join(dir, asset.name)
		old, e := read(path)
		if e != nil {
			return e
		}
		if e = write(path, asset.data, asset.mode); e != nil {
			return e
		}
		changed = append(changed, old)
	}
	if err = check(client, declaredPath, text); err != nil {
		return err
	}
	backup := configPath + ".rex-review.bak"
	if _, e := os.Stat(backup); os.IsNotExist(e) && config.exists {
		if err = write(backup, config.data, config.mode); err != nil {
			return err
		}
	}
	if err = write(configPath, []byte(text), config.mode); err != nil {
		return err
	}
	return nil
}

func Uninstall(configPath string, client rex.Client) error {
	configPath, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	declaredPath := configPath
	dir := filepath.Join(filepath.Dir(configPath), "rex-review")
	configPath, err = configTarget(configPath)
	if err != nil {
		return err
	}
	config, err := read(configPath)
	if err != nil {
		return err
	}
	text, err := withoutBlock(string(config.data))
	if err != nil {
		return err
	}
	if !config.exists || text == string(config.data) {
		return nil
	}
	if err = check(client, declaredPath, text); err != nil {
		return err
	}
	if err = write(configPath, []byte(text), config.mode); err != nil {
		return err
	}
	for _, name := range []string{"config.lua", "rex-review"} {
		if e := os.Remove(filepath.Join(dir, name)); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	// Leave extra user files alone if the directory is not empty.
	_ = os.Remove(dir)
	return nil
}
