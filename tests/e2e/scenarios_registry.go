package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/verify"
)

// RegistryScenario tests the skill registry model's integration with CLI commands.
// It verifies that LoadedSkill provenance (SourceType, PhysicalPath) is correctly
// surfaced through show, search, list --grouped, and tree commands.
func RegistryScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "skill-registry",
		Description: "Verify skill registry model provides correct provenance through CLI commands",
		Steps: []harness.Step{
			harness.NewStep("setup registry test environment", setupRegistryEnvironment),
			harness.NewStep("show builtin skill has correct provenance", registryShowBuiltinProvenance),
			harness.NewStep("show project skill has correct provenance", registryShowProjectProvenance),
			harness.NewStep("show --json returns provenance fields", registryShowJSONProvenance),
			harness.NewStep("search returns results with registry-backed source info", registrySearchProvenance),
			harness.NewStep("search --json includes source and file_path", registrySearchJSONProvenance),
			harness.NewStep("list --grouped resolves domains via registry", registryListGrouped),
			harness.NewStep("tree resolves dependencies via registry without access control", registryTreeResolution),
		},
	}
}

// setupRegistryEnvironment creates a workspace with project-level skills in a notebook,
// covering multiple domains for grouped list testing and a dependency chain for tree testing.
func setupRegistryEnvironment(ctx *harness.Context) error {
	// 1. Configure notebook in the sandboxed home
	notebookRoot := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb")
	globalTOML := fmt.Sprintf(`version = "1.0"

[groves.e2e-registry]
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

	// 2. Create a project directory with a git repo
	projectDir := ctx.NewDir("registry-test-project")
	if err := fs.WriteString(filepath.Join(projectDir, "grove.toml"), `name = "registry-test-project"
version = "1.0"
`); err != nil {
		return err
	}
	repo, err := git.SetupTestRepo(projectDir)
	if err != nil {
		return err
	}
	if err := repo.AddCommit("initial commit"); err != nil {
		return err
	}

	// 3. Create project-level skills in notebook with distinct domains
	skillsDir := filepath.Join(notebookRoot, "workspaces", "registry-test-project", "skills")

	// Skill A: domain "registry-testing"
	skillADir := filepath.Join(skillsDir, "registry-test-alpha")
	if err := fs.CreateDir(skillADir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(skillADir, "SKILL.md"), `---
name: registry-test-alpha
description: Alpha skill for registry provenance testing
domain: registry-testing
---

# Registry Test Alpha

A project-level skill to verify registry provenance tracking.
`); err != nil {
		return err
	}

	// Skill B: domain "registry-ops" with a dependency on a builtin
	skillBDir := filepath.Join(skillsDir, "registry-test-beta")
	if err := fs.CreateDir(skillBDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(skillBDir, "SKILL.md"), `---
name: registry-test-beta
description: Beta skill with builtin dependency
domain: registry-ops
requires:
  - explain-with-analogy
---

# Registry Test Beta

A project-level skill that depends on a builtin skill.
`); err != nil {
		return err
	}

	// Skill C: same domain as A, for grouped list verification
	skillCDir := filepath.Join(skillsDir, "registry-test-gamma")
	if err := fs.CreateDir(skillCDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(skillCDir, "SKILL.md"), `---
name: registry-test-gamma
description: Gamma skill in the same domain as alpha
domain: registry-testing
---

# Registry Test Gamma

Another skill in the registry-testing domain for grouped listing.
`); err != nil {
		return err
	}

	ctx.Set("project_dir", projectDir)
	ctx.Set("notebook_root", notebookRoot)

	return nil
}

// registryShowBuiltinProvenance verifies show command displays correct provenance for builtin skills.
func registryShowBuiltinProvenance(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	cmd := command.New(binary, "show", "explain-with-analogy").
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("show builtin provenance", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("show command failed: %s", result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("source is builtin", result.Stdout, "builtin")
		v.Contains("path is read-only", result.Stdout, "(builtin - read only)")
	})
}

// registryShowProjectProvenance verifies show command displays correct provenance for project skills.
func registryShowProjectProvenance(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	projectDir := ctx.GetString("project_dir")

	cmd := command.New(binary, "show", "registry-test-alpha").
		Dir(projectDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("show project provenance", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("show command failed: %s", result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows skill name", result.Stdout, "registry-test-alpha")
		v.Contains("source is project", result.Stdout, "project")
		v.Contains("shows domain", result.Stdout, "registry-testing")
		// Path should point to the actual notebook location, not "(builtin - read only)"
		v.True("path is not builtin", !strings.Contains(result.Stdout, "(builtin - read only)"))
		v.Contains("path contains skill name", result.Stdout, "registry-test-alpha")
	})
}

// registryShowJSONProvenance verifies JSON output includes registry-backed provenance fields.
func registryShowJSONProvenance(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	projectDir := ctx.GetString("project_dir")

	// Test builtin skill JSON provenance
	cmd := command.New(binary, "show", "explain-with-analogy", "--json").
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("show builtin --json", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("show --json failed for builtin: %s", result.Stderr)
	}

	var builtinResult struct {
		Name     string `json:"name"`
		Source   string `json:"source"`
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &builtinResult); err != nil {
		return fmt.Errorf("invalid JSON output: %w\nOutput: %s", err, result.Stdout)
	}

	if builtinResult.Source != "builtin" {
		return fmt.Errorf("expected source 'builtin', got '%s'", builtinResult.Source)
	}
	if builtinResult.FilePath != "(builtin - read only)" {
		return fmt.Errorf("expected file_path '(builtin - read only)', got '%s'", builtinResult.FilePath)
	}

	// Test project skill JSON provenance
	cmd = command.New(binary, "show", "registry-test-alpha", "--json").
		Dir(projectDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result = cmd.Run()

	ctx.ShowCommandOutput("show project --json", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("show --json failed for project skill: %s", result.Stderr)
	}

	var projectResult struct {
		Name     string `json:"name"`
		Source   string `json:"source"`
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &projectResult); err != nil {
		return fmt.Errorf("invalid JSON output: %w\nOutput: %s", err, result.Stdout)
	}

	if projectResult.Source != "project" {
		return fmt.Errorf("expected source 'project', got '%s'", projectResult.Source)
	}
	if !strings.Contains(projectResult.FilePath, "registry-test-alpha") {
		return fmt.Errorf("expected file_path to contain 'registry-test-alpha', got '%s'", projectResult.FilePath)
	}

	return nil
}

// registrySearchProvenance verifies search returns results with correct registry-backed source info.
func registrySearchProvenance(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	projectDir := ctx.GetString("project_dir")

	// Search for our custom project skill
	cmd := command.New(binary, "search", "registry-test-alpha").
		Dir(projectDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("search project skill", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("search command failed: %s", result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("found the skill", result.Stdout, "registry-test-alpha")
		v.Contains("match reason is name", result.Stdout, "name")
	})
}

// registrySearchJSONProvenance verifies search --json includes source and file_path fields.
func registrySearchJSONProvenance(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	projectDir := ctx.GetString("project_dir")

	cmd := command.New(binary, "search", "registry-test-alpha", "--json").
		Dir(projectDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("search --json project skill", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("search --json command failed: %s", result.Stderr)
	}

	var searchResults []struct {
		Name        string `json:"name"`
		Source      string `json:"source"`
		FilePath    string `json:"file_path"`
		MatchReason string `json:"match_reason"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &searchResults); err != nil {
		return fmt.Errorf("invalid JSON: %w\nOutput: %s", err, result.Stdout)
	}

	if len(searchResults) == 0 {
		return fmt.Errorf("expected at least one search result, got empty array")
	}

	// Find the registry-test-alpha result
	var found bool
	for _, r := range searchResults {
		if r.Name == "registry-test-alpha" {
			found = true
			if r.Source != "project" {
				return fmt.Errorf("expected source 'project', got '%s'", r.Source)
			}
			if !strings.Contains(r.FilePath, "registry-test-alpha") {
				return fmt.Errorf("expected file_path to contain 'registry-test-alpha', got '%s'", r.FilePath)
			}
			if r.MatchReason == "" {
				return fmt.Errorf("expected non-empty match_reason")
			}
			break
		}
	}

	if !found {
		return fmt.Errorf("registry-test-alpha not found in search results: %v", searchResults)
	}

	return nil
}

// registryListGrouped verifies that list --grouped reads domains via the registry loader.
func registryListGrouped(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	projectDir := ctx.GetString("project_dir")

	cmd := command.New(binary, "list", "--grouped").
		Dir(projectDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("list --grouped output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("list --grouped failed: %s", result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		// Verify our custom domains appear as group headers
		v.Contains("registry-testing domain header", result.Stdout, "registry-testing")
		v.Contains("registry-ops domain header", result.Stdout, "registry-ops")
		// Verify skills appear under their domains
		v.Contains("alpha skill listed", result.Stdout, "registry-test-alpha")
		v.Contains("beta skill listed", result.Stdout, "registry-test-beta")
		v.Contains("gamma skill listed", result.Stdout, "registry-test-gamma")
	})
}

// registryTreeResolution verifies tree uses LoadSkillBypassingAccessWithService to resolve
// dependencies across source types (project -> builtin) without access control blocking.
func registryTreeResolution(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	projectDir := ctx.GetString("project_dir")

	// registry-test-beta requires explain-with-analogy (builtin)
	cmd := command.New(binary, "tree", "registry-test-beta").
		Dir(projectDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()

	ctx.ShowCommandOutput("tree registry-test-beta", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("tree command failed: %s", result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows project skill as root", result.Stdout, "registry-test-beta")
		v.Contains("shows builtin dependency", result.Stdout, "explain-with-analogy")
		// Verify tree structure characters
		v.True("has tree branch characters",
			strings.Contains(result.Stdout, "├──") || strings.Contains(result.Stdout, "└──"))
	})
}
