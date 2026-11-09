package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"rex-review/internal/install"
	"rex-review/internal/review"
	"rex-review/internal/rex"
	"rex-review/internal/ui"
)

func run() error {
	session := flag.String("session", os.Getenv("REX_SESSION"), "originating Rex session")
	window := flag.String("window", "", "originating tab ID")
	block := flag.String("from", "", "originating agent block ID")
	server := flag.String("server", "", "optional explicit Rex server address")
	repository := flag.String("repo", "", "explicit Git project folder")
	flag.Parse()
	if *session == "" || *window == "" || *block == "" {
		return fmt.Errorf("open this plugin using Rex's rex_review action from an agent pane")
	}
	c := rex.New()
	c.Server = *server
	if err := c.InWindow(*session, *window, *block); err != nil {
		return err
	}
	p, err := c.Process(*session, *block)
	if err != nil {
		return err
	}
	if p.Agent() == "" {
		return fmt.Errorf("open Rex Review from a supported agent pane (Codex, Claude, Gemini, Aider, OpenCode, Cursor, Amp, Goose, or pi)")
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	base = filepath.Join(base, "rex-review")
	hint := review.RepositoryHintPath(base, *session, *block, p.PID)
	// Explicit paths win, then a choice for this agent, then its OS directory.
	// The agent's in-app /cd need not change its OS process directory.
	repo := ""
	if *repository != "" {
		repo, err = review.RepositoryPath(*repository)
	} else {
		repo, _ = review.RepositoryPath(review.ReadRepositoryHint(hint))
		if repo == "" && p.Cwd != "" {
			repo, err = review.RepositoryPath(p.Cwd)
		}
	}
	choose := repo == ""
	for {
		if choose {
			initial := repo
			if initial == "" {
				initial = *repository
			}
			if initial == "" {
				initial = p.Cwd
			}
			repo, err = ui.SelectRepository(initial)
			if err != nil {
				return err
			}
			if repo == "" {
				return nil
			}
			if err = review.SaveRepositoryHint(hint, repo); err != nil {
				return fmt.Errorf("save project choice: %w", err)
			}
		}
		path := review.DraftPath(filepath.Join(base, "drafts"), repo, *session, *block)
		d, err := review.ReadDraft(path, review.Draft{Repo: repo, Session: *session, Block: *block})
		if err != nil {
			return err
		}
		m := ui.New(c, p, *window, path, d)
		result, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
		if err != nil {
			return err
		}
		if !result.(ui.Model).ChooseRepository {
			return nil
		}
		choose = true
	}
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "install" || os.Args[1] == "uninstall") {
		if err := configure(os.Args[1], os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "Rex Review:", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "theme" {
		theme, err := rex.CurrentTerminalTheme()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			fmt.Println("{}")
			return
		}
		if err = json.NewEncoder(os.Stdout).Encode(theme); err != nil {
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Rex Review:", err, "\nPress Enter to close.")
		if tty, e := os.Open("/dev/tty"); e == nil {
			bufio.NewReader(tty).ReadString('\n')
			tty.Close()
		}
		os.Exit(1)
	}
}

func configure(action string, args []string) error {
	defaultPath, err := install.DefaultConfig()
	if err != nil {
		return err
	}
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	path := flags.String("config", defaultPath, "Rex init.lua path")
	server := flags.String("server", "", "optional Rex server address")
	if err = flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments: %v", flags.Args())
	}
	client := rex.New()
	client.Server = *server
	if action == "install" {
		executable, e := os.Executable()
		if e != nil {
			return e
		}
		if err = install.Install(*path, *server, executable, client); err != nil {
			return err
		}
	} else {
		if err = install.Uninstall(*path, client); err != nil {
			return err
		}
	}
	fmt.Printf("Rex Review %sed in %s\n", action, *path)
	if _, err = client.Run("", "config", "reload"); err != nil {
		fmt.Fprintln(os.Stderr, "Configuration saved. Reload Rex when it is available: rex config reload")
	}
	return nil
}
