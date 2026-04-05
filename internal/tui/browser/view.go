package browser

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
	"github.com/grovetools/skills/pkg/skills"
)

// View renders the TUI.
func (m Model) View() string {
	// Show help overlay if active
	if m.help != nil && m.help.ShowAll {
		return m.help.View()
	}

	// Show loading state
	if m.loading {
		return m.renderLoading()
	}

	// Main two-pane layout with padding
	return lipgloss.NewStyle().Padding(1, 2).Render(m.renderMainView())
}

// renderLoading renders the loading state.
func (m Model) renderLoading() string {
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		m.theme.Muted.Render("Loading skills..."),
	)
}

// renderMainView renders the main two-pane layout.
func (m Model) renderMainView() string {
	// Account for padding (4 horizontal = 2 left + 2 right, 2 vertical = 1 top + 1 bottom)
	effectiveWidth := m.width - 4
	effectiveHeight := m.height - 2

	// Calculate pane widths dynamically based on content
	leftWidth := m.getLeftPaneWidth()
	rightWidth := effectiveWidth - leftWidth - 1 // Account for divider

	// Calculate content height (total height minus header and footer)
	// Header: 2 lines (title + separator)
	// Footer: 2 lines (separator + status)
	contentHeight := effectiveHeight - 4
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Build sections
	header := m.renderHeader(effectiveWidth)
	leftPane := m.renderLeftPane(leftWidth, contentHeight)
	rightPane := m.renderRightPane(rightWidth, contentHeight)
	footer := m.renderFooter(effectiveWidth)

	// Join panes horizontally with a divider
	// Build divider line by line to match the content height
	dividerLines := make([]string, contentHeight)
	for i := 0; i < contentHeight; i++ {
		dividerLines[i] = m.theme.Muted.Render("│")
	}
	divider := strings.Join(dividerLines, "\n")

	mainContent := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, divider, rightPane)

	// Stack vertically and trim any trailing newlines
	result := lipgloss.JoinVertical(lipgloss.Left,
		header,
		mainContent,
		footer,
	)
	return strings.TrimSuffix(result, "\n")
}

// renderHeader renders the title bar.
func (m Model) renderHeader(width int) string {
	// Mode indicator
	modeIndicator := m.theme.Muted.Render(" [Active Context]")
	if m.showAllSkills {
		modeIndicator = m.theme.Highlight.Render(" [All Skills]")
	}

	// Use Bold style for title with cyan color for consistency
	title := lipgloss.NewStyle().
		Foreground(m.theme.Colors.Cyan).
		Bold(true).
		Render("Skills Browser") + modeIndicator

	// Right-aligned search indicator
	var searchInfo string
	if m.filterText != "" {
		searchInfo = m.theme.Muted.Render(fmt.Sprintf("Filter: %s", m.filterText))
	}

	// Build header line
	gap := width - lipgloss.Width(title) - lipgloss.Width(searchInfo)
	if gap < 0 {
		gap = 0
	}

	header := title + strings.Repeat(" ", gap) + searchInfo
	separator := m.theme.Muted.Render(strings.Repeat("─", width))

	return lipgloss.JoinVertical(lipgloss.Left, header, separator)
}

// renderLeftPane renders the skill tree list.
func (m Model) renderLeftPane(width int, height int) string {
	nodes := m.filteredNodes()

	var lines []string

	// Calculate scroll window
	startIdx := 0
	if m.cursor >= height {
		startIdx = m.cursor - height + 1
	}
	endIdx := startIdx + height
	if endIdx > len(nodes) {
		endIdx = len(nodes)
	}

	for i := startIdx; i < endIdx; i++ {
		node := nodes[i]
		line := m.renderNode(node, i == m.cursor, width-4)
		lines = append(lines, line)
	}

	// Pad to fill height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width-2))
	}

	content := strings.Join(lines, "\n")

	// Apply left pane styling
	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxHeight(height)

	return style.Render(content)
}

// renderNode renders a single node (group or skill).
func (m Model) renderNode(node DisplayNode, selected bool, maxWidth int) string {
	var line string
	var indicator string

	if node.IsGroup {
		// Group header - cyan and bold
		groupStyle := lipgloss.NewStyle().
			Foreground(m.theme.Colors.Cyan).
			Bold(true)

		if selected {
			if m.previewFocused {
				indicator = m.theme.Muted.Render(theme.IconArrowRightBold + " ")
				line = m.theme.Muted.Render(node.Name)
			} else {
				indicator = m.theme.Highlight.Render(theme.IconArrowRightBold + " ")
				line = groupStyle.Render(node.Name)
			}
		} else {
			indicator = "  "
			line = groupStyle.Render(node.Name)
		}
	} else {
		// Skill with tree prefix (faint muted)
		prefix := m.theme.Muted.Faint(true).Render(node.Prefix)

		// Source type icon (muted)
		var sourceIcon string
		switch node.Source {
		case skills.SourceTypeBuiltin:
			sourceIcon = m.theme.Muted.Faint(true).Render(theme.IconGear) + " "
		case skills.SourceTypeUser:
			sourceIcon = m.theme.Muted.Faint(true).Render(theme.IconHome) + " "
		case skills.SourceTypeEcosystem, skills.SourceTypeProject:
			sourceIcon = m.theme.Muted.Faint(true).Render(theme.IconNotebook) + " "
		}

		// Build skill name with optional workspace tag (only show if different from group)
		skillName := sourceIcon + node.Name
		if node.Workspace != "" && node.Workspace != node.Group {
			skillName = skillName + " " + m.theme.Muted.Render("["+node.Workspace+"]")
		}

		// Build configuration indicators (muted icons)
		var tags string
		mutedStyle := m.theme.Muted.Faint(true)
		if node.ConfiguredProject {
			tags += " " + mutedStyle.Foreground(m.theme.Colors.Green).Render(theme.IconRepo)
		}
		if node.ConfiguredEcosystem {
			tags += " " + mutedStyle.Foreground(m.theme.Colors.Blue).Render(theme.IconEcosystem)
		}
		if node.ConfiguredGlobal {
			tags += " " + mutedStyle.Foreground(m.theme.Colors.Violet).Render(theme.IconEarth)
		}
		if node.ConfiguredUserProject || node.ConfiguredUserEcosystem {
			tags += " " + mutedStyle.Foreground(lipgloss.Color("#d33682")).Render(theme.IconHome)
		}

		if selected {
			// Use highlight style (orange, no background) with arrow icon
			if m.previewFocused {
				// When preview is focused, dim the left pane selection
				indicator = m.theme.Muted.Render(theme.IconArrowRightBold + " ")
				line = prefix + sourceIcon + m.theme.Muted.Render(node.Name)
				if node.Workspace != "" && node.Workspace != node.Group {
					line = line + " " + m.theme.Muted.Faint(true).Render("["+node.Workspace+"]")
				}
				line = line + tags
			} else {
				indicator = m.theme.Highlight.Render(theme.IconArrowRightBold + " ")
				line = prefix + sourceIcon + m.theme.Highlight.Render(node.Name)
				if node.Workspace != "" && node.Workspace != node.Group {
					line = line + " " + m.theme.Muted.Render("["+node.Workspace+"]")
				}
				line = line + tags
			}
		} else {
			indicator = "  "
			line = prefix + skillName + tags
		}
	}

	// Truncate if needed
	fullLine := indicator + line
	if lipgloss.Width(fullLine) > maxWidth {
		// Simple truncation
		fullLine = truncateString(fullLine, maxWidth-1) + "…"
	}

	// Pad to width
	padding := maxWidth - lipgloss.Width(fullLine)
	if padding > 0 {
		fullLine += strings.Repeat(" ", padding)
	}

	return fullLine
}

// renderRightPane renders the skill details pane.
func (m Model) renderRightPane(width int, height int) string {
	// Use viewport content
	content := m.viewport.View()

	// Determine border color based on focus state
	borderColor := m.theme.Colors.Border
	if m.previewFocused {
		borderColor = m.theme.Colors.Orange
	}

	// Apply rounded border with focus-dependent color
	// Don't set Height - let content determine height, border adds to it
	style := lipgloss.NewStyle().
		Width(width - 4). // Account for border (2) and padding (2)
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor)

	return style.Render(content)
}

// renderSkillDetails renders the skill details for the viewport.
func (m *Model) renderSkillDetails(skill *DisplayNode) string {
	var sb strings.Builder

	// Get the content width for wrapping (viewport width minus some padding)
	contentWidth := m.viewport.Width - 2
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Helper to wrap text
	wrapText := func(text string) string {
		return lipgloss.NewStyle().Width(contentWidth).Render(text)
	}

	// Header - use highlight color for skill name
	sb.WriteString(m.theme.Highlight.Render(skill.Name))
	sb.WriteString("\n\n")

	// Metadata section with colored labels
	labelStyle := m.theme.Muted
	valueStyle := lipgloss.NewStyle()

	sb.WriteString(labelStyle.Render("Source: "))
	sb.WriteString(valueStyle.Render(string(skill.Source)))
	sb.WriteString("\n")

	sb.WriteString(labelStyle.Render("Domain: "))
	sb.WriteString(valueStyle.Render(skill.Domain))
	sb.WriteString("\n")

	if skill.Workspace != "" {
		sb.WriteString(labelStyle.Render("Workspace: "))
		sb.WriteString(lipgloss.NewStyle().Foreground(m.theme.Colors.Blue).Render(skill.Workspace))
		sb.WriteString("\n")
	}

	if skill.Path != "" && skill.Path != "(builtin)" {
		sb.WriteString(labelStyle.Render("Path: "))
		sb.WriteString(wrapText(skill.Path))
		sb.WriteString("\n")
	}

	// Show where this skill is configured with file paths
	var configSources []string
	globalConfigPath := skills.GetGlobalConfigPath()

	if skill.ConfiguredProject && m.currentNode != nil {
		projPath := filepath.Join(m.currentNode.Path, "grove.toml")
		configSources = append(configSources,
			lipgloss.NewStyle().Foreground(m.theme.Colors.Green).Render(theme.IconRepo+" "+projPath))
	}
	if skill.ConfiguredEcosystem && m.currentNode != nil {
		ecoPath := m.currentNode.RootEcosystemPath
		if ecoPath == "" && m.currentNode.IsEcosystem() {
			ecoPath = m.currentNode.Path
		}
		if ecoPath != "" {
			configSources = append(configSources,
				lipgloss.NewStyle().Foreground(m.theme.Colors.Blue).Render(theme.IconEcosystem+" "+filepath.Join(ecoPath, "grove.toml")))
		}
	}
	if skill.ConfiguredGlobal && globalConfigPath != "" {
		configSources = append(configSources,
			lipgloss.NewStyle().Foreground(m.theme.Colors.Violet).Render(theme.IconEarth+" "+globalConfigPath+" [skills]"))
	}
	if skill.ConfiguredUserProject && globalConfigPath != "" && m.currentNode != nil {
		projectName := m.currentNode.Name
		if m.currentNode.ParentProjectPath != "" {
			projectName = filepath.Base(m.currentNode.ParentProjectPath)
		}
		configSources = append(configSources,
			lipgloss.NewStyle().Foreground(lipgloss.Color("#d33682")).Render(theme.IconHome+" "+globalConfigPath+" [skills.projects."+projectName+"]"))
	}
	if skill.ConfiguredUserEcosystem && globalConfigPath != "" && m.currentNode != nil {
		var ecoName string
		if m.currentNode.RootEcosystemPath != "" && m.currentNode.RootEcosystemPath != m.currentNode.Path {
			ecoName = filepath.Base(m.currentNode.RootEcosystemPath)
		} else if m.currentNode.IsEcosystem() {
			ecoName = m.currentNode.Name
		}
		if ecoName != "" {
			configSources = append(configSources,
				lipgloss.NewStyle().Foreground(lipgloss.Color("#cb4b16")).Render(theme.IconHome+" "+globalConfigPath+" [skills.ecosystems."+ecoName+"]"))
		}
	}
	if len(configSources) > 0 {
		sb.WriteString(labelStyle.Render("Enabled:\n"))
		for _, src := range configSources {
			sb.WriteString("  " + src + "\n")
		}
	}
	sb.WriteString("\n")

	// Section header style
	sectionStyle := lipgloss.NewStyle().
		Foreground(m.theme.Colors.Cyan).
		Bold(true)

	// Description
	if skill.Description != "" {
		sb.WriteString(sectionStyle.Render("Description"))
		sb.WriteString("\n")
		sb.WriteString(wrapText(skill.Description))
		sb.WriteString("\n\n")
	}

	// Dependencies tree
	if m.cachedTree != "" {
		sb.WriteString(sectionStyle.Render("Dependencies"))
		sb.WriteString("\n")
		// Color the tree markers with green
		treeLines := strings.Split(m.cachedTree, "\n")
		for _, line := range treeLines {
			// Replace tree markers with colored versions
			colored := strings.ReplaceAll(line, "├─", lipgloss.NewStyle().Foreground(m.theme.Colors.Green).Render("├─"))
			colored = strings.ReplaceAll(colored, "└─", lipgloss.NewStyle().Foreground(m.theme.Colors.Green).Render("└─"))
			colored = strings.ReplaceAll(colored, "│", lipgloss.NewStyle().Foreground(m.theme.Colors.Green).Render("│"))
			sb.WriteString(wrapText(colored))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Skill Sequence (nested sub-skills tree)
	if m.cachedMetadata != nil && len(m.cachedMetadata.SkillSequence) > 0 {
		sb.WriteString(sectionStyle.Render("Skill Sequence"))
		sb.WriteString("\n")
		visited := make(map[string]bool)
		m.renderSkillSequenceTree(&sb, m.cachedMetadata.SkillSequence, "  ", visited)
		sb.WriteString("\n")
	}

	// Produces (structured artifact display)
	if m.cachedMetadata != nil && len(m.cachedMetadata.Produces) > 0 {
		sb.WriteString(sectionStyle.Render("Produces"))
		sb.WriteString("\n")
		for _, artifact := range m.cachedMetadata.Produces {
			sb.WriteString(fmt.Sprintf("  %s %s\n",
				lipgloss.NewStyle().Foreground(m.theme.Colors.Green).Render(theme.IconStatusCompleted),
				artifact))
		}
		sb.WriteString("\n")
	}

	// Preview (full SKILL.md content - viewport handles scrolling)
	if m.cachedContent != "" {
		sb.WriteString(sectionStyle.Render("Preview"))
		sb.WriteString("\n")
		// Render content with wrapping applied to the whole block
		sb.WriteString(m.theme.Muted.Width(contentWidth).Render(m.cachedContent))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderSkillSequenceTree recursively renders a skill sequence as a nested tree.
// Each sub-skill shows its produces artifacts and recurses into its own skill_sequence.
func (m *Model) renderSkillSequenceTree(sb *strings.Builder, sequence []string, indent string, visited map[string]bool) {
	blueStyle := lipgloss.NewStyle().Foreground(m.theme.Colors.Blue)
	greenStyle := lipgloss.NewStyle().Foreground(m.theme.Colors.Green)

	for i, subSkill := range sequence {
		isLast := i == len(sequence)-1
		prefix := "├─"
		childIndent := indent + "│  "
		if isLast {
			prefix = "└─"
			childIndent = indent + "   "
		}

		// Load sub-skill metadata for produces and nested sequences
		var subMeta *skills.SkillMetadata
		if !visited[subSkill] {
			subMeta = m.loadSkillMetadata(subSkill)
		}

		// Render skill name with description if available
		line := subSkill
		if subMeta != nil && subMeta.Description != "" {
			line += " " + m.theme.Muted.Render("— "+subMeta.Description)
		}
		sb.WriteString(fmt.Sprintf("%s%s %s\n", indent, blueStyle.Render(prefix), line))

		// Show produces for this sub-skill
		if subMeta != nil && len(subMeta.Produces) > 0 {
			for j, art := range subMeta.Produces {
				artPrefix := "├─"
				if j == len(subMeta.Produces)-1 && (subMeta.SkillSequence == nil || len(subMeta.SkillSequence) == 0) {
					artPrefix = "└─"
				}
				sb.WriteString(fmt.Sprintf("%s%s %s %s\n", childIndent,
					m.theme.Muted.Render(artPrefix),
					greenStyle.Render(theme.IconStatusCompleted),
					m.theme.Muted.Render(art)))
			}
		}

		// Recursively resolve sub-skill's own skill_sequence
		if subMeta != nil && len(subMeta.SkillSequence) > 0 {
			visited[subSkill] = true
			m.renderSkillSequenceTree(sb, subMeta.SkillSequence, childIndent, visited)
			visited[subSkill] = false
		}
	}
}

// loadSkillMetadata loads metadata for a skill by name (for sub-skill resolution).
func (m *Model) loadSkillMetadata(name string) *skills.SkillMetadata {
	loaded, err := skills.LoadSkillBypassingAccessWithService(m.service, m.currentNode, name)
	if err != nil {
		return nil
	}
	content, ok := loaded.Files["SKILL.md"]
	if !ok {
		return nil
	}
	meta, err := skills.ParseSkillFrontmatter(content)
	if err != nil {
		return nil
	}
	return meta
}

// renderGroupDetails renders the group details for the viewport.
func (m *Model) renderGroupDetails(group *DisplayNode) string {
	var sb strings.Builder

	// Get content width for wrapping
	contentWidth := m.viewport.Width - 2
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Header
	sb.WriteString(m.theme.Highlight.Render(group.Name))
	sb.WriteString("\n\n")

	labelStyle := m.theme.Muted

	// Get skills in this group
	groupSkills := m.GetGroupSkills(group.Name)

	// Determine group type and show appropriate info
	if len(groupSkills) > 0 {
		firstSkill := groupSkills[0]

		// Show source type
		sb.WriteString(labelStyle.Render("Source: "))
		switch firstSkill.Source {
		case skills.SourceTypeBuiltin:
			sb.WriteString(theme.IconGear + " Built-in (embedded in binary)\n")
		case skills.SourceTypeUser:
			sb.WriteString(theme.IconHome + " User (~/.config/grove/skills/)\n")
		case skills.SourceTypeEcosystem, skills.SourceTypeProject:
			sb.WriteString(theme.IconNotebook + " Workspace/Notebook\n")
			if firstSkill.Workspace != "" {
				sb.WriteString(labelStyle.Render("Workspace: "))
				sb.WriteString(firstSkill.Workspace + "\n")
			}
		}

		// Show path if available
		if firstSkill.Path != "" && firstSkill.Source != skills.SourceTypeBuiltin {
			// Get parent directory (skills directory)
			skillsDir := filepath.Dir(firstSkill.Path)
			sb.WriteString(labelStyle.Render("Path: "))
			sb.WriteString(skillsDir + "\n")
		}

		sb.WriteString("\n")

		// Show skill count
		sb.WriteString(labelStyle.Render("Skills: "))
		sb.WriteString(fmt.Sprintf("%d\n\n", len(groupSkills)))

		// List skills with icons
		sectionStyle := lipgloss.NewStyle().
			Foreground(m.theme.Colors.Cyan).
			Bold(true)
		sb.WriteString(sectionStyle.Render("Contents"))
		sb.WriteString("\n")

		for _, s := range groupSkills {
			var icon string
			switch s.Source {
			case skills.SourceTypeBuiltin:
				icon = theme.IconGear
			case skills.SourceTypeUser:
				icon = theme.IconHome
			default:
				icon = theme.IconNotebook
			}
			desc := s.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			if desc != "" {
				sb.WriteString(fmt.Sprintf("  %s %s - %s\n", icon, s.Name, m.theme.Muted.Render(desc)))
			} else {
				sb.WriteString(fmt.Sprintf("  %s %s\n", icon, s.Name))
			}
		}
	}

	return sb.String()
}

// renderFooter renders the status bar and help hints.
func (m Model) renderFooter(width int) string {
	separator := m.theme.Muted.Render(strings.Repeat("─", width))

	// Status line
	var status string
	nodes := m.filteredNodes()
	skillCount := 0
	for _, n := range nodes {
		if !n.IsGroup {
			skillCount++
		}
	}
	status = fmt.Sprintf("%d skills", skillCount)

	if m.errorMsg != "" {
		status = m.theme.Error.Render(m.errorMsg)
	} else if m.statusMsg != "" {
		status = m.theme.Info.Render(m.statusMsg)
	}

	// Help hints (minimal per design)
	var helpText string
	if m.searching {
		helpText = m.theme.Muted.Render("Type to search • Enter to confirm • Esc to cancel")
	} else if m.previewFocused {
		helpText = m.theme.Muted.Render("Tab to switch • j/k C-d/u gg/G to scroll")
	} else {
		helpText = m.help.View()
	}

	// Search input if active
	var searchLine string
	if m.searching {
		searchLine = "Search: " + m.filterInput.View()
	}

	// Build footer
	var footer string
	if searchLine != "" {
		footer = lipgloss.JoinVertical(lipgloss.Left,
			separator,
			searchLine,
			status+" │ "+helpText,
		)
	} else {
		footer = lipgloss.JoinVertical(lipgloss.Left,
			separator,
			status+" │ "+helpText,
		)
	}

	return footer
}

// truncateString truncates a string to a maximum width.
func truncateString(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}

	// Simple byte-based truncation
	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		if lipgloss.Width(string(runes[:i])) <= maxWidth {
			return string(runes[:i])
		}
	}
	return ""
}
