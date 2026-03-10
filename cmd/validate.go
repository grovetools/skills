package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/skills/pkg/skills"
	"github.com/spf13/cobra"
)

func newSkillsValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate skills declared in grove.toml",
		Long: `Validate that all skills declared in grove.toml can be resolved.

This command reads the [skills] block from grove.toml and verifies that
each declared skill exists and can be found in the available sources
(built-in, user, ecosystem, or project).

Exit codes:
  0 - All skills validated successfully
  1 - One or more skills could not be resolved`,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := GetService()

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}

			node, err := workspace.GetProjectByPath(cwd)
			if err != nil {
				return fmt.Errorf("validate requires a workspace context: %w", err)
			}

			// Create service if needed
			if svc == nil {
				svc, err = skills.NewServiceForNode(node)
				if err != nil {
					return fmt.Errorf("could not create service: %w", err)
				}
			}

			// Load skills configuration
			skillsCfg, err := skills.LoadSkillsConfig(svc.Config, node)
			if err != nil {
				return fmt.Errorf("failed to load [skills] config: %w", err)
			}

			if skillsCfg == nil || len(skillsCfg.Use) == 0 && len(skillsCfg.Dependencies) == 0 {
				fmt.Println("No skills declared in grove.toml")
				return nil
			}

			// Try to resolve all declared skills
			resolved, err := skills.ResolveConfiguredSkills(svc, node, skillsCfg)
			if err != nil {
				fmt.Printf("✗ Validation failed: %v\n", err)
				os.Exit(1)
			}

			// Print success message with details
			fmt.Println("✓ All declared skills resolved successfully:")
			fmt.Println()

			// Sort skill names for consistent output
			var names []string
			for name := range resolved {
				names = append(names, name)
			}
			sort.Strings(names)

			for _, name := range names {
				r := resolved[name]
				fmt.Printf("  ✓ %s (source: %s, providers: %v)\n", name, r.SourceType, r.Providers)
			}

			return nil
		},
	}
}
