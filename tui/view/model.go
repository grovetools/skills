// Package view is a tabbed meta-panel wrapping skills/tui/browser.
// Single tab today; designed to grow.
package view

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/pager"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/skills/pkg/service"
	"github.com/grovetools/skills/tui/browser"
)

// Model is the skills meta-panel.
type Model struct {
	pager pager.Model
}

// New constructs a Model wrapping a fresh browser.
func New(svc *service.Service, cfg *config.Config, node *workspace.WorkspaceNode) Model {
	b := browser.New(svc, cfg, node)
	page := &browserPage{inner: b}
	return Model{pager: pager.NewWith([]pager.Page{page}, pager.KeyMapFromBase(keymap.NewBase()), pager.Config{
		OuterPadding: [4]int{1, 2, 0, 2},
	})}
}

func (m Model) Init() tea.Cmd { return m.pager.Init() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)
	return m, cmd
}

// View delegates entirely to the pager. The browserPage adapter
// strips the browser's leading newline so the pager's blank-row
// separator (bar → blank → body) isn't doubled up.
func (m Model) View() string {
	return m.pager.View()
}

func (m Model) Close() error { return nil }

// browserPage adapts skills browser.Model to pager.Page.
type browserPage struct {
	inner  browser.Model
	width  int
	height int
}

func (p *browserPage) Name() string  { return "Browser" }
func (p *browserPage) Init() tea.Cmd { return p.inner.Init() }
func (p *browserPage) View() string {
	// Inner browser prefixes its own layout with a leading "\n"; the
	// pager already inserts a blank row between bar and body, so
	// strip the leading newline here to avoid double-spacing.
	return strings.TrimPrefix(p.inner.View(), "\n")
}

func (p *browserPage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	updated, cmd := p.inner.Update(msg)
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
	return p, cmd
}

func (p *browserPage) Focus() tea.Cmd {
	updated, cmd := p.inner.Update(embed.FocusMsg{})
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
	return cmd
}

func (p *browserPage) Blur() {
	updated, _ := p.inner.Update(embed.BlurMsg{})
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
}

func (p *browserPage) SetSize(w, h int) {
	p.width = w
	p.height = h
	updated, _ := p.inner.Update(tea.WindowSizeMsg{Width: w, Height: h})
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
}
