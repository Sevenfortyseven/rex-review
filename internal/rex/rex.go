package rex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Client struct {
	Bin    string
	Server string
}

func New() Client {
	bin, err := exec.LookPath("rex")
	if err != nil {
		bin = filepath.Join(os.Getenv("REX_BIN_DIR"), "rex")
	}
	if _, err = os.Stat(bin); err != nil {
		bin = "/Applications/Rex Beta.app/Contents/Helpers/Rex Server.rex/Contents/MacOS/rex"
	}
	return Client{Bin: bin}
}

func (c Client) Run(input string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	base := []string{"--autostart=false"}
	if c.Server != "" {
		base = append(base, "-S", c.Server)
	}
	cmd := exec.CommandContext(ctx, c.Bin, append(base, args...)...)
	cmd.WaitDelay = time.Second
	cmd.Stdin = strings.NewReader(input)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rex: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.Bytes(), nil
}

func (c Client) Call(method string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	b, err = c.Run(string(b), "api", "call", method, "-")
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

type Process struct {
	PID    int    `json:"pid"`
	Name   string `json:"name"`
	Argv0  string `json:"argv0"`
	Script string `json:"script"`
	Path   string `json:"path"`
	Cwd    string `json:"cwd"`
}

func (p Process) Agent() string {
	// Interpreter hosts expose script separately; never infer agents from pane titles.
	for _, value := range []string{p.Script, p.Argv0, p.Name} {
		name := strings.TrimPrefix(filepath.Base(value), "-")
		switch name {
		case "codex", "claude", "gemini", "aider", "opencode", "cursor-agent", "amp", "goose", "pi", "claude.js", "codex.js":
			return name
		}
	}
	return ""
}

func (c Client) Process(session, block string) (Process, error) {
	var out struct {
		Foreground *Process `json:"foreground"`
	}
	err := c.Call("block.method", map[string]any{"session_id": session, "block_id": block, "method": "process"}, &out)
	if err != nil {
		return Process{}, err
	}
	if out.Foreground == nil {
		return Process{}, fmt.Errorf("the source pane has no foreground process")
	}
	return *out.Foreground, nil
}

func (c Client) InWindow(session, window, block string) error {
	var out struct {
		Windows []struct {
			ID     string `json:"window_id"`
			Layers []struct {
				Blocks []struct {
					ID string `json:"block_id"`
				} `json:"blocks"`
			} `json:"layers"`
		} `json:"windows"`
	}
	if err := c.Call("session.view", map[string]string{"session_id": session}, &out); err != nil {
		return err
	}
	for _, w := range out.Windows {
		if w.ID == window {
			for _, l := range w.Layers {
				for _, b := range l.Blocks {
					if b.ID == block {
						return nil
					}
				}
			}
		}
	}
	return fmt.Errorf("the originating agent pane is no longer in this tab")
}

func (c Client) Validate(session, window, block string, expected Process) error {
	if err := c.InWindow(session, window, block); err != nil {
		return err
	}
	p, err := c.Process(session, block)
	if err != nil {
		return err
	}
	if p.Agent() == "" || p.PID != expected.PID || p.Agent() != expected.Agent() || p.Cwd != expected.Cwd {
		return fmt.Errorf("the originating agent changed; reopen the review from its pane")
	}
	var out struct {
		Records []struct {
			State string `json:"state"`
			Kind  string `json:"kind"`
		} `json:"records"`
	}
	if err = c.Call("block.method", map[string]any{"session_id": session, "block_id": block, "method": "program_status"}, &out); err != nil {
		return err
	}
	for _, r := range out.Records {
		if r.State == "blocked" || r.Kind == "auth" || r.Kind == "permission" || r.Kind == "question" {
			return fmt.Errorf("the agent is waiting on a prompt; resolve it before sending a review")
		}
		if r.State == "working" {
			return fmt.Errorf("the agent is working; wait until it finishes before sending")
		}
	}
	return nil
}

func (c Client) Paste(session, block, text string) error {
	var out any
	return c.Call("block.event", map[string]any{"session_id": session, "block_id": block, "event": map[string]any{"type": "paste", "payload": map[string]string{"text": text}}}, &out)
}

func (c Client) Submit(session, block string) error {
	_, err := c.Run("", "send-key", "-s", session, "-b", block, "enter")
	return err
}

func (c Client) Focus(session, block string) error {
	var out any
	return c.Call("session.focus_block", map[string]string{"session_id": session, "block_id": block}, &out)
}
