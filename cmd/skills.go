package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/mattsolo1/grove-core/git"
	"github.com/mattsolo1/grove-core/logging"
	"github.com/mattsolo1/grove-core/pkg/workspace"
	"github.com/mattsolo1/grove-skills/pkg/service"
	"github.com/mattsolo1/grove-skills/pkg/skills"
	"github.com/spf13/cobra"
)

var ulog = logging.NewUnifiedLogger("grove-skills")

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "skills",
		Short:      "(deprecated) Use top-level commands instead: install, list, sync, remove",
		Long:       "This command group is deprecated. Use the top-level commands directly:\n  grove-skills install\n  grove-skills list\n  grove-skills sync\n  grove-skills remove",
		Aliases:    []string{"skill"},
		Deprecated: "use top-level commands instead (e.g., 'grove-skills install' instead of 'grove-skills skills install')",
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
			svc := GetService()

			if name == "all" {
				allSkills, _, err := skills.ListSkillsWithService(svc)
				if err != nil {
					return err
				}
				for _, skillName := range allSkills {
					if err := installSkill(logger, basePath, skillName, force, skipValidation, svc); err != nil {
						logger.WarnPretty(fmt.Sprintf("Failed to install skill '%s': %v", skillName, err))
					}
				}
				logger.Success(fmt.Sprintf("Installed all %d skills to %s for %s.", len(allSkills), scope, provider))
			} else {
				return installSkill(logger, basePath, name, force, skipValidation, svc)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "user", "Installation scope ('project', 'user', 'repo-root', or 'admin' for codex).")
	cmd.Flags().StringVar(&provider, "provider", "claude", "Agent provider ('claude', 'codex', 'opencode').")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Overwrite existing skill without prompting.")
	cmd.Flags().BoolVar(&skipValidation, "skip-validation", false, "Skip SKILL.md validation.")
	return cmd
}

func newSkillsListCmd() *cobra.Command {
	var showPath bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available skills from all sources",
		Long: `List all available skills from user, ecosystem, and project sources.

Skills are discovered from:
  - User skills: ~/.config/grove/skills
  - Ecosystem skills: notebook skills for the parent ecosystem
  - Project skills: notebook skills for the current project
  - Built-in skills: embedded in the grove-skills binary`,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := GetService()

			// Get current workspace context
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}

			node, err := workspace.GetProjectByPath(cwd)
			if err != nil {
				// Fall back to old behavior if not in a workspace
				return listSkillsLegacy(svc, showPath)
			}

			// Use the new multi-source discovery
			if svc == nil {
				svc, err = skills.NewServiceForNode(node)
				if err != nil {
					return listSkillsLegacy(nil, showPath)
				}
			}

			sources := skills.ListSkillSources(svc, node)
			if len(sources) == 0 {
				ulog.Info("No skills found").
					Pretty("No skills found.").
					Emit()
				return nil
			}

			// Sort skill names for consistent output
			var names []string
			for name := range sources {
				names = append(names, name)
			}
			sort.Strings(names)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if showPath {
				fmt.Fprintln(w, "SKILL\tSOURCE\tPATH")
				for _, name := range names {
					src := sources[name]
					fmt.Fprintf(w, "%s\t%s\t%s\n", name, src.Type, src.Path)
				}
			} else {
				fmt.Fprintln(w, "SKILL\tSOURCE")
				for _, name := range names {
					fmt.Fprintf(w, "%s\t%s\n", name, sources[name].Type)
				}
			}
			w.Flush()
			return nil
		},
	}
	cmd.Flags().BoolVar(&showPath, "path", false, "Show the full path to each skill")
	return cmd
}

// listSkillsLegacy falls back to the old listing behavior when not in a workspace
func listSkillsLegacy(svc *service.Service, showPath bool) error {
	allSkills, sources, err := skills.ListSkillsWithService(svc)
	if err != nil {
		return err
	}
	if len(allSkills) == 0 {
		ulog.Info("No skills found").
			Pretty("No skills found.").
			Emit()
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SKILL\tSOURCE")
	for _, name := range allSkills {
		fmt.Fprintf(w, "%s\t%s\n", name, sources[name])
	}
	w.Flush()
	return nil
}

func newSkillsSyncCmd() *cobra.Command {
	var scope, provider string
	var prune, skipValidation, ecosystem, here bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync all available skills to the target directory",
		Long: `Sync all available skills to the target directory.

When run with --here, syncs skills from all sources (user, ecosystem, project)
to .claude/skills/ in the current directory. This is useful for setting up
skills in a worktree or any project directory.

When run with --ecosystem from an ecosystem root, skills from the ecosystem's
notebook will be synced to all child projects within the ecosystem.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logging.NewPrettyLogger()
			svc := GetService()

			// Get current workspace context to determine sync behavior
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}

			// Simple --here mode: sync skills to the git root of the current directory
			if here {
				node, err := workspace.GetProjectByPath(cwd)
				if err != nil {
					return fmt.Errorf("could not determine workspace context: %w", err)
				}

				// Find the git root - this is where skills should be installed
				gitRoot, err := git.GetGitRoot(cwd)
				if err != nil {
					return fmt.Errorf("could not find git root: %w", err)
				}

				destDir := skills.GetSkillsDirectoryForWorktree(gitRoot, provider)
				logger.InfoPretty(fmt.Sprintf("Syncing skills to %s...", destDir))
				logger.InfoPretty(fmt.Sprintf("  Context: %s (%s)", node.Name, node.Kind))

				// Create service if needed
				if svc == nil {
					svc, err = skills.NewServiceForNode(node)
					if err != nil {
						return fmt.Errorf("could not create service: %w", err)
					}
				}

				syncedCount, err := skills.SyncSkillsToDirectory(svc, node, destDir)
				if err != nil {
					logger.WarnPretty(fmt.Sprintf("Some skills failed to sync: %v", err))
				}

				if syncedCount > 0 {
					logger.Success(fmt.Sprintf("Synced %d skills to %s", syncedCount, destDir))
				} else {
					logger.InfoPretty("No skills found to sync.")
				}
				return nil
			}

			// Ecosystem-aware sync: if --ecosystem flag is set and we're in an ecosystem
			if ecosystem {
				// Use svc.Provider for consistent workspace lookup
				if svc == nil || svc.Provider == nil {
					return fmt.Errorf("workspace discovery failed - cannot determine ecosystem context")
				}

				node := svc.Provider.FindByPath(cwd)
				if node == nil {
					return fmt.Errorf("could not find workspace for current directory: %s", cwd)
				}
				if !node.IsEcosystem() {
					return fmt.Errorf("--ecosystem flag requires running from an ecosystem root (current: %s, kind: %s)", node.Name, node.Kind)
				}

				logger.InfoPretty(fmt.Sprintf("Ecosystem sync mode. Syncing skills across all projects in '%s'.", node.Name))

				// Get all skills available from the ecosystem's notebook
				allSkills, _, err := skills.ListSkillsWithService(svc)
				if err != nil {
					return err
				}

				if len(allSkills) == 0 {
					logger.InfoPretty("No skills found to sync.")
					return nil
				}

				// Get all child projects of this ecosystem
				// Check both ParentEcosystemPath and RootEcosystemPath for proper linking
				var childProjects []*workspace.WorkspaceNode
				for _, p := range svc.Provider.All() {
					// Include projects that are children of this ecosystem
					// (exclude worktrees and the ecosystem itself)
					isChild := (p.ParentEcosystemPath == node.Path || p.RootEcosystemPath == node.Path) &&
						!p.IsWorktree() && p.Path != node.Path
					if isChild {
						childProjects = append(childProjects, p)
					}
				}

				if len(childProjects) == 0 {
					logger.InfoPretty("No child projects found in this ecosystem.")
					return nil
				}

				logger.InfoPretty(fmt.Sprintf("Found %d skills and %d child projects.", len(allSkills), len(childProjects)))

				// For each child project, sync all skills
				for _, project := range childProjects {
					logger.InfoPretty(fmt.Sprintf("Syncing skills to project '%s'...", project.Name))

					// Get the install path for this project
					projectSkillPath, err := getInstallPathForDir(provider, "project", project.Path)
					if err != nil {
						logger.WarnPretty(fmt.Sprintf("Could not get install path for '%s': %v", project.Name, err))
						continue
					}

					installed := make(map[string]bool)
					for _, skillName := range allSkills {
						if err := installSkill(logger, projectSkillPath, skillName, true, skipValidation, svc); err != nil {
							logger.WarnPretty(fmt.Sprintf("  Failed to sync skill '%s': %v", skillName, err))
						}
						installed[skillName] = true
					}

					// Prune if requested
					if prune {
						pruneSkills(logger, projectSkillPath, installed)
					}
				}

				logger.Success("Ecosystem sync complete.")
				return nil
			}

			// Standard single-project sync
			basePath, err := getInstallPath(provider, scope)
			if err != nil {
				return err
			}
			logger.InfoPretty(fmt.Sprintf("Syncing skills to %s for %s...", scope, provider))

			allSkills, _, err := skills.ListSkillsWithService(svc)
			if err != nil {
				return err
			}

			installed := make(map[string]bool)
			for _, skillName := range allSkills {
				// Sync always overwrites (force=true)
				if err := installSkill(logger, basePath, skillName, true, skipValidation, svc); err != nil {
					logger.WarnPretty(fmt.Sprintf("Failed to sync skill '%s': %v", skillName, err))
				}
				installed[skillName] = true
			}

			if prune {
				pruneSkills(logger, basePath, installed)
			}
			logger.Success("Sync complete.")
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "user", "Sync scope ('project', 'user', 'repo-root', or 'admin' for codex).")
	cmd.Flags().StringVar(&provider, "provider", "claude", "Agent provider ('claude', 'codex', 'opencode').")
	cmd.Flags().BoolVar(&prune, "prune", false, "Remove skills from destination that no longer exist in source.")
	cmd.Flags().BoolVar(&skipValidation, "skip-validation", false, "Skip SKILL.md validation.")
	cmd.Flags().BoolVar(&ecosystem, "ecosystem", false, "Sync skills across all projects in the ecosystem (must be run from ecosystem root).")
	cmd.Flags().BoolVar(&here, "here", false, "Sync all skills (user, ecosystem, project) to .claude/skills/ in current directory.")
	return cmd
}

// pruneSkills removes skills from the destination that are not in the installed map.
func pruneSkills(logger *logging.PrettyLogger, basePath string, installed map[string]bool) {
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

// getInstallPathForDir returns the installation path for a given provider and scope,
// using the specified directory as the base instead of CWD.
func getInstallPathForDir(provider, scope, baseDir string) (string, error) {
	var pathParts []string

	switch scope {
	case "user":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		pathParts = append(pathParts, home)
	case "project":
		// Use the provided baseDir as the project root
		pathParts = append(pathParts, baseDir)
	case "repo-root":
		gitRoot, err := git.GetGitRoot(baseDir)
		if err != nil {
			return "", fmt.Errorf("could not find git repository root for 'repo-root' scope: %w", err)
		}
		pathParts = append(pathParts, gitRoot)
	case "admin":
		if strings.ToLower(provider) != "codex" {
			return "", fmt.Errorf("'admin' scope is only supported for the 'codex' provider")
		}
		pathParts = append(pathParts, "/etc")
	default:
		return "", fmt.Errorf("invalid scope: %s (valid: 'user', 'project', 'repo-root', 'admin')", scope)
	}

	switch strings.ToLower(provider) {
	case "claude":
		pathParts = append(pathParts, ".claude", "skills")
	case "codex":
		if scope == "admin" {
			pathParts = append(pathParts, "codex", "skills")
		} else {
			pathParts = append(pathParts, ".codex", "skills")
		}
	case "opencode":
		pathParts = append(pathParts, ".opencode", "skill")
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	return filepath.Join(pathParts...), nil
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
	cmd.Flags().StringVar(&scope, "scope", "user", "Scope to remove from ('project', 'user', 'repo-root', or 'admin' for codex).")
	cmd.Flags().StringVar(&provider, "provider", "claude", "Agent provider ('claude', 'codex', 'opencode').")
	return cmd
}

func getInstallPath(provider, scope string) (string, error) {
	var pathParts []string

	switch scope {
	case "user":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		pathParts = append(pathParts, home)
	case "project":
		// Uses current working directory, so pathParts remains empty initially
		pathParts = []string{}
	case "repo-root":
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		gitRoot, err := git.GetGitRoot(cwd)
		if err != nil {
			return "", fmt.Errorf("could not find git repository root for 'repo-root' scope: %w", err)
		}
		pathParts = append(pathParts, gitRoot)
	case "admin":
		if strings.ToLower(provider) != "codex" {
			return "", fmt.Errorf("'admin' scope is only supported for the 'codex' provider")
		}
		// For admin scope, the path is absolute under /etc
		pathParts = append(pathParts, "/etc")
	default:
		return "", fmt.Errorf("invalid scope: %s (valid: 'user', 'project', 'repo-root', 'admin')", scope)
	}

	switch strings.ToLower(provider) {
	case "claude":
		pathParts = append(pathParts, ".claude", "skills")
	case "codex":
		if scope == "admin" {
			// Admin scope uses /etc/codex/skills (no leading dot)
			pathParts = append(pathParts, "codex", "skills")
		} else {
			pathParts = append(pathParts, ".codex", "skills")
		}
	case "opencode":
		pathParts = append(pathParts, ".opencode", "skill")
	default:
		return "", fmt.Errorf("unsupported provider: %s", provider)
	}

	return filepath.Join(pathParts...), nil
}

func installSkill(logger *logging.PrettyLogger, basePath, name string, force, skipValidation bool, svc *service.Service) error {
	skillFiles, err := skills.GetSkillWithService(svc, name)
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
