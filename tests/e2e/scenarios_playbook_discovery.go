package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
)

// PlaybookDiscoveryScenario verifies that playbook-owned skills are
// discovered, configured, and synced to .claude/skills/ when the
// playbook is authorized via `[playbooks] use = [...]` in grove.toml.
//
// This scenario was written to cover the MVP playbook surface and is
// expected to fail against a sync layer that doesn't auto-authorize
// playbook-owned skills through the skill resolver.
func PlaybookDiscoveryScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "playbook-discovery",
		Description: "Verify playbook-owned skills are discovered, configured, and synced",
		Steps: []harness.Step{
			harness.NewStep("setup playbook environment", setupPlaybookDiscoveryEnvironment),
			harness.NewStep("list discovers playbook skills as configured", listPlaybookSkills),
			harness.NewStep("sync flattens playbook skills to .claude/skills/", syncPlaybookSkills),
			harness.NewStep("validate succeeds for playbook skills", validatePlaybookSkills),
		},
	}
}

// setupPlaybookDiscoveryEnvironment configures an ecosystem whose
// grove.toml authorizes a test playbook (test-pb) that lives in the
// notebook at workspaces/<ecosystem>/playbooks/test-pb/.
func setupPlaybookDiscoveryEnvironment(ctx *harness.Context) error {
	notebookRoot := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb")

	globalTOML := fmt.Sprintf(`version = "1.0"

[groves.e2e-projects]
path = "%s"

[notebooks.rules]
default = "main"

[notebooks.definitions.main]
root_dir = "%s"
`, ctx.RootDir, notebookRoot)

	globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
	if err := fs.CreateDir(globalConfigDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.toml"), globalTOML); err != nil {
		return err
	}

	// Ecosystem with playbook authorization
	ecosystemDir := ctx.NewDir("test-ecosystem")
	ecosystemTOML := `name = "test-ecosystem"
workspaces = ["project-A"]

[playbooks]
use = ["test-pb"]
`
	if err := fs.WriteString(filepath.Join(ecosystemDir, "grove.toml"), ecosystemTOML); err != nil {
		return err
	}
	repoEco, err := git.SetupTestRepo(ecosystemDir)
	if err != nil {
		return err
	}

	projectADir := filepath.Join(ecosystemDir, "project-A")
	if err := fs.CreateDir(projectADir); err != nil {
		return err
	}
	projectATOML := `name = "project-A"
version = "1.0"
`
	if err := fs.WriteString(filepath.Join(projectADir, "grove.toml"), projectATOML); err != nil {
		return err
	}

	if err := repoEco.AddCommit("initial commit for test ecosystem"); err != nil {
		return err
	}

	// Test playbook in the notebook's workspaces/<ecosystem>/playbooks/ dir
	pbDir := filepath.Join(notebookRoot, "workspaces", "test-ecosystem", "playbooks", "test-pb")
	if err := fs.CreateDir(pbDir); err != nil {
		return err
	}
	manifest := `name = "test-pb"
version = "1.0.0"
description = "Minimal test playbook for e2e coverage"
default_recipe = "test-recipe"
`
	if err := fs.WriteString(filepath.Join(pbDir, "playbook.toml"), manifest); err != nil {
		return err
	}

	// Two skills to verify multi-skill sync
	helloDir := filepath.Join(pbDir, "skills", "test-pb-hello")
	if err := fs.CreateDir(helloDir); err != nil {
		return err
	}
	helloSkill := `---
name: test-pb-hello
description: Playbook-owned greeting skill. Use when the user says "playbook hello".
---

# Test Playbook Hello

When triggered, respond with "PLAYBOOK HELLO".
`
	if err := fs.WriteString(filepath.Join(helloDir, "SKILL.md"), helloSkill); err != nil {
		return err
	}

	goodbyeDir := filepath.Join(pbDir, "skills", "test-pb-goodbye")
	if err := fs.CreateDir(goodbyeDir); err != nil {
		return err
	}
	goodbyeSkill := `---
name: test-pb-goodbye
description: Playbook-owned farewell skill. Use when the user says "playbook goodbye".
---

# Test Playbook Goodbye

When triggered, respond with "PLAYBOOK GOODBYE".
`
	if err := fs.WriteString(filepath.Join(goodbyeDir, "SKILL.md"), goodbyeSkill); err != nil {
		return err
	}

	// A prompt + recipe for later scenario coverage (loaded by LoadPlaybook).
	promptsDir := filepath.Join(pbDir, "prompts")
	if err := fs.CreateDir(promptsDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(promptsDir, "greet.md"), "<!-- purpose: greet the user -->\n\nHello from test-pb.\n"); err != nil {
		return err
	}

	recipesDir := filepath.Join(pbDir, "recipes")
	if err := fs.CreateDir(recipesDir); err != nil {
		return err
	}
	recipeContent := `---
description: Trivial test recipe
---

# Test Recipe
`
	if err := fs.WriteString(filepath.Join(recipesDir, "test-recipe.md"), recipeContent); err != nil {
		return err
	}

	ctx.Set("ecosystem_dir", ecosystemDir)
	ctx.Set("project_a_dir", projectADir)
	ctx.Set("notebook_root", notebookRoot)
	ctx.Set("playbook_dir", pbDir)
	return nil
}

// listPlaybookSkills verifies that `skills list` discovers and marks
// playbook-owned skills as configured.
func listPlaybookSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	ecosystemDir := ctx.GetString("ecosystem_dir")
	cmd := command.New(binary, "list").
		Dir(ecosystemDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("skills list output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("skills list failed: %s", result.Stderr)
	}

	for _, skillName := range []string{"test-pb-hello", "test-pb-goodbye"} {
		if !strings.Contains(result.Stdout, skillName) {
			return fmt.Errorf("expected playbook skill %q in list output, got:\n%s", skillName, result.Stdout)
		}
		// Verify the skill row shows CONFIGURED: Yes
		found := false
		for _, line := range strings.Split(result.Stdout, "\n") {
			if strings.Contains(line, skillName) {
				if !strings.Contains(line, "Yes") {
					return fmt.Errorf("expected %q to be configured (Yes), got line: %s", skillName, line)
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("could not locate row for %q", skillName)
		}
	}

	return nil
}

// syncPlaybookSkills verifies that `skills sync` flattens both
// playbook-owned skills into the worktree's .claude/skills/ directory.
func syncPlaybookSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	ecosystemDir := ctx.GetString("ecosystem_dir")
	cmd := command.New(binary, "sync").
		Dir(ecosystemDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("skills sync output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("skills sync failed: %s", result.Stderr)
	}

	for _, skillName := range []string{"test-pb-hello", "test-pb-goodbye"} {
		skillPath := filepath.Join(ecosystemDir, ".claude", "skills", skillName, "SKILL.md")
		content, err := os.ReadFile(skillPath) //nolint:gosec // G304: test path
		if err != nil {
			return fmt.Errorf("expected playbook skill synced to %s: %w", skillPath, err)
		}
		if !strings.Contains(string(content), "name: "+skillName) {
			return fmt.Errorf("synced skill %s missing expected frontmatter", skillName)
		}
	}

	return nil
}

// validatePlaybookSkills verifies that `skills validate` accepts both
// playbook-owned skills without error.
func validatePlaybookSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	ecosystemDir := ctx.GetString("ecosystem_dir")
	cmd := command.New(binary, "validate").
		Dir(ecosystemDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("skills validate output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("skills validate failed: %s", result.Stderr)
	}

	for _, skillName := range []string{"test-pb-hello", "test-pb-goodbye"} {
		if !strings.Contains(result.Stdout, skillName) {
			return fmt.Errorf("expected %q in validate output", skillName)
		}
	}

	return nil
}
