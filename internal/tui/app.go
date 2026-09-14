// Package tui is climan's terminal user interface for graphical management
// of the tools declared in the manifest. Built on bubbletea.
package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.solved.gg/climan/internal/config"
	"go.solved.gg/climan/internal/installer"
	"go.solved.gg/climan/internal/registry"
)

// ---- styles ----

var (
	styleHeader   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Padding(0, 1)
	styleSubtitle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleCursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	styleOK       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	styleWarn     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleErr      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleKey      = lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Bold(true)
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleCat      = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleLog      = lipgloss.NewStyle().Foreground(lipgloss.Color("251"))
	styleBox      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("236")).Padding(0, 1)
)

// row is one tool's live status in the UI.
type row struct {
	tool *registry.Tool
	st   installer.Status
}

// logBuf is a thread-safe line buffer used as the installer's output sink.
type logBuf struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func newLogBuf(max int) *logBuf {
	return &logBuf{max: max}
}

func (l *logBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, ln := range strings.Split(string(p), "\n") {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		l.lines = append(l.lines, ln)
	}
	if len(l.lines) > l.max {
		l.lines = l.lines[len(l.lines)-l.max:]
	}
	return len(p), nil
}

func (l *logBuf) Snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.lines))
	copy(out, l.lines)
	return out
}

func (l *logBuf) Add(s string) {
	_, _ = l.Write([]byte(s))
}

// ---- messages ----

type doneMsg struct{ err error }
type tickMsg time.Time

// Model is the bubbletea state.
type Model struct {
	inst *installer.Installer
	cfg  *config.Config
	all  []*registry.Tool // every catalog tool (when showAll)
	mfst []*registry.Tool // tools declared in the manifest

	rows   []row // live statuses, in display order
	cursor int

	filter    textinput.Model
	filtering bool
	showAll   bool

	spinner spinner.Model
	logs    *logBuf
	busy    bool
	action  string

	confirmRemove string // tool name awaiting a second 'd' press
	err           error

	width, height int
}

// New builds the TUI model.
func New(inst *installer.Installer, cfg *config.Config) *Model {
	m := &Model{
		inst: inst,
		cfg:  cfg,
		logs: newLogBuf(500),
		spinner: spinner.New(
			spinner.WithSpinner(spinner.Dot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("212"))),
		),
		filter: textinput.New(),
	}
	m.filter.Prompt = "filter: "
	m.filter.Placeholder = "name or category"
	m.all = inst.Registry().All()
	m.mfst = make([]*registry.Tool, 0, len(cfg.Tools))
	for _, ref := range cfg.Tools {
		if t, ok := inst.Registry().Get(ref.Name); ok {
			m.mfst = append(m.mfst, t)
		}
	}
	if len(m.mfst) == 0 {
		m.mfst = m.all
		m.showAll = true
	}
	m.refresh()
	return m
}

// refresh re-runs status checks for every displayed tool.
func (m *Model) refresh() {
	tools := m.displayedTools()
	m.rows = m.rows[:0]
	for _, t := range tools {
		m.rows = append(m.rows, row{tool: t, st: m.inst.Status(t, m.cfg.Get(t.Name))})
	}
	if m.cursor >= len(m.rows) {
		m.cursor = 0
	}
}

// displayedTools returns tools per the showAll/filter state.
func (m *Model) displayedTools() []*registry.Tool {
	base := m.mfst
	if m.showAll {
		base = m.all
	}
	q := strings.ToLower(strings.TrimSpace(m.filter.Value()))
	if q == "" {
		return base
	}
	var out []*registry.Tool
	for _, t := range base {
		if strings.Contains(strings.ToLower(t.Name), q) ||
			strings.Contains(strings.ToLower(t.Category.Label()), q) ||
			strings.Contains(strings.ToLower(t.Description), q) {
			out = append(out, t)
		}
	}
	return out
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tickMsg(time.Now()) }))
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tickMsg:
		return m, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tickMsg(time.Now()) })

	case doneMsg:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
			m.logs.Add("✗ " + msg.err.Error())
		} else {
			m.err = nil
			m.logs.Add("✓ " + m.action + " complete")
		}
		m.action = ""
		m.refresh()
		return m, nil
	}
	return m, nil
}

// handleKey routes keyboard input.
func (m *Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Filter mode owns the keyboard.
	if m.filtering {
		switch key {
		case "esc", "enter":
			m.filtering = false
			m.filter.Blur()
			return m, nil
		case "ctrl+c", "q":
			return m, tea.Quit
		default:
			m.filter, _ = m.filter.Update(msg)
			m.cursor = 0
			m.refresh()
			return m, nil
		}
	}

	switch key {
	case "q", "ctrl+c", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		m.cursor = len(m.rows) - 1
	case "/":
		m.filtering = true
		m.filter.SetValue("")
		return m, m.filter.Focus()
	case "a":
		if m.busy {
			return m, nil
		}
		return m, m.installAll()
	case "U":
		if m.busy {
			return m, nil
		}
		return m, m.updateAll()
	case "i", "enter":
		if m.busy || len(m.rows) == 0 {
			return m, nil
		}
		return m, m.toggleAction(m.rows[m.cursor].tool)
	case "u":
		if m.busy || len(m.rows) == 0 {
			return m, nil
		}
		return m, m.updateOne(m.rows[m.cursor].tool)
	case "d":
		if m.busy || len(m.rows) == 0 {
			return m, nil
		}
		t := m.rows[m.cursor].tool
		if m.confirmRemove == t.Name {
			m.confirmRemove = ""
			return m, m.removeOne(t)
		}
		m.confirmRemove = t.Name
	case "r":
		if m.busy {
			return m, nil
		}
		m.refresh()
	case "t":
		if m.busy {
			return m, nil
		}
		m.showAll = !m.showAll
		m.confirmRemove = ""
		m.refresh()
	}
	m.confirmRemove = ""
	return m, nil
}

// ---- actions ----

func (m *Model) start(action string, fn func(ctx context.Context) error) tea.Cmd {
	m.busy = true
	m.action = action
	m.confirmRemove = ""
	m.logs.Add(fmt.Sprintf("── %s", action))
	done := make(chan error, 1)
	go func() {
		done <- fn(context.Background())
	}()
	return func() tea.Msg { return doneMsg{err: <-done} }
}

func (m *Model) installAll() tea.Cmd {
	return m.start("install all missing", func(ctx context.Context) error {
		for _, r := range m.rows {
			if r.st.State != installer.StateMissing {
				continue
			}
			if err := m.inst.Install(ctx, r.tool, m.cfg.Get(r.tool.Name)); err != nil {
				return fmt.Errorf("%s: %w", r.tool.Name, err)
			}
		}
		return nil
	})
}

func (m *Model) updateAll() tea.Cmd {
	return m.start("update all installed", func(ctx context.Context) error {
		for _, r := range m.rows {
			if r.st.State == installer.StateMissing {
				continue
			}
			if err := m.inst.Update(ctx, r.tool); err != nil {
				return fmt.Errorf("%s: %w", r.tool.Name, err)
			}
		}
		return nil
	})
}

func (m *Model) toggleAction(t *registry.Tool) tea.Cmd {
	st := m.inst.Status(t, m.cfg.Get(t.Name))
	if st.State == installer.StateMissing {
		return m.start("install "+t.Name, func(ctx context.Context) error {
			return m.inst.Install(ctx, t, m.cfg.Get(t.Name))
		})
	}
	return m.updateOne(t)
}

func (m *Model) updateOne(t *registry.Tool) tea.Cmd {
	return m.start("update "+t.Name, func(ctx context.Context) error {
		return m.inst.Update(ctx, t)
	})
}

func (m *Model) removeOne(t *registry.Tool) tea.Cmd {
	return m.start("remove "+t.Name, func(ctx context.Context) error {
		return m.inst.Remove(ctx, t)
	})
}

// ---- view ----

func (m *Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("loading…")
	}
	header := m.viewHeader()
	list := m.viewList()
	logs := m.viewLogs()
	footer := m.viewFooter()

	body := lipgloss.JoinVertical(lipgloss.Left, header, list, logs, footer)
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

func (m *Model) viewHeader() string {
	installed, missing := 0, 0
	for _, r := range m.rows {
		if r.st.State == installer.StateMissing {
			missing++
		} else {
			installed++
		}
	}
	var status string
	if m.busy {
		status = m.spinner.View() + " " + styleDim.Render(m.action+"…")
	} else if m.err != nil {
		status = styleErr.Render("last action failed — see log")
	} else {
		status = styleOK.Render("idle")
	}
	sub := styleSubtitle.Render(fmt.Sprintf("%d installed · %d missing · %d total", installed, missing, len(m.rows)))
	return styleHeader.Render("climan") + " " + sub + "    " + status
}

func (m *Model) viewList() string {
	var sb strings.Builder
	if m.filtering {
		sb.WriteString(m.filter.View() + "\n")
	}
	if len(m.rows) == 0 {
		return styleDim.Render("no tools match — press / to clear the filter, t to toggle catalog/manifest\n")
	}
	avail := m.height - 8
	if m.filtering {
		avail--
	}
	if avail < 1 {
		avail = 1
	}
	// Reserve space for the log and footer.
	logLines := 6
	avail -= logLines + 1
	if avail < 1 {
		avail = 1
	}
	start, end := 0, len(m.rows)
	if len(m.rows) > avail {
		// Keep the cursor visible.
		start = m.cursor - avail/2
		if start < 0 {
			start = 0
		}
		end = start + avail
		if end > len(m.rows) {
			end = len(m.rows)
			start = end - avail
		}
	}
	for i := start; i < end; i++ {
		sb.WriteString(m.renderRow(m.rows[i], i == m.cursor) + "\n")
	}
	return sb.String()
}

func (m *Model) renderRow(r row, selected bool) string {
	icon, color := "○", styleDim
	switch r.st.State {
	case installer.StateReady:
		icon, color = "●", styleOK
	case installer.StateOutdated:
		icon, color = "◉", styleWarn
	case installer.StateMissing:
		icon, color = "○", styleDim
	case installer.StateUnknown:
		icon, color = "?", styleErr
	}
	marker := "  "
	if selected {
		marker = styleCursor.Render("▸ ")
	}
	ver := r.st.Version
	if ver == "" {
		ver = "—"
	}
	name := styleTitle.Render(r.tool.Name)
	cat := styleCat.Render("[" + r.tool.Category.Label() + "]")
	state := color.Render(r.st.State.Label())
	if r.st.State == installer.StateMissing && r.st.InstallBy != "" {
		state = color.Render("not installed")
	}
	return marker + color.Render(icon) + " " + name + " " + styleDim.Render(ver) + " " + cat + " " + state
}

func (m *Model) viewLogs() string {
	lines := m.logs.Snapshot()
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	content := strings.Join(lines, "\n")
	return styleBox.Render(styleLog.Render(content))
}

func (m *Model) viewFooter() string {
	var b strings.Builder
	keys := [][2]string{
		{"i", "install"}, {"u", "update"}, {"d", "remove"}, {"a", "all missing"},
		{"U", "all installed"}, {"/", "filter"}, {"t", "catalog/manifest"}, {"r", "refresh"}, {"q", "quit"},
	}
	for _, k := range keys {
		b.WriteString(styleKey.Render(k[0]) + " " + styleDim.Render(k[1]) + "  ")
	}
	if m.confirmRemove != "" {
		b.WriteString(styleErr.Render("press d again to remove " + m.confirmRemove))
	}
	return styleDim.Render(strings.TrimRight(b.String(), " "))
}
