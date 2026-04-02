package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/grovetools/core/pkg/workspace" // used by GetProjectByPath
	"github.com/grovetools/skills/pkg/skills"
	"github.com/spf13/cobra"
)

// SearchResult represents a skill search match
type SearchResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Domain      string `json:"domain,omitempty"`
	Source      string `json:"source"`
	FilePath    string `json:"file_path"`
	MatchReason string `json:"match_reason"`
}

func newSkillsSearchCmd() *cobra.Command {
	var jsonOutput, filesOnly bool

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for skills by keyword and locate their source files",
		Long: `Search for skills by keyword and find their authoritative source files.

This command helps agents and users discover skills and, crucially, find the
original SKILL.md file that should be edited (not the compiled copy in
.claude/skills/).

The search matches against:
  - Skill name
  - Skill description
  - Skill domain (if set)

Output modes:
  --json        Output structured JSON for agent consumption
  --files-only  Output only editable file paths (one per line)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.ToLower(args[0])
			svc := GetService()

			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}

			node, err := workspace.GetProjectByPath(cwd)
			if err != nil {
				// Not in a workspace, but we can still search built-in skills
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

			sources := skills.ListSkillSources(svc, node)
			var results []SearchResult

			for name, src := range sources {
				loadedSkill, loadErr := skills.LoadSkillFromSource(name, src)
				if loadErr != nil {
					continue
				}

				content := loadedSkill.Files["SKILL.md"]
				if content == nil {
					continue
				}

				meta, parseErr := skills.ParseSkillFrontmatter(content)
				if parseErr != nil {
					continue
				}

				matchReason := ""
				if strings.Contains(strings.ToLower(meta.Name), query) {
					matchReason = "name"
				} else if strings.Contains(strings.ToLower(meta.Domain), query) {
					matchReason = "domain"
				} else if strings.Contains(strings.ToLower(meta.Description), query) {
					matchReason = "description"
				}

				if matchReason != "" {
					filePath := filepath.Join(loadedSkill.PhysicalPath, "SKILL.md")
					if loadedSkill.SourceType == skills.SourceTypeBuiltin {
						filePath = "[READ-ONLY BUILTIN]"
					}
					results = append(results, SearchResult{
						Name:        meta.Name,
						Description: meta.Description,
						Domain:      meta.Domain,
						Source:      string(loadedSkill.SourceType),
						FilePath:    filePath,
						MatchReason: matchReason,
					})
				}
			}

			if len(results) == 0 {
				if !jsonOutput && !filesOnly {
					fmt.Println("No skills found matching query:", args[0])
				}
				if jsonOutput {
					fmt.Println("[]")
				}
				return nil
			}

			if filesOnly {
				for _, r := range results {
					if r.Source != string(skills.SourceTypeBuiltin) {
						fmt.Println(r.FilePath)
					}
				}
				return nil
			}

			if jsonOutput {
				out, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					return fmt.Errorf("failed to marshal JSON: %w", err)
				}
				fmt.Println(string(out))
				return nil
			}

			// Human-readable tabular output
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tSOURCE\tMATCH\tEDIT PATH")
			for _, r := range results {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Name, r.Source, r.MatchReason, r.FilePath)
			}
			w.Flush()
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output results as JSON")
	cmd.Flags().BoolVar(&filesOnly, "files-only", false, "Output only editable file paths")

	return cmd
}
