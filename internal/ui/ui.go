package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"rex-review/internal/review"
	"rex-review/internal/rex"
)

var (
	// ANSI colors 0–15 follow the terminal's theme. Extended palette colors and
	// explicit editor backgrounds would override the user's chosen appearance.
	muted    = lipgloss.NewStyle().Faint(true)
	accent   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	green    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	red      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	selected = lipgloss.NewStyle().Reverse(true)
)

type Model struct {
	ChooseRepository                      bool
	Client                                rex.Client
	Process                               rex.Process
	Window                                string
	Draft                                 review.Draft
	DraftPath                             string
	files                                 []review.File
	w, h                                  int
	file, row, offset, fileOffset         int
	anchor                                int
	horizontal                            int
	filesFocused, comments, confirm, busy bool
	comment                               int
	editor                                textarea.Model
	editing                               int
	original                              string
	newComment                            bool
	status                                string
	help                                  bool
	helpOffset                            int
}

type loaded struct {
	files []review.File
	err   error
}
type sent struct {
	draft   review.Draft
	err     error
	success bool
}

func New(c rex.Client, p rex.Process, window, path string, d review.Draft) Model {
	e := textarea.New()
	e.Placeholder = "What should the agent change?"
	e.CharLimit = 16000
	e.ShowLineNumbers = false
	e.FocusedStyle = textarea.Style{Placeholder: muted, Prompt: accent, LineNumber: muted, EndOfBuffer: muted}
	e.BlurredStyle = e.FocusedStyle
	return Model{Client: c, Process: p, Window: window, Draft: d, DraftPath: path, w: 100, h: 30, anchor: -1, editing: -1, editor: e, status: "Loading Git changes…"}
}

func (m Model) Init() tea.Cmd { return m.load() }
func (m Model) load() tea.Cmd {
	return func() tea.Msg { f, e := review.Load(m.Draft.Repo); return loaded{f, e} }
}
func (m *Model) save() bool {
	return m.persist(m.Draft)
}
func (m *Model) persist(d review.Draft) bool {
	if err := review.SaveDraft(m.DraftPath, d); err != nil {
		m.status = "Could not save draft: " + err.Error()
		return false
	}
	m.Draft = d
	return true
}
func (m Model) bodyHeight() int { return max(3, m.h-7) }
func (m Model) fileWidth() int  { return min(34, max(18, m.w/4)) }
func (m Model) current() review.File {
	if len(m.files) == 0 {
		return review.File{}
	}
	return m.files[m.file]
}
func (m *Model) clamp() {
	m.file = max(0, min(m.file, len(m.files)-1))
	m.row = max(0, min(m.row, len(m.current().Lines)-1))
	n := m.bodyHeight()
	if m.row < m.offset {
		m.offset = m.row
	}
	if m.row >= m.offset+n {
		m.offset = m.row - n + 1
	}
	m.offset = max(0, m.offset)
	if m.file < m.fileOffset {
		m.fileOffset = m.file
	}
	if m.file >= m.fileOffset+n {
		m.fileOffset = m.file - n + 1
	}
	m.comment = max(0, min(m.comment, len(m.Draft.Comments)-1))
}

func (m *Model) beginEdit(index int) {
	m.editing = index
	m.original = m.Draft.Comments[index].Text
	m.editor.SetValue(m.original)
	m.editor.SetWidth(max(10, m.w-8))
	m.editor.SetHeight(max(3, min(12, m.h-11)))
	m.editor.Focus()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w = msg.Width
		m.h = msg.Height
		m.clamp()
		if m.editing >= 0 {
			m.editor.SetWidth(max(10, m.w-8))
			m.editor.SetHeight(max(3, min(12, m.h-11)))
		}
		return m, nil
	case loaded:
		m.busy = false
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		old := m.current().ID()
		m.files = msg.files
		for i, f := range m.files {
			if f.ID() == old {
				m.file = i
				break
			}
		}
		m.anchor = -1
		m.clamp()
		m.status = fmt.Sprintf("%d file entries · %d saved comments", len(m.files), len(m.Draft.Comments))
		return m, nil
	case sent:
		m.busy = false
		m.confirm = false
		m.Draft = msg.draft
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		if msg.success {
			m.Client.Focus(m.Draft.Session, m.Draft.Block)
			return m, tea.Quit
		}
	}
	if m.busy {
		return m, nil
	}
	if m.help {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "?", "esc", "q":
				m.help = false
			case "down", "j":
				m.scrollHelp(1)
			case "up", "k":
				m.scrollHelp(-1)
			case "pgdown", "ctrl+d":
				m.scrollHelp(m.h - 5)
			case "pgup", "ctrl+u":
				m.scrollHelp(-(m.h - 5))
			case "ctrl+c":
				m.save()
				return m, tea.Quit
			}
		}
		if mouse, ok := msg.(tea.MouseMsg); ok {
			switch mouse.Button {
			case tea.MouseButtonWheelDown:
				m.scrollHelp(3)
			case tea.MouseButtonWheelUp:
				m.scrollHelp(-3)
			}
		}
		return m, nil
	}
	if m.editing >= 0 {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "ctrl+s":
				if strings.TrimSpace(m.editor.Value()) == "" {
					m.status = "Write a comment, or Esc to cancel"
					return m, nil
				}
				m.Draft.Comments[m.editing].Text = m.editor.Value()
				if !m.save() {
					return m, nil
				}
				m.editing = -1
				m.editor.Blur()
				m.newComment = false
				m.anchor = -1
				m.status = "Comment saved · s to send review"
				return m, nil
			case "esc":
				candidate := m.Draft
				candidate.Comments = append([]review.Comment(nil), m.Draft.Comments...)
				if m.newComment {
					candidate.Comments = append(candidate.Comments[:m.editing], candidate.Comments[m.editing+1:]...)
				} else {
					candidate.Comments[m.editing].Text = m.original
				}
				if !m.persist(candidate) {
					return m, nil
				}
				m.editing = -1
				m.newComment = false
				m.editor.Blur()
				m.status = "Edit cancelled"
				m.clamp()
				return m, nil
			case "ctrl+c":
				m.save()
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		m.Draft.Comments[m.editing].Text = m.editor.Value()
		m.save()
		return m, cmd
	}
	if m.confirm {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "y", "enter":
				m.busy = true
				m.status = "Checking changes and destination…"
				return m, m.send()
			case "n", "esc":
				m.confirm = false
				m.status = "Review kept as draft"
			}
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "?":
			m.help = true
			m.helpOffset = 0
		case "p":
			if m.save() {
				m.ChooseRepository = true
				return m, tea.Quit
			}
		case "q", "esc", "ctrl+c":
			if m.save() {
				return m, tea.Quit
			}
			return m, nil
		case "tab":
			m.filesFocused = !m.filesFocused
			m.anchor = -1
		case "r":
			m.busy = true
			m.status = "Refreshing…"
			return m, m.load()
		case "s":
			if m.Draft.Pasted {
				m.status = "Review may already be in the agent input. Inspect it there; x in Comments clears the delivery marker."
				return m, nil
			}
			if len(m.Draft.Comments) == 0 {
				m.status = "Add a line comment with c first"
				return m, nil
			}
			m.confirm = true
			return m, nil
		case "C":
			m.comments = !m.comments
			m.anchor = -1
		case "j", "down":
			if m.comments {
				m.comment++
			} else if m.filesFocused {
				m.file++
				m.row = 0
				m.offset = 0
				m.anchor = -1
			} else {
				m.row++
			}
		case "k", "up":
			if m.comments {
				m.comment--
			} else if m.filesFocused {
				m.file--
				m.row = 0
				m.offset = 0
				m.anchor = -1
			} else {
				m.row--
			}
		case "pgdown", "ctrl+d":
			m.row += max(1, m.bodyHeight()-2)
		case "pgup", "ctrl+u":
			m.row -= max(1, m.bodyHeight()-2)
		case "g", "home":
			m.row = 0
		case "G", "end":
			m.row = len(m.current().Lines) - 1
		case "h", "left":
			m.horizontal = max(0, m.horizontal-8)
		case "l", "right":
			m.horizontal += 8
		case "[":
			m.file--
			m.row = 0
			m.offset = 0
			m.anchor = -1
		case "]":
			m.file++
			m.row = 0
			m.offset = 0
			m.anchor = -1
		case "v":
			if m.anchor < 0 {
				m.anchor = m.row
			} else {
				m.anchor = -1
			}
			m.filesFocused = false
			m.comments = false
		case "c":
			if len(m.files) == 0 {
				return m, nil
			}
			a := m.row
			if m.anchor >= 0 {
				a = m.anchor
			}
			c, err := review.NewComment(m.current(), a, m.row, "draft")
			if err != nil {
				m.status = err.Error()
				return m, nil
			}
			c.Text = ""
			m.Draft.Comments = append(m.Draft.Comments, c)
			m.newComment = true
			m.beginEdit(len(m.Draft.Comments) - 1)
			m.save()
			return m, textarea.Blink
		case "e":
			if m.comments && len(m.Draft.Comments) > 0 {
				m.newComment = false
				m.beginEdit(m.comment)
				return m, textarea.Blink
			}
		case "d":
			if m.comments && len(m.Draft.Comments) > 0 {
				candidate := m.Draft
				candidate.Comments = append([]review.Comment(nil), m.Draft.Comments...)
				candidate.Comments = append(candidate.Comments[:m.comment], candidate.Comments[m.comment+1:]...)
				if m.persist(candidate) {
					m.status = "Comment deleted"
				}
			}
		case "x":
			if m.comments && m.Draft.Pasted {
				candidate := m.Draft
				candidate.Pasted = false
				if m.persist(candidate) {
					m.status = "Delivery marker cleared. Check the agent input before resending."
				}
			}
		}
		m.clamp()
		return m, nil
	}
	if mouse, ok := msg.(tea.MouseMsg); ok && !m.comments {
		switch mouse.Button {
		case tea.MouseButtonWheelUp:
			m.row -= 3
		case tea.MouseButtonWheelDown:
			m.row += 3
		case tea.MouseButtonLeft:
			if mouse.Y >= 4 && mouse.Y < 4+m.bodyHeight() {
				if mouse.X < m.fileWidth() {
					m.filesFocused = true
					m.file = m.fileOffset + mouse.Y - 4
					m.row = 0
					m.offset = 0
					m.anchor = -1
				} else {
					m.filesFocused = false
					m.row = m.offset + mouse.Y - 4
					if mouse.Action == tea.MouseActionPress {
						m.anchor = m.row
					}
				}
			}
		}
		m.clamp()
	}
	return m, nil
}

func (m Model) send() tea.Cmd {
	return func() tea.Msg {
		d := m.Draft
		fail := func(e error) tea.Msg { return sent{draft: d, err: e} }
		if d.Pasted {
			return fail(fmt.Errorf("review may already be pasted; inspect the agent pane before retrying"))
		}
		for _, c := range d.Comments {
			if strings.TrimSpace(c.Text) == "" {
				return fail(fmt.Errorf("finish or delete the empty comment on %s", c.Path))
			}
			f, err := review.LoadFile(d.Repo, c.Path, c.Scope)
			if err != nil {
				return fail(err)
			}
			if f.Hash != c.Hash {
				return fail(fmt.Errorf("%s changed since you commented; keep the draft, refresh, and re-add its comments", c.Path))
			}
		}
		if err := m.Client.Validate(d.Session, m.Window, d.Block, m.Process); err != nil {
			return fail(err)
		}
		// Persist uncertainty before touching agent input. A timeout/crash must not cause an automatic duplicate.
		d.Pasted = true
		if err := review.SaveDraft(m.DraftPath, d); err != nil {
			return fail(err)
		}
		if err := m.Client.Paste(d.Session, d.Block, d.Message()); err != nil {
			return fail(fmt.Errorf("paste did not confirm delivery; inspect agent input before retrying: %w", err))
		}
		if err := m.Client.Submit(d.Session, d.Block); err != nil {
			return fail(fmt.Errorf("review pasted, but Enter failed; submit it in the agent pane: %w", err))
		}
		cleared := d
		cleared.Comments = nil
		cleared.Pasted = false
		if err := review.SaveDraft(m.DraftPath, cleared); err != nil {
			return fail(fmt.Errorf("review sent, but draft cleanup failed; do not resend: %w", err))
		}
		return sent{draft: cleared, success: true}
	}
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}
func fit(s string, w int) string {
	s = ansi.Truncate(s, max(0, w), "…")
	return s + strings.Repeat(" ", max(0, w-lipgloss.Width(s)))
}

func (m Model) helpLines() []string {
	body := "Keys\n\nTab         Switch files / diff\n↑↓ or j/k   Move through files or lines\n[ / ]       Previous / next file\n←→ or h/l   Scroll long lines\nv           Start or finish line selection\nc           Comment on the selected lines\nCtrl+S      Save comment (Enter inserts newline)\nShift+C     Show saved comments\ne / d       Edit / delete selected comment\ns           Send review, then y confirms\nr           Refresh Git changes\np           Choose another project folder\nPgUp/PgDn   Scroll diff\ng / G       First / last diff line\nq / Esc     Close; keep drafts\n\nMouse: click files, drag diff lines, wheel to scroll."
	return strings.Split(lipgloss.NewStyle().Width(max(1, m.w)).Render(body), "\n")
}
func (m *Model) scrollHelp(delta int) {
	m.helpOffset = max(0, min(m.helpOffset+delta, len(m.helpLines())-max(1, m.h-5)))
}

func (m Model) View() string {
	if m.w < 60 || m.h < 16 {
		return "Rex Review needs at least 60 columns × 16 rows. Resize the pane, or press q to close."
	}
	title := accent.Bold(true).Render(" Rex Review") + muted.Render("  "+clean(filepath.Base(m.Draft.Repo)))
	sub := muted.Render(" " + clean(m.Process.Agent()) + " · " + fmt.Sprint(len(m.Draft.Comments)) + " comments · drafts saved locally")
	footer := muted.Render(" Tab files · ↑↓ move · v range · c comment · C drafts · s send · ? help · q close")
	status := m.status
	if m.confirm {
		status = fmt.Sprintf("Send %d comments to %s in the originating pane?  y / Enter send · Esc cancel", len(m.Draft.Comments), m.Process.Agent())
	}
	if m.help {
		lines := m.helpLines()
		height := max(1, m.h-5)
		start := max(0, min(m.helpOffset, len(lines)-height))
		body := strings.Join(lines[start:min(len(lines), start+height)], "\n")
		return fit(title, m.w) + "\n" + fit(sub, m.w) + "\n\n" + lipgloss.NewStyle().Height(height).Render(body) + "\n" + fit(muted.Render(" ↑↓ / PgUp / PgDn scroll help · ? / Esc close help"), m.w)
	}
	if m.editing >= 0 {
		c := m.Draft.Comments[m.editing]
		body := accent.Render(" "+clean(c.Location())) + "\n\n" + m.editor.View() + "\n\n" + muted.Render(" Ctrl+S save · Enter newline · Esc cancel · edits saved as draft")
		return title + "\n" + sub + "\n\n" + lipgloss.NewStyle().Height(m.bodyHeight()+1).Render(body) + "\n" + fit(clean(status), m.w)
	}
	if m.comments {
		var lines []string
		for i, c := range m.Draft.Comments {
			mark := "  "
			if i == m.comment {
				mark = "› "
			}
			lines = append(lines, accent.Render(mark+clean(c.Location())))
			for _, l := range strings.Split(lipgloss.NewStyle().Width(max(10, m.w-4)).Render(clean(c.Text)), "\n") {
				lines = append(lines, "  "+l)
			}
			lines = append(lines, "")
		}
		if len(lines) == 0 {
			lines = []string{" No comments yet. Press C to return to the diff, then c on a code line."}
		}
		// Scroll to the selected comment even when earlier comments wrap over several rows.
		start := 0
		for i := 0; i < m.comment; i++ {
			start += 2 + lipgloss.Height(lipgloss.NewStyle().Width(max(10, m.w-4)).Render(clean(m.Draft.Comments[i].Text)))
		}
		start = max(0, min(start, len(lines)-m.bodyHeight()))
		body := strings.Join(lines[start:min(len(lines), start+m.bodyHeight())], "\n")
		return title + "\n" + sub + "\n\n" + lipgloss.NewStyle().Height(m.bodyHeight()+1).Render(body) + "\n" + fit(clean(status), m.w) + "\n" + muted.Render(" ↑↓ comment · e edit · d delete · C diff · s send · x clear delivery marker · q close")
	}
	fw := m.fileWidth()
	dw := m.w - fw - 2
	left := " FILES"
	right := " DIFF · " + clean(m.current().Path)
	if m.filesFocused {
		left = accent.Render(left)
	} else {
		right = accent.Render(right)
	}
	var rows []string
	for i := 0; i < m.bodyHeight(); i++ {
		li := m.fileOffset + i
		ri := m.offset + i
		l, r := "", ""
		if li < len(m.files) {
			f := m.files[li]
			scope := map[string]string{"staged": "S", "unstaged": "M", "untracked": "?"}[f.Scope]
			l = " " + scope + " " + clean(f.Path)
			if li == m.file {
				l = selected.Render(fit(l, fw))
			}
		}
		if ri < len(m.current().Lines) {
			line := m.current().Lines[ri]
			old, cur := "", ""
			if line.Old > 0 {
				old = fmt.Sprint(line.Old)
			}
			if line.New > 0 {
				cur = fmt.Sprint(line.New)
			}
			r = fmt.Sprintf("%4s %4s │ %s", old, cur, ansi.Cut(clean(line.Text), m.horizontal, m.horizontal+max(1, dw-12)))
			switch line.Kind {
			case '+':
				r = green.Render(r)
			case '-':
				r = red.Render(r)
			case '@':
				r = accent.Render(r)
			case 'h':
				r = muted.Render(r)
			}
			a, b := m.row, m.row
			if m.anchor >= 0 {
				a = min(m.anchor, m.row)
				b = max(m.anchor, m.row)
			}
			if ri >= a && ri <= b {
				r = selected.Render(fit(r, dw))
			}
		}
		if len(m.files) == 0 && i == 1 {
			r = "No uncommitted changes. Press r to refresh."
		}
		rows = append(rows, fit(l, fw)+" │"+fit(r, dw))
	}
	return fit(title, m.w) + "\n" + fit(sub, m.w) + "\n\n" + fit(left, fw) + " │" + fit(right, dw) + "\n" + strings.Join(rows, "\n") + "\n" + fit(clean(status), m.w) + "\n" + fit(footer, m.w)
}
