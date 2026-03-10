package browser

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

	// Main two-pane layout
	return m.renderMainView()
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
	// Calculate pane widths
	leftWidth := m.width / 2
	rightWidth := m.width - leftWidth - 3 // Account for divider

	// Calculate content height (total height minus header and footer)
	// Header: 2 lines (title + separator)
	// Footer: 2 lines (separator + status)
	contentHeight := m.height - 4
	if contentHeight < 1 {
		contentHeight = 1
	}

	// Build sections
	header := m.renderHeader()
	leftPane := m.renderLeftPane(leftWidth, contentHeight)
	rightPane := m.renderRightPane(rightWidth, contentHeight)
	footer := m.renderFooter()

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
func (m Model) renderHeader() string {
	// Use Bold style for title (Header style may add margins)
	title := m.theme.Bold.Render("Skills Browser")

	// Right-aligned search indicator
	var searchInfo string
	if m.filterText != "" {
		searchInfo = m.theme.Muted.Render(fmt.Sprintf("Filter: %s", m.filterText))
	}

	// Build header line
	gap := m.width - lipgloss.Width(title) - lipgloss.Width(searchInfo) - 2
	if gap < 0 {
		gap = 0
	}

	header := title + strings.Repeat(" ", gap) + searchInfo
	separator := m.theme.Muted.Render(strings.Repeat("─", m.width))

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

// renderNode renders a single node (domain or skill).
func (m Model) renderNode(node DisplayNode, selected bool, maxWidth int) string {
	var line string

	if node.IsDomain {
		// Domain header - no selection indicator
		line = m.theme.Bold.Render(node.Name)
	} else {
		// Skill with tree prefix
		prefix := m.theme.Muted.Render(node.Prefix)
		line = prefix + node.Name
	}

	// Selection indicator
	var indicator string
	if selected {
		indicator = m.theme.Selected.Render("▶ ")
	} else {
		indicator = "  "
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

	style := lipgloss.NewStyle().
		Width(width).
		Height(height).
		MaxHeight(height)

	return style.Render(content)
}

// renderSkillDetails renders the skill details for the viewport.
func (m *Model) renderSkillDetails(skill *DisplayNode) string {
	var sb strings.Builder

	// Header
	sb.WriteString(m.theme.Header.Render(skill.Name))
	sb.WriteString("\n\n")

	// Metadata
	sb.WriteString(m.theme.Muted.Render("Source: "))
	sb.WriteString(string(skill.Source))
	sb.WriteString("\n")

	sb.WriteString(m.theme.Muted.Render("Domain: "))
	sb.WriteString(skill.Domain)
	sb.WriteString("\n")

	if skill.Path != "" && skill.Path != "(builtin)" {
		sb.WriteString(m.theme.Muted.Render("Path: "))
		sb.WriteString(skill.Path)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Description
	if skill.Description != "" {
		sb.WriteString(m.theme.Bold.Render("Description"))
		sb.WriteString("\n")
		sb.WriteString(skill.Description)
		sb.WriteString("\n\n")
	}

	// Dependencies tree
	if m.cachedTree != "" {
		sb.WriteString(m.theme.Bold.Render("Dependencies"))
		sb.WriteString("\n")
		sb.WriteString(m.cachedTree)
		sb.WriteString("\n")
	}

	// Preview (raw SKILL.md content)
	if m.cachedContent != "" {
		sb.WriteString(m.theme.Bold.Render("Preview"))
		sb.WriteString("\n")
		// Show first 20 lines of content
		lines := strings.Split(m.cachedContent, "\n")
		maxLines := 20
		if len(lines) < maxLines {
			maxLines = len(lines)
		}
		for i := 0; i < maxLines; i++ {
			sb.WriteString(m.theme.Muted.Render(lines[i]))
			sb.WriteString("\n")
		}
		if len(lines) > 20 {
			sb.WriteString(m.theme.Muted.Render("..."))
		}
	}

	return sb.String()
}

// renderFooter renders the status bar and help hints.
func (m Model) renderFooter() string {
	separator := m.theme.Muted.Render(strings.Repeat("─", m.width))

	// Status line
	var status string
	nodes := m.filteredNodes()
	skillCount := 0
	for _, n := range nodes {
		if !n.IsDomain {
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
