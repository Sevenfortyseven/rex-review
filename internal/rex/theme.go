package rex

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type TerminalTheme struct {
	Foreground string   `json:"foreground,omitempty"`
	Background string   `json:"background,omitempty"`
	Cursor     string   `json:"cursor,omitempty"`
	Palette    []string `json:"palette,omitempty"`
	Scheme     string   `json:"scheme,omitempty"`
}

type catalogTheme struct {
	ID     string `json:"id"`
	Source struct {
		Foreground string   `json:"foreground"`
		Background string   `json:"background"`
		Cursor     string   `json:"cursor"`
		ANSI       []string `json:"ansi"`
	} `json:"source"`
	Semantic map[string]struct {
		Red   float64 `json:"red"`
		Green float64 `json:"green"`
		Blue  float64 `json:"blue"`
	} `json:"semantic"`
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func selectedTheme(prefs map[string]any, systemDark bool, catalog []byte) (TerminalTheme, error) {
	scheme := "light"
	if systemDark {
		scheme = "dark"
	}
	if appearance, ok := prefs["Workspace.appearance"].(string); ok && (appearance == "light" || appearance == "dark") {
		scheme = appearance
	}
	key := "Workspace.lightThemeID"
	if scheme == "dark" {
		key = "Workspace.darkThemeID"
	}
	id, _ := prefs[key].(string)
	if id == "" {
		return TerminalTheme{}, fmt.Errorf("Rex has no saved %s theme selection", scheme)
	}
	var themes []catalogTheme
	if err := json.Unmarshal(catalog, &themes); err != nil {
		return TerminalTheme{}, err
	}
	for _, theme := range themes {
		if theme.ID != id {
			continue
		}
		out := TerminalTheme{Foreground: theme.Source.Foreground, Background: theme.Source.Background, Cursor: theme.Source.Cursor, Scheme: scheme}
		p3, _ := prefs["Workspace.terminalUsesDisplayP3"].(bool)
		color := func(name, fallback string) string {
			if rgb, ok := theme.Semantic[name]; ok && p3 {
				channel := func(v float64) int { return int(math.Round(max(0, min(1, v)) * 255)) }
				return fmt.Sprintf("#%02x%02x%02x", channel(rgb.Red), channel(rgb.Green), channel(rgb.Blue))
			}
			return fallback
		}
		out.Foreground = color("terminalForeground", out.Foreground)
		out.Background = color("terminalBackground", out.Background)
		out.Cursor = color("terminalCursor", out.Cursor)
		if !hexColor.MatchString(out.Foreground) || !hexColor.MatchString(out.Background) || !hexColor.MatchString(out.Cursor) {
			return TerminalTheme{}, fmt.Errorf("unsupported Rex theme colors")
		}
		if len(theme.Source.ANSI) != 16 {
			return TerminalTheme{}, fmt.Errorf("Rex theme has no ANSI palette")
		}
		for i, entry := range theme.Source.ANSI {
			entry = color(fmt.Sprintf("terminalAnsi%d", i), entry)
			if !hexColor.MatchString(entry) {
				return TerminalTheme{}, fmt.Errorf("unsupported Rex palette color")
			}
			out.Palette = append(out.Palette, fmt.Sprintf("%d=%s", i, entry))
		}
		return out, nil
	}
	return TerminalTheme{}, fmt.Errorf("selected Rex theme %q is not in its bundled catalogue", id)
}

// CurrentTerminalTheme reads local Rex preferences, without changing settings.
// These macOS preference/catalogue formats are implementation details: callers
// fall back to Rex defaults if a future version changes them or the app is remote.
func CurrentTerminalTheme() (TerminalTheme, error) {
	if runtime.GOOS != "darwin" {
		return TerminalTheme{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Read only appearance keys. Exporting the whole preferences domain can
	// contain binary plist values unrelated to themes and not representable as JSON.
	prefs := make(map[string]any)
	for _, key := range []string{"Workspace.appearance", "Workspace.darkThemeID", "Workspace.lightThemeID", "Workspace.terminalUsesDisplayP3"} {
		value, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "com.superlogical.rex", key).Output()
		if err != nil {
			continue
		}
		text := strings.TrimSpace(string(value))
		if key == "Workspace.terminalUsesDisplayP3" {
			prefs[key] = text == "1" || strings.EqualFold(text, "true")
		} else {
			prefs[key] = text
		}
	}
	system, _ := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleInterfaceStyle").Output()
	var apps []string
	if cli, err := filepath.EvalSymlinks(New().Bin); err == nil {
		for dir := filepath.Dir(cli); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if strings.HasSuffix(dir, ".app") {
				apps = append(apps, dir)
				break
			}
		}
	}
	apps = append(apps, "/Applications/Rex Beta.app", "/Applications/Rex.app")
	for _, app := range apps {
		path := filepath.Join(app, "Contents/Resources/SuperUI_SuperDesignTokens.bundle/Contents/Resources/themes.json")
		catalog, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return selectedTheme(prefs, strings.EqualFold(strings.TrimSpace(string(system)), "dark"), catalog)
	}
	return TerminalTheme{}, fmt.Errorf("Rex theme catalogue was not found locally")
}
