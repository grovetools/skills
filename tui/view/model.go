// Package view is the tabbed meta-panel wrapper for the skills TUI.
// It hosts skills/tui/browser as its sole tab today — future
// expansion (skill dependency graph, hot-reload status, cached
// rulepacks inspector, usage telemetry) becomes a one-line page
// append instead of a follow-on refactor.
//
// The meta-panel uses the shared core/tui/components/pager component
// so key handling (Tab1..Tab9, NextTab/PrevTab) and auto-switch
// (embed.SwitchTabMsg) stay consistent with cx, memory, nb, and flow.
package view

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/pager"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/skills/pkg/service"
	"github.com/grovetools/skills/tui/browser"
)

// Model is the skills view meta-panel. It wraps a browser.Model in a
// pager.Page and exposes the standard tea.Model interface.
type Model struct {
	pager pager.Model
}

// New constructs a view.Model around a freshly-built browser. The
// constructor signature mirrors browser.New so hosts can swap the
// package import without rewriting call sites.
func New(svc *service.Service, cfg *config.Config, node *workspace.WorkspaceNode) Model {
	b := browser.New(svc, cfg, node)
	page := &browserPage{inner: b}
	p := pager.New([]pager.Page{page}, pager.DefaultKeyMap())
	return Model{pager: p}
}

// Init forwards to the pager's active page.
func (m Model) Init() tea.Cmd { return m.pager.Init() }

// Update routes messages through the pager so tab navigation,
// window resizing, and auto-switch all work the same way they do
// for the other meta-panels.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)
	return m, cmd
}

// View renders the tab bar directly above the active page. The
// wrapped skills browser already applies its own Padding(1, 2)
// around its two-pane layout, so we must NOT wrap the pager output
// in another Padding here — that would double the top margin and
// stack pager.View()'s blank-row separator on top of the browser's
// built-in top pad.
//
// Instead we render the tab bar with PaddingLeft(2) to align with
// the browser's content column and prepend one blank row for the
// top margin. The browser's own Padding(1, 2) provides the single
// blank row of separation between the bar and its header.
func (m Model) View() string {
	bar := lipgloss.NewStyle().PaddingLeft(2).Render(m.pager.RenderTabBar())
	body := ""
	if active := m.pager.Active(); active != nil {
		body = active.View()
	}
	return "\n" + bar + body
}

// Close is a no-op for symmetry with the other meta-panel Close
// methods. Skills browser owns no async resources.
func (m Model) Close() error { return nil }

// browserPage adapts skills/tui/browser.Model to the pager.Page
// interface, translating SetSize into a WindowSizeMsg and Focus/Blur
// into embed contract messages.
type browserPage struct {
	inner  browser.Model
	width  int
	height int
}

func (p *browserPage) Name() string { return "Browser" }

func (p *browserPage) Init() tea.Cmd { return p.inner.Init() }

func (p *browserPage) Update(msg tea.Msg) (pager.Page, tea.Cmd) {
	updated, cmd := p.inner.Update(msg)
	if bm, ok := updated.(browser.Model); ok {
		p.inner = bm
	}
	return p, cmd
}

func (p *browserPage) View() string { return p.inner.View() }

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
