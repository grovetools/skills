package cmd

import (
	"github.com/grovetools/core/tui/keymap"
	skillskeymap "github.com/grovetools/skills/pkg/keymap"
)

// BrowserKeymapInfo exports the browser keymap info for the docgen registry.
func BrowserKeymapInfo() keymap.TUIInfo {
	return skillskeymap.KeymapInfo()
}
