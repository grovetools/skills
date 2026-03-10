package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/grovetools/core/git"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/skills/pkg/service"
	"github.com/grovetools/skills/pkg/skills"
	"github.com/spf13/cobra"
)

var ulog = logging.NewUnifiedLogger("grove-skills")

func newSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "skills",
		Short:      "(deprecated) Use top-level commands instead: list, sync, validate, remove",
		Long:       "This command group is deprecated. Use the top-level commands directly:\n  grove-skills list\n  grove-skills sync\n  grove-skills validate\n  grove-skills remove",
		Aliases:    []string{"skill"},
		Deprecated: "use top-level commands instead (e.g., 'grove-skills sync' instead of 'grove-skills skills sync')",
	}

	cmd.AddCommand(newSkillsListCmd())
	cmd.AddCommand(newSkillsSyncCmd())
	cmd.AddCommand(newSkillsRemoveCmd())
	cmd.AddCommand(newSkillsTreeCmd())
	cmd.AddCommand(newSkillsValidateCmd())

	return cmd
}

func newSkillsListCmd() *cobra.Command {
	var showPath, grouped, ecosystem, allWorkspaces, jsonOutput bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available skills from all sources",
		Long: `List all available skills from user, ecosystem, and project sources.

Skills are discovered from:
  - User skills: ~/.config/grove/skills
  - Ecosystem skills: notebook skills for the parent ecosystem
  - Project skills: notebook skills for the current project
  - Built-in skills: embedded in the grove-skills binary

Use --ecosystem to list skills from all workspaces in the current ecosystem.
Use --all-workspaces to list skills from all registered workspaces.

The CONFIGURED column shows whether a skill is declared in grove.toml:
  - Yes: skill is in the [skills.use] array
  - No: skill is available but not configured

Skills from other workspaces can be referenced as "workspace:skill-name" in grove.toml.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc := GetService()

			// Get current workspace context
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}

			node, err := workspace.GetProjectByPath(cwd)
			if err != nil && !allWorkspaces {
				// Fall back to old behavior if not in a workspace
				return listSkillsLegacy(svc, showPath)
			}

			// Use the new multi-source discovery
			if svc == nil && node != nil {
				svc, err = skills.NewServiceForNode(node)
				if err != nil {
					return listSkillsLegacy(nil, showPath)
				}
			}

			// Handle --all-workspaces and --ecosystem flags
			if allWorkspaces || ecosystem {
				return listWorkspaceSkills(svc, node, allWorkspaces, jsonOutput, showPath)
			}

			sources := skills.ListSkillSources(svc, node)
			if len(sources) == 0 {
				ulog.Info("No skills found").
					Pretty("No skills found.").
					Emit()
				return nil
			}

			// Load skills configuration to check which skills are configured
			skillsCfg, _ := skills.LoadSkillsConfig(svc.Config, node)
			configuredMap := make(map[string]bool)
			if skillsCfg != nil {
				for _, u := range skillsCfg.Use {
					configuredMap[u] = true
				}
				for name := range skillsCfg.Dependencies {
					configuredMap[name] = true
				}
			}

			// Sort skill names for consistent output
			var names []string
			for name := range sources {
				names = append(names, name)
			}
			sort.Strings(names)

			// Grouped output mode
			if grouped {
				return listSkillsGrouped(svc, sources, names)
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			if showPath {
				fmt.Fprintln(w, "SKILL\tCONFIGURED\tSOURCE\tPATH")
				for _, name := range names {
					src := sources[name]
					conf := "No"
					if configuredMap[name] {
						conf = "Yes"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, conf, src.Type, src.Path)
				}
			} else {
				fmt.Fprintln(w, "SKILL\tCONFIGURED\tSOURCE")
				for _, name := range names {
					conf := "No"
					if configuredMap[name] {
						conf = "Yes"
					}
					fmt.Fprintf(w, "%s\t%s\t%s\n", name, conf, sources[name].Type)
				}
			}
			w.Flush()
			return nil
		},
	}
	cmd.Flags().BoolVar(&showPath, "path", false, "Show the full path to each skill")
	cmd.Flags().BoolVar(&grouped, "grouped", false, "Group skills by domain")
	cmd.Flags().BoolVar(&ecosystem, "ecosystem", false, "List skills from all workspaces in the ecosystem")
	cmd.Flags().BoolVar(&allWorkspaces, "all-workspaces", false, "List skills from all registered workspaces")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON")
	return cmd
}

// listSkillsGrouped displays skills organized by their domain field.
func listSkillsGrouped(svc *service.Service, sources map[string]skills.SkillSource, names []string) error {
	// Map of domain -> list of skills
	domainSkills := make(map[string][]string)

	for _, name := range names {
		src := sources[name]
		domain := "uncategorized"

		// Read skill content to get domain
		var content []byte
		var err error
		if src.Type == skills.SourceTypeBuiltin {
			files, e := skills.GetSkill(name)
			if e == nil {
				content = files["SKILL.md"]
			}
		} else {
			content, err = os.ReadFile(filepath.Join(src.Path, "SKILL.md"))
			if err != nil {
				content = nil
			}
		}

		if content != nil {
			meta, err := skills.ParseSkillFrontmatter(content)
			if err == nil && meta.Domain != "" {
				domain = meta.Domain
			}
		}

		domainSkills[domain] = append(domainSkills[domain], name)
	}

	// Sort domain names
	var domains []string
	for d := range domainSkills {
		domains = append(domains, d)
	}
	sort.Strings(domains)

	// Print grouped output
	for i, domain := range domains {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("## %s\n", domain)
		for _, name := range domainSkills[domain] {
			src := sources[name]
			fmt.Printf("  %s (%s)\n", name, src.Type)
		}
	}

	return nil
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

// listWorkspaceSkills lists skills from all workspaces (--ecosystem or --all-workspaces)
func listWorkspaceSkills(svc *service.Service, node *workspace.WorkspaceNode, allWorkspaces bool, jsonOutput bool, showPath bool) error {
	var workspaceSkills []skills.WorkspaceSkill
	var err error

	if allWorkspaces {
		workspaceSkills, err = skills.ListAllWorkspaceSkills(svc)
	} else {
		workspaceSkills, err = skills.ListEcosystemSkills(svc, node)
	}

	if err != nil {
		return fmt.Errorf("failed to list workspace skills: %w", err)
	}

	// Also include builtin skills
	builtinSkills := skills.ListBuiltinSkills()
	for _, name := range builtinSkills {
		workspaceSkills = append(workspaceSkills, skills.WorkspaceSkill{
			Name:          name,
			Workspace:     "(builtin)",
			QualifiedName: name,
			Path:          "(embedded)",
		})
	}

	if len(workspaceSkills) == 0 {
		ulog.Info("No skills found").
			Pretty("No skills found.").
			Emit()
		return nil
	}

	// Sort by workspace then name
	sort.Slice(workspaceSkills, func(i, j int) bool {
		if workspaceSkills[i].Workspace != workspaceSkills[j].Workspace {
			return workspaceSkills[i].Workspace < workspaceSkills[j].Workspace
		}
		return workspaceSkills[i].Name < workspaceSkills[j].Name
	})

	if jsonOutput {
		type skillOutput struct {
			Name          string `json:"name"`
			Workspace     string `json:"workspace"`
			QualifiedName string `json:"qualified_name"`
			Path          string `json:"path"`
			Description   string `json:"description,omitempty"`
		}

		var output []skillOutput
		for _, s := range workspaceSkills {
			output = append(output, skillOutput{
				Name:          s.Name,
				Workspace:     s.Workspace,
				QualifiedName: s.QualifiedName,
				Path:          s.Path,
				Description:   s.Description,
			})
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if showPath {
		fmt.Fprintln(w, "WORKSPACE\tSKILL\tPATH")
		for _, s := range workspaceSkills {
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.Workspace, s.Name, s.Path)
		}
	} else {
		fmt.Fprintln(w, "WORKSPACE\tSKILL\tDESCRIPTION")
		for _, s := range workspaceSkills {
			desc := s.Description
			if len(desc) > 60 {
				desc = desc[:57] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", s.Workspace, s.Name, desc)
		}
	}
	w.Flush()
	return nil
}

func newSkillsSyncCmd() *cobra.Command {
	var prune, dryRun bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync skills declared in grove.toml to provider directories",
		Long: `Sync skills declared in grove.toml to the configured provider directories.

This command reads the [skills] block from grove.toml and syncs the declared
skills to the appropriate provider directories (e.g., .claude/skills/).

Example grove.toml configuration:

  [skills]
  use = ["explain-with-analogy", "grove-maintainer"]
  providers = ["claude", "codex"]  # default: ["claude"]

Use --dry-run to preview what would be synced without making changes.
Use --prune to remove skills that are no longer declared in the configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logging.NewPrettyLogger()
			svc := GetService()

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}

			node, err := workspace.GetProjectByPath(cwd)
			if err != nil {
				return fmt.Errorf("sync requires a workspace context: %w", err)
			}

			// Create service if needed
			if svc == nil {
				svc, err = skills.NewServiceForNode(node)
				if err != nil {
					return fmt.Errorf("could not create service: %w", err)
				}
			}

			// Load skills configuration from grove.toml
			skillsCfg, err := skills.LoadSkillsConfig(svc.Config, node)
			if err != nil {
				return fmt.Errorf("invalid skills config: %w", err)
			}

			if skillsCfg == nil || (len(skillsCfg.Use) == 0 && len(skillsCfg.Dependencies) == 0) {
				logger.InfoPretty("No skills declared in grove.toml. Nothing to sync.")
				return nil
			}

			// Resolve all declared skills
			resolved, err := skills.ResolveConfiguredSkills(svc, node, skillsCfg)
			if err != nil {
				return fmt.Errorf("resolution failed: %w", err)
			}

			if len(resolved) == 0 {
				logger.InfoPretty("No skills to sync.")
				return nil
			}

			// Find the git root - this is where skills should be installed
			gitRoot, err := git.GetGitRoot(cwd)
			if err != nil {
				return fmt.Errorf("could not find git root: %w", err)
			}

			// Dry run mode
			if dryRun {
				logger.InfoPretty("DRY RUN: The following skills would be synced:")
				for name, r := range resolved {
					logger.InfoPretty(fmt.Sprintf("  - %s (%s) -> %v", name, r.SourceType, r.Providers))
				}
				return nil
			}

			// Perform the sync
			logger.InfoPretty(fmt.Sprintf("Syncing %d skills to %s...", len(resolved), gitRoot))

			syncedCount, err := skills.SyncConfiguredSkills(gitRoot, resolved, prune, logger)
			if err != nil {
				logger.WarnPretty(fmt.Sprintf("Sync completed with errors: %v", err))
			}

			logger.Success(fmt.Sprintf("Successfully synced %d skills.", syncedCount))
			return nil
		},
	}
	cmd.Flags().BoolVar(&prune, "prune", false, "Remove skills from destination that are not in config.")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be synced without making changes.")
	return cmd
}

func newSkillsTreeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tree <name>",
		Short: "Visualize the dependency tree of a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			svc := GetService()

			treeStr, err := skills.BuildDependencyTreeString(svc, name)
			if err != nil {
				return err
			}
			fmt.Print(treeStr)
			return nil
		},
	}
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
	cmd.Flags().StringVar(&scope, "scope", "user", "Scope to remove from ('project', 'user', 'ecosystem', 'repo-root', or 'admin' for codex).")
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
	case "ecosystem":
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		node, err := workspace.GetProjectByPath(cwd)
		if err != nil {
			return "", fmt.Errorf("could not determine workspace context for ecosystem scope: %w", err)
		}
		// Prefer RootEcosystemPath if set - this means we're in a child project of an ecosystem
		if node.RootEcosystemPath != "" {
			pathParts = append(pathParts, node.RootEcosystemPath)
		} else if node.Kind == workspace.KindEcosystemRoot || node.Kind == workspace.KindEcosystemWorktree {
			// This is an actual ecosystem root - use its path
			pathParts = append(pathParts, node.Path)
		} else {
			return "", fmt.Errorf("current directory is not part of an ecosystem (kind=%s)", node.Kind)
		}
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
		return "", fmt.Errorf("invalid scope: %s (valid: 'user', 'project', 'ecosystem', 'repo-root', 'admin')", scope)
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
