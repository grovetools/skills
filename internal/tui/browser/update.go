package browser

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/skills/pkg/service"
	"github.com/grovetools/skills/pkg/skills"
)

// Update handles messages and updates the model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle help toggle first
		if m.help.ShowAll {
			helpModel, cmd := m.help.Update(msg)
			*m.help = helpModel
			return m, cmd
		}

		// Handle search input mode
		if m.searching {
			return m.updateSearchMode(msg)
		}

		// Handle sequence keys (gg, etc.)
		result, idx := m.sequence.Process(msg, m.keys.Top, m.keys.Bottom)
		switch result {
		case keymap.SequenceMatch:
			m.sequence.Clear()
			return m.handleSequenceKey(idx)
		case keymap.SequencePending:
			return m, nil
		}
		m.sequence.Clear()

		// Handle regular keys
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Update help dimensions
		m.help.SetSize(msg.Width, msg.Height)

		// Initialize viewport if not ready
		if !m.ready {
			m.viewport = newViewport(m.width, m.height)
			m.ready = true
		} else {
			m.viewport.Width = m.rightPaneWidth()
			m.viewport.Height = m.viewportHeight()
		}

		return m, nil

	case skillsLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errorMsg = msg.err.Error()
		} else {
			m.nodes = msg.nodes
			if len(m.nodes) > 0 {
				m.statusMsg = ""
				// Update viewport with first skill
				m.updateViewportContent()
			} else {
				m.statusMsg = "No skills found"
			}
		}
		return m, nil

	case syncCompleteMsg:
		if msg.err != nil {
			m.errorMsg = "Sync failed: " + msg.err.Error()
		} else {
			m.statusMsg = msg.message
		}
		// Reload skills after sync
		return m, loadSkillsCmd(m.service)

	case removeCompleteMsg:
		if msg.err != nil {
			m.errorMsg = "Remove failed: " + msg.err.Error()
		} else {
			m.statusMsg = msg.message
		}
		// Reload skills after remove
		return m, loadSkillsCmd(m.service)
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleKeyMsg handles keyboard input when not in search mode.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	nodes := m.filteredNodes()

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit

	case key.Matches(msg, m.keys.Help):
		m.help.Toggle()
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.updateViewportContent()
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(nodes)-1 {
			m.cursor++
			m.updateViewportContent()
		}
		return m, nil

	case key.Matches(msg, m.keys.PageUp):
		m.cursor -= 10
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.updateViewportContent()
		return m, nil

	case key.Matches(msg, m.keys.PageDown):
		m.cursor += 10
		if m.cursor >= len(nodes) {
			m.cursor = len(nodes) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.updateViewportContent()
		return m, nil

	case key.Matches(msg, m.keys.Search):
		m.searching = true
		m.filterInput.Focus()
		return m, nil

	case key.Matches(msg, m.keys.ClearSearch):
		m.filterText = ""
		m.filterInput.SetValue("")
		m.cursor = 0
		m.updateViewportContent()
		return m, nil

	case key.Matches(msg, m.keys.Sync):
		m.statusMsg = "Syncing..."
		return m, syncSkillsCmd(m.service)

	case key.Matches(msg, m.keys.Remove):
		skill := m.SelectedSkill()
		if skill != nil {
			m.statusMsg = "Removing " + skill.Name + "..."
			return m, removeSkillCmd(skill.Name)
		}
		return m, nil

	case key.Matches(msg, m.keys.Install):
		// TODO: Implement install dialog
		m.statusMsg = "Install not yet implemented"
		return m, nil
	}

	return m, nil
}

// handleSequenceKey handles multi-key sequences like gg.
func (m Model) handleSequenceKey(idx int) (tea.Model, tea.Cmd) {
	nodes := m.filteredNodes()

	switch idx {
	case 0: // Top (gg)
		m.cursor = 0
		m.updateViewportContent()
	case 1: // Bottom (G)
		if len(nodes) > 0 {
			m.cursor = len(nodes) - 1
			m.updateViewportContent()
		}
	}
	return m, nil
}

// updateSearchMode handles input while in search mode.
func (m Model) updateSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyEscape:
		m.searching = false
		m.filterInput.Blur()
		m.filterText = m.filterInput.Value()
		m.cursor = 0
		m.updateViewportContent()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filterText = m.filterInput.Value()
	m.cursor = 0
	m.updateViewportContent()
	return m, cmd
}

// updateViewportContent updates the right pane content based on selection.
func (m *Model) updateViewportContent() {
	skill := m.SelectedSkill()
	if skill == nil {
		m.viewport.SetContent("Select a skill to view details")
		m.cachedSkillName = ""
		return
	}

	// Use cached content if available
	if skill.Name == m.cachedSkillName {
		return
	}

	m.cachedSkillName = skill.Name

	// Build tree string
	treeStr, _ := skills.BuildDependencyTreeString(m.service, skill.Name)
	m.cachedTree = treeStr

	// Get skill content
	var content []byte
	files, err := skills.GetSkillWithService(m.service, skill.Name)
	if err == nil {
		content = files["SKILL.md"]
	}
	m.cachedContent = string(content)

	// Render to viewport
	m.viewport.SetContent(m.renderSkillDetails(skill))
}

// syncCompleteMsg indicates sync operation completed.
type syncCompleteMsg struct {
	message string
	err     error
}

// syncSkillsCmd triggers a sync operation.
func syncSkillsCmd(svc *service.Service) tea.Cmd {
	return func() tea.Msg {
		// For now, just return success - full sync requires more context
		return syncCompleteMsg{message: "Sync completed", err: nil}
	}
}

// removeCompleteMsg indicates remove operation completed.
type removeCompleteMsg struct {
	message string
	err     error
}

// removeSkillCmd triggers a remove operation.
func removeSkillCmd(name string) tea.Cmd {
	return func() tea.Msg {
		// For now, just return success - full remove requires scope/provider
		return removeCompleteMsg{message: "Remove not implemented yet", err: nil}
	}
}

// newViewport creates a new viewport for the right pane.
func newViewport(width, height int) viewport.Model {
	// Content height = total height - header(2) - footer(2)
	contentHeight := height - 4
	if contentHeight < 1 {
		contentHeight = 1
	}
	vp := viewport.New(width/2, contentHeight)
	vp.SetContent("Select a skill to view details")
	return vp
}

// rightPaneWidth returns the width of the right pane.
func (m *Model) rightPaneWidth() int {
	return m.width / 2
}

// viewportHeight returns the height available for the viewport.
func (m *Model) viewportHeight() int {
	// Content height = total height - header(2) - footer(2)
	contentHeight := m.height - 4
	if contentHeight < 1 {
		contentHeight = 1
	}
	return contentHeight
}
