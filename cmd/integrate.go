package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/spf13/cobra"
)

const integrationPayload = `
# Grove Ecosystem Agent Instructions
To discover capabilities, ALWAYS run ` + "`grove-skills search \"<task>\" --json`" + `.
Do not guess CLI commands if a skill exists for the domain.

## Delegation Principle
1. Check if a ` + "`{domain}-coordinator`" + ` or ` + "`{domain}-developer`" + ` skill exists.
2. Run ` + "`grove-skills tree <skill-name>`" + ` to understand dependencies before executing.
3. Delegate to sub-skills rather than executing raw infrastructure commands.

IMPORTANT: If you need to fix or update a skill, DO NOT edit the files in ` + "`.claude/skills/`" + ` or ` + "`.opencode/skill/`" + `.
Use the ` + "`file_path`" + ` returned by the ` + "`grove-skills search`" + ` command to edit the original source ` + "`SKILL.md`" + ` file.
`

func newSkillsIntegrateCmd() *cobra.Command {
	var scope string

	cmd := &cobra.Command{
		Use:   "integrate",
		Short: "Inject agent instruction block into CLAUDE.md",
		Long: `Inject grove-skills agent instructions into a CLAUDE.md file.

This command adds a managed block to the target CLAUDE.md that teaches
agents how to discover and use skills properly. The block is bounded by
special markers (<!-- GROVE:SKILLS:START --> and <!-- GROVE:SKILLS:END -->)
so it can be safely updated on subsequent runs without affecting other
content in the file.

Scopes:
  project    - Target CLAUDE.md in the current directory
  ecosystem  - Target CLAUDE.md in the ecosystem root
  global     - Target CLAUDE.md in the user's home directory`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("could not get current directory: %w", err)
			}

			var targetDir string

			switch scope {
			case "project":
				targetDir = cwd
			case "ecosystem":
				node, err := workspace.GetProjectByPath(cwd)
				if err != nil {
					return fmt.Errorf("could not resolve ecosystem: %w", err)
				}
				if node.RootEcosystemPath != "" {
					targetDir = node.RootEcosystemPath
				} else if node.IsEcosystem() {
					targetDir = node.Path
				} else {
					return fmt.Errorf("current directory is not inside an ecosystem")
				}
			case "global":
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("could not get home directory: %w", err)
				}
				targetDir = home
			default:
				return fmt.Errorf("invalid scope: %s (valid: project, ecosystem, global)", scope)
			}

			claudePath := filepath.Join(targetDir, "CLAUDE.md")
			content := []byte{}

			// Read existing file if it exists
			if _, err := os.Stat(claudePath); err == nil {
				content, err = os.ReadFile(claudePath) //nolint:gosec // G304: path constructed from user's workspace
				if err != nil {
					return fmt.Errorf("failed to read existing CLAUDE.md: %w", err)
				}
			}

			startTag := "<!-- GROVE:SKILLS:START -->"
			endTag := "<!-- GROVE:SKILLS:END -->"
			block := fmt.Sprintf("%s\n%s\n%s", startTag, strings.TrimSpace(integrationPayload), endTag)

			// Regex to match existing block
			re := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(startTag) + `.*?` + regexp.QuoteMeta(endTag))
			var newContent []byte

			if re.Match(content) {
				// Replace existing block
				newContent = re.ReplaceAll(content, []byte(block))
			} else {
				// Append new block
				if len(content) > 0 && !strings.HasSuffix(string(content), "\n") {
					content = append(content, '\n')
				}
				newContent = append(content, []byte("\n"+block+"\n")...)
			}

			if err := os.WriteFile(claudePath, newContent, 0644); err != nil { //nolint:gosec // G306: CLAUDE.md must be world-readable
				return fmt.Errorf("failed to write CLAUDE.md: %w", err)
			}

			fmt.Printf("Successfully integrated grove-skills payload into %s\n", claudePath)
			return nil
		},
	}

	cmd.Flags().StringVar(&scope, "scope", "project", "Target scope for CLAUDE.md (project, ecosystem, global)")

	return cmd
}
