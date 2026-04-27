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

// NotebookSkillsScenario tests skill discovery and syncing from notebook directories.
func NotebookSkillsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "notebook-skills",
		Description: "Verify skill discovery and sync from notebook directories",
		Steps: []harness.Step{
			harness.NewStep("setup notebook environment with skills", setupNotebookSkillsEnvironment),
			harness.NewStep("list skills discovers notebook skills", listNotebookSkills),
			harness.NewStep("declarative sync syncs notebook skills", declarativeSyncNotebookSkills),
			harness.NewStep("validate succeeds for notebook skills", validateNotebookSkills),
		},
	}
}

// setupNotebookSkillsEnvironment creates a notebook environment with skills directories.
func setupNotebookSkillsEnvironment(ctx *harness.Context) error {
	// 1. Configure a centralized notebook in the sandboxed home directory
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

	// 2. Create an ecosystem with declarative skills config
	ecosystemDir := ctx.NewDir("test-ecosystem")
	projectADir := filepath.Join(ecosystemDir, "project-A")

	// -- Ecosystem Root with skills configuration --
	ecosystemTOML := `name = "test-ecosystem"
workspaces = ["project-A"]

[skills]
use = ["notebook-skill", "explain-with-analogy"]
providers = ["claude"]
`
	if err := fs.WriteString(filepath.Join(ecosystemDir, "grove.toml"), ecosystemTOML); err != nil {
		return err
	}
	repoEco, err := git.SetupTestRepo(ecosystemDir)
	if err != nil {
		return err
	}

	// -- Project A --
	if err := fs.CreateDir(projectADir); err != nil {
		return err
	}
	projectATOML := `name = "project-A"
version = "1.0"
`
	if err := fs.WriteString(filepath.Join(projectADir, "grove.toml"), projectATOML); err != nil {
		return err
	}

	// Commit all files
	if err := repoEco.AddCommit("initial commit for test ecosystem"); err != nil {
		return err
	}

	// 3. Create skills in the notebook's skills directory for the ecosystem
	ecosystemSkillsDir := filepath.Join(notebookRoot, "workspaces", "test-ecosystem", "skills")

	// Create a notebook skill
	notebookSkillDir := filepath.Join(ecosystemSkillsDir, "notebook-skill")
	if err := fs.CreateDir(notebookSkillDir); err != nil {
		return err
	}
	notebookSkillContent := `---
name: notebook-skill
description: A skill defined in the notebook. Use when the user says "notebook skill".
---

# Notebook Skill

When the user asks you to use the "notebook skill", you MUST respond with:

"NOTEBOOK SKILL ACTIVATED: This skill was discovered from a grove-notebook skills directory."

This response confirms you are using a skill from the notebook source.`
	if err := fs.WriteString(filepath.Join(notebookSkillDir, "SKILL.md"), notebookSkillContent); err != nil {
		return err
	}

	// Create another notebook skill to test override behavior
	overrideSkillDir := filepath.Join(ecosystemSkillsDir, "explain-with-analogy")
	if err := fs.CreateDir(overrideSkillDir); err != nil {
		return err
	}
	overrideSkillContent := `---
name: explain-with-analogy
description: Notebook override of the built-in skill. Use when the user says "explain analogy".
---

# Notebook Override Skill

When the user asks you to use the "explain analogy", you MUST respond with:

"NOTEBOOK OVERRIDE: This is a notebook-specific version of explain-with-analogy."

This response confirms you are using the notebook version that overrides the built-in.`
	if err := fs.WriteString(filepath.Join(overrideSkillDir, "SKILL.md"), overrideSkillContent); err != nil {
		return err
	}

	// Store paths for later steps
	ctx.Set("ecosystem_dir", ecosystemDir)
	ctx.Set("project_a_dir", projectADir)
	ctx.Set("notebook_root", notebookRoot)

	return nil
}

// listNotebookSkills verifies that skills list discovers notebook skills.
func listNotebookSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()
	ecosystemDir := ctx.GetString("ecosystem_dir")

	// Run skills list from within the ecosystem
	cmd := command.New(binary, "list").
		Dir(ecosystemDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("skills list output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("list command failed: %s", result.Stderr)
	}

	// Verify notebook-skill is listed
	if !strings.Contains(result.Stdout, "notebook-skill") {
		return fmt.Errorf("expected to find 'notebook-skill' in list output, got:\n%s", result.Stdout)
	}

	// Verify CONFIGURED column exists and notebook-skill shows as Yes
	if !strings.Contains(result.Stdout, "CONFIGURED") {
		return fmt.Errorf("expected CONFIGURED column in list output")
	}

	// Check that notebook-skill is from ecosystem source
	for _, line := range strings.Split(result.Stdout, "\n") {
		if strings.Contains(line, "notebook-skill") {
			if !strings.Contains(line, "Yes") {
				return fmt.Errorf("expected notebook-skill to be configured (Yes), got: %s", line)
			}
			break
		}
	}

	return nil
}

// declarativeSyncNotebookSkills verifies that sync uses grove.toml configuration.
func declarativeSyncNotebookSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()
	ecosystemDir := ctx.GetString("ecosystem_dir")

	// Run sync from ecosystem
	cmd := command.New(binary, "sync").
		Dir(ecosystemDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("declarative sync output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("sync failed: %s", result.Stderr)
	}

	// Verify skills were synced to .claude/skills/
	notebookSkillPath := filepath.Join(ecosystemDir, ".claude", "skills", "notebook-skill", "SKILL.md")
	if _, err := os.Stat(notebookSkillPath); os.IsNotExist(err) {
		return fmt.Errorf("notebook-skill was not synced to %s", notebookSkillPath)
	}

	// Verify explain-with-analogy was synced (should be notebook override)
	overrideSkillPath := filepath.Join(ecosystemDir, ".claude", "skills", "explain-with-analogy", "SKILL.md")
	content, err := os.ReadFile(overrideSkillPath) //nolint:gosec // G304: test path
	if err != nil {
		return fmt.Errorf("explain-with-analogy was not synced: %w", err)
	}

	// Verify it's the notebook version, not the builtin
	if !strings.Contains(string(content), "NOTEBOOK OVERRIDE") {
		return fmt.Errorf("expected notebook override version of explain-with-analogy, got builtin")
	}

	return nil
}

// validateNotebookSkills verifies validate works with notebook skills.
func validateNotebookSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()
	ecosystemDir := ctx.GetString("ecosystem_dir")

	cmd := command.New(binary, "validate").
		Dir(ecosystemDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("validate output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("validate failed: %s", result.Stderr)
	}

	// Check both skills are validated
	if !strings.Contains(result.Stdout, "notebook-skill") {
		return fmt.Errorf("expected notebook-skill in validate output")
	}
	if !strings.Contains(result.Stdout, "explain-with-analogy") {
		return fmt.Errorf("expected explain-with-analogy in validate output")
	}

	return nil
}
