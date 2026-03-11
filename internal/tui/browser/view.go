package browser

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/theme"
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
	// Use Bold style for title with cyan color for consistency
	title := lipgloss.NewStyle().
		Foreground(m.theme.Colors.Cyan).
		Bold(true).
		Render("Skills Browser")

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

		// Build skill name with optional workspace tag (only show if different from group)
		skillName := node.Name
		if node.Workspace != "" && node.Workspace != node.Group {
			skillName = skillName + " " + m.theme.Muted.Render("["+node.Workspace+"]")
		}

		if selected {
			// Use highlight style (orange, no background) with arrow icon
			if m.previewFocused {
				// When preview is focused, dim the left pane selection
				indicator = m.theme.Muted.Render(theme.IconArrowRightBold + " ")
				line = prefix + m.theme.Muted.Render(node.Name)
				if node.Workspace != "" && node.Workspace != node.Group {
					line = line + " " + m.theme.Muted.Faint(true).Render("["+node.Workspace+"]")
				}
			} else {
				indicator = m.theme.Highlight.Render(theme.IconArrowRightBold + " ")
				line = prefix + m.theme.Highlight.Render(node.Name)
				if node.Workspace != "" && node.Workspace != node.Group {
					line = line + " " + m.theme.Muted.Render("["+node.Workspace+"]")
				}
			}
		} else {
			indicator = "  "
			line = prefix + skillName
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

	// Preview (full SKILL.md content - viewport handles scrolling)
	if m.cachedContent != "" {
		sb.WriteString(sectionStyle.Render("Preview"))
		sb.WriteString("\n")
		// Show all content - let viewport handle scrolling
		lines := strings.Split(m.cachedContent, "\n")
		for _, line := range lines {
			sb.WriteString(wrapText(m.theme.Muted.Render(line)))
			sb.WriteString("\n")
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
