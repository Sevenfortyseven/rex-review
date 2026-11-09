package rex

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// Child copies of this test executable implement only the Rex API used by delivery.
func TestMain(m *testing.M) {
	if os.Getenv("REX_REVIEW_FAKE") == "1" {
		input, _ := io.ReadAll(os.Stdin)
		args := os.Args[1:]
		if len(args) > 3 && args[1] == "api" && args[2] == "call" {
			method := args[3]
			switch method {
			case "session.view":
				io.WriteString(os.Stdout, `{"windows":[{"window_id":"w","layers":[{"blocks":[{"block_id":"b"}]}]}]}`)
			case "block.method":
				var req map[string]any
				json.Unmarshal(input, &req)
				if req["method"] == "process" {
					io.WriteString(os.Stdout, `{"foreground":{"pid":42,"name":"codex","cwd":"/repo"}}`)
				} else {
					io.WriteString(os.Stdout, `{"records":`+os.Getenv("REX_REVIEW_STATUS")+`}`)
				}
			case "block.event":
				os.WriteFile(os.Getenv("REX_REVIEW_PASTE"), input, 0600)
				io.WriteString(os.Stdout, `{}`)
			}
			os.Exit(0)
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func fake(t *testing.T) Client {
	t.Helper()
	t.Setenv("REX_REVIEW_FAKE", "1")
	t.Setenv("REX_REVIEW_STATUS", "[]")
	return Client{Bin: os.Args[0]}
}
func TestValidatePinnedAgentAndTab(t *testing.T) {
	c := fake(t)
	p := Process{PID: 42, Name: "codex", Cwd: "/repo"}
	if err := c.Validate("s", "w", "b", p); err != nil {
		t.Fatal(err)
	}
	p.PID = 43
	if err := c.Validate("s", "w", "b", p); err == nil {
		t.Fatal("accepted replacement agent")
	}
	p.PID = 42
	if err := c.Validate("s", "other", "b", p); err == nil {
		t.Fatal("accepted another tab")
	}
	for _, status := range []string{`[{"state":"blocked","kind":"permission"}]`, `[{"state":"working"}]`, `[{"kind":"auth"}]`} {
		t.Setenv("REX_REVIEW_STATUS", status)
		if err := c.Validate("s", "w", "b", p); err == nil {
			t.Fatalf("accepted %s", status)
		}
	}
}
func TestPasteUsesOneLiteralEvent(t *testing.T) {
	c := fake(t)
	path := t.TempDir() + "/paste.json"
	t.Setenv("REX_REVIEW_PASTE", path)
	text := "file.go:2\n$(no shell) `literal`\nUnicode: ქართული"
	if err := c.Paste("s", "b", text); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var req struct {
		Block string `json:"block_id"`
		Event struct {
			Type    string `json:"type"`
			Payload struct {
				Text string `json:"text"`
			} `json:"payload"`
		} `json:"event"`
	}
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if req.Block != "b" || req.Event.Type != "paste" || req.Event.Payload.Text != text {
		t.Fatalf("paste corrupted: %s", b)
	}
}
func TestAgentIdentification(t *testing.T) {
	if (Process{Name: "node", Script: "/some/claude.js"}).Agent() != "claude.js" {
		t.Fatal("script host not detected")
	}
	if (Process{Name: "zsh", Argv0: "-zsh"}).Agent() != "" {
		t.Fatal("shell mistaken for agent")
	}
	if !strings.Contains((Process{Name: "codex"}).Agent(), "codex") {
		t.Fatal("codex missing")
	}
}
