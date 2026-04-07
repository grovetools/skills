package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/skills/tui/browser"
	"github.com/spf13/cobra"
)

func newTuiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Browse skills in an interactive terminal UI",
		Long: `Opens an interactive terminal user interface for browsing and managing skills.

The TUI provides a two-pane layout:
  - Left pane: Tree view of skills organized by domain
  - Right pane: Details of the selected skill including dependencies

Navigation:
  j/k or arrows  Navigate up/down
  gg             Jump to top
  G              Jump to bottom
  /              Search skills
  ?              Show help
  q              Quit

Actions:
  s              Sync configured skills
  x              Remove selected skill
  i              Install skill (coming soon)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := GetService()
			if svc == nil {
				return fmt.Errorf("service not initialized")
			}

			// Try to determine current workspace context
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}
			node, _ := workspace.GetProjectByPath(cwd) // Ignore error, node will be nil if not in workspace

			model := browser.New(svc, svc.Config, node)
			p := tea.NewProgram(model, tea.WithAltScreen())

			_, err = p.Run()
			return err
		},
	}

	return cmd
}
