package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type filter int

const (
	filterAll filter = iota
	filterUnreviewed
	filterReviewed
)

func (f filter) String() string {
	switch f {
	case filterUnreviewed:
		return "unreviewed"
	case filterReviewed:
		return "reviewed"
	default:
		return "all"
	}
}

type prStatus int

const (
	statusPending prStatus = iota
	statusReviewing
	statusDone
	statusError
)

type prItem struct {
	pr            PullRequest
	reviewed      bool
	status        prStatus
	result        *ReviewResult
	commitResults []CommitReviewResult
	branchResult  *ReviewResult
	err           error
	selected      bool
}

type model struct {
	items    []prItem
	cursor   int
	filter   filter
	width    int
	height   int
	forge    Forge
	reviewer Reviewer
	cfg      Config
	state    *State
	quitting bool
	message  string
}

type reviewDoneMsg struct {
	id            int64
	result        *ReviewResult
	commitResults []CommitReviewResult
	branchResult  *ReviewResult
	err           error
}

type postDoneMsg struct {
	id  int64
	err error
}

type listLoadedMsg struct {
	items []prItem
	err   error
}

func newModel(forge Forge, reviewer Reviewer, cfg Config, state *State) model {
	return model{
		forge:    forge,
		reviewer: reviewer,
		cfg:      cfg,
		state:    state,
	}
}

func (m model) Init() tea.Cmd {
	return m.loadList()
}

func (m model) loadList() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		prs, err := m.forge.ListAll(ctx)
		if err != nil {
			return listLoadedMsg{err: err}
		}
		items := make([]prItem, len(prs))
		for i, pr := range prs {
			items[i] = prItem{
				pr:       pr,
				reviewed: m.state.IsReviewed(m.cfg.Project, pr.ID),
			}
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].pr.UpdatedAt.After(items[j].pr.UpdatedAt)
		})
		return listLoadedMsg{items: items}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if m.quitting {
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "j", "down":
			m.moveCursor(1)
		case "k", "up":
			m.moveCursor(-1)
		case "g":
			m.cursor = 0
			m.clampCursor()
		case "G":
			vis := m.visible()
			if len(vis) > 0 {
				m.cursor = len(vis) - 1
			}
		case " ":
			if idx, ok := m.cursorIndex(); ok {
				m.items[idx].selected = !m.items[idx].selected
				m.moveCursor(1)
			}
		case "a":
			vis := m.visible()
			allSelected := true
			for _, i := range vis {
				if !m.items[i].selected {
					allSelected = false
					break
				}
			}
			for _, i := range vis {
				m.items[i].selected = !allSelected
			}
		case "tab":
			m.filter = (m.filter + 1) % 3
			m.cursor = 0
		case "r":
			return m, m.reviewSelected()
		case "R":
			return m, m.reviewAll()
		case "p":
			return m, m.postSelected()
		case "P":
			return m, m.postAll()
		case "l":
			m.message = "loading..."
			return m, m.loadList()
		}
		return m, nil

	case listLoadedMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("error: %v", msg.err)
			return m, nil
		}
		m.items = msg.items
		m.cursor = 0
		m.message = fmt.Sprintf("loaded %d MR(s)", len(m.items))
		return m, nil

	case reviewDoneMsg:
		for i := range m.items {
			if m.items[i].pr.ID == msg.id {
				if msg.err != nil {
					m.items[i].status = statusError
					m.items[i].err = msg.err
				} else {
					m.items[i].status = statusDone
					m.items[i].result = msg.result
					m.items[i].commitResults = msg.commitResults
					m.items[i].branchResult = msg.branchResult
					m.state.MarkReviewed(m.cfg.Project, msg.id)
					m.items[i].reviewed = true
					m.state.Save()
				}
				break
			}
		}
		return m, nil

	case postDoneMsg:
		for i := range m.items {
			if m.items[i].pr.ID == msg.id {
				if msg.err != nil {
					m.message = fmt.Sprintf("post #%d failed: %v", msg.id, msg.err)
				} else {
					m.message = fmt.Sprintf("posted review for #%d", msg.id)
				}
				break
			}
		}
		return m, nil
	}

	return m, nil
}

func (m *model) moveCursor(delta int) {
	vis := m.visible()
	if len(vis) == 0 {
		return
	}
	m.cursor += delta
	m.clampCursor()
}

func (m *model) clampCursor() {
	vis := m.visible()
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(vis) {
		m.cursor = len(vis) - 1
	}
}

func (m model) visible() []int {
	var indices []int
	for i, item := range m.items {
		switch m.filter {
		case filterUnreviewed:
			if item.reviewed {
				continue
			}
		case filterReviewed:
			if !item.reviewed {
				continue
			}
		}
		indices = append(indices, i)
	}
	return indices
}

func (m *model) cursorIndex() (int, bool) {
	vis := m.visible()
	if m.cursor < 0 || m.cursor >= len(vis) {
		return 0, false
	}
	return vis[m.cursor], true
}

func (m model) reviewSelected() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.items {
		if !m.items[i].selected || m.items[i].status == statusReviewing {
			continue
		}
		m.items[i].status = statusReviewing
		m.items[i].selected = false
		cmds = append(cmds, m.reviewOne(m.items[i].pr))
	}
	if len(cmds) == 0 {
		if idx, ok := m.cursorIndex(); ok && m.items[idx].status != statusReviewing {
			m.items[idx].status = statusReviewing
			cmds = append(cmds, m.reviewOne(m.items[idx].pr))
		}
	}
	return tea.Batch(cmds...)
}

func (m model) reviewAll() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.items {
		if m.items[i].reviewed || m.items[i].status == statusReviewing || m.items[i].status == statusDone {
			continue
		}
		m.items[i].status = statusReviewing
		cmds = append(cmds, m.reviewOne(m.items[i].pr))
	}
	return tea.Batch(cmds...)
}

func (m model) reviewOne(pr PullRequest) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		worktreeDir, cleanup, err := CreateWorktree(ctx, m.cfg.RepoPath, pr.ID, m.forge.Name())
		if err != nil {
			return reviewDoneMsg{id: pr.ID, err: fmt.Errorf("worktree: %w", err)}
		}
		defer cleanup()

		switch m.cfg.ReviewMode {
		case "both":
			commitResults, err := reviewByCommits(ctx, m.forge, m.reviewer, worktreeDir, pr)
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: err}
			}

			var digest string
			if llmr, ok := m.reviewer.(*LLMReviewer); ok {
				digest, _ = digestFindings(ctx, llmr.LLM, llmr.Model, commitResults)
			} else {
				digest = digestFindingsPlain(commitResults)
			}

			diff, err := m.forge.GetDiff(ctx, pr.ID)
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: fmt.Errorf("get diff: %w", err)}
			}
			branchResult, err := m.reviewer.ReviewWithContext(ctx, worktreeDir, diff, digest)
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: fmt.Errorf("branch repass: %w", err)}
			}

			merged := mergeCommitResults(commitResults)
			merged.Findings = append(merged.Findings, branchResult.Findings...)
			return reviewDoneMsg{id: pr.ID, result: merged, commitResults: commitResults, branchResult: branchResult}

		case "commits":
			commitResults, err := reviewByCommits(ctx, m.forge, m.reviewer, worktreeDir, pr)
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: err}
			}
			return reviewDoneMsg{id: pr.ID, result: mergeCommitResults(commitResults), commitResults: commitResults}

		default:
			diff, err := m.forge.GetDiff(ctx, pr.ID)
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: fmt.Errorf("get diff: %w", err)}
			}
			result, err := m.reviewer.Review(ctx, worktreeDir, diff)
			return reviewDoneMsg{id: pr.ID, result: result, err: err}
		}
	}
}

func (m model) postSelected() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.items {
		if !m.items[i].selected || m.items[i].status != statusDone || m.items[i].result == nil {
			continue
		}
		m.items[i].selected = false
		cmds = append(cmds, m.postOne(m.items[i]))
	}
	if len(cmds) == 0 {
		if idx, ok := m.cursorIndex(); ok && m.items[idx].status == statusDone && m.items[idx].result != nil {
			cmds = append(cmds, m.postOne(m.items[idx]))
		}
	}
	return tea.Batch(cmds...)
}

func (m model) postAll() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.items {
		if m.items[i].status != statusDone || m.items[i].result == nil || m.items[i].reviewed {
			continue
		}
		cmds = append(cmds, m.postOne(m.items[i]))
	}
	return tea.Batch(cmds...)
}

func (m model) postOne(item prItem) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var comment string
		if item.branchResult != nil && len(item.commitResults) > 0 {
			comment = FormatBothReviewComment(item.commitResults, item.branchResult, item.pr.Title, item.result.Model)
		} else if len(item.commitResults) > 0 {
			comment = FormatCommitReviewComment(item.commitResults, item.pr.Title, item.result.Model)
		} else {
			comment = FormatComment(item.result, item.pr.Title)
		}
		if m.cfg.InlineComments && item.result != nil && len(item.result.Findings) > 0 {
			inlineComments, _ := routeFindings(item.result.Findings)
			if len(inlineComments) > 0 {
				if err := m.forge.PostInlineComments(ctx, item.pr, inlineComments); err != nil {
					log.Printf("#%d: inline comments failed: %v", item.pr.ID, err)
				}
			}
		}
		if err := m.forge.PostComment(ctx, item.pr.ID, comment); err != nil {
			return postDoneMsg{id: item.pr.ID, err: err}
		}
		return postDoneMsg{id: item.pr.ID}
	}
}

// --- View ---

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	cursorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	statusOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	statusErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	statusWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	actionBar     = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Background(lipgloss.Color("0"))
	sevCritical   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	sevMajor      = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	sevMinor      = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	sevInfo       = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

func (m model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "loading..."
	}

	vis := m.visible()

	detailHeight := m.height / 3
	if detailHeight < 5 {
		detailHeight = 5
	}
	listHeight := m.height - detailHeight - 2
	if listHeight < 3 {
		listHeight = 3
	}

	var b strings.Builder

	// Header
	total := len(m.items)
	unreviewed := 0
	for _, item := range m.items {
		if !item.reviewed {
			unreviewed++
		}
	}
	header := fmt.Sprintf(" %s  %d MR(s)  %d unreviewed  filter: %s",
		m.forge.Name(), total, unreviewed, m.filter)
	if m.message != "" {
		header += "  │  " + m.message
	}
	b.WriteString(headerStyle.Render(header))
	b.WriteByte('\n')

	// MR list
	if len(vis) == 0 {
		b.WriteString(dimStyle.Render("  no merge requests match filter"))
		b.WriteByte('\n')
	}

	scrollOffset := 0
	if m.cursor >= listHeight {
		scrollOffset = m.cursor - listHeight + 1
	}

	rendered := 0
	for vi := scrollOffset; vi < len(vis) && rendered < listHeight; vi++ {
		idx := vis[vi]
		item := m.items[idx]
		isCursor := vi == m.cursor

		prefix := "  "
		if item.selected {
			prefix = "● "
		}
		if isCursor {
			prefix = "▸ "
			if item.selected {
				prefix = "▸●"
			}
		}

		statusIcon := " "
		switch item.status {
		case statusReviewing:
			statusIcon = statusWarn.Render("⟳")
		case statusDone:
			n := 0
			if item.result != nil {
				n = len(item.result.Findings)
			}
			statusIcon = statusOK.Render(fmt.Sprintf("✓%d", n))
		case statusError:
			statusIcon = statusErr.Render("✗")
		}

		reviewedMark := " "
		if item.reviewed {
			reviewedMark = dimStyle.Render("R")
		}

		age := relativeTime(item.pr.UpdatedAt)
		line := fmt.Sprintf("%s %s %s #%-5d %-15s %-6s %s",
			prefix, statusIcon, reviewedMark, item.pr.ID, item.pr.Author, age, item.pr.Title)

		if len(line) > m.width {
			line = line[:m.width]
		}

		if isCursor {
			line = cursorStyle.Render(line)
		} else if item.selected {
			line = selectedStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteByte('\n')
		rendered++
	}

	for rendered < listHeight {
		b.WriteByte('\n')
		rendered++
	}

	// Detail pane
	b.WriteString(strings.Repeat("─", m.width))
	b.WriteByte('\n')

	detailLines := 0
	if idx, ok := m.cursorIndex(); ok {
		item := m.items[idx]
		switch item.status {
		case statusDone:
			if item.result != nil && len(item.result.Findings) > 0 {
				for _, f := range item.result.Findings {
					if detailLines >= detailHeight-1 {
						break
					}
					sev := renderSeverity(f.Severity)
					loc := ""
					if f.Location != "" {
						loc = dimStyle.Render(" " + f.Location)
					}
					line := fmt.Sprintf("  %s %s%s — %s", sev, f.Category, loc, f.Description)
					if len(line) > m.width {
						line = line[:m.width]
					}
					b.WriteString(line)
					b.WriteByte('\n')
					detailLines++
				}
				if item.branchResult != nil && len(item.branchResult.Findings) > 0 && detailLines < detailHeight-1 {
					sep := dimStyle.Render("  ── branch-level ──")
					b.WriteString(sep)
					b.WriteByte('\n')
					detailLines++
					for _, f := range item.branchResult.Findings {
						if detailLines >= detailHeight-1 {
							break
						}
						sev := renderSeverity(f.Severity)
						loc := ""
						if f.Location != "" {
							loc = dimStyle.Render(" " + f.Location)
						}
						line := fmt.Sprintf("  %s %s%s — %s", sev, f.Category, loc, f.Description)
						if len(line) > m.width {
							line = line[:m.width]
						}
						b.WriteString(line)
						b.WriteByte('\n')
						detailLines++
					}
				}
			} else {
				b.WriteString(dimStyle.Render("  no findings"))
				b.WriteByte('\n')
				detailLines++
			}
		case statusReviewing:
			b.WriteString(statusWarn.Render("  reviewing..."))
			b.WriteByte('\n')
			detailLines++
		case statusError:
			b.WriteString(statusErr.Render(fmt.Sprintf("  error: %v", item.err)))
			b.WriteByte('\n')
			detailLines++
		default:
			b.WriteString(dimStyle.Render("  select and press r to review"))
			b.WriteByte('\n')
			detailLines++
		}
	}
	for detailLines < detailHeight-1 {
		b.WriteByte('\n')
		detailLines++
	}

	// Action bar
	actions := []string{
		"j/k:navigate", "space:select", "a:select all",
		"tab:filter", "r:review", "R:review all",
		"p:post", "P:post all", "l:reload", "q:quit",
	}
	bar := " " + strings.Join(actions, "  ")
	if len(bar) > m.width {
		bar = bar[:m.width]
	}
	b.WriteString(actionBar.Render(bar))

	return b.String()
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func renderSeverity(sev string) string {
	switch strings.ToLower(sev) {
	case "critical":
		return sevCritical.Render("CRIT")
	case "major":
		return sevMajor.Render("MAJR")
	case "minor":
		return sevMinor.Render("MINR")
	default:
		return sevInfo.Render("INFO")
	}
}
