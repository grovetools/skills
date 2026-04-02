package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/workspace" // used by GetProjectByPath
	"github.com/grovetools/skills/pkg/skills"
	"github.com/spf13/cobra"
)

// ShowResult represents the JSON output of the show command
type ShowResult struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Domain      string   `json:"domain,omitempty"`
	Requires    []string `json:"requires,omitempty"`
	Source      string   `json:"source"`
	FilePath    string   `json:"file_path"`
	Content     string   `json:"content"`
}

func newSkillsShowCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "show <skill-name>",
		Short: "Display the full content and metadata of a skill",
		Long: `Display the complete content and metadata of a skill.

This command is designed for LLM agents to read skill definitions without needing
to know the physical file location. It resolves the skill name across all sources
(builtin, user, ecosystem, project) respecting the standard precedence order.

The skill name can be:
  - A simple name: "explain-with-analogy"
  - A workspace-qualified name: "grovetools:concept-maintainer"

Output modes:
  --json    Output structured JSON with metadata and full content (recommended for agents)
  (default) Human-readable format with metadata header and raw content`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillName := args[0]
			svc := GetService()

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}

			node, err := workspace.GetProjectByPath(cwd)
			if err != nil {
				// Not in a workspace, but we can still show builtin/user skills
				node = nil
			}

			// Create service if needed for workspace context
			if svc == nil && node != nil {
				svc, err = skills.NewServiceForNode(node)
				if err != nil {
					// Proceed without service
					svc = nil
				}
			}

			loadedSkill, err := skills.LoadSkillBypassingAccessWithService(svc, node, skillName)
			if err != nil {
				return err
			}

			content, ok := loadedSkill.Files["SKILL.md"]
			if !ok || len(content) == 0 {
				return fmt.Errorf("skill '%s' has no SKILL.md content", skillName)
			}

			// Parse frontmatter for metadata
			meta, err := skills.ParseSkillFrontmatter(content)
			if err != nil {
				return fmt.Errorf("failed to parse skill metadata: %w", err)
			}

			// Determine file path for display
			filePath := filepath.Join(loadedSkill.PhysicalPath, "SKILL.md")
			if loadedSkill.SourceType == skills.SourceTypeBuiltin {
				filePath = "(builtin - read only)"
			}

			if jsonOutput {
				result := ShowResult{
					Name:        meta.Name,
					Description: meta.Description,
					Domain:      meta.Domain,
					Requires:    meta.Requires,
					Source:      string(loadedSkill.SourceType),
					FilePath:    filePath,
					Content:     string(content),
				}

				out, err := json.MarshalIndent(result, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal JSON: %w", err)
				}
				fmt.Println(string(out))
				return nil
			}

			// Human-readable output
			fmt.Println("=== Skill Metadata ===")
			fmt.Printf("Name:        %s\n", meta.Name)
			fmt.Printf("Description: %s\n", meta.Description)
			if meta.Domain != "" {
				fmt.Printf("Domain:      %s\n", meta.Domain)
			}
			if len(meta.Requires) > 0 {
				fmt.Printf("Requires:    %s\n", strings.Join(meta.Requires, ", "))
			}
			fmt.Printf("Source:      %s\n", loadedSkill.SourceType)
			fmt.Printf("Path:        %s\n", filePath)
			fmt.Println()
			fmt.Println("=== Content ===")
			fmt.Println(string(content))

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON (recommended for agents)")

	return cmd
}
