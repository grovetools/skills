package cmd

import (
	"github.com/mattsolo1/grove-core/cli"
	"github.com/spf13/cobra"
)

var rootCmd *cobra.Command

func init() {
	rootCmd = cli.NewStandardCommand("grove-skills", "Agent Skill Integrations")

	// Add commands
	rootCmd.AddCommand(newVersionCmd())
}

func Execute() error {
	return rootCmd.Execute()
}
