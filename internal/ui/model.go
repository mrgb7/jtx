package ui

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mragab/jtx/internal/config"
	"github.com/mragab/jtx/internal/jira"
)

type viewState int

const (
	viewBoard      viewState = iota
	viewDetail     viewState = iota
	viewSearch     viewState = iota
	viewComment    viewState = iota
	viewEditTitle  viewState = iota
	viewEditDesc   viewState = iota
	viewCreate     viewState = iota
	viewTransition viewState = iota
	viewAssign     viewState = iota
)

type createStep int

const (
	createStepType  createStep = iota // pick issue type
	createStepTitle createStep = iota // enter summary
	createStepDesc  createStep = iota // enter description
)

var issueKeyPattern = regexp.MustCompile(`(?i)^[A-Z][A-Z0-9]+-\d+$`)

// ── messages ─────────────────────────────────────────────────────────────────

type fetchDoneMsg struct {
	issues []jira.Issue
	err    error
}

type fetchDetailMsg struct {
	issue *jira.Issue
	err   error
}

type actionDoneMsg struct{ err error }

type fetchTransitionsMsg struct {
	transitions []jira.Transition
	err         error
}

type transitionDoneMsg struct{ err error }

type fetchIssueTypesMsg struct {
	types []jira.IssueTypeRef
	err   error
}

type fetchAssignableUsersMsg struct {
	users []jira.User
	err   error
}

type assignDoneMsg struct{ err error }

type createDoneMsg struct {
	key string
	err error
}

// ── data types ────────────────────────────────────────────────────────────────

type column struct {
	title    string
	colorKey string
	issues   []jira.Issue
}

// ── model ─────────────────────────────────────────────────────────────────────

type Model struct {
	client          *jira.Client
	cfg             *config.Config
	jql             string
	defaultJQL      string
	effectiveProject string // cfg.Project or extracted from default JQL
	columns         []column

	colIdx     int
	rowIdx     int
	allIssues  []jira.Issue
	searchMode bool // true when showing flat JQL search results

	state          viewState
	detailIssue    *jira.Issue
	detailViewport viewport.Model
	detailLoading  bool

	// single-line input: search
	searchInput textinput.Model
	searchErr   string

	// single-line input: edit title
	titleInput textinput.Model

	// multi-line input: comment / edit description
	textArea    textarea.Model
	popupTitle  string
	popupErr    string
	popupSaving bool

	// transition picker
	transitions      []jira.Transition
	transitionIdx    int
	transitionErr    string
	transitionSaving bool

	// assign picker
	assignUsers      []jira.User // nil = loading
	assignIdx        int
	assignErr        string
	assignSaving     bool
	assignLoading    bool
	currentUser      *jira.User

	// create ticket wizard
	createStep       createStep
	createIssueTypes []jira.IssueTypeRef
	createTypeIdx    int
	createTitle      textinput.Model
	createDesc       textarea.Model
	createErr        string
	createSaving     bool

	loading bool
	spinner spinner.Model
	err     error

	width  int
	height int
}

func New(client *jira.Client, jql string, cfg *config.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = loadingStyle

	cols := make([]column, len(cfg.Columns))
	for i, def := range cfg.Columns {
		cols[i] = column{title: def.Title, colorKey: def.Color}
	}

	si := textinput.New()
	si.Placeholder = "ticket ID (OBS-123) or JQL query…"
	si.CharLimit = 300
	si.Width = 60

	ti := textinput.New()
	ti.CharLimit = 300
	ti.Width = 60

	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.CharLimit = 5000

	ct := textinput.New()
	ct.Placeholder = "Issue summary…"
	ct.CharLimit = 300
	ct.Width = 60

	cd := textarea.New()
	cd.ShowLineNumbers = false
	cd.CharLimit = 5000
	cd.Placeholder = "Description (optional)…"

	ep := cfg.Project
	if ep == "" {
		ep = extractProjectFromJQL(jql)
	}

	return Model{
		client:           client,
		cfg:              cfg,
		jql:              jql,
		defaultJQL:       jql,
		effectiveProject: ep,
		columns:          cols,
		loading:     true,
		spinner:     s,
		searchInput: si,
		titleInput:  ti,
		textArea:    ta,
		createTitle: ct,
		createDesc:  cd,
	}
}

// ── init ──────────────────────────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.fetchIssues())
}

// ── commands ──────────────────────────────────────────────────────────────────

func (m Model) fetchIssues() tea.Cmd {
	jql := m.jql
	return func() tea.Msg {
		issues, err := m.client.SearchIssues(jql, 200)
		return fetchDoneMsg{issues: issues, err: err}
	}
}

func (m Model) fetchDetail(key string) tea.Cmd {
	return func() tea.Msg {
		issue, err := m.client.GetIssue(key)
		return fetchDetailMsg{issue: issue, err: err}
	}
}

func (m Model) cmdAddComment(key, text string) tea.Cmd {
	return func() tea.Msg {
		return actionDoneMsg{err: m.client.AddComment(key, text)}
	}
}

func (m Model) cmdUpdateTitle(key, title string) tea.Cmd {
	return func() tea.Msg {
		return actionDoneMsg{err: m.client.UpdateSummary(key, title)}
	}
}

func (m Model) cmdUpdateDesc(key, desc string) tea.Cmd {
	return func() tea.Msg {
		return actionDoneMsg{err: m.client.UpdateDescription(key, desc)}
	}
}

func (m Model) cmdFetchTransitions(key string) tea.Cmd {
	return func() tea.Msg {
		transitions, err := m.client.GetTransitions(key)
		return fetchTransitionsMsg{transitions: transitions, err: err}
	}
}

func (m Model) cmdTransitionIssue(key, transitionID string) tea.Cmd {
	return func() tea.Msg {
		return transitionDoneMsg{err: m.client.TransitionIssue(key, transitionID)}
	}
}

func (m Model) cmdFetchIssueTypes(projectKey string) tea.Cmd {
	return func() tea.Msg {
		types, err := m.client.GetIssueTypes(projectKey)
		return fetchIssueTypesMsg{types: types, err: err}
	}
}

func (m Model) cmdFetchAssignableUsers(issueKey string) tea.Cmd {
	return func() tea.Msg {
		users, err := m.client.SearchAssignableUsers(issueKey, "")
		return fetchAssignableUsersMsg{users: users, err: err}
	}
}

func (m Model) cmdAssignIssue(key, accountID string) tea.Cmd {
	return func() tea.Msg {
		return assignDoneMsg{err: m.client.AssignIssue(key, accountID)}
	}
}

func (m Model) cmdCreateIssue(projectKey, typeID, summary, desc string) tea.Cmd {
	return func() tea.Msg {
		key, err := m.client.CreateIssue(projectKey, typeID, summary, desc)
		return createDoneMsg{key: key, err: err}
	}
}

// ── update ────────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.detailViewport = viewport.New(m.width-4, m.height-8)
		w := m.width - 10
		if w > 80 {
			w = 80
		}
		m.searchInput.Width = w
		m.titleInput.Width = w
		m.textArea.SetWidth(w)
		m.textArea.SetHeight(10)
		m.createTitle.Width = w
		m.createDesc.SetWidth(w)
		m.createDesc.SetHeight(10)
		return m, nil

	case spinner.TickMsg:
		if m.loading || m.detailLoading || m.popupSaving || m.transitionSaving || m.assignLoading || m.assignSaving {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case fetchDoneMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.allIssues = msg.issues
		m.distributeIssues()
		m.clampCursor()
		return m, nil

	case fetchDetailMsg:
		m.detailLoading = false
		if msg.err != nil {
			if m.state == viewSearch {
				m.searchErr = msg.err.Error()
				return m, nil
			}
			m.err = msg.err
			return m, nil
		}
		m.detailIssue = msg.issue
		m.detailViewport.SetContent(m.renderDetailContent())
		m.state = viewDetail
		m.searchErr = ""
		return m, nil

	case actionDoneMsg:
		m.popupSaving = false
		if msg.err != nil {
			m.popupErr = msg.err.Error()
			return m, nil
		}
		key := m.detailIssue.Key
		m.state = viewDetail
		m.popupErr = ""
		m.detailLoading = true
		return m, tea.Batch(m.spinner.Tick, m.fetchDetail(key))

	case fetchTransitionsMsg:
		m.transitionSaving = false
		if msg.err != nil {
			m.transitionErr = msg.err.Error()
			return m, nil
		}
		m.transitions = msg.transitions
		m.transitionIdx = 0
		return m, nil

	case transitionDoneMsg:
		m.transitionSaving = false
		if msg.err != nil {
			m.transitionErr = msg.err.Error()
			return m, nil
		}
		key := m.detailIssue.Key
		m.state = viewDetail
		m.transitionErr = ""
		m.detailLoading = true
		// reload board issues in background so columns update
		return m, tea.Batch(m.spinner.Tick, m.fetchDetail(key), m.fetchIssues())

	case fetchIssueTypesMsg:
		if msg.err != nil {
			m.createErr = msg.err.Error()
			m.createSaving = false
			return m, nil
		}
		// Filter to common types only
		allowed := map[string]bool{
			"Task": true, "Bug": true, "Story": true,
			"Epic": true, "New Feature": true, "Improvement": true,
			"Sub-task": true, "Subtask": true,
		}
		m.createIssueTypes = nil
		for _, t := range msg.types {
			if allowed[t.Name] {
				m.createIssueTypes = append(m.createIssueTypes, t)
			}
		}
		if len(m.createIssueTypes) == 0 {
			m.createIssueTypes = msg.types // fallback: show all
		}
		m.createTypeIdx = 0
		m.createSaving = false
		return m, nil

	case fetchAssignableUsersMsg:
		m.assignLoading = false
		if msg.err != nil {
			m.assignErr = msg.err.Error()
			return m, nil
		}
		m.assignUsers = msg.users
		m.assignIdx = 0
		return m, nil

	case assignDoneMsg:
		m.assignSaving = false
		if msg.err != nil {
			m.assignErr = msg.err.Error()
			return m, nil
		}
		key := m.detailIssue.Key
		m.state = viewDetail
		m.assignErr = ""
		m.detailLoading = true
		return m, tea.Batch(m.spinner.Tick, m.fetchDetail(key), m.fetchIssues())

	case createDoneMsg:
		m.createSaving = false
		if msg.err != nil {
			m.createErr = msg.err.Error()
			return m, nil
		}
		// Success: open the new ticket's detail view
		m.state = viewDetail
		m.createErr = ""
		m.detailLoading = true
		return m, tea.Batch(m.spinner.Tick, m.fetchDetail(msg.key))

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	// Delegate non-key msgs to focused widgets
	switch m.state {
	case viewDetail:
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	case viewSearch:
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	case viewComment, viewEditDesc:
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		return m, cmd
	case viewEditTitle:
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	case viewCreate:
		switch m.createStep {
		case createStepTitle:
			var cmd tea.Cmd
			m.createTitle, cmd = m.createTitle.Update(msg)
			return m, cmd
		case createStepDesc:
			var cmd tea.Cmd
			m.createDesc, cmd = m.createDesc.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// ── key handling ──────────────────────────────────────────────────────────────

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// ── create ticket wizard ─────────────────────────────────────────────────
	if m.state == viewCreate {
		switch m.createStep {
		case createStepType:
			switch key {
			case "esc", "q":
				m.state = viewBoard
				m.createErr = ""
				return m, nil
			case "up", "k":
				if m.createTypeIdx > 0 {
					m.createTypeIdx--
				}
			case "down", "j":
				if m.createTypeIdx < len(m.createIssueTypes)-1 {
					m.createTypeIdx++
				}
			case "enter":
				if len(m.createIssueTypes) == 0 {
					return m, nil
				}
				m.createStep = createStepTitle
				m.createTitle.SetValue("")
				m.createErr = ""
				return m, m.createTitle.Focus()
			}
			return m, nil

		case createStepTitle:
			switch key {
			case "esc":
				m.createStep = createStepType
				m.createTitle.Blur()
				m.createErr = ""
				return m, nil
			case "enter":
				if strings.TrimSpace(m.createTitle.Value()) == "" {
					m.createErr = "title cannot be empty"
					return m, nil
				}
				m.createStep = createStepDesc
				m.createDesc.SetValue("")
				m.createErr = ""
				m.createTitle.Blur()
				return m, m.createDesc.Focus()
			}
			var cmd tea.Cmd
			m.createTitle, cmd = m.createTitle.Update(msg)
			return m, cmd

		case createStepDesc:
			switch key {
			case "esc":
				m.createStep = createStepTitle
				m.createDesc.Blur()
				m.createErr = ""
				return m, m.createTitle.Focus()
			case "ctrl+s":
				if m.createSaving {
					return m, nil
				}
				project := m.cfg.Project
				if project == "" {
					m.createErr = "no project configured — set PROJECT_ID or add project to config.toml"
					return m, nil
				}
				typeID := m.createIssueTypes[m.createTypeIdx].ID
				summary := strings.TrimSpace(m.createTitle.Value())
				desc := m.createDesc.Value()
				m.createSaving = true
				m.createErr = ""
				m.createDesc.Blur()
				return m, tea.Batch(m.spinner.Tick, m.cmdCreateIssue(project, typeID, summary, desc))
			}
			var cmd tea.Cmd
			m.createDesc, cmd = m.createDesc.Update(msg)
			return m, cmd
		}
	}

	// ── textarea popups (comment / edit desc) ────────────────────────────────
	if m.state == viewComment || m.state == viewEditDesc {
		switch key {
		case "esc":
			m.state = viewDetail
			m.textArea.Blur()
			m.popupErr = ""
			return m, nil
		case "ctrl+s":
			if m.popupSaving || m.detailIssue == nil {
				return m, nil
			}
			text := strings.TrimSpace(m.textArea.Value())
			if text == "" {
				m.popupErr = "text cannot be empty"
				return m, nil
			}
			m.popupSaving = true
			m.popupErr = ""
			issueKey := m.detailIssue.Key
			if m.state == viewComment {
				return m, tea.Batch(m.spinner.Tick, m.cmdAddComment(issueKey, text))
			}
			return m, tea.Batch(m.spinner.Tick, m.cmdUpdateDesc(issueKey, text))
		}
		var cmd tea.Cmd
		m.textArea, cmd = m.textArea.Update(msg)
		return m, cmd
	}

	// ── title edit popup ─────────────────────────────────────────────────────
	if m.state == viewEditTitle {
		switch key {
		case "esc":
			m.state = viewDetail
			m.titleInput.Blur()
			m.popupErr = ""
			return m, nil
		case "enter":
			if m.popupSaving || m.detailIssue == nil {
				return m, nil
			}
			text := strings.TrimSpace(m.titleInput.Value())
			if text == "" {
				m.popupErr = "title cannot be empty"
				return m, nil
			}
			m.popupSaving = true
			m.popupErr = ""
			return m, tea.Batch(m.spinner.Tick, m.cmdUpdateTitle(m.detailIssue.Key, text))
		}
		var cmd tea.Cmd
		m.titleInput, cmd = m.titleInput.Update(msg)
		return m, cmd
	}

	// ── search popup ─────────────────────────────────────────────────────────
	if m.state == viewSearch {
		switch key {
		case "esc":
			m.state = viewBoard
			m.searchInput.SetValue("")
			m.searchInput.Blur()
			m.searchErr = ""
			return m, nil
		case "enter":
			query := strings.TrimSpace(m.searchInput.Value())
			if query == "" {
				return m, nil
			}
			if issueKeyPattern.MatchString(query) {
				m.detailLoading = true
				m.searchErr = ""
				return m, tea.Batch(m.spinner.Tick, m.fetchDetail(strings.ToUpper(query)))
			}
			if m.effectiveProject != "" && !containsProjectFilter(query) {
				query = "project = " + m.effectiveProject + " AND " + query
			}
			m.jql = query
			m.loading = true
			m.searchErr = ""
			m.searchMode = true
			m.state = viewBoard
			m.searchInput.SetValue("")
			m.searchInput.Blur()
			return m, tea.Batch(m.spinner.Tick, m.fetchIssues())
		}
		var cmd tea.Cmd
		m.searchInput, cmd = m.searchInput.Update(msg)
		return m, cmd
	}

	// ── assign picker ────────────────────────────────────────────────────────
	if m.state == viewAssign {
		switch key {
		case "esc", "q":
			m.state = viewDetail
			m.assignErr = ""
			return m, nil
		case "up", "k":
			if m.assignIdx > 0 {
				m.assignIdx--
			}
		case "down", "j":
			// +1 for "Unassign" sentinel at top
			if m.assignIdx < len(m.assignUsers) {
				m.assignIdx++
			}
		case "enter":
			if m.assignSaving || m.assignLoading || m.detailIssue == nil {
				return m, nil
			}
			// index 0 = unassign, 1..n = assignUsers[idx-1]
			var accountID string
			if m.assignIdx > 0 {
				accountID = m.assignUsers[m.assignIdx-1].AccountID
			}
			m.assignSaving = true
			m.assignErr = ""
			return m, tea.Batch(m.spinner.Tick, m.cmdAssignIssue(m.detailIssue.Key, accountID))
		}
		return m, nil
	}

	// ── transition picker ────────────────────────────────────────────────────
	if m.state == viewTransition {
		switch key {
		case "esc", "q":
			m.state = viewDetail
			m.transitionErr = ""
			return m, nil
		case "up", "k":
			if m.transitionIdx > 0 {
				m.transitionIdx--
			}
		case "down", "j":
			if m.transitionIdx < len(m.transitions)-1 {
				m.transitionIdx++
			}
		case "enter":
			if m.transitionSaving || len(m.transitions) == 0 || m.detailIssue == nil {
				return m, nil
			}
			tid := m.transitions[m.transitionIdx].ID
			m.transitionSaving = true
			m.transitionErr = ""
			return m, tea.Batch(m.spinner.Tick, m.cmdTransitionIssue(m.detailIssue.Key, tid))
		}
		return m, nil
	}

	// ── detail view ──────────────────────────────────────────────────────────
	if m.state == viewDetail {
		switch key {
		case "q", "esc", "backspace":
			m.state = viewBoard
			m.detailIssue = nil
			return m, nil
		case "n":
			return m.openCreate()
		case "s":
			return m.openSearch()
		case "o":
			if m.detailIssue != nil {
				_ = exec.Command("open", m.client.BrowseURL(m.detailIssue.Key)).Start()
			}
			return m, nil
		case "m":
			return m.openTransition()
		case "c":
			return m.openComment()
		case "e":
			return m.openEditDesc()
		case "t":
			return m.openEditTitle()
		case "a":
			return m.openAssign()
		case "r":
			if m.detailIssue != nil {
				m.detailLoading = true
				return m, tea.Batch(m.spinner.Tick, m.fetchDetail(m.detailIssue.Key))
			}
		}
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	}

	// ── board view ───────────────────────────────────────────────────────────
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "n":
		return m.openCreate()
	case "s":
		return m.openSearch()
	case "left", "h", "shift+tab":
		if m.colIdx > 0 {
			m.colIdx--
			m.rowIdx = 0
		}
	case "right", "l", "tab":
		if m.colIdx < len(m.columns)-1 {
			m.colIdx++
			m.rowIdx = 0
		}
	case "up", "k":
		if m.rowIdx > 0 {
			m.rowIdx--
		}
	case "down", "j":
		col := m.columns[m.colIdx]
		if m.rowIdx < len(col.issues)-1 {
			m.rowIdx++
		}
	case "enter":
		col := m.columns[m.colIdx]
		if len(col.issues) > 0 && m.rowIdx < len(col.issues) {
			m.detailLoading = true
			return m, tea.Batch(m.spinner.Tick, m.fetchDetail(col.issues[m.rowIdx].Key))
		}
	case "o":
		col := m.columns[m.colIdx]
		if len(col.issues) > 0 && m.rowIdx < len(col.issues) {
			_ = exec.Command("open", m.client.BrowseURL(col.issues[m.rowIdx].Key)).Start()
		}
	case "m":
		col := m.columns[m.colIdx]
		if len(col.issues) > 0 && m.rowIdx < len(col.issues) {
			issue := col.issues[m.rowIdx]
			m.detailIssue = &issue
			return m.openTransition()
		}
	case "esc":
		if m.searchMode {
			m.jql = m.defaultJQL
			m.loading = true
			m.err = nil
			m.searchMode = false
			return m, tea.Batch(m.spinner.Tick, m.fetchIssues())
		}
	case "r":
		m.loading = true
		m.err = nil
		m.searchMode = false
		m.jql = m.defaultJQL
		return m, tea.Batch(m.spinner.Tick, m.fetchIssues())
	}
	return m, nil
}

// ── popup helpers ─────────────────────────────────────────────────────────────

func (m Model) openCreate() (Model, tea.Cmd) {
	project := m.cfg.Project
	if project == "" {
		m.createErr = "no project configured — set PROJECT_ID env var or add project = \"KEY\" to config.toml"
		m.createIssueTypes = nil
		m.createStep = createStepType
		m.state = viewCreate
		return m, nil
	}
	m.state = viewCreate
	m.createStep = createStepType
	m.createTypeIdx = 0
	m.createErr = ""
	m.createSaving = true // show spinner while loading types
	return m, tea.Batch(m.spinner.Tick, m.cmdFetchIssueTypes(project))
}

func (m Model) openSearch() (Model, tea.Cmd) {
	m.state = viewSearch
	m.searchInput.SetValue("")
	m.searchErr = ""
	return m, m.searchInput.Focus()
}

func (m Model) openComment() (Model, tea.Cmd) {
	m.state = viewComment
	m.popupTitle = "Add Comment  (ctrl+s to save · esc to cancel)"
	m.popupErr = ""
	m.textArea.SetValue("")
	m.textArea.Placeholder = "Write your comment…"
	return m, m.textArea.Focus()
}

func (m Model) openEditDesc() (Model, tea.Cmd) {
	m.state = viewEditDesc
	m.popupTitle = "Edit Description  (ctrl+s to save · esc to cancel)"
	m.popupErr = ""
	existing := ""
	if m.detailIssue != nil {
		existing = strings.TrimSpace(jira.ExtractText(m.detailIssue.Fields.Description))
	}
	m.textArea.SetValue(existing)
	m.textArea.Placeholder = "Write description…"
	return m, m.textArea.Focus()
}

func (m Model) openEditTitle() (Model, tea.Cmd) {
	m.state = viewEditTitle
	m.popupTitle = "Edit Title  (enter to save · esc to cancel)"
	m.popupErr = ""
	existing := ""
	if m.detailIssue != nil {
		existing = m.detailIssue.Fields.Summary
	}
	m.titleInput.SetValue(existing)
	m.titleInput.CursorEnd()
	return m, m.titleInput.Focus()
}

func (m Model) openTransition() (Model, tea.Cmd) {
	if m.detailIssue == nil {
		return m, nil
	}
	m.state = viewTransition
	m.transitionIdx = 0
	m.transitionErr = ""
	m.transitions = nil
	m.transitionSaving = true
	return m, tea.Batch(m.spinner.Tick, m.cmdFetchTransitions(m.detailIssue.Key))
}

func (m Model) openAssign() (Model, tea.Cmd) {
	if m.detailIssue == nil {
		return m, nil
	}
	m.state = viewAssign
	m.assignIdx = 0
	m.assignErr = ""
	m.assignUsers = nil
	m.assignLoading = true
	return m, tea.Batch(m.spinner.Tick, m.cmdFetchAssignableUsers(m.detailIssue.Key))
}

// ── distribute ────────────────────────────────────────────────────────────────

func (m *Model) distributeIssues() {
	for i := range m.columns {
		m.columns[i].issues = nil
	}
	if m.searchMode {
		// Show all results in a single flat list, ignoring column/status mapping.
		if len(m.columns) == 0 {
			m.columns = []column{{title: "Search Results"}}
		}
		m.columns[0].issues = append(m.columns[0].issues, m.allIssues...)
		m.colIdx = 0
		return
	}
	nameToCol := make(map[string]int, len(m.cfg.Columns)*4)
	for i, def := range m.cfg.Columns {
		for _, s := range def.Statuses {
			nameToCol[strings.ToLower(s)] = i
		}
	}
	for _, issue := range m.allIssues {
		statusName := strings.ToLower(issue.Fields.Status.Name)
		catKey := issue.Fields.Status.StatusCategory.Key
		if idx, ok := nameToCol[statusName]; ok {
			m.columns[idx].issues = append(m.columns[idx].issues, issue)
			continue
		}
		switch catKey {
		case "indeterminate":
			if idx := m.colByColor("inprogress"); idx >= 0 {
				m.columns[idx].issues = append(m.columns[idx].issues, issue)
			}
		case "done":
			if idx := m.colByColor("done"); idx >= 0 {
				m.columns[idx].issues = append(m.columns[idx].issues, issue)
			}
		}
	}
}

func (m *Model) colByColor(color string) int {
	for i, c := range m.columns {
		if c.colorKey == color {
			return i
		}
	}
	return -1
}

func (m *Model) clampCursor() {
	if m.colIdx >= len(m.columns) {
		m.colIdx = len(m.columns) - 1
	}
	if m.colIdx < 0 {
		m.colIdx = 0
	}
	col := m.columns[m.colIdx]
	if len(col.issues) == 0 {
		m.rowIdx = 0
		return
	}
	if m.rowIdx >= len(col.issues) {
		m.rowIdx = len(col.issues) - 1
	}
	if m.rowIdx < 0 {
		m.rowIdx = 0
	}
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.err != nil {
		return errorStyle.Render("Error: "+m.err.Error()) + "\n" +
			helpStyle.Render("  q quit    r retry")
	}
	if m.loading {
		return "\n  " + m.spinner.View() + " " + loadingStyle.Render("Fetching Jira issues…")
	}

	switch m.state {
	case viewSearch:
		return m.renderSearchOverlay()
	case viewComment, viewEditDesc:
		return m.renderTextAreaPopup()
	case viewEditTitle:
		return m.renderTitlePopup()
	case viewCreate:
		return m.renderCreate()
	case viewTransition:
		return m.renderTransition()
	case viewAssign:
		return m.renderAssign()
	case viewDetail:
		return m.renderDetail()
	default:
		return m.renderBoard()
	}
}

// ── board ─────────────────────────────────────────────────────────────────────

func (m Model) renderBoard() string {
	if m.width == 0 {
		return ""
	}
	numCols := len(m.columns)
	colWidth := m.width / numCols

	const ticketH = 5
	boardHeight := m.height - 3
	maxVisible := (boardHeight - 1) / ticketH
	if maxVisible < 1 {
		maxVisible = 1
	}

	total := 0
	for _, c := range m.columns {
		total += len(c.issues)
	}

	var titleText string
	if m.searchMode {
		titleText = fmt.Sprintf("  Search Results  —  %d issues  (r: back to board)", total)
	} else {
		titleText = fmt.Sprintf("  Jira Board  —  %d issues", total)
	}
	title := titleStyle.Width(m.width).Render(titleText)
	sep := sepStyle(m.width)

	var cols []string
	if m.searchMode {
		cols = append(cols, m.renderColumn(m.columns[0], 0, m.width, maxVisible))
	} else {
		for i, col := range m.columns {
			cols = append(cols, m.renderColumn(col, i, colWidth, maxVisible))
		}
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	var help string
	if m.searchMode {
		help = helpStyle.Render("  ↑/↓  tickets    enter  detail    o  open web    s  new search    esc  back to board    q  quit")
	} else {
		help = helpStyle.Render("  ←/→  columns    ↑/↓  tickets    enter  detail    m  move    o  open web    n  new ticket    s  search    r  refresh    q  quit")
	}
	return lipgloss.JoinVertical(lipgloss.Left, title, sep, board, help)
}

func (m Model) renderColumn(col column, colI int, colWidth, maxVisible int) string {
	color := columnColor(col.colorKey)
	active := colI == m.colIdx

	cursor := "  "
	if active {
		cursor = "▶ "
	}
	hStyle := lipgloss.NewStyle().Bold(true).Width(colWidth).Padding(0, 1)
	if active {
		hStyle = hStyle.Foreground(lipgloss.Color("#000000")).Background(color)
	} else {
		hStyle = hStyle.Foreground(color).Background(lipgloss.Color("#1F2937"))
	}
	header := hStyle.Render(fmt.Sprintf("%s%s (%d)", cursor, col.title, len(col.issues)))

	start := 0
	if active && m.rowIdx >= maxVisible {
		start = m.rowIdx - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(col.issues) {
		end = len(col.issues)
	}

	var tickets []string
	for idx := start; idx < end; idx++ {
		tickets = append(tickets, m.renderTicket(col.issues[idx], active && idx == m.rowIdx, colWidth))
	}
	if len(col.issues) > maxVisible {
		tickets = append(tickets, dimStyle.Render(fmt.Sprintf("  %d–%d of %d", start+1, end, len(col.issues))))
	}

	return lipgloss.NewStyle().Width(colWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, tickets...)...),
	)
}

func (m Model) renderTicket(issue jira.Issue, selected bool, colWidth int) string {
	f := issue.Fields
	innerWidth := colWidth - 4

	typeColor := issueTypeColor(f.IssueType.Name)
	line1 := lipgloss.NewStyle().Foreground(typeColor).Bold(true).Render("["+issueTypeIcon(f.IssueType.Name)+"]") +
		" " + lipgloss.NewStyle().Foreground(priorityColor(f.Priority.Name)).Render(priorityIcon(f.Priority.Name)) +
		" " + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24")).Render(issue.Key)
	if m.searchMode {
		statusLabel := dimStyle.Render("[" + f.Status.Name + "]")
		line1 += " " + statusLabel
	}

	maxW := innerWidth - 2
	if maxW < 4 {
		maxW = 4
	}
	summary := truncate(f.Summary, maxW)

	assigneeName := "unassigned"
	if f.Assignee != nil {
		assigneeName = "@" + truncate(f.Assignee.DisplayName, innerWidth-3)
	}

	if selected {
		summary = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Render(summary)
		assigneeName = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1D5DB")).Render(assigneeName)
		return lipgloss.NewStyle().
			Width(innerWidth).Padding(0, 1).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#F59E0B")).
			Background(lipgloss.Color("#1C1917")).Bold(true).
			Render(lipgloss.JoinVertical(lipgloss.Left, line1, summary, assigneeName))
	}

	assigneeName = dimStyle.Render(assigneeName)
	return lipgloss.NewStyle().
		Width(innerWidth).Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#374151")).
		Render(lipgloss.JoinVertical(lipgloss.Left, line1, summary, assigneeName))
}

// ── detail ────────────────────────────────────────────────────────────────────

func (m Model) renderDetail() string {
	if m.detailLoading {
		return "\n  " + m.spinner.View() + " " + loadingStyle.Render("Loading issue details…")
	}
	if m.detailIssue == nil {
		return ""
	}
	f := m.detailIssue.Fields
	header := lipgloss.NewStyle().Bold(true).
		Foreground(issueTypeColor(f.IssueType.Name)).
		Background(lipgloss.Color("#1F2937")).
		Width(m.width).Padding(0, 1).
		Render(fmt.Sprintf("[%s]  %s  —  %s", f.IssueType.Name, m.detailIssue.Key, f.Summary))

	sep := sepStyle(m.width)
	m.detailViewport.SetContent(m.renderDetailContent())
	help := helpStyle.Render("  ↑/↓  scroll    m  move    a  assign    c  comment    e  edit desc    t  edit title    o  open web    n  new ticket    s  search    r  refresh    esc  back")
	return lipgloss.JoinVertical(lipgloss.Left, header, sep, m.detailViewport.View(), sep, help)
}

func (m Model) renderDetailContent() string {
	if m.detailIssue == nil {
		return ""
	}
	f := m.detailIssue.Fields

	row := func(label, value string) string {
		return detailLabelStyle.Render(label+":") + " " + value
	}

	statusStr := lipgloss.NewStyle().Foreground(statusColor(f.Status.StatusCategory.Key)).Bold(true).Render(f.Status.Name)
	assignee := "Unassigned"
	if f.Assignee != nil {
		assignee = f.Assignee.DisplayName
	}
	reporter := ""
	if f.Reporter != nil {
		reporter = f.Reporter.DisplayName
	}

	var sb strings.Builder
	sb.WriteString(row("Status", statusStr) + "\n")
	sb.WriteString(row("Type", lipgloss.NewStyle().Foreground(issueTypeColor(f.IssueType.Name)).Render(f.IssueType.Name)) + "\n")
	sb.WriteString(row("Priority", lipgloss.NewStyle().Foreground(priorityColor(f.Priority.Name)).Render(f.Priority.Name)) + "\n")
	sb.WriteString(row("Assignee", assignee) + "\n")
	if reporter != "" {
		sb.WriteString(row("Reporter", reporter) + "\n")
	}
	sb.WriteString(row("Created", formatTime(f.Created)) + "\n")
	sb.WriteString(row("Updated", formatTime(f.Updated)) + "\n")
	if len(f.Labels) > 0 {
		sb.WriteString(row("Labels", strings.Join(f.Labels, ", ")) + "\n")
	}

	// Description
	sb.WriteString("\n" + sectionHeader("Description") + "\n")
	sb.WriteString(strings.Repeat("─", 60) + "\n")
	desc := strings.TrimSpace(jira.ExtractText(f.Description))
	if desc == "" {
		desc = dimStyle.Render("(no description)")
	}
	sb.WriteString(desc + "\n")

	// Comments
	if f.Comment.Total > 0 {
		sb.WriteString("\n" + sectionHeader(fmt.Sprintf("Comments (%d)", f.Comment.Total)) + "\n")
		sb.WriteString(strings.Repeat("─", 60) + "\n")
		for _, c := range f.Comment.Comments {
			author := lipgloss.NewStyle().Bold(true).Render(c.Author.DisplayName)
			ts := dimStyle.Render(formatTime(c.Created))
			sb.WriteString(author + "  " + ts + "\n")
			body := strings.TrimSpace(jira.ExtractText(c.Body))
			if body != "" {
				sb.WriteString(body + "\n")
			}
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// ── popups ────────────────────────────────────────────────────────────────────

func (m Model) renderTextAreaPopup() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24")).Render(m.popupTitle)
	var errRow string
	if m.popupErr != "" {
		errRow = "\n" + errorStyle.Render("✗ "+m.popupErr)
	}
	var savingRow string
	if m.popupSaving {
		savingRow = "\n" + m.spinner.View() + " saving…"
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", m.textArea.View(), errRow, savingRow)
	box := popupBoxStyle(m.searchInput.Width+12).Render(inner)
	return centerPopup(box, m.renderBoard(), m.width, m.height)
}

func (m Model) renderTitlePopup() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24")).Render(m.popupTitle)
	var errRow string
	if m.popupErr != "" {
		errRow = "\n" + errorStyle.Render("✗ "+m.popupErr)
	}
	var savingRow string
	if m.popupSaving {
		savingRow = "\n" + m.spinner.View() + " saving…"
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", m.titleInput.View(), errRow, savingRow)
	box := popupBoxStyle(m.searchInput.Width+12).Render(inner)
	return centerPopup(box, m.renderBoard(), m.width, m.height)
}

func (m Model) renderSearchOverlay() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24")).Render("  Search")
	hint := dimStyle.Render("  ticket ID (e.g. OBS-123) → open detail\n  JQL query  (e.g. project = OBS AND assignee = currentUser()) → load board")
	var errRow string
	if m.searchErr != "" {
		errRow = "\n  " + errorStyle.Render("✗ "+m.searchErr)
	}
	var loadRow string
	if m.detailLoading {
		loadRow = "\n  " + m.spinner.View() + " looking up ticket…"
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", "  "+m.searchInput.View(), "", hint, errRow, loadRow)
	box := popupBoxStyle(m.searchInput.Width + 8).Render(inner)
	help := helpStyle.Render("  enter  confirm    esc  cancel")
	return centerPopup(box+"\n"+help, m.renderBoard(), m.width, m.height)
}

// centerPopup overlays popup lines over a dimmed board at 30% from top.
func centerPopup(popup, board string, width, height int) string {
	leftPad := (width - lipgloss.Width(strings.SplitN(popup, "\n", 2)[0])) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	topPad := height / 4

	boardLines := strings.Split(board, "\n")
	popupLines := strings.Split(popup, "\n")

	lines := make([]string, height)
	for i := range lines {
		bl := ""
		if i < len(boardLines) {
			bl = boardLines[i]
		}
		lines[i] = dimStyle.Render(bl)
	}
	for i, pl := range popupLines {
		row := topPad + i
		if row >= height {
			break
		}
		lines[row] = strings.Repeat(" ", leftPad) + pl
	}
	return strings.Join(lines, "\n")
}

// ── create ────────────────────────────────────────────────────────────────────

func (m Model) renderCreate() string {
	var inner string
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24")).
		Render("  New Ticket" + func() string {
			if m.cfg.Project != "" {
				return "  [" + m.cfg.Project + "]"
			}
			return ""
		}())

	var errRow string
	if m.createErr != "" {
		errRow = "\n" + errorStyle.Render("✗ "+m.createErr)
	}

	switch m.createStep {
	case createStepType:
		if m.createSaving {
			inner = lipgloss.JoinVertical(lipgloss.Left,
				title, "",
				"  "+m.spinner.View()+" loading issue types…",
				errRow,
			)
		} else if len(m.createIssueTypes) == 0 {
			inner = lipgloss.JoinVertical(lipgloss.Left, title, "", errRow,
				dimStyle.Render("  esc cancel"),
			)
		} else {
			var rows []string
			rows = append(rows, title, "", dimStyle.Render("  Select issue type:"), "")
			for i, t := range m.createIssueTypes {
				cursor := "  "
				style := lipgloss.NewStyle()
				if i == m.createTypeIdx {
					cursor = "▶ "
					style = style.Bold(true).Foreground(lipgloss.Color("#FBBF24"))
				}
				icon := lipgloss.NewStyle().Foreground(issueTypeColor(t.Name)).Render("[" + issueTypeIcon(t.Name) + "]")
				rows = append(rows, "  "+cursor+icon+" "+style.Render(t.Name))
			}
			rows = append(rows, "", errRow, helpStyle.Render("  ↑/↓  select    enter  confirm    esc  cancel"))
			inner = lipgloss.JoinVertical(lipgloss.Left, rows...)
		}

	case createStepTitle:
		selectedType := m.createIssueTypes[m.createTypeIdx].Name
		typeLabel := lipgloss.NewStyle().Foreground(issueTypeColor(selectedType)).Bold(true).
			Render("[" + issueTypeIcon(selectedType) + "] " + selectedType)
		inner = lipgloss.JoinVertical(lipgloss.Left,
			title, "",
			"  Type: "+typeLabel, "",
			dimStyle.Render("  Summary:"),
			"  "+m.createTitle.View(),
			"", errRow,
			helpStyle.Render("  enter  next    esc  back"),
		)

	case createStepDesc:
		selectedType := m.createIssueTypes[m.createTypeIdx].Name
		typeLabel := lipgloss.NewStyle().Foreground(issueTypeColor(selectedType)).Bold(true).
			Render("[" + issueTypeIcon(selectedType) + "] " + selectedType)
		var savingRow string
		if m.createSaving {
			savingRow = "\n" + m.spinner.View() + " creating…"
		}
		inner = lipgloss.JoinVertical(lipgloss.Left,
			title, "",
			"  Type: "+typeLabel,
			"  Summary: "+lipgloss.NewStyle().Bold(true).Render(m.createTitle.Value()), "",
			dimStyle.Render("  Description (optional):"),
			m.createDesc.View(),
			"", errRow, savingRow,
			helpStyle.Render("  ctrl+s  create    esc  back"),
		)
	}

	boxW := m.createTitle.Width + 12
	box := popupBoxStyle(boxW).Render(inner)
	return centerPopup(box, m.renderBoard(), m.width, m.height)
}

// ── transition ────────────────────────────────────────────────────────────────

func (m Model) renderTransition() string {
	issueKey := ""
	currentStatus := ""
	if m.detailIssue != nil {
		issueKey = m.detailIssue.Key
		currentStatus = m.detailIssue.Fields.Status.Name
	}

	titleText := "  Move  " + issueKey
	if currentStatus != "" {
		titleText += "  [" + currentStatus + "]"
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24")).Render(titleText)

	var body string
	if m.transitionSaving && len(m.transitions) == 0 {
		body = "  " + m.spinner.View() + " loading transitions…"
	} else if len(m.transitions) == 0 {
		body = dimStyle.Render("  no transitions available")
	} else {
		var rows []string
		rows = append(rows, dimStyle.Render("  Select target status:"), "")
		for i, t := range m.transitions {
			cursor := "  "
			style := lipgloss.NewStyle()
			if i == m.transitionIdx {
				cursor = "▶ "
				style = style.Bold(true).Foreground(lipgloss.Color("#34D399"))
			}
			rows = append(rows, "  "+cursor+style.Render(t.Name))
		}
		body = lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	var errRow string
	if m.transitionErr != "" {
		errRow = "\n" + errorStyle.Render("✗ "+m.transitionErr)
	}
	var savingRow string
	if m.transitionSaving && len(m.transitions) > 0 {
		savingRow = "\n" + m.spinner.View() + " transitioning…"
	}

	help := helpStyle.Render("  ↑/↓  select    enter  confirm    esc  cancel")
	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", body, errRow, savingRow, "", help)
	w := m.searchInput.Width + 8
	if w < 50 {
		w = 50
	}
	box := popupBoxStyle(w).Render(inner)
	return centerPopup(box, m.renderBoard(), m.width, m.height)
}

// ── assign ────────────────────────────────────────────────────────────────────

func (m Model) renderAssign() string {
	issueKey := ""
	if m.detailIssue != nil {
		issueKey = m.detailIssue.Key
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FBBF24")).
		Render("  Assign  " + issueKey)

	var body string
	if m.assignLoading {
		body = "  " + m.spinner.View() + " loading assignable users…"
	} else {
		// Build list: index 0 = Unassign, 1..n = users
		type entry struct {
			label     string
			accountID string
		}
		entries := []entry{{label: "(unassign)"}}
		for _, u := range m.assignUsers {
			label := u.DisplayName
			if u.EmailAddress != "" {
				label += "  " + dimStyle.Render("<"+u.EmailAddress+">")
			}
			entries = append(entries, entry{label: label, accountID: u.AccountID})
		}

		var rows []string
		rows = append(rows, dimStyle.Render("  Select assignee:"), "")
		for i, e := range entries {
			cursor := "  "
			style := lipgloss.NewStyle()
			if i == m.assignIdx {
				cursor = "▶ "
				style = style.Bold(true).Foreground(lipgloss.Color("#34D399"))
			}
			rows = append(rows, "  "+cursor+style.Render(e.label))
		}
		body = lipgloss.JoinVertical(lipgloss.Left, rows...)
	}

	var errRow string
	if m.assignErr != "" {
		errRow = "\n" + errorStyle.Render("✗ "+m.assignErr)
	}
	var savingRow string
	if m.assignSaving {
		savingRow = "\n" + m.spinner.View() + " assigning…"
	}

	help := helpStyle.Render("  ↑/↓  select    enter  confirm    esc  cancel")
	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", body, errRow, savingRow, "", help)
	w := m.searchInput.Width + 8
	if w < 50 {
		w = 50
	}
	box := popupBoxStyle(w).Render(inner)
	return centerPopup(box, m.renderBoard(), m.width, m.height)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func popupBoxStyle(width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FBBF24")).
		Padding(1, 2).
		Width(width)
}

func sepStyle(width int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#374151")).Render(strings.Repeat("─", width))
}

func sectionHeader(s string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9CA3AF")).Render(s)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

func columnColor(key string) lipgloss.Color {
	switch key {
	case "inprogress":
		return colorInProgress
	case "done":
		return colorDone
	case "review":
		return colorReview
	case "blocked":
		return colorBlocked
	}
	return colorTodo
}

// containsProjectFilter reports whether a JQL query already scopes to a project.
func containsProjectFilter(jql string) bool {
	return strings.Contains(strings.ToLower(jql), "project")
}

// extractProjectFromJQL pulls the project key out of a simple
// "project = KEY ..." JQL string. Returns "" if not found.
var projectJQLRe = regexp.MustCompile(`(?i)\bproject\s*=\s*"?([A-Z][A-Z0-9]*)`)

func extractProjectFromJQL(jql string) string {
	if m := projectJQLRe.FindStringSubmatch(jql); len(m) > 1 {
		return strings.ToUpper(m[1])
	}
	return ""
}

func formatTime(s string) string {
	if s == "" {
		return ""
	}
	for _, layout := range []string{"2006-01-02T15:04:05.999-0700", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02 15:04")
		}
	}
	return s
}
