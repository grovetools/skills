package browser

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/atotto/clipboard"
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

		// Handle preview pane focus mode - route navigation keys to viewport
		if m.previewFocused {
			// Handle sequence keys (gg, G) for viewport navigation
			result, idx := m.sequence.Process(msg, m.keys.Top, m.keys.Bottom)
			switch result {
			case keymap.SequenceMatch:
				m.sequence.Clear()
				switch idx {
				case 0: // Top (gg)
					m.viewport.GotoTop()
				case 1: // Bottom (G)
					m.viewport.GotoBottom()
				}
				return m, nil
			case keymap.SequencePending:
				return m, nil
			}
			m.sequence.Clear()

			switch {
			case key.Matches(msg, m.keys.SwitchView), key.Matches(msg, m.keys.Back):
				// Tab or Esc returns focus to left pane
				m.previewFocused = false
				return m, nil
			case msg.Type == tea.KeyShiftTab:
				// Shift+Tab also returns focus to left pane
				m.previewFocused = false
				return m, nil
			case key.Matches(msg, m.keys.Quit):
				return m, tea.Quit
			case key.Matches(msg, m.keys.Help):
				m.help.Toggle()
				return m, nil
			case msg.Type == tea.KeyCtrlD:
				// Half-page down
				m.viewport.HalfViewDown()
				return m, nil
			case msg.Type == tea.KeyCtrlU:
				// Half-page up
				m.viewport.HalfViewUp()
				return m, nil
			case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'j':
				// Scroll down one line
				m.viewport.LineDown(1)
				return m, nil
			case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'k':
				// Scroll up one line
				m.viewport.LineUp(1)
				return m, nil
			case msg.Type == tea.KeyDown:
				// Arrow down
				m.viewport.LineDown(1)
				return m, nil
			case msg.Type == tea.KeyUp:
				// Arrow up
				m.viewport.LineUp(1)
				return m, nil
			case key.Matches(msg, m.keys.PageDown):
				// Page down
				m.viewport.ViewDown()
				return m, nil
			case key.Matches(msg, m.keys.PageUp):
				// Page up
				m.viewport.ViewUp()
				return m, nil
			default:
				// Route all other keys to the viewport
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
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
				// Update viewport width now that we know the left pane width
				if m.ready {
					m.viewport.Width = m.rightPaneWidth()
				}
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
		return m, loadSkillsCmd(m.service, m.currentNode)

	case removeCompleteMsg:
		if msg.err != nil {
			m.errorMsg = "Remove failed: " + msg.err.Error()
		} else {
			m.statusMsg = msg.message
		}
		// Reload skills after remove
		return m, loadSkillsCmd(m.service, m.currentNode)

	case editCompleteMsg:
		if msg.err != nil {
			m.errorMsg = "Edit failed: " + msg.err.Error()
		}
		// Reload skills after edit to refresh any changes
		return m, loadSkillsCmd(m.service, m.currentNode)

	case toggleCompleteMsg:
		if msg.err != nil {
			m.errorMsg = "Toggle failed: " + msg.err.Error()
		} else {
			m.statusMsg = msg.message
		}
		// Reload skills after toggle to refresh the view
		return m, loadSkillsCmd(m.service, m.currentNode)
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

	case key.Matches(msg, m.keys.SwitchView), msg.Type == tea.KeyShiftTab:
		// Tab or Shift+Tab switches focus to preview pane
		m.previewFocused = true
		return m, nil

	case key.Matches(msg, m.keys.CopyPath):
		skill := m.SelectedSkill()
		if skill != nil {
			var path string
			if skill.Source == skills.SourceTypeBuiltin {
				path = skill.Name + " (builtin)"
			} else {
				path = filepath.Join(skill.Path, "SKILL.md")
			}
			if err := clipboard.WriteAll(path); err != nil {
				m.statusMsg = "Copy failed: " + err.Error()
			} else {
				m.statusMsg = "Copied: " + path
			}
		}
		return m, nil

	case key.Matches(msg, m.keys.Edit), key.Matches(msg, m.keys.Confirm):
		skill := m.SelectedSkill()
		if skill != nil && skill.Source != skills.SourceTypeBuiltin {
			return m, editSkillCmd(skill)
		} else if skill != nil {
			m.statusMsg = "Cannot edit builtin skills"
		}
		return m, nil

	case key.Matches(msg, m.keys.ToggleAll):
		m.showAllSkills = !m.showAllSkills
		m.cursor = 0
		m.updateViewportContent()
		return m, nil

	case key.Matches(msg, m.keys.ToggleProject):
		skill := m.SelectedSkill()
		if skill != nil && m.currentNode != nil {
			tomlPath := filepath.Join(m.currentNode.Path, "grove.toml")
			return m, toggleSkillCmd(tomlPath, skill.Name, "Project")
		} else if skill == nil {
			m.statusMsg = "Select a skill first"
		} else {
			m.statusMsg = "Not in a project context"
		}
		return m, nil

	case key.Matches(msg, m.keys.ToggleEcosystem):
		skill := m.SelectedSkill()
		if skill != nil && m.currentNode != nil {
			ecoPath := m.currentNode.RootEcosystemPath
			if ecoPath == "" && m.currentNode.IsEcosystem() {
				ecoPath = m.currentNode.Path
			}
			if ecoPath != "" {
				tomlPath := filepath.Join(ecoPath, "grove.toml")
				return m, toggleSkillCmd(tomlPath, skill.Name, "Ecosystem")
			} else {
				m.statusMsg = "Not in an ecosystem context"
			}
		} else if skill == nil {
			m.statusMsg = "Select a skill first"
		} else {
			m.statusMsg = "Not in a project context"
		}
		return m, nil

	case key.Matches(msg, m.keys.ToggleGlobal):
		skill := m.SelectedSkill()
		if skill != nil {
			globalPath := skills.GetGlobalConfigPath()
			if globalPath != "" {
				return m, toggleSkillCmd(globalPath, skill.Name, "Global")
			} else {
				m.statusMsg = "Could not determine global config path"
			}
		} else {
			m.statusMsg = "Select a skill first"
		}
		return m, nil

	case key.Matches(msg, m.keys.ToggleUser):
		skill := m.SelectedSkill()
		if skill != nil && m.currentNode != nil {
			globalPath := skills.GetGlobalConfigPath()
			if globalPath != "" {
				// Use repository name, not worktree name
				projectName := m.currentNode.Name
				if m.currentNode.ParentProjectPath != "" {
					projectName = filepath.Base(m.currentNode.ParentProjectPath)
				}
				return m, toggleUserSkillCmd(globalPath, skill.Name, projectName)
			} else {
				m.statusMsg = "Could not determine global config path"
			}
		} else if skill == nil {
			m.statusMsg = "Select a skill first"
		} else {
			m.statusMsg = "Not in a project context"
		}
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
	node := m.SelectedNode()
	if node == nil {
		m.viewport.SetContent("Select a skill to view details")
		m.cachedSkillName = ""
		return
	}

	// Handle group nodes
	if node.IsGroup {
		m.cachedSkillName = "group:" + node.Name
		m.cachedTree = ""
		m.cachedContent = ""
		m.viewport.SetContent(m.renderGroupDetails(node))
		return
	}

	skill := node

	// Use cached content if available (use path as cache key for workspace skills)
	cacheKey := skill.Name
	if skill.Path != "" {
		cacheKey = skill.Path
	}
	if cacheKey == m.cachedSkillName {
		return
	}

	m.cachedSkillName = cacheKey

	// Build tree string (compact, without descriptions)
	treeStr, _ := skills.BuildCompactDependencyTreeString(m.service, skill.Name)
	m.cachedTree = treeStr

	// Get skill content - use path directly for workspace skills
	var content []byte
	if skill.Path != "" && skill.Workspace != "" {
		// Workspace skill - read directly from path
		data, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
		if err == nil {
			content = data
		}
	} else {
		// Builtin or user skill - use registry lookup
		if loadedSkill, err := skills.LoadSkillBypassingAccessWithService(m.service, nil, skill.Name); err == nil {
			content = loadedSkill.Files["SKILL.md"]
		}
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

// editSkillCmd opens the skill's SKILL.md file in the user's editor.
func editSkillCmd(skill *DisplayNode) tea.Cmd {
	skillPath := filepath.Join(skill.Path, "SKILL.md")

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	return tea.ExecProcess(exec.Command(editor, skillPath), func(err error) tea.Msg {
		if err != nil {
			return editCompleteMsg{err: err}
		}
		return editCompleteMsg{}
	})
}

// editCompleteMsg indicates edit operation completed.
type editCompleteMsg struct {
	err error
}

// toggleCompleteMsg indicates toggle operation completed.
type toggleCompleteMsg struct {
	message string
	err     error
}

// toggleSkillCmd toggles a skill in the specified config file.
func toggleSkillCmd(tomlPath, skillName, scope string) tea.Cmd {
	return func() tea.Msg {
		if err := skills.ToggleSkillInConfig(tomlPath, skillName); err != nil {
			return toggleCompleteMsg{err: err}
		}
		return toggleCompleteMsg{message: "Toggled " + skillName + " in " + scope}
	}
}

// toggleUserSkillCmd toggles a skill in the user's global config, scoped to a project.
func toggleUserSkillCmd(tomlPath, skillName, projectName string) tea.Cmd {
	return func() tea.Msg {
		if err := skills.ToggleUserProjectSkillInConfig(tomlPath, skillName, projectName); err != nil {
			return toggleCompleteMsg{err: err}
		}
		return toggleCompleteMsg{message: "Toggled " + skillName + " in user preferences for " + projectName}
	}
}

// newViewport creates a new viewport for the right pane.
func newViewport(width, height int) viewport.Model {
	// Content height = total height - header(2) - footer(2) - padding(2) - border(2)
	contentHeight := height - 8
	if contentHeight < 1 {
		contentHeight = 1
	}
	// Start with placeholder width, it will be updated once skills load
	vp := viewport.New(30, contentHeight)
	vp.SetContent("Select a skill to view details")
	return vp
}

// rightPaneWidth returns the width of the right pane.
func (m *Model) rightPaneWidth() int {
	effectiveWidth := m.width - 4 // Account for outer padding
	leftWidth := m.getLeftPaneWidth()
	rightWidth := effectiveWidth - leftWidth - 1 // Account for divider
	return rightWidth - 6                         // Account for border (2) + padding (2) + safety margin (2)
}

// viewportHeight returns the height available for the viewport.
func (m *Model) viewportHeight() int {
	// effectiveHeight = m.height - 2 (outer padding)
	// contentHeight = effectiveHeight - 4 (header + footer) = m.height - 6
	// viewport height = contentHeight - 2 (border top + bottom) = m.height - 8
	contentHeight := m.height - 8
	if contentHeight < 1 {
		contentHeight = 1
	}
	return contentHeight
}
