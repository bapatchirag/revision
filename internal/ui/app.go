package ui

import (
	"fmt"
	"strings"

	"github.com/bapatchirag/revision/internal/svn"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the root Bubble Tea model for the revision TUI.
type Model struct {
	client *svn.Client
	info   *svn.Info

	list list.Model
	keys keyMap
	help help.Model

	width  int
	height int

	loading bool
	err     error
}

// New creates the root model for the given client and working-copy info.
func New(client *svn.Client, info *svn.Info) Model {
	return Model{
		client:  client,
		info:    info,
		list:    newStatusList(),
		keys:    defaultKeys(),
		help:    help.New(),
		loading: true,
	}
}

// Init loads the initial working-copy status.
func (m Model) Init() tea.Cmd {
	return loadStatusCmd(m.client)
}

// Update handles messages and key input.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.list.SetSize(msg.Width, m.listHeight())
		return m, nil

	case statusLoadedMsg:
		m.loading = false
		m.err = nil
		m.list.SetItems(rowsFromItems(msg.items))
		return m, nil

	case errMsg:
		m.loading = false
		m.err = msg.err
		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Refresh):
			m.loading = true
			return m, loadStatusCmd(m.client)
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			m.list.SetSize(m.width, m.listHeight())
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View renders the full TUI.
func (m Model) View() string {
	if m.err != nil {
		return strings.Join([]string{
			m.headerView(),
			"",
			errorStyle.Render("Error: " + m.err.Error()),
			"",
			subtleStyle.Render("Press r to retry • q to quit"),
		}, "\n")
	}

	var body string
	switch {
	case m.loading && len(m.list.Items()) == 0:
		body = subtleStyle.Render("Loading working-copy status…")
	case len(m.list.Items()) == 0:
		body = subtleStyle.Render("Working copy is clean — no changes.")
	default:
		body = m.list.View()
	}

	return strings.Join([]string{
		m.headerView(),
		body,
		m.footerView(),
	}, "\n")
}

func (m Model) headerView() string {
	header := titleStyle.Render("revision")
	if m.info != nil {
		header += subtleStyle.Render(fmt.Sprintf("  %s @ r%s", m.info.URL, m.info.Revision))
	}
	header += subtleStyle.Render(fmt.Sprintf("  (%d changes)", len(m.list.Items())))
	return header
}

func (m Model) footerView() string {
	return m.help.View(m.keys)
}

// listHeight returns the number of rows available for the status list.
func (m Model) listHeight() int {
	reserved := 4 // header + blank + short help
	if m.help.ShowAll {
		reserved = 6
	}
	if h := m.height - reserved; h > 0 {
		return h
	}
	return 1
}
