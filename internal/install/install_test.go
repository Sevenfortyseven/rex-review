package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rex-review/internal/rex"
)

func fakeRex(t *testing.T) rex.Client {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nif [ \"$REX_REVIEW_FAIL_CHECK\" = 1 ]; then exit 1; fi\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return rex.Client{Bin: path}
}
func sourceBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(path, []byte("fixture executable"), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestStandaloneInstallUpdateAndUninstallPreserveConfig(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "init.lua")
	original := "-- My settings\nrex.bind(\"cmd+shift+r\", \"my_action\")\n"
	if err := os.WriteFile(config, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	client := fakeRex(t)
	binary := sourceBinary(t)
	for i := 0; i < 2; i++ {
		if err := Install(config, "unix:///test.sock", binary, client); err != nil {
			t.Fatal(err)
		}
	}
	text, _ := os.ReadFile(config)
	if strings.Count(string(text), begin) != 1 || !strings.HasSuffix(string(text), original) {
		t.Fatal("install changed existing config or duplicated registration")
	}
	if !strings.Contains(string(text), "unix:///test.sock") {
		t.Fatal("server override missing")
	}
	backup, _ := os.ReadFile(config + ".rex-review.bak")
	if string(backup) != original {
		t.Fatal("initial backup was replaced")
	}
	path := filepath.Join(root, "rex-review/rex-review")
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0755 {
		t.Fatal("binary is not installed as executable")
	}
	if err = Uninstall(config, client); err != nil {
		t.Fatal(err)
	}
	text, _ = os.ReadFile(config)
	if string(text) != original {
		t.Fatal("uninstall did not preserve original config")
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("installed binary remains")
	}
	if _, err = os.Stat(config + ".rex-review.bak"); err != nil {
		t.Fatal("uninstall removed backup")
	}
}
func TestRejectedConfigRollsBackInstalledAssets(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "init.lua")
	original := "-- original\n"
	if err := os.WriteFile(config, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "rex-review")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rex-review", "config.lua"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("REX_REVIEW_FAIL_CHECK", "1")
	if err := Install(config, "", sourceBinary(t), fakeRex(t)); err == nil {
		t.Fatal("installer accepted failed config check")
	}
	text, _ := os.ReadFile(config)
	if string(text) != original {
		t.Fatal("rejected config replaced init.lua")
	}
	for _, name := range []string{"rex-review", "config.lua"} {
		text, _ = os.ReadFile(filepath.Join(dir, name))
		if string(text) != "old "+name {
			t.Fatal("failed install did not restore assets")
		}
	}
}
func TestConfigSymlinkIsPreserved(t *testing.T) {
	root := t.TempDir()
	actual := filepath.Join(root, "dotfiles.lua")
	config := filepath.Join(root, "init.lua")
	if err := os.WriteFile(actual, []byte("-- dotfiles\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, config); err != nil {
		t.Fatal(err)
	}
	client := fakeRex(t)
	if err := Install(config, "", sourceBinary(t), client); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(config)
	if err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("install replaced config symlink")
	}
	if err := Uninstall(config, client); err != nil {
		t.Fatal(err)
	}
	text, _ := os.ReadFile(actual)
	if string(text) != "-- dotfiles\n" {
		t.Fatal("symlink target was not restored")
	}
}
func TestMalformedRegistrationDoesNotChangeConfig(t *testing.T) {
	root := t.TempDir()
	config := filepath.Join(root, "init.lua")
	original := begin + "\n-- missing end\n"
	if err := os.WriteFile(config, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Install(config, "", sourceBinary(t), fakeRex(t)); err == nil {
		t.Fatal("accepted incomplete registration")
	}
	text, _ := os.ReadFile(config)
	if string(text) != original {
		t.Fatal("malformed registration was overwritten")
	}
}
