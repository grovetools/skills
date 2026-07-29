// Package view is a tabbed meta-panel wrapping skills/tui/browser.
// Single tab today; designed to grow.
package view

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/pager"
	"github.com/grovetools/skills/pkg/service"
	"github.com/grovetools/skills/tui/browser"
)

// Model is the skills meta-panel.
type Model struct {
	meta pager.Meta[browser.Model]
}

// New constructs a Model wrapping a fresh browser.
func New(svc *service.Service, cfg *config.Config, node *workspace.WorkspaceNode, hosted ...bool) Model {
	h := len(hosted) > 0 && hosted[0]
	return Model{meta: pager.Wrap(browser.New(svc, cfg, node, h), pager.WrapConfig{
		Name: "Browser",
		Config: pager.Config{
			OuterPadding: [4]int{1, 2, 0, 2},
			FooterHeight: 1, // help/status line pinned from the browser's FooterView
		},
		// The browser prefixes its layout with a leading "\n"; the pager
		// already inserts a blank row between the tab bar and the body, so
		// keeping it would double-space.
		TrimLeadingNewline: true,
	})}
}

func (m Model) Init() tea.Cmd { return m.meta.Init() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.meta, cmd = m.meta.Step(msg)
	return m, cmd
}

func (m Model) View() string { return m.meta.View() }

func (m Model) Close() error { return m.meta.Close() }
