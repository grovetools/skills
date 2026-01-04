package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-skills/pkg/skills"
	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "skills",
		Short:   "Manage Agent Skills for different providers",
		Long:    "Install, list, sync, and remove agent skills for providers like Claude Code, Codex, and OpenCode.",
		Aliases: []string{"skill"},
	}

	cmd.AddCommand(newSkillsInstallCmd())
	cmd.AddCommand(newSkillsListCmd())
	cmd.AddCommand(newSkillsSyncCmd())
	cmd.AddCommand(newSkillsRemoveCmd())

	return cmd
}

func newSkillsInstallCmd() *cobra.Command {
	var scope, provider string
	var force, skipValidation bool
	cmd := &cobra.Command{
		Use:   "install <name|all>",
		Short: "Install a skill or all available skills",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			basePath, err := getInstallPath(provider, scope)
			if err != nil {
				return err
			}
			logger := logging.NewPrettyLogger()

			if name == "all" {
				allSkills, _, err := skills.ListSkills()
				if err != nil {
					return err
				}
				for _, skillName := range allSkills {
					if err := installSkill(logger, basePath, skillName, force, skipValidation); err != nil {
						logger.WarnPretty(fmt.Sprintf("Failed to install skill '%s': %v", skillName, err))
					}
				}
				logger.Success(fmt.Sprintf("Installed all %d skills to %s for %s.", len(allSkills), scope, provider))
			} else {
				return installSkill(logger, basePath, name, force, skipValidation)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "user", "Installation scope ('project' or 'user').")
	cmd.Flags().StringVar(&provider, "provider", "claude", "Agent provider ('claude', 'codex', 'opencode').")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing skill without prompting.")
	cmd.Flags().BoolVar(&skipValidation, "skip-validation", false, "Skip SKILL.md validation.")
	return cmd
}

func newSkillsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available skills from all sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			allSkills, sources, err := skills.ListSkills()
			if err != nil {
				return err
			}
			if len(allSkills) == 0 {
				fmt.Println("No skills found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SKILL\tSOURCE")
			for _, name := range allSkills {
				fmt.Fprintf(w, "%s\t%s\n", name, sources[name])
			}
			w.Flush()
			return nil
		},
	}
}

func newSkillsSyncCmd() *cobra.Command {
	var scope, provider string
	var prune, skipValidation bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync all available skills to the target directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			basePath, err := getInstallPath(provider, scope)
			if err != nil {
				return err
			}
			logger := logging.NewPrettyLogger()
			logger.InfoPretty(fmt.Sprintf("Syncing skills to %s for %s...", scope, provider))

			allSkills, _, err := skills.ListSkills()
			if err != nil {
				return err
			}

			installed := make(map[string]bool)
			for _, skillName := range allSkills {
				// Sync always overwrites (force=true)
				if err := installSkill(logger, basePath, skillName, true, skipValidation); err != nil {
					logger.WarnPretty(fmt.Sprintf("Failed to sync skill '%s': %v", skillName, err))
				}
				installed[skillName] = true
			}

			if prune {
				entries, err := os.ReadDir(basePath)
				if err == nil {
					for _, entry := range entries {
						if entry.IsDir() && !installed[entry.Name()] {
							pathToRemove := filepath.Join(basePath, entry.Name())
							if err := os.RemoveAll(pathToRemove); err != nil {
								logger.WarnPretty(fmt.Sprintf("Failed to prune skill '%s': %v", entry.Name(), err))
							} else {
								logger.InfoPretty(fmt.Sprintf("Pruned skill '%s'.", entry.Name()))
							}
						}
					}
				}
			}
			logger.Success("Sync complete.")
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "user", "Sync scope ('project' or 'user').")
	cmd.Flags().StringVar(&provider, "provider", "claude", "Agent provider ('claude', 'codex', 'opencode').")
	cmd.Flags().BoolVar(&prune, "prune", false, "Remove skills from destination that no longer exist in source.")
	cmd.Flags().BoolVar(&skipValidation, "skip-validation", false, "Skip SKILL.md validation.")
	return cmd
}

func newSkillsRemoveCmd() *cobra.Command {
	var scope, provider string
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an installed skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			basePath, err := getInstallPath(provider, scope)
			if err != nil {
				return err
			}
			logger := logging.NewPrettyLogger()

			skillPath := filepath.Join(basePath, name)
			if _, err := os.Stat(skillPath); os.IsNotExist(err) {
				return fmt.Errorf("skill '%s' not found at %s", name, skillPath)
			}

			if err := os.RemoveAll(skillPath); err != nil {
				return fmt.Errorf("failed to remove skill '%s': %w", name, err)
			}

			logger.Success(fmt.Sprintf("Skill '%s' removed.", name))
			logger.Path("  Removed from", skillPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "user", "Scope to remove from ('project' or 'user').")
	cmd.Flags().StringVar(&provider, "provider", "claude", "Agent provider ('claude', 'codex', 'opencode').")
	return cmd
}

func getInstallPath(provider, scope string) (string, error) {
	var pathParts []string
	if scope == "user" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		pathParts = append(pathParts, home)
	} else if scope != "project" {
		return "", fmt.Errorf("invalid scope: %s", scope)
	}

	switch strings.ToLower(provider) {
	case "claude":
		pathParts = append(pathParts, ".claude", "skills")
	case "codex":
		pathParts = append(pathParts, ".codex", "skills")
	case "opencode":
		pathParts = append(pathParts, ".opencode", "skill")
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	return filepath.Join(pathParts...), nil
}

func installSkill(logger *logging.PrettyLogger, basePath, name string, force, skipValidation bool) error {
	skillFiles, err := skills.GetSkill(name)
	if err != nil {
		return err
	}

	// Validate SKILL.md if validation is enabled
	if !skipValidation {
		if skillContent, ok := skillFiles["SKILL.md"]; ok {
			if err := skills.ValidateSkillContent(skillContent, name); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("skill '%s' is missing required SKILL.md file", name)
		}
	}

	skillDestDir := filepath.Join(basePath, name)

	// Check if skill already exists
	if _, err := os.Stat(skillDestDir); err == nil {
		if !force {
			return fmt.Errorf("skill '%s' already exists at %s (use --force to overwrite)", name, skillDestDir)
		}
		// Remove existing skill directory before reinstalling
		if err := os.RemoveAll(skillDestDir); err != nil {
			return fmt.Errorf("failed to remove existing skill '%s': %w", name, err)
		}
	}

	if err := os.MkdirAll(skillDestDir, 0755); err != nil {
		return fmt.Errorf("failed to create skill directory '%s': %w", skillDestDir, err)
	}

	for relPath, content := range skillFiles {
		destPath := filepath.Join(skillDestDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return err
		}
	}

	logger.Success(fmt.Sprintf("Skill '%s' installed.", name))
	logger.Path("  Location", skillDestDir)
	return nil
}
