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
	"github.com/charmbracelet/x/ansi"
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

type commitProgress struct {
	index    int
	total    int
	sha      string
	message  string
	status   CommitStatus
	err      error
	findings []Finding
}

type prItem struct {
	pr             PullRequest
	reviewed       bool
	posted         bool
	status         prStatus
	result         *ReviewResult
	commitResults  []CommitReviewResult
	branchResult   *ReviewResult
	err            error
	selected       bool
	reviewMode     string
	progress       string
	commitProgress []commitProgress
}

var modeLabels = []string{"full", "commits", "both"}

type model struct {
	items       []prItem
	cursor      int
	filter      filter
	width       int
	height      int
	forge       Forge
	reviewer    Reviewer
	cfg         Config
	state       *State
	programRef  **tea.Program
	quitting    bool
	message     string
	pickingMode  bool
	modeChoice   int
	pickTarget   int
	detailScroll int
}

type reviewProgressMsg struct {
	id      int64
	current int
	total   int
	sha     string
	message string
}

type commitStartedMsg struct {
	id    int64
	index int
	total int
	sha   string
	msg   string
}

type commitDoneMsg struct {
	id       int64
	index    int
	sha      string
	findings []Finding
}

type commitFailedMsg struct {
	id    int64
	index int
	sha   string
	err   error
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
		seen := make(map[int64]bool)
		var items []prItem
		for _, pr := range prs {
			if seen[pr.ID] {
				continue
			}
			seen[pr.ID] = true
			items = append(items, prItem{
				pr:       pr,
				reviewed: m.state.IsReviewed(m.cfg.Project, pr.ID),
			})
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
		if m.pickingMode {
			switch msg.String() {
			case "j", "down":
				m.modeChoice = (m.modeChoice + 1) % len(modeLabels)
			case "k", "up":
				m.modeChoice = (m.modeChoice - 1 + len(modeLabels)) % len(modeLabels)
			case "enter":
				m.pickingMode = false
				m.items[m.pickTarget].reviewMode = modeLabels[m.modeChoice]
				m.items[m.pickTarget].status = statusReviewing
				m.items[m.pickTarget].progress = fmt.Sprintf("mode: %s — fetching...", modeLabels[m.modeChoice])
				m.items[m.pickTarget].commitProgress = nil
				return m, m.reviewOne(m.items[m.pickTarget].pr, modeLabels[m.modeChoice])
			case "esc", "q":
				m.pickingMode = false
			}
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
			return m, m.openModePicker()
		case "R":
			return m, m.reviewAll()
		case "p":
			return m, m.postSelected()
		case "P":
			return m, m.postAll()
		case "J":
			m.detailScroll++
		case "K":
			if m.detailScroll > 0 {
				m.detailScroll--
			}
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

	case reviewProgressMsg:
		for i := range m.items {
			if m.items[i].pr.ID == msg.id {
				if msg.total > 0 {
					m.items[i].progress = fmt.Sprintf("commit %d/%d: %s %s", msg.current, msg.total, msg.sha, msg.message)
				} else {
					m.items[i].progress = msg.message
				}
				break
			}
		}
		return m, nil

	case commitStartedMsg:
		for i := range m.items {
			if m.items[i].pr.ID == msg.id {
				m.items[i].commitProgress = append(m.items[i].commitProgress, commitProgress{
					index:   msg.index,
					total:   msg.total,
					sha:     msg.sha,
					message: msg.msg,
					status:  CommitStarted,
				})
				break
			}
		}
		return m, nil

	case commitDoneMsg:
		for i := range m.items {
			if m.items[i].pr.ID == msg.id {
				found := false
				for j := range m.items[i].commitProgress {
					if m.items[i].commitProgress[j].index == msg.index {
						m.items[i].commitProgress[j].status = CommitDone
						m.items[i].commitProgress[j].findings = msg.findings
						found = true
						break
					}
				}
				if !found {
					m.items[i].commitProgress = append(m.items[i].commitProgress, commitProgress{
						index:    msg.index,
						sha:      msg.sha,
						status:   CommitDone,
						findings: msg.findings,
					})
				}
				break
			}
		}
		return m, nil

	case commitFailedMsg:
		for i := range m.items {
			if m.items[i].pr.ID == msg.id {
				found := false
				for j := range m.items[i].commitProgress {
					if m.items[i].commitProgress[j].index == msg.index {
						m.items[i].commitProgress[j].status = CommitFailed
						m.items[i].commitProgress[j].err = msg.err
						found = true
						break
					}
				}
				if !found {
					m.items[i].commitProgress = append(m.items[i].commitProgress, commitProgress{
						index:  msg.index,
						sha:    msg.sha,
						status: CommitFailed,
						err:    msg.err,
					})
				}
				break
			}
		}
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
					m.items[i].posted = true
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
	m.detailScroll = 0
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

func (m *model) openModePicker() tea.Cmd {
	idx, ok := m.cursorIndex()
	if !ok || m.items[idx].status == statusReviewing {
		return nil
	}
	m.pickingMode = true
	m.pickTarget = idx
	m.modeChoice = 0
	for i, label := range modeLabels {
		if label == m.cfg.ReviewMode {
			m.modeChoice = i
			break
		}
	}
	return nil
}

func (m model) reviewSelected() tea.Cmd {
	var cmds []tea.Cmd
	mode := m.cfg.ReviewMode
	if mode == "" {
		mode = "full"
	}
	for i := range m.items {
		if !m.items[i].selected || m.items[i].status == statusReviewing {
			continue
		}
		m.items[i].status = statusReviewing
		m.items[i].selected = false
		m.items[i].reviewMode = mode
		m.items[i].progress = fmt.Sprintf("mode: %s — fetching...", mode)
		m.items[i].commitProgress = nil
		cmds = append(cmds, m.reviewOne(m.items[i].pr, mode))
	}
	return tea.Batch(cmds...)
}

func (m model) reviewAll() tea.Cmd {
	var cmds []tea.Cmd
	mode := m.cfg.ReviewMode
	if mode == "" {
		mode = "full"
	}
	for i := range m.items {
		if m.items[i].reviewed || m.items[i].status == statusReviewing || m.items[i].status == statusDone {
			continue
		}
		m.items[i].status = statusReviewing
		m.items[i].reviewMode = mode
		m.items[i].progress = fmt.Sprintf("mode: %s — fetching...", mode)
		m.items[i].commitProgress = nil
		cmds = append(cmds, m.reviewOne(m.items[i].pr, mode))
	}
	return tea.Batch(cmds...)
}

func (m model) reviewOne(pr PullRequest, mode string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		worktreeDir, cleanup, err := CreateWorktree(ctx, m.cfg.RepoPath, pr.ID, m.forge.Name())
		if err != nil {
			return reviewDoneMsg{id: pr.ID, err: fmt.Errorf("worktree: %w", err)}
		}
		defer cleanup()

		progress := ProgressFunc(func(ev CommitProgressEvent) {
			if *m.programRef == nil {
				return
			}
			switch ev.Status {
			case CommitStarted:
				(*m.programRef).Send(commitStartedMsg{
					id: pr.ID, index: ev.Index, total: ev.Total, sha: ev.SHA, msg: ev.Message,
				})
			case CommitDone:
				var findings []Finding
				if ev.Result != nil {
					findings = ev.Result.Findings
				}
				(*m.programRef).Send(commitDoneMsg{
					id: pr.ID, index: ev.Index, sha: ev.SHA, findings: findings,
				})
			case CommitFailed:
				(*m.programRef).Send(commitFailedMsg{
					id: pr.ID, index: ev.Index, sha: ev.SHA, err: ev.Err,
				})
			}
		})

		switch mode {
		case "both":
			commits, err := m.forge.ListCommits(ctx, pr.ID)
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: fmt.Errorf("list commits: %w", err)}
			}
			commitResults, err := reviewByCommits(ctx, m.reviewer, worktreeDir, commits, &ReviewByCommitsOpts{State: m.state, Project: m.cfg.Project, OnProgress: progress, Concurrency: m.cfg.CommitConcurrency()})
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: err}
			}

			var digest string
			if llmr, ok := m.reviewer.(*LLMReviewer); ok {
				digest, _ = digestFindings(ctx, llmr.LLM, llmr.Model, commitResults)
			} else {
				digest = digestFindingsPlain(commitResults)
			}

			if *m.programRef != nil {
				(*m.programRef).Send(reviewProgressMsg{id: pr.ID, message: "branch repass..."})
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
			commits, err := m.forge.ListCommits(ctx, pr.ID)
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: fmt.Errorf("list commits: %w", err)}
			}
			commitResults, err := reviewByCommits(ctx, m.reviewer, worktreeDir, commits, &ReviewByCommitsOpts{State: m.state, Project: m.cfg.Project, OnProgress: progress, Concurrency: m.cfg.CommitConcurrency()})
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: err}
			}
			return reviewDoneMsg{id: pr.ID, result: mergeCommitResults(commitResults), commitResults: commitResults}

		default:
			diff, err := m.forge.GetDiff(ctx, pr.ID)
			if err != nil {
				return reviewDoneMsg{id: pr.ID, err: fmt.Errorf("get diff: %w", err)}
			}
			if *m.programRef != nil {
				(*m.programRef).Send(reviewProgressMsg{id: pr.ID, message: "reviewing full diff..."})
			}
			result, err := m.reviewer.ReviewFull(ctx, worktreeDir, diff)
			return reviewDoneMsg{id: pr.ID, result: result, err: err}
		}
	}
}

func (m model) postSelected() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.items {
		if !m.items[i].selected || m.items[i].posted || m.items[i].status != statusDone || m.items[i].result == nil {
			continue
		}
		m.items[i].selected = false
		cmds = append(cmds, m.postOne(m.items[i]))
	}
	if len(cmds) == 0 {
		if idx, ok := m.cursorIndex(); ok && m.items[idx].status == statusDone && m.items[idx].result != nil && !m.items[idx].posted {
			cmds = append(cmds, m.postOne(m.items[idx]))
		}
	}
	if len(cmds) == 0 {
		m.message = "nothing to post"
	}
	return tea.Batch(cmds...)
}

func (m model) postAll() tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.items {
		if m.items[i].status != statusDone || m.items[i].result == nil || m.items[i].posted {
			continue
		}
		cmds = append(cmds, m.postOne(m.items[i]))
	}
	if len(cmds) == 0 {
		m.message = "nothing to post"
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
		style := m.cfg.CommentStyle
		if style == "" {
			style = "both"
		}
		if (style == "inline" || style == "both") && item.result != nil && len(item.result.Findings) > 0 {
			inlineComments, _ := routeFindings(item.result.Findings, m.cfg.InlineSeverity)
			if len(inlineComments) > 0 {
				if err := m.forge.PostInlineComments(ctx, item.pr, inlineComments); err != nil {
					log.Printf("#%d: inline comments failed: %v", item.pr.ID, err)
				}
			}
		}
		if style == "summary" || style == "both" {
			if err := m.forge.PostComment(ctx, item.pr.ID, comment); err != nil {
				return postDoneMsg{id: item.pr.ID, err: err}
			}
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
	actionKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Background(lipgloss.Color("0"))
	sevCritical   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	sevMajor      = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	sevMinor      = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	sevInfo       = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	mrNumStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	authorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	titleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	ageStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	detailTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	detailLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
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
		if item.posted {
			reviewedMark = statusOK.Render("P")
		} else if item.reviewed {
			reviewedMark = dimStyle.Render("R")
		}

		age := relativeTime(item.pr.UpdatedAt)

		var line string
		if isCursor {
			line = cursorStyle.Render(fmt.Sprintf("%s %s %s #%-5d %-15s %-6s %s",
				prefix, statusIcon, reviewedMark, item.pr.ID, item.pr.Author, age, item.pr.Title))
		} else if item.selected {
			line = selectedStyle.Render(fmt.Sprintf("%s %s %s #%-5d %-15s %-6s %s",
				prefix, statusIcon, reviewedMark, item.pr.ID, item.pr.Author, age, item.pr.Title))
		} else {
			line = fmt.Sprintf("%s %s %s %s %-15s %s %s",
				prefix, statusIcon, reviewedMark,
				mrNumStyle.Render(fmt.Sprintf("#%-5d", item.pr.ID)),
				authorStyle.Render(item.pr.Author),
				ageStyle.Render(fmt.Sprintf("%-6s", age)),
				titleStyle.Render(item.pr.Title))
		}

		if lipgloss.Width(line) > m.width {
			line = ansi.Truncate(line, m.width, "")
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
	b.WriteString(separatorStyle.Render(strings.Repeat("─", m.width)))
	b.WriteByte('\n')

	detailLines := 0
	if m.pickingMode {
		b.WriteString(headerStyle.Render("  Review mode:"))
		b.WriteByte('\n')
		detailLines++
		modeDescs := []string{"Full diff review", "Commit-by-commit", "Both (commits + branch repass)"}
		for i, desc := range modeDescs {
			prefix := "  "
			if i == m.modeChoice {
				prefix = cursorStyle.Render("▸ ")
			}
			b.WriteString(fmt.Sprintf("  %s%s", prefix, desc))
			b.WriteByte('\n')
			detailLines++
		}
	} else if idx, ok := m.cursorIndex(); ok {
		item := m.items[idx]
		switch item.status {
		case statusDone:
			lines := m.buildDetailLines(item)
			if len(lines) == 0 {
				b.WriteString(dimStyle.Render("  no findings"))
				b.WriteByte('\n')
				detailLines++
			} else {
				if m.detailScroll >= len(lines) {
					m.detailScroll = len(lines) - 1
				}
				for i := m.detailScroll; i < len(lines) && detailLines < detailHeight-1; i++ {
					line := lines[i]
					if lipgloss.Width(line) > m.width {
						line = ansi.Truncate(line, m.width, "")
					}
					b.WriteString(line)
					b.WriteByte('\n')
					detailLines++
				}
			}
		case statusReviewing:
			if len(item.commitProgress) > 0 {
				if item.progress != "" {
					b.WriteString(statusWarn.Render(fmt.Sprintf("  reviewing — %s", item.progress)))
					b.WriteByte('\n')
					detailLines++
				}
				var inflight, completed, failed []commitProgress
				for _, cp := range item.commitProgress {
					switch cp.status {
					case CommitStarted:
						inflight = append(inflight, cp)
					case CommitDone:
						completed = append(completed, cp)
					case CommitFailed:
						failed = append(failed, cp)
					}
				}
				sort.Slice(completed, func(i, j int) bool {
					return completed[i].index < completed[j].index
				})
				for _, cp := range inflight {
					if detailLines >= detailHeight-1 {
						break
					}
					line := statusWarn.Render(fmt.Sprintf("  ⟳ commit %d/%d: %s %s", cp.index, cp.total, cp.sha, cp.message))
					if lipgloss.Width(line) > m.width {
						line = ansi.Truncate(line, m.width, "")
					}
					b.WriteString(line)
					b.WriteByte('\n')
					detailLines++
				}
				for _, cp := range failed {
					if detailLines >= detailHeight-1 {
						break
					}
					errMsg := "unknown error"
					if cp.err != nil {
						errMsg = cp.err.Error()
					}
					line := statusErr.Render(fmt.Sprintf("  ✗ commit %d/%d: %s — %s", cp.index, cp.total, cp.sha, errMsg))
					if lipgloss.Width(line) > m.width {
						line = ansi.Truncate(line, m.width, "")
					}
					b.WriteString(line)
					b.WriteByte('\n')
					detailLines++
				}
				for _, cp := range completed {
					if detailLines >= detailHeight-1 {
						break
					}
					n := len(cp.findings)
					line := statusOK.Render(fmt.Sprintf("  ✓ commit %d/%d: %s %s (%d findings)", cp.index, cp.total, cp.sha, cp.message, n))
					if lipgloss.Width(line) > m.width {
						line = ansi.Truncate(line, m.width, "")
					}
					b.WriteString(line)
					b.WriteByte('\n')
					detailLines++
				}
			} else if item.progress != "" {
				b.WriteString(statusWarn.Render(fmt.Sprintf("  reviewing — %s", item.progress)))
				b.WriteByte('\n')
				detailLines++
			} else {
				b.WriteString(statusWarn.Render("  reviewing..."))
				b.WriteByte('\n')
				detailLines++
			}
		case statusError:
			b.WriteString(statusErr.Render(fmt.Sprintf("  error: %v", item.err)))
			b.WriteByte('\n')
			detailLines++
		default:
			b.WriteString("  " + detailTitle.Render(item.pr.Title))
			b.WriteByte('\n')
			detailLines++
			if detailLines < detailHeight-1 {
				meta := detailLabel.Render("  author: ") + authorStyle.Render(item.pr.Author)
				if !item.pr.UpdatedAt.IsZero() {
					meta += detailLabel.Render("  updated: ") + ageStyle.Render(relativeTime(item.pr.UpdatedAt)+" ago")
				}
				b.WriteString(meta)
				b.WriteByte('\n')
				detailLines++
			}
			if detailLines < detailHeight-1 {
				b.WriteString(dimStyle.Render("  press r to review"))
				b.WriteByte('\n')
				detailLines++
			}
		}
	}
	for detailLines < detailHeight-1 {
		b.WriteByte('\n')
		detailLines++
	}

	// Action bar
	actions := [][2]string{
		{"j/k", "navigate"}, {"J/K", "scroll"}, {"space", "select"}, {"a", "select all"},
		{"tab", "filter"}, {"r", "review"}, {"R", "review all"},
		{"p", "post"}, {"P", "post all"}, {"l", "reload"}, {"q", "quit"},
	}
	var barParts []string
	for _, a := range actions {
		barParts = append(barParts, actionKey.Render(a[0])+actionBar.Render(":"+a[1]))
	}
	b.WriteString(actionBar.Render(" ") + strings.Join(barParts, actionBar.Render("  ")))

	return b.String()
}

func (m model) buildDetailLines(item prItem) []string {
	var lines []string

	if len(item.commitResults) > 0 {
		for _, cr := range item.commitResults {
			sha := cr.Commit.SHA
			if len(sha) > 8 {
				sha = sha[:8]
			}
			msg := firstline(cr.Commit.Message)
			header := dimStyle.Render(fmt.Sprintf("  ── %s %s ──", sha, msg))
			lines = append(lines, header)
			if cr.Result == nil || len(cr.Result.Findings) == 0 {
				lines = append(lines, dimStyle.Render("    no findings"))
				continue
			}
			for _, f := range cr.Result.Findings {
				lines = append(lines, formatDetailFinding(f))
			}
		}
		if item.branchResult != nil && len(item.branchResult.Findings) > 0 {
			lines = append(lines, dimStyle.Render("  ── branch-level ──"))
			for _, f := range item.branchResult.Findings {
				lines = append(lines, formatDetailFinding(f))
			}
		}
	} else if item.result != nil {
		for _, f := range item.result.Findings {
			lines = append(lines, formatDetailFinding(f))
		}
	}
	return lines
}

func formatDetailFinding(f Finding) string {
	sev := renderSeverity(f.Severity)
	loc := ""
	if f.Location != "" {
		loc = dimStyle.Render(" " + f.Location)
	}
	return fmt.Sprintf("  %s %s%s — %s", sev, f.Category, loc, f.Description)
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
