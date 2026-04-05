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

// NestedSkillsScenario tests deep discovery and resolution of nested skill directories.
func NestedSkillsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "nested-skills",
		Description: "Verify discovery, sync, and resolution of nested skill directories",
		Tags:        []string{"nested", "discovery", "sync"},
		Steps: []harness.Step{
			harness.NewStep("setup environment with nested skills", setupNestedSkillsEnv),
			harness.NewStep("list discovers nested and org-folder skills", listNestedSkills),
			harness.NewStep("show resolves nested skill paths", showNestedSkill),
			harness.NewStep("sync preserves nested paths and transitive deps", syncNestedSkills),
		},
	}
}

func setupNestedSkillsEnv(ctx *harness.Context) error {
	// Configure notebook
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

	// Create project with skills config that only lists "sear"
	projectDir := ctx.NewDir("nested-project")
	projectTOML := `name = "nested-project"
version = "1.0"
[skills]
use = ["sear"]
`
	if err := fs.WriteString(filepath.Join(projectDir, "grove.toml"), projectTOML); err != nil {
		return err
	}
	repo, err := git.SetupTestRepo(projectDir)
	if err != nil {
		return err
	}
	if err := repo.AddCommit("initial"); err != nil {
		return err
	}

	skillsDir := filepath.Join(notebookRoot, "workspaces", "nested-project", "skills")

	// 1. Flat skill
	prepDir := filepath.Join(skillsDir, "prep")
	if err := fs.CreateDir(prepDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(prepDir, "SKILL.md"), "---\nname: prep\ndescription: A flat skill\n---\n# prep\n"); err != nil {
		return err
	}

	// 2. Organizational folder skill: dev/cx-builder (dev/ has no SKILL.md)
	cxDir := filepath.Join(skillsDir, "dev", "cx-builder")
	if err := fs.CreateDir(cxDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(cxDir, "SKILL.md"), "---\nname: cx-builder\ndescription: Org folder skill\n---\n# cx-builder\n"); err != nil {
		return err
	}

	// 3. Parent skill with sub-skills: sear/ -> heat-pan
	searDir := filepath.Join(skillsDir, "sear")
	if err := fs.CreateDir(searDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(searDir, "SKILL.md"), "---\nname: sear\ndescription: Parent skill\nskill_sequence:\n  - heat-pan\n---\n# sear\n"); err != nil {
		return err
	}

	heatPanDir := filepath.Join(searDir, "heat-pan")
	if err := fs.CreateDir(heatPanDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(heatPanDir, "SKILL.md"), "---\nname: heat-pan\ndescription: Sub skill\n---\n# heat-pan\n"); err != nil {
		return err
	}

	ctx.Set("project_dir", projectDir)
	ctx.Set("notebook_root", notebookRoot)
	return nil
}

func listNestedSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}
	projectDir := ctx.GetString("project_dir")

	// Search should find the flat skill
	cmd := command.New(binary, "search", "flat skill").Dir(projectDir).Env(
		"HOME="+ctx.HomeDir(),
		"XDG_CONFIG_HOME="+filepath.Join(ctx.HomeDir(), ".config"),
	)
	res := cmd.Run()
	ctx.ShowCommandOutput("search flat skill", res.Stdout, res.Stderr)
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "prep") {
		return fmt.Errorf("failed to find flat prep skill, stdout: %s, stderr: %s", res.Stdout, res.Stderr)
	}

	// Search should find the org-folder skill
	cmd = command.New(binary, "search", "org folder skill").Dir(projectDir).Env(
		"HOME="+ctx.HomeDir(),
		"XDG_CONFIG_HOME="+filepath.Join(ctx.HomeDir(), ".config"),
	)
	res = cmd.Run()
	ctx.ShowCommandOutput("search org folder skill", res.Stdout, res.Stderr)
	if res.ExitCode != 0 || !strings.Contains(res.Stdout, "cx-builder") {
		return fmt.Errorf("failed to find org-folder cx-builder skill, stdout: %s, stderr: %s", res.Stdout, res.Stderr)
	}

	return nil
}

func showNestedSkill(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}
	projectDir := ctx.GetString("project_dir")

	// Show the nested sub-skill
	cmd := command.New(binary, "show", "heat-pan").Dir(projectDir).Env(
		"HOME="+ctx.HomeDir(),
		"XDG_CONFIG_HOME="+filepath.Join(ctx.HomeDir(), ".config"),
	)
	res := cmd.Run()
	ctx.ShowCommandOutput("show heat-pan", res.Stdout, res.Stderr)

	if res.ExitCode != 0 {
		return fmt.Errorf("show heat-pan failed: %s", res.Stderr)
	}

	if !strings.Contains(res.Stdout, "heat-pan") {
		return fmt.Errorf("show output missing heat-pan name, got: %s", res.Stdout)
	}

	return nil
}

func syncNestedSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}
	projectDir := ctx.GetString("project_dir")

	// Run sync
	cmd := command.New(binary, "sync").Dir(projectDir).Env(
		"HOME="+ctx.HomeDir(),
		"XDG_CONFIG_HOME="+filepath.Join(ctx.HomeDir(), ".config"),
	)
	res := cmd.Run()
	ctx.ShowCommandOutput("sync", res.Stdout, res.Stderr)

	if res.ExitCode != 0 {
		return fmt.Errorf("sync failed: %s", res.Stderr)
	}

	// Verify transitive sub-skill was synced to the correct nested path
	heatPanPath := filepath.Join(projectDir, ".claude", "skills", "sear", "heat-pan", "SKILL.md")
	if _, err := os.Stat(heatPanPath); os.IsNotExist(err) {
		return fmt.Errorf("transitive sub-skill heat-pan not synced to nested location %s", heatPanPath)
	}

	// Verify the parent skill was also synced
	searPath := filepath.Join(projectDir, ".claude", "skills", "sear", "SKILL.md")
	if _, err := os.Stat(searPath); os.IsNotExist(err) {
		return fmt.Errorf("parent skill sear not synced to %s", searPath)
	}

	// Ensure org folder "dev" was NOT synced (since it's not in grove.toml use)
	devPath := filepath.Join(projectDir, ".claude", "skills", "dev")
	if _, err := os.Stat(devPath); !os.IsNotExist(err) {
		return fmt.Errorf("org folder 'dev' should not be synced unless explicitly configured, but found at %s", devPath)
	}

	return nil
}
