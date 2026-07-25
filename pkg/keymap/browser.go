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
	Inject          key.Binding
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
		// A two-key chord, not a capital letter. I sat one shift away from i
		// (Install) and the two were mistyped for each other; injecting into a
		// live agent prompt is the action here with a visible side effect
		// outside the TUI, so it gets the deliberate keystroke. Matched by the
		// browser's SequenceState, the same matcher that resolves gg.
		Inject: key.NewBinding(
			key.WithKeys("sa"),
			key.WithHelp("sa", "send to agent"),
		),
		Remove: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "remove"),
		),
		// Moved off lowercase s to free it as the "sa" prefix. The sequence
		// state has no timeout, so a lone s arms the chord and waits — leaving
		// sync on s would have made it unreachable rather than merely slow.
		Sync: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "sync"),
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

// ShortHelp returns the keybindings shown inline in the footer: quit only.
//
// The help component unconditionally prefixes the short view with
// "Press ? for help", so this one entry renders as "Press ? for help • q •
// quit" — the q and ? the footer is meant to carry, and nothing else. Listing
// k.Help here as well would print ? twice.
//
// The footer used to advertise Open/Edit/TogglePreview too. It was dropped
// deliberately: a four-pair line was already truncating at ordinary widths, and
// it went actively wrong in inject mode, where space selects rather than opens.
// FullHelp and Sections below are untouched, so ? still lists everything.
func (k BrowserKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Quit}
}

// FullHelp returns all keybindings organized by category.
func (k BrowserKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Open(), k.Edit, k.Inject, k.TogglePreview, k.ToggleAll, k.ToggleProject, k.ToggleEcosystem, k.ToggleGlobal, k.ToggleUser, k.Sync},
		{k.Search, k.ClearSearch},
		{k.SwitchView, k.Help, k.Quit},
	}
}

// Open re-labels the shared Select binding (space) for help output: in the
// skills browser there is no multi-select, so space opens SKILL.md in its own
// treemux pane. Enter (Confirm) does the same; only one is advertised.
func (k BrowserKeyMap) Open() key.Binding {
	return key.NewBinding(
		key.WithKeys(k.Select.Keys()...),
		key.WithHelp("space", "open in pane"),
	)
}

// Sections implements keymap.SectionedKeyMap for the help component.
func (k BrowserKeyMap) Sections() []keymap.Section {
	return []keymap.Section{
		k.Base.NavigationSection(),
		keymap.NewSection(keymap.SectionActions,
			k.Open(),
			k.Edit,
			k.Install,
			k.Inject,
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
			k.TogglePreview,
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
