package rex

import (
	"encoding/json"
	"strings"
	"testing"
)

func themeCatalog(t *testing.T) []byte {
	t.Helper()
	var themes []map[string]any
	for _, id := range []string{"day", "night"} {
		ansi := make([]string, 16)
		for i := range ansi {
			ansi[i] = "#aabbcc"
		}
		themes = append(themes, map[string]any{"id": id, "source": map[string]any{"foreground": "#ffffff", "background": "#112233", "cursor": "#abcdef", "ansi": ansi}, "semantic": map[string]any{"terminalBackground": map[string]float64{"red": 0.5, "green": 0.25, "blue": 0.0}}})
	}
	b, err := json.Marshal(themes)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestThemeSelectionRespectsRexAppearance(t *testing.T) {
	prefs := map[string]any{"Workspace.appearance": "dark", "Workspace.darkThemeID": "night", "Workspace.lightThemeID": "day"}
	got, err := selectedTheme(prefs, false, themeCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Scheme != "dark" || got.Background != "#112233" || len(got.Palette) != 16 || got.Palette[6] != "6=#aabbcc" {
		t.Fatalf("wrong creation options: %+v", got)
	}
	prefs["Workspace.appearance"] = "auto"
	got, err = selectedTheme(prefs, false, themeCatalog(t))
	if err != nil || got.Scheme != "light" {
		t.Fatalf("did not follow system light mode: %+v %v", got, err)
	}
	got, err = selectedTheme(prefs, true, themeCatalog(t))
	if err != nil || got.Scheme != "dark" {
		t.Fatalf("did not follow system dark mode: %+v %v", got, err)
	}
}

func TestThemeUsesDisplayP3ChannelsWhenEnabled(t *testing.T) {
	prefs := map[string]any{"Workspace.appearance": "dark", "Workspace.darkThemeID": "night", "Workspace.terminalUsesDisplayP3": true}
	got, err := selectedTheme(prefs, false, themeCatalog(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.Background != "#804000" {
		t.Fatalf("P3 channels were not preserved: %s", got.Background)
	}
	if got.Foreground != "#ffffff" {
		t.Fatal("missing semantic channel should retain source color")
	}
}

func TestUnavailableThemeDoesNotGuessColors(t *testing.T) {
	prefs := map[string]any{"Workspace.appearance": "dark", "Workspace.darkThemeID": "missing-custom-theme"}
	if _, err := selectedTheme(prefs, false, themeCatalog(t)); err == nil {
		t.Fatal("guessed unavailable custom theme")
	}
	b := strings.ReplaceAll(string(themeCatalog(t)), "#ffffff", "invalid")
	prefs["Workspace.darkThemeID"] = "night"
	if _, err := selectedTheme(prefs, false, []byte(b)); err == nil {
		t.Fatal("accepted invalid terminal colors")
	}
}
