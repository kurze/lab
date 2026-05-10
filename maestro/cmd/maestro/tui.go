package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"maestro/internal/fsm"
	"maestro/internal/planner"
	"maestro/internal/task"
)

// ---------------------------------------------------------------------------
// Tokyo Night OLED palette
// ---------------------------------------------------------------------------

var (
	colBg      = lipgloss.Color("#000000")
	colSurface = lipgloss.Color("#0d0e17")
	colBorder  = lipgloss.Color("#1a1b26")
	colText    = lipgloss.Color("#c0caf5")
	colMuted   = lipgloss.Color("#565f89")

	colGreen  = lipgloss.Color("#9ece6a")
	colYellow = lipgloss.Color("#e0af68")
	colCyan   = lipgloss.Color("#7dcfff")
	colBlue   = lipgloss.Color("#7aa2f7")
	colPurple = lipgloss.Color("#bb9af7")
	colOrange = lipgloss.Color("#ff9e64")
	colRed    = lipgloss.Color("#f7768e")
)

// State badge tint colors.
type badgeColors struct{ bg, fg lipgloss.Color }

var stateBadge = map[fsm.State]badgeColors{
	fsm.Grill:       {lipgloss.Color("#1a1530"), colPurple},
	fsm.Plan:        {lipgloss.Color("#1a1a0d"), colYellow},
	fsm.Code:        {lipgloss.Color("#0d1a1a"), colCyan},
	fsm.AIReview:    {lipgloss.Color("#121e0d"), colGreen},
	fsm.AIFix:       {lipgloss.Color("#121e0d"), colGreen},
	fsm.LocalReview: {lipgloss.Color("#1e140d"), colOrange},
	fsm.Push:        {lipgloss.Color("#0d1525"), colBlue},
	fsm.Abandoned:   {lipgloss.Color("#1a0d0d"), colRed},
}

// ---------------------------------------------------------------------------
// Layout modes
// ---------------------------------------------------------------------------

type layoutMode int

const (
	layoutThreePane layoutMode = iota
	layoutTwoPane
	layoutSinglePane
)

func layoutFor(width int) layoutMode {
	if width >= 180 {
		return layoutThreePane
	}
	if width >= 100 {
		return layoutTwoPane
	}
	return layoutSinglePane
}

// ---------------------------------------------------------------------------
// Bubble Tea model
// ---------------------------------------------------------------------------

// TUIAction signals what the caller should do after the TUI exits.
type TUIAction int

const (
	TUIQuit      TUIAction = iota
	TUINewTask             // user pressed 'n' to create a new task
	TUIApprove             // user pressed 'a' to approve a PLAN task
	TUIReplan              // user pressed 'r' to replan a PLAN task
	TUIRework              // user pressed 'r' to rework a LOCAL_REVIEW task
	TUIPush                // user pressed 'p' to push a LOCAL_REVIEW task
	TUIResume              // user pressed 'R' to resume a stuck task
)

type tuiModel struct {
	tasks     []*task.Task
	store     *task.Store
	cursor    int
	width     int
	height    int
	layout    layoutMode
	graph     *planner.Graph // plan for selected task
	singleTab int           // 0=list, 1=dag, 2=detail (single-pane mode)
	action     TUIAction
	selectedID string // task ID for approve/replan/push/rework
	inputMode  bool   // true when typing a new task title
	inputBuf   string // title being typed
}

func runTUI(tasks []*task.Task, store *task.Store) (TUIAction, string, error) {
	m := tuiModel{
		tasks:  tasks,
		store:  store,
		width:  120,
		height: 40,
		layout: layoutTwoPane,
	}
	if len(tasks) > 0 {
		m.loadGraphForSelected()
	}
	p := tea.NewProgram(m, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return TUIQuit, "", err
	}
	final := result.(tuiModel)
	out := final.inputBuf
	if final.selectedID != "" {
		out = final.selectedID
	}
	return final.action, out, nil
}

func (m tuiModel) Init() tea.Cmd {
	return nil
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout = layoutFor(m.width)
		return m, nil

	case tea.KeyMsg:
		if m.inputMode {
			switch msg.String() {
			case "enter":
				if m.inputBuf != "" {
					m.action = TUINewTask
					return m, tea.Quit
				}
			case "esc":
				m.inputMode = false
				m.inputBuf = ""
			case "backspace":
				if len(m.inputBuf) > 0 {
					m.inputBuf = m.inputBuf[:len(m.inputBuf)-1]
				}
			default:
				if len(msg.String()) == 1 || msg.String() == " " {
					m.inputBuf += msg.String()
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "n":
			m.inputMode = true
			m.inputBuf = ""
		case "a":
			if m.cursor < len(m.tasks) && m.tasks[m.cursor].State == fsm.Plan {
				m.action = TUIApprove
				m.selectedID = m.tasks[m.cursor].ID
				return m, tea.Quit
			}
		case "r":
			if m.cursor < len(m.tasks) {
				switch m.tasks[m.cursor].State {
				case fsm.Plan:
					m.action = TUIReplan
					m.selectedID = m.tasks[m.cursor].ID
					return m, tea.Quit
				case fsm.LocalReview:
					m.action = TUIRework
					m.selectedID = m.tasks[m.cursor].ID
					return m, tea.Quit
				}
			}
		case "p":
			if m.cursor < len(m.tasks) && m.tasks[m.cursor].State == fsm.LocalReview {
				m.action = TUIPush
				m.selectedID = m.tasks[m.cursor].ID
				return m, tea.Quit
			}
		case "R":
			if m.cursor < len(m.tasks) {
				s := m.tasks[m.cursor].State
				if s == fsm.AIReview || s == fsm.AIFix || s == fsm.Code {
					m.action = TUIResume
					m.selectedID = m.tasks[m.cursor].ID
					return m, tea.Quit
				}
			}
		case "up", "k":
			if m.layout == layoutSinglePane && m.singleTab != 0 {
				break
			}
			if m.cursor > 0 {
				m.cursor--
				m.loadGraphForSelected()
			}
		case "down", "j":
			if m.layout == layoutSinglePane && m.singleTab != 0 {
				break
			}
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
				m.loadGraphForSelected()
			}
		case "tab":
			if m.layout == layoutSinglePane {
				m.singleTab = (m.singleTab + 1) % 3
			}
		case "shift+tab":
			if m.layout == layoutSinglePane {
				m.singleTab = (m.singleTab + 2) % 3
			}
		}
	}
	return m, nil
}

func (m *tuiModel) loadGraphForSelected() {
	m.graph = nil
	if m.cursor >= len(m.tasks) {
		return
	}
	t := m.tasks[m.cursor]
	// Try loading plan.json from the task directory.
	maestroDir := m.store.Root()
	planPath := filepath.Join(maestroDir, t.ID, "plan.json")
	data, err := os.ReadFile(planPath)
	if err != nil {
		return
	}
	var g planner.Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return
	}
	m.graph = &g
}

func (m tuiModel) View() string {
	if len(m.tasks) == 0 {
		return m.renderEmpty()
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	var body string
	switch m.layout {
	case layoutThreePane:
		body = m.renderThreePane(bodyHeight)
	case layoutTwoPane:
		body = m.renderTwoPane(bodyHeight)
	case layoutSinglePane:
		body = m.renderSinglePane(bodyHeight)
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// ---------------------------------------------------------------------------
// Header
// ---------------------------------------------------------------------------

func (m tuiModel) renderHeader() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(colPurple).
		Render("maestro")

	info := lipgloss.NewStyle().
		Foreground(colMuted).
		Render(fmt.Sprintf("%d tasks", len(m.tasks)))

	gap := m.width - lipgloss.Width(title) - lipgloss.Width(info)
	if gap < 1 {
		gap = 1
	}

	return lipgloss.NewStyle().
		Background(colBg).
		Width(m.width).
		Render(title + strings.Repeat(" ", gap) + info)
}

// ---------------------------------------------------------------------------
// Footer
// ---------------------------------------------------------------------------

func (m tuiModel) renderFooter() string {
	if m.inputMode {
		return m.renderInputBar()
	}

	keys := []struct{ key, label string }{
		{"↑↓", "nav"},
		{"n", "new"},
		{"q", "quit"},
	}

	if m.layout == layoutSinglePane {
		keys = append(keys, struct{ key, label string }{"tab", "switch"})
	}

	// Add context-sensitive keys based on selected task state.
	if m.cursor < len(m.tasks) {
		t := m.tasks[m.cursor]
		switch t.State {
		case fsm.Plan:
			keys = append(keys,
				struct{ key, label string }{"a", "approve"},
				struct{ key, label string }{"r", "replan"},
			)
		case fsm.LocalReview:
			keys = append(keys,
				struct{ key, label string }{"p", "push"},
				struct{ key, label string }{"r", "rework"},
			)
		case fsm.AIReview, fsm.AIFix:
			keys = append(keys,
				struct{ key, label string }{"R", "resume"},
			)
		}
	}

	pillStyle := lipgloss.NewStyle().
		Background(colSurface).
		Foreground(colText).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(colMuted)

	var parts []string
	for _, k := range keys {
		pill := pillStyle.Render(k.key) + " " + labelStyle.Render(k.label)
		parts = append(parts, pill)
	}

	return lipgloss.NewStyle().
		Background(colBg).
		Width(m.width).
		Render(strings.Join(parts, "  "))
}

func (m tuiModel) renderInputBar() string {
	label := lipgloss.NewStyle().
		Foreground(colPurple).
		Bold(true).
		Render("new task: ")

	cursor := lipgloss.NewStyle().
		Background(colText).
		Foreground(colBg).
		Render(" ")

	input := lipgloss.NewStyle().
		Foreground(colText).
		Render(m.inputBuf)

	hint := lipgloss.NewStyle().
		Foreground(colMuted).
		Render("  enter confirm · esc cancel")

	return lipgloss.NewStyle().
		Background(colSurface).
		Width(m.width).
		Padding(0, 1).
		Render(label + input + cursor + hint)
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

func (m tuiModel) renderEmpty() string {
	msg := lipgloss.NewStyle().
		Foreground(colMuted).
		Padding(2, 4).
		Render("No tasks found.\n\nRun: maestro new \"<title>\" to start.")

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg,
		lipgloss.WithWhitespaceBackground(colBg))
}

// ---------------------------------------------------------------------------
// Task list pane
// ---------------------------------------------------------------------------

func (m tuiModel) renderTaskList(width, height int) string {
	titleStyle := lipgloss.NewStyle().
		Foreground(colMuted).
		Bold(true)

	header := titleStyle.Render("TASKS")

	var rows []string
	rows = append(rows, header)

	for i, t := range m.tasks {
		row := m.renderTaskRow(i, t, width)
		rows = append(rows, row)
		if len(rows) >= height {
			break
		}
	}

	content := strings.Join(rows, "\n")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(colBg).
		Render(content)
}

func (m tuiModel) renderTaskRow(index int, t *task.Task, width int) string {
	selected := index == m.cursor

	// Badge.
	badge := renderBadge(t.State)

	// ID (truncated).
	idStr := t.ID
	if len(idStr) > 14 {
		idStr = idStr[:14]
	}
	idStyle := lipgloss.NewStyle().Foreground(colMuted)

	// Title (truncated to fit).
	titleWidth := width - lipgloss.Width(badge) - len(idStr) - 4
	if titleWidth < 5 {
		titleWidth = 5
	}
	title := t.Title
	if len(title) > titleWidth {
		title = title[:titleWidth-1] + "…"
	}

	row := idStyle.Render(idStr) + " " + badge + " " + title

	if selected {
		return lipgloss.NewStyle().
			Background(colSurface).
			Foreground(colText).
			Bold(true).
			Width(width).
			Render("▶ " + row)
	}

	return lipgloss.NewStyle().
		Foreground(colText).
		Width(width).
		Render("  " + row)
}

func renderBadge(state fsm.State) string {
	bc, ok := stateBadge[state]
	if !ok {
		bc = badgeColors{colSurface, colMuted}
	}

	label := shortState(state)

	return lipgloss.NewStyle().
		Background(bc.bg).
		Foreground(bc.fg).
		Padding(0, 1).
		Render(label)
}

func shortState(s fsm.State) string {
	switch s {
	case fsm.Grill:
		return "GRILL"
	case fsm.Plan:
		return "PLAN"
	case fsm.Code:
		return "CODE"
	case fsm.AIReview:
		return "REVIEW"
	case fsm.AIFix:
		return "FIX"
	case fsm.LocalReview:
		return "LOCAL"
	case fsm.Push:
		return "PUSH"
	case fsm.Abandoned:
		return "ABANDONED"
	default:
		return string(s)
	}
}

// ---------------------------------------------------------------------------
// DAG pane
// ---------------------------------------------------------------------------

func (m tuiModel) renderDAG(width, height int) string {
	if m.graph == nil || len(m.graph.Tasks) == 0 {
		msg := lipgloss.NewStyle().
			Foreground(colMuted).
			Render("No plan available")
		return lipgloss.NewStyle().
			Width(width).
			Height(height).
			Background(colBg).
			Padding(1, 2).
			Render(msg)
	}

	t := m.tasks[m.cursor]

	// Header: task title + progress.
	done, active, pending, totalTokens := graphStats(m.graph)

	headerStyle := lipgloss.NewStyle().
		Foreground(colText).
		Bold(true)
	header := headerStyle.Render(fmt.Sprintf("%s  %d/%d", t.Title, done, len(m.graph.Tasks)))

	// Node rows.
	sorted, _ := m.graph.TopologicalSort()
	if sorted == nil {
		sorted = m.graph.Tasks
	}

	var rows []string
	rows = append(rows, header)
	rows = append(rows, "")

	for i, task := range sorted {
		nodeRow := m.renderDAGNode(task, width-4)
		rows = append(rows, nodeRow)

		// Draw connectors to next node if there are dependencies.
		if i < len(sorted)-1 {
			next := sorted[i+1]
			if hasDep(next, task.ID) {
				connector := lipgloss.NewStyle().
					Foreground(colMuted).
					Render("  ├─→")
				rows = append(rows, connector)
			} else if hasAnyDep(next, sorted[:i+1]) {
				connector := lipgloss.NewStyle().
					Foreground(colMuted).
					Render("  └─→")
				rows = append(rows, connector)
			}
		}

		if len(rows) >= height-4 {
			break
		}
	}

	// Progress bar.
	rows = append(rows, "")
	rows = append(rows, m.renderProgressBar(done, active, pending, totalTokens, width-4))

	content := strings.Join(rows, "\n")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(colBg).
		Padding(0, 2).
		Render(content)
}

func (m tuiModel) renderDAGNode(t planner.Task, width int) string {
	// Status icon.
	var icon string
	var iconColor lipgloss.Color
	switch t.Status {
	case planner.StatusDone:
		icon = "✓"
		iconColor = colGreen
	case planner.StatusInProgress:
		icon = "◆"
		iconColor = colCyan
	case planner.StatusFailed:
		icon = "✗"
		iconColor = colRed
	default:
		icon = "○"
		iconColor = colMuted
	}

	iconStr := lipgloss.NewStyle().Foreground(iconColor).Render(icon)

	// ID.
	idStr := lipgloss.NewStyle().Foreground(colMuted).Render(t.ID)

	// Tokens (right-aligned).
	tokenStr := lipgloss.NewStyle().
		Foreground(colMuted).
		Render(fmt.Sprintf("~%dk", t.EstimatedTokens/1000))

	// Title.
	titleStyle := lipgloss.NewStyle().Foreground(colText)
	if t.Status == planner.StatusDone {
		titleStyle = titleStyle.Faint(true)
	}
	if t.Status == planner.StatusInProgress {
		titleStyle = titleStyle.Foreground(colCyan).Bold(true)
	}

	usedWidth := 2 + lipgloss.Width(idStr) + 1 + lipgloss.Width(tokenStr) + 2
	titleWidth := width - usedWidth
	if titleWidth < 5 {
		titleWidth = 5
	}
	title := t.Title
	if len(title) > titleWidth {
		title = title[:titleWidth-1] + "…"
	}
	titleStr := titleStyle.Render(title)

	// Pad between title and tokens.
	gap := width - 2 - lipgloss.Width(idStr) - 1 - lipgloss.Width(titleStr) - lipgloss.Width(tokenStr)
	if gap < 1 {
		gap = 1
	}

	row := "  " + iconStr + " " + idStr + " " + titleStr + strings.Repeat(" ", gap) + tokenStr

	// Active node gets a surface background with cyan left border.
	if t.Status == planner.StatusInProgress {
		return lipgloss.NewStyle().
			Background(colSurface).
			BorderLeft(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colCyan).
			Width(width).
			Render(row)
	}

	return row
}

func (m tuiModel) renderProgressBar(done, active, pending, totalTokens, width int) string {
	total := done + active + pending
	if total == 0 {
		return ""
	}

	statsStyle := lipgloss.NewStyle().Foreground(colMuted)
	stats := statsStyle.Render(fmt.Sprintf("✓ %d  ◆ %d  ○ %d  ~%dk", done, active, pending, totalTokens/1000))

	barWidth := width - 4
	if barWidth < 10 {
		barWidth = 10
	}

	pct := float64(done) / float64(total)
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	barColor := colCyan
	if pct > 0.9 {
		barColor = colGreen
	}

	bar := lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(colBorder).Render(strings.Repeat("░", barWidth-filled))

	pctStr := lipgloss.NewStyle().Foreground(colMuted).Render(fmt.Sprintf(" %d%%", int(pct*100)))

	boxStyle := lipgloss.NewStyle().
		Background(colSurface).
		Padding(0, 1).
		Width(width)

	return boxStyle.Render(stats + "\n" + bar + pctStr)
}

// ---------------------------------------------------------------------------
// Detail pane
// ---------------------------------------------------------------------------

func (m tuiModel) renderDetail(width, height int) string {
	if m.cursor >= len(m.tasks) {
		return ""
	}

	t := m.tasks[m.cursor]

	titleStyle := lipgloss.NewStyle().
		Foreground(colMuted).
		Bold(true)
	labelStyle := lipgloss.NewStyle().
		Foreground(colMuted)
	valueStyle := lipgloss.NewStyle().
		Foreground(colText)
	cyanValue := lipgloss.NewStyle().
		Foreground(colCyan)

	var rows []string
	rows = append(rows, titleStyle.Render("SELECTED"))
	rows = append(rows, "")

	rows = append(rows, labelStyle.Render("status"))
	rows = append(rows, "  "+renderBadge(t.State))
	rows = append(rows, "")

	rows = append(rows, labelStyle.Render("id"))
	rows = append(rows, "  "+cyanValue.Render(t.ID))
	rows = append(rows, "")

	if t.JiraID != nil {
		rows = append(rows, labelStyle.Render("jira"))
		rows = append(rows, "  "+lipgloss.NewStyle().Foreground(colBlue).Render(*t.JiraID))
		rows = append(rows, "")
	}

	rows = append(rows, labelStyle.Render("started"))
	rows = append(rows, "  "+valueStyle.Render(t.CreatedAt.Format("2006-01-02 15:04")))
	rows = append(rows, "")

	if t.BranchName != nil {
		rows = append(rows, labelStyle.Render("branch"))
		rows = append(rows, "  "+valueStyle.Render(*t.BranchName))
		rows = append(rows, "")
	}

	// Show sub-task progress if there's a graph.
	if m.graph != nil && len(m.graph.Tasks) > 0 {
		done, _, _, totalTokens := graphStats(m.graph)
		rows = append(rows, labelStyle.Render("progress"))
		rows = append(rows, "  "+valueStyle.Render(fmt.Sprintf("%d / %d tasks", done, len(m.graph.Tasks))))
		rows = append(rows, "")

		rows = append(rows, labelStyle.Render("tokens"))
		rows = append(rows, "  "+valueStyle.Render(fmt.Sprintf("~%dk / %dk", totalTokens/1000, m.graph.TokenBudget/1000)))

		// Token budget bar.
		pct := float64(totalTokens) / float64(m.graph.TokenBudget)
		barWidth := width - 4
		if barWidth < 5 {
			barWidth = 5
		}
		filled := int(pct * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		barColor := colYellow
		if pct > 0.9 {
			barColor = colRed
		}
		bar := lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", filled)) +
			lipgloss.NewStyle().Foreground(colBorder).Render(strings.Repeat("░", barWidth-filled))
		rows = append(rows, "  "+bar)
		rows = append(rows, "")

		// Show selected DAG node details if available.
		// Show dependencies for the first non-done task.
		for _, gt := range m.graph.Tasks {
			if gt.Status != planner.StatusDone && len(gt.DependsOn) > 0 {
				rows = append(rows, labelStyle.Render("depends on"))
				var deps []string
				for _, d := range gt.DependsOn {
					icon := "○"
					for _, dt := range m.graph.Tasks {
						if dt.ID == d && dt.Status == planner.StatusDone {
							icon = "✓"
							break
						}
					}
					deps = append(deps, icon+" "+d)
				}
				rows = append(rows, "  "+valueStyle.Render(strings.Join(deps, " · ")))
				rows = append(rows, "")
				break
			}
		}

		// File hints for the active or first pending task.
		for _, gt := range m.graph.Tasks {
			if (gt.Status == planner.StatusInProgress || gt.Status == planner.StatusPending) && len(gt.FilesHint) > 0 {
				rows = append(rows, labelStyle.Render("files hint"))
				for _, f := range gt.FilesHint {
					prefix := "M"
					color := colYellow
					if strings.HasSuffix(f, "_test.go") {
						prefix = "A"
						color = colGreen
					}
					rows = append(rows, "  "+lipgloss.NewStyle().Foreground(color).Render(prefix+" "+f))
				}
				break
			}
		}
	}

	if t.ReviewIteration > 0 {
		rows = append(rows, "")
		rows = append(rows, labelStyle.Render("reviews"))
		rows = append(rows, "  "+valueStyle.Render(fmt.Sprintf("%d / %d", t.ReviewIteration, t.MaxReviewIterations)))
	}

	content := strings.Join(rows, "\n")

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Background(colBg).
		Padding(0, 1).
		Render(content)
}

// ---------------------------------------------------------------------------
// Layout renderers
// ---------------------------------------------------------------------------

func (m tuiModel) renderThreePane(height int) string {
	leftWidth := 30
	rightWidth := 35
	centerWidth := m.width - leftWidth - rightWidth - 2 // 2 for dividers
	if centerWidth < 20 {
		centerWidth = 20
	}

	left := m.renderTaskList(leftWidth, height)
	center := m.renderDAG(centerWidth, height)
	right := m.renderDetail(rightWidth, height)

	divider := lipgloss.NewStyle().
		Background(colBorder).
		Width(1).
		Height(height).
		Render(" ")

	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, center, divider, right)
}

func (m tuiModel) renderTwoPane(height int) string {
	leftWidth := 24
	rightWidth := m.width - leftWidth - 1 // 1 for divider
	if rightWidth < 20 {
		rightWidth = 20
	}

	left := m.renderTaskList(leftWidth, height)

	// Right side: DAG with inline detail below.
	dagHeight := height * 2 / 3
	detailHeight := height - dagHeight
	dag := m.renderDAG(rightWidth, dagHeight)
	detail := m.renderDetail(rightWidth, detailHeight)
	right := lipgloss.JoinVertical(lipgloss.Left, dag, detail)

	divider := lipgloss.NewStyle().
		Background(colBorder).
		Width(1).
		Height(height).
		Render(" ")

	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

func (m tuiModel) renderSinglePane(height int) string {
	switch m.singleTab {
	case 0:
		return m.renderTaskList(m.width, height)
	case 1:
		return m.renderDAG(m.width, height)
	case 2:
		return m.renderDetail(m.width, height)
	default:
		return m.renderTaskList(m.width, height)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func graphStats(g *planner.Graph) (done, active, pending, totalTokens int) {
	for _, t := range g.Tasks {
		totalTokens += t.EstimatedTokens
		switch t.Status {
		case planner.StatusDone:
			done++
		case planner.StatusInProgress:
			active++
		default:
			pending++
		}
	}
	return
}

func hasDep(t planner.Task, depID string) bool {
	for _, d := range t.DependsOn {
		if d == depID {
			return true
		}
	}
	return false
}

func hasAnyDep(t planner.Task, candidates []planner.Task) bool {
	for _, c := range candidates {
		if hasDep(t, c.ID) {
			return true
		}
	}
	return false
}
