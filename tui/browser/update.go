package browser

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/skills/pkg/service"
	"github.com/grovetools/skills/pkg/skills"
	tuimuxmsg "github.com/grovetools/tuimux/messages"
)

// noWorkspaceMsg explains why grove.toml toggle keys are disabled when the
// browser was opened without a resolvable grove workspace context.
const noWorkspaceMsg = "No workspace context — open the browser from a grove workspace to toggle skills"

// defaultPreviewSplitRatio is the fraction of the BSP split the skills tree
// keeps the first time v promotes a preview. Even 50/50 rather than a sliver
// derived from the tree's content width: the tree pane already narrows itself
// to fit whatever it gets (getLeftPaneWidth caps the tree at 40% of the panel
// and reserves a floor for the detail column, and the detail pane truncates
// long lines instead of reflowing them — a layout regression-tested down to 20
// columns), whereas the editor on the other side has real minimum-width
// content and used to be squeezed into the leftovers. This mirrors flow's
// defaultBSPJobPaneRatio for the same reason.
//
// Only the FIRST time: the host remembers the direction and ratio the user
// sets afterwards, per origin panel, and replays it on the next open. So this
// value must not be re-asserted on later requests — sticky-navigation re-emits
// send Ratio 0 instead, which means "preserve the existing split geometry".
const defaultPreviewSplitRatio = 0.5

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
				m.viewport.HalfPageDown()
				return m, nil
			case msg.Type == tea.KeyCtrlU:
				// Half-page up
				m.viewport.HalfPageUp()
				return m, nil
			case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'j':
				// Scroll down one line
				m.viewport.ScrollDown(1)
				return m, nil
			case msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == 'k':
				// Scroll up one line
				m.viewport.ScrollUp(1)
				return m, nil
			case msg.Type == tea.KeyDown:
				// Arrow down
				m.viewport.ScrollDown(1)
				return m, nil
			case msg.Type == tea.KeyUp:
				// Arrow up
				m.viewport.ScrollUp(1)
				return m, nil
			case key.Matches(msg, m.keys.PageDown):
				// Page down
				m.viewport.PageDown()
				return m, nil
			case key.Matches(msg, m.keys.PageUp):
				// Page up
				m.viewport.PageUp()
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
			return m, nil
		}
		m.nodes = msg.nodes
		if len(m.nodes) == 0 {
			m.statusMsg = "No skills found"
			return m, nil
		}
		m.statusMsg = ""
		// Update viewport width now that we know the left pane width
		if m.ready {
			m.viewport.Width = m.rightPaneWidth()
		}
		// A reload rebuilds the whole node list (sync, remove, toggle, edit),
		// so the row under the cursor may now be a different skill — route
		// through selectionChanged so an open preview follows it too.
		return m, m.selectionChanged()

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

	case embed.SplitEditorClosedMsg:
		// A plain tuimux host reports a split-editor exit this way
		// (Model.CleanupSplitEditor calls Update on the origin panel with it).
		m.previewOpen = false
		m.previewPath = ""
		return m, nil

	case tuimuxmsg.DetailPaneClosedMsg:
		// treemux — the host skills actually runs in — takes the other road:
		// both leader-x on the preview and the editor exiting collapse the
		// pane through tuimux.ClosePane, which notifies the origin with
		// DetailPaneClosedMsg. SplitEditorClosedMsg above never arrives there,
		// so without this case previewOpen stayed true after the user closed
		// the split themselves: v inverted (it emitted a close for a pane that
		// no longer exists) and the next cursor move re-emitted a preview
		// request that respawned the split unasked.
		//
		// A detail-slot *swap* must not reach here. The host closes the
		// outgoing pane with ClosePaneForSwap precisely so this message keeps
		// meaning "your detail pane is gone" and nothing else; if that ever
		// regresses, skills stops driving a preview it still owns.
		m.previewOpen = false
		m.previewPath = ""
		return m, nil
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
			return m, m.selectionChanged()
		}
		return m, nil

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(nodes)-1 {
			m.cursor++
			return m, m.selectionChanged()
		}
		return m, nil

	case key.Matches(msg, m.keys.PageUp):
		m.cursor -= 10
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, m.selectionChanged()

	case key.Matches(msg, m.keys.PageDown):
		m.cursor += 10
		if m.cursor >= len(nodes) {
			m.cursor = len(nodes) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		return m, m.selectionChanged()

	case key.Matches(msg, m.keys.Search):
		m.searching = true
		m.filterInput.Focus()
		return m, nil

	case key.Matches(msg, m.keys.ClearSearch):
		m.filterText = ""
		m.filterInput.SetValue("")
		m.cursor = 0
		return m, m.selectionChanged()

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

	case key.Matches(msg, m.keys.Confirm), key.Matches(msg, m.keys.Select):
		// space / enter: dedicated open — the host pins SKILL.md to its own
		// per-file treemux pane, exactly as enter does in nb and flow. The
		// pane's rail label is qualified with the skill directory, so several
		// open skills stay tellable apart instead of all reading "SKILL.md".
		// Dedicated opens also skip the singleton editor's socket handshake,
		// which is what stalls a quick open on a cold editor pane.
		skill := m.SelectedSkill()
		if skill != nil && skill.Source != skills.SourceTypeBuiltin {
			if m.hosted {
				skillPath := filepath.Join(skill.Path, "SKILL.md")
				return m, func() tea.Msg {
					return embed.EditRequestMsg{Path: skillPath, Dedicated: true}
				}
			}
			return m, editSkillCmd(skill)
		} else if skill != nil {
			m.statusMsg = "Cannot edit builtin skills"
		}
		return m, nil

	case key.Matches(msg, m.keys.Edit):
		// e: quick open — route into the host's singleton "Editor" rail pane,
		// replacing whatever buffer it shows. Matches nb/flow's e.
		skill := m.SelectedSkill()
		if skill != nil && skill.Source != skills.SourceTypeBuiltin {
			if m.hosted {
				skillPath := filepath.Join(skill.Path, "SKILL.md")
				return m, func() tea.Msg {
					return embed.EditRequestMsg{Path: skillPath}
				}
			}
			return m, editSkillCmd(skill)
		} else if skill != nil {
			m.statusMsg = "Cannot edit builtin skills"
		}
		return m, nil

	case key.Matches(msg, m.keys.TogglePreview):
		skill := m.SelectedSkill()
		if skill != nil && skill.Source != skills.SourceTypeBuiltin {
			skillPath := filepath.Join(skill.Path, "SKILL.md")
			if m.hosted {
				if m.previewOpen {
					m.previewOpen = false
					m.previewPath = ""
					return m, func() tea.Msg { return embed.SplitEditorCloseRequestMsg{} }
				}
				m.previewOpen = true
				m.previewPath = skillPath
				return m, func() tea.Msg {
					return embed.SplitEditorRequestMsg{Path: skillPath, Ratio: defaultPreviewSplitRatio, Focus: false}
				}
			}
			// standalone: detail pane already shows the rendered SKILL.md; fall back to opening it.
			return m, editSkillCmd(skill)
		} else if skill != nil {
			m.statusMsg = "Cannot preview builtin skills"
		}
		return m, nil

	case key.Matches(msg, m.keys.ToggleAll):
		m.showAllSkills = !m.showAllSkills
		m.cursor = 0
		return m, m.selectionChanged()

	case key.Matches(msg, m.keys.ToggleProject):
		skill := m.SelectedSkill()
		if skill != nil && m.currentNode != nil {
			tomlPath := filepath.Join(m.currentNode.Path, "grove.toml")
			return m, toggleSkillCmd(tomlPath, skill.Name, "Project")
		} else if skill == nil {
			m.statusMsg = "Select a skill first"
		} else {
			m.statusMsg = noWorkspaceMsg
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
				m.statusMsg = "Workspace is not part of an ecosystem"
			}
		} else if skill == nil {
			m.statusMsg = "Select a skill first"
		} else {
			m.statusMsg = noWorkspaceMsg
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
			m.statusMsg = noWorkspaceMsg
		}
		return m, nil
	}

	return m, nil
}

// handleSequenceKey handles multi-key sequences like gg.
func (m Model) handleSequenceKey(idx int) (tea.Model, tea.Cmd) {
	nodes := m.filteredNodes()

	var cmd tea.Cmd
	switch idx {
	case 0: // Top (gg)
		m.cursor = 0
		cmd = m.selectionChanged()
	case 1: // Bottom (G)
		if len(nodes) > 0 {
			m.cursor = len(nodes) - 1
			cmd = m.selectionChanged()
		}
	}
	return m, cmd
}

// updateSearchMode handles input while in search mode.
func (m Model) updateSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter, tea.KeyEscape:
		m.searching = false
		m.filterInput.Blur()
		m.filterText = m.filterInput.Value()
		m.cursor = 0
		return m, m.selectionChanged()
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filterText = m.filterInput.Value()
	m.cursor = 0
	// Filtering re-seats the cursor on a different skill with every keystroke,
	// so the preview tracks the search results the same way it tracks j/k.
	return m, tea.Batch(cmd, m.selectionChanged())
}

// selectionChanged re-renders the detail viewport for the new selection and,
// when the hosted preview split is open, retargets it at the newly selected
// skill. Every path that moves the cursor or rebuilds the node list routes
// through here, so the split can never drift away from what the tree
// highlights.
func (m *Model) selectionChanged() tea.Cmd {
	m.updateViewportContent()
	return m.stickyPreviewCmd()
}

// stickyPreviewCmd re-emits SplitEditorRequestMsg for the current selection so
// the host swaps the buffer in the split it already owns — flow's "sticky
// navigation", now shared. Ratio 0 means "preserve the existing split
// geometry"; the host answers by retargeting the live editor over its Neovim
// RPC socket, so there is no respawn and no flicker per keystroke.
//
// A selection with no previewable file emits nothing, leaving the split parked
// on the last skill that had one. That covers three cases with one rule:
//
//   - Builtin skills ship inside the binary and have no SKILL.md on disk to
//     hand an editor, which is why v refuses them outright.
//   - Group headers are pure scaffolding that the cursor crosses between every
//     pair of groups.
//   - An empty or over-filtered list has no selection at all.
//
// Parking beats closing on all three. It keeps sticky navigation consistent
// with the v key itself — v on a builtin reports "Cannot preview builtin
// skills" and leaves an open preview alone, it does not close it — and closing
// would additionally clear previewOpen, so the next v on a real skill would
// reopen rather than close, exactly the inversion previewOpen exists to
// prevent. Reopening is also the expensive direction: a retarget is one RPC
// call, whereas a respawned split costs a process launch and a render freeze.
func (m *Model) stickyPreviewCmd() tea.Cmd {
	if !m.hosted || !m.previewOpen {
		return nil
	}
	path := m.previewablePath()
	if path == "" || path == m.previewPath {
		return nil
	}
	m.previewPath = path
	return func() tea.Msg {
		return embed.SplitEditorRequestMsg{Path: path, Ratio: 0, Focus: false}
	}
}

// previewablePath returns the SKILL.md path for the current selection, or ""
// when the selection has no file the host could open: no selection, a group
// header (SelectedSkill already returns nil for those), or a builtin skill.
func (m *Model) previewablePath() string {
	skill := m.SelectedSkill()
	if skill == nil || skill.Source == skills.SourceTypeBuiltin || skill.Path == "" {
		return ""
	}
	return filepath.Join(skill.Path, "SKILL.md")
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
		m.cachedMetadata = nil
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

	// Parse metadata for skill_sequence and produces
	m.cachedMetadata = nil
	if len(content) > 0 {
		if meta, err := skills.ParseSkillFrontmatter(content); err == nil {
			m.cachedMetadata = meta
		}
	}

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

	return tea.ExecProcess(exec.Command(editor, skillPath), func(err error) tea.Msg { //nolint:gosec // G204: editor from $EDITOR
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
	// Content height = total height - header(2) - border(2).
	// Footer is handled by the pager wrapper, not the browser.
	contentHeight := height - 4
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
	effectiveWidth := m.width // Padding handled by pager wrapper
	leftWidth := m.getLeftPaneWidth()
	rightWidth := effectiveWidth - leftWidth - 1 // Account for divider
	vpWidth := rightWidth - 6                    // Account for border (2) + padding (2) + safety margin (2)
	if vpWidth < 1 {
		vpWidth = 1 // Never feed a non-positive width into the viewport
	}
	return vpWidth
}

// viewportHeight returns the height available for the viewport.
func (m *Model) viewportHeight() int {
	// contentHeight = m.height - 2 (header) - 2 (border top + bottom).
	// Footer is handled by the pager wrapper, not the browser.
	contentHeight := m.height - 4
	if contentHeight < 1 {
		contentHeight = 1
	}
	return contentHeight
}
