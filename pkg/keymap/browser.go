package keymap

import (
	"github.com/charmbracelet/bubbles/key"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/keymap"
)

// BrowserKeyMap defines keybindings for the skills browser TUI.
// It embeds keymap.Base for standard navigation and adds TUI-specific actions.
type BrowserKeyMap struct {
	keymap.Base

	// TUI-specific bindings
	Install         key.Binding
	Remove          key.Binding
	Sync            key.Binding
	ToggleAll       key.Binding
	ToggleProject   key.Binding
	ToggleEcosystem key.Binding
	ToggleGlobal    key.Binding
	ToggleUser      key.Binding
}

// NewBrowserKeyMap creates a new BrowserKeyMap with the given configuration.
func NewBrowserKeyMap(cfg *config.Config) BrowserKeyMap {
	km := BrowserKeyMap{
		Base: keymap.Load(cfg, "skills.browser"),

		Install: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "install"),
		),
		Remove: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "remove"),
		),
		Sync: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "sync"),
		),
		ToggleAll: key.NewBinding(
			key.WithKeys("A", "0"),
			key.WithHelp("A/0", "toggle all/active"),
		),
		ToggleProject: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "toggle in project"),
		),
		ToggleEcosystem: key.NewBinding(
			key.WithKeys("E"),
			key.WithHelp("E", "toggle in ecosystem"),
		),
		ToggleGlobal: key.NewBinding(
			key.WithKeys("ctrl+g"),
			key.WithHelp("ctrl+g", "toggle in global"),
		),
		ToggleUser: key.NewBinding(
			key.WithKeys("U"),
			key.WithHelp("U", "toggle user preference"),
		),
	}

	// Apply TUI-specific overrides from config
	keymap.ApplyTUIOverrides(cfg, "skills", "browser", &km)

	return km
}

// ShortHelp returns a minimal set of keybindings to show in the footer.
// Only Quit is returned since the help component already shows "Press ? for help".
func (k BrowserKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit}
}

// FullHelp returns all keybindings organized by category.
func (k BrowserKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Edit, k.ToggleAll, k.ToggleProject, k.ToggleEcosystem, k.ToggleGlobal, k.ToggleUser, k.Sync},
		{k.Search, k.ClearSearch},
		{k.SwitchView, k.Help, k.Quit},
	}
}

// Sections implements keymap.SectionedKeyMap for the help component.
func (k BrowserKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		k.Base.NavigationSection(),
		keymap.NewSection(keymap.SectionActions,
			k.Edit,
			k.Install,
			k.Remove,
			k.Sync,
			k.ToggleAll,
			k.ToggleProject,
			k.ToggleEcosystem,
			k.ToggleGlobal,
			k.ToggleUser,
			k.CopyPath,
		),
		k.Base.SearchSection(),
		keymap.NewSection(keymap.SectionView,
			k.SwitchView,
		),
		k.Base.SystemSection(),
	}
}

// KeymapInfo returns TUI info for the keys registry / docgen.
func KeymapInfo() keymap.TUIInfo {
	km := NewBrowserKeyMap(nil)
	return keymap.MakeTUIInfo(
		"skills-browser",
		"skills",
		"Skills browser",
		km,
	)
}
