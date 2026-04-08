package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
)

// PlaybookNestedSkillsScenario verifies that playbook bundles with
// nested skill hierarchies (including references/ directories and
// nested skill subdirectories) sync correctly to .claude/skills/.
func PlaybookNestedSkillsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "playbook-nested-skills",
		Description: "Verify nested playbook skill hierarchies and references/ directories sync correctly",
		Steps: []harness.Step{
			harness.NewStep("setup nested playbook", setupNestedPlaybookEnvironment),
			harness.NewStep("sync copies nested skills and references", syncNestedPlaybookSkills),
			harness.NewStep("modifying references and resyncing picks up changes", resyncNestedPlaybookReferences),
		},
	}
}

func setupNestedPlaybookEnvironment(ctx *harness.Context) error {
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

	ecosystemDir := ctx.NewDir("nested-ecosystem")
	ecosystemTOML := `name = "nested-ecosystem"
workspaces = []

[playbooks]
use = ["nested-pb"]
`
	if err := fs.WriteString(filepath.Join(ecosystemDir, "grove.toml"), ecosystemTOML); err != nil {
		return err
	}
	repo, err := git.SetupTestRepo(ecosystemDir)
	if err != nil {
		return err
	}
	if err := repo.AddCommit("init"); err != nil {
		return err
	}

	pbDir := filepath.Join(notebookRoot, "workspaces", "nested-ecosystem", "playbooks", "nested-pb")
	if err := fs.CreateDir(pbDir); err != nil {
		return err
	}
	manifest := `name = "nested-pb"
version = "1.0.0"
description = "Playbook with nested skill hierarchy"
`
	if err := fs.WriteString(filepath.Join(pbDir, "playbook.toml"), manifest); err != nil {
		return err
	}

	// Parent skill with references/ directory
	parentSKILL := `---
name: parent-skill
description: Parent skill with references and nested children.
---

# Parent
`
	if err := fs.WriteString(filepath.Join(pbDir, "skills", "parent-skill", "SKILL.md"), parentSKILL); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(pbDir, "skills", "parent-skill", "references", "example.md"), "initial example\n"); err != nil {
		return err
	}

	// Nested children under parent
	child1 := `---
name: parent-skill-child1
description: First nested child skill.
---

# Child1
`
	if err := fs.WriteString(filepath.Join(pbDir, "skills", "parent-skill", "parent-skill-child1", "SKILL.md"), child1); err != nil {
		return err
	}
	child2 := `---
name: parent-skill-child2
description: Second nested child skill.
---

# Child2
`
	if err := fs.WriteString(filepath.Join(pbDir, "skills", "parent-skill", "parent-skill-child2", "SKILL.md"), child2); err != nil {
		return err
	}

	ctx.Set("ecosystem_dir", ecosystemDir)
	ctx.Set("playbook_dir", pbDir)
	return nil
}

func syncNestedPlaybookSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}
	ecosystemDir := ctx.GetString("ecosystem_dir")

	cmd := command.New(binary, "sync").
		Dir(ecosystemDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()
	ctx.ShowCommandOutput("nested sync", result.Stdout, result.Stderr)
	if result.ExitCode != 0 {
		return fmt.Errorf("sync failed: %s", result.Stderr)
	}

	// All 3 skills should be present, flattened into .claude/skills/
	for _, skillName := range []string{"parent-skill", "parent-skill-child1", "parent-skill-child2"} {
		path := filepath.Join(ecosystemDir, ".claude", "skills", skillName, "SKILL.md")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return fmt.Errorf("expected nested skill %s at %s", skillName, path)
		}
	}

	// references/example.md under parent-skill must have been copied.
	refPath := filepath.Join(ecosystemDir, ".claude", "skills", "parent-skill", "references", "example.md")
	data, err := os.ReadFile(refPath)
	if err != nil {
		return fmt.Errorf("expected parent-skill references/example.md at %s: %w", refPath, err)
	}
	if string(data) != "initial example\n" {
		return fmt.Errorf("unexpected references content: %q", string(data))
	}
	return nil
}

func resyncNestedPlaybookReferences(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}
	ecosystemDir := ctx.GetString("ecosystem_dir")
	pbDir := ctx.GetString("playbook_dir")

	// Modify the references file in the source playbook.
	srcRef := filepath.Join(pbDir, "skills", "parent-skill", "references", "example.md")
	if err := fs.WriteString(srcRef, "updated example\n"); err != nil {
		return err
	}

	cmd := command.New(binary, "sync").
		Dir(ecosystemDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	if result := cmd.Run(); result.ExitCode != 0 {
		return fmt.Errorf("resync failed: %s", result.Stderr)
	}

	destRef := filepath.Join(ecosystemDir, ".claude", "skills", "parent-skill", "references", "example.md")
	data, err := os.ReadFile(destRef)
	if err != nil {
		return fmt.Errorf("destination references file missing after resync: %w", err)
	}
	if string(data) != "updated example\n" {
		return fmt.Errorf("expected updated content after resync, got: %q", string(data))
	}
	return nil
}
