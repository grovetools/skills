package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/verify"
)

// TreeScenario tests the skill tree visualization command.
func TreeScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "skill-tree",
		Description: "Verify skill tree command visualizes skill dependencies",
		Steps: []harness.Step{
			harness.NewStep("setup environment with skill tree", setupSkillTreeEnvironment),
			harness.NewStep("tree command shows root skill", treeCommandShowsRootSkill),
			harness.NewStep("tree command shows dependencies recursively", treeCommandShowsDependencies),
			harness.NewStep("tree command handles missing skills gracefully", treeCommandHandlesMissingSkills),
			harness.NewStep("tree command detects circular dependencies", treeCommandDetectsCircular),
		},
	}
}

// setupSkillTreeEnvironment creates a notebook environment with skills that form a dependency tree.
// The tree structure:
//
//	grove-refactor-coordinator (L1 Orchestrator)
//	├── grove-flow-builder (L3 Tooling)
//	├── grove-cx-builder (L3 Tooling)
//	└── genohype-pool-developer (L2 Domain Protocol)
//	    ├── genohype-cluster-ops (L3 Operator)
//	    └── genohype-pool-analyzer (L3 Operator)
func setupSkillTreeEnvironment(ctx *harness.Context) error {
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

	// 2. Create a project directory
	projectDir := ctx.NewDir("tree-test-project")
	if err := fs.WriteString(filepath.Join(projectDir, "grove.toml"), `name = "tree-test-project"
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

	// 3. Create skills in the notebook's skills directory
	skillsDir := filepath.Join(notebookRoot, "workspaces", "tree-test-project", "skills")

	// --- L3 Operators (leaf nodes) ---

	// genohype-cluster-ops (no dependencies)
	clusterOpsDir := filepath.Join(skillsDir, "genohype-cluster-ops")
	if err := fs.CreateDir(clusterOpsDir); err != nil {
		return err
	}
	clusterOpsContent := `---
name: genohype-cluster-ops
description: Creates and destroys VMs for testing
domain: genohype-cluster
---

# Cluster Operations

Commands for managing GCP cluster lifecycle.
`
	if err := fs.WriteString(filepath.Join(clusterOpsDir, "SKILL.md"), clusterOpsContent); err != nil {
		return err
	}

	// genohype-pool-analyzer (no dependencies)
	poolAnalyzerDir := filepath.Join(skillsDir, "genohype-pool-analyzer")
	if err := fs.CreateDir(poolAnalyzerDir); err != nil {
		return err
	}
	poolAnalyzerContent := `---
name: genohype-pool-analyzer
description: Reads telemetry and logs from pool clusters
domain: genohype-pool
---

# Pool Analyzer

Commands for observing pool state and metrics.
`
	if err := fs.WriteString(filepath.Join(poolAnalyzerDir, "SKILL.md"), poolAnalyzerContent); err != nil {
		return err
	}

	// --- L3 Tooling (leaf nodes) ---

	// grove-flow-builder (no dependencies)
	flowBuilderDir := filepath.Join(skillsDir, "grove-flow-builder")
	if err := fs.CreateDir(flowBuilderDir); err != nil {
		return err
	}
	flowBuilderContent := `---
name: grove-flow-builder
description: Designs and executes flow plans
domain: grove-flow
---

# Flow Builder

Creates and manages execution plans.
`
	if err := fs.WriteString(filepath.Join(flowBuilderDir, "SKILL.md"), flowBuilderContent); err != nil {
		return err
	}

	// grove-cx-builder (no dependencies)
	cxBuilderDir := filepath.Join(skillsDir, "grove-cx-builder")
	if err := fs.CreateDir(cxBuilderDir); err != nil {
		return err
	}
	cxBuilderContent := `---
name: grove-cx-builder
description: Curates file context rules
domain: grove-cx
---

# Context Builder

Curates context for LLM agents.
`
	if err := fs.WriteString(filepath.Join(cxBuilderDir, "SKILL.md"), cxBuilderContent); err != nil {
		return err
	}

	// --- L2 Domain Protocol (middle tier) ---

	// genohype-pool-developer (depends on cluster-ops and pool-analyzer)
	poolDeveloperDir := filepath.Join(skillsDir, "genohype-pool-developer")
	if err := fs.CreateDir(poolDeveloperDir); err != nil {
		return err
	}
	poolDeveloperContent := `---
name: genohype-pool-developer
description: SOP for testing pool logic
domain: genohype-pool
requires:
  - genohype-cluster-ops
  - genohype-pool-analyzer
---

# Pool Developer

Standard operating procedure for testing pool changes.

## Required Sub-Skills

This skill requires:
- genohype-cluster-ops: For creating/destroying test clusters
- genohype-pool-analyzer: For observing test results
`
	if err := fs.WriteString(filepath.Join(poolDeveloperDir, "SKILL.md"), poolDeveloperContent); err != nil {
		return err
	}

	// --- L1 Orchestrator (top tier) ---

	// grove-refactor-coordinator (depends on flow-builder, cx-builder, pool-developer)
	refactorCoordinatorDir := filepath.Join(skillsDir, "grove-refactor-coordinator")
	if err := fs.CreateDir(refactorCoordinatorDir); err != nil {
		return err
	}
	refactorCoordinatorContent := `---
name: grove-refactor-coordinator
description: Orchestrates multi-phase refactoring
domain: grove-refactor
requires:
  - grove-flow-builder
  - grove-cx-builder
  - genohype-pool-developer
---

# Refactor Coordinator

Orchestrates the refactoring workflow from analysis to verification.

## Required Sub-Skills

This orchestrator delegates to:
- grove-flow-builder: For creating execution plans
- grove-cx-builder: For curating context
- genohype-pool-developer: For domain-specific verification
`
	if err := fs.WriteString(filepath.Join(refactorCoordinatorDir, "SKILL.md"), refactorCoordinatorContent); err != nil {
		return err
	}

	// --- Skill with missing dependency (for error handling test) ---

	// broken-skill (depends on non-existent skill)
	brokenSkillDir := filepath.Join(skillsDir, "broken-skill")
	if err := fs.CreateDir(brokenSkillDir); err != nil {
		return err
	}
	brokenSkillContent := `---
name: broken-skill
description: A skill that depends on a non-existent skill
requires:
  - non-existent-skill
---

# Broken Skill

This skill has a dependency that doesn't exist.
`
	if err := fs.WriteString(filepath.Join(brokenSkillDir, "SKILL.md"), brokenSkillContent); err != nil {
		return err
	}

	// --- Skills with circular dependency (for cycle detection test) ---

	// circular-a (depends on circular-b)
	circularADir := filepath.Join(skillsDir, "circular-a")
	if err := fs.CreateDir(circularADir); err != nil {
		return err
	}
	circularAContent := `---
name: circular-a
description: First skill in a circular dependency
requires:
  - circular-b
---

# Circular A

This skill is part of a circular dependency.
`
	if err := fs.WriteString(filepath.Join(circularADir, "SKILL.md"), circularAContent); err != nil {
		return err
	}

	// circular-b (depends on circular-a)
	circularBDir := filepath.Join(skillsDir, "circular-b")
	if err := fs.CreateDir(circularBDir); err != nil {
		return err
	}
	circularBContent := `---
name: circular-b
description: Second skill in a circular dependency
requires:
  - circular-a
---

# Circular B

This skill is part of a circular dependency.
`
	if err := fs.WriteString(filepath.Join(circularBDir, "SKILL.md"), circularBContent); err != nil {
		return err
	}

	// Store paths for later steps
	ctx.Set("project_dir", projectDir)
	ctx.Set("notebook_root", notebookRoot)

	return nil
}

// treeCommandShowsRootSkill verifies that tree command shows the root skill with description.
func treeCommandShowsRootSkill(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()
	projectDir := ctx.GetString("project_dir")

	// Test tree with a leaf skill (no dependencies)
	cmd := command.New(binary, "tree", "grove-flow-builder").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("tree grove-flow-builder", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("tree command failed: %s", result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows skill name", result.Stdout, "grove-flow-builder")
		v.Contains("shows description", result.Stdout, "Designs and executes flow plans")
	})
}

// treeCommandShowsDependencies verifies that tree shows the full dependency tree recursively.
func treeCommandShowsDependencies(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()
	projectDir := ctx.GetString("project_dir")

	// Test tree with the top-level orchestrator
	cmd := command.New(binary, "tree", "grove-refactor-coordinator").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("tree grove-refactor-coordinator", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("tree command failed: %s", result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		// Root skill
		v.Contains("shows root skill", result.Stdout, "grove-refactor-coordinator")
		v.Contains("shows root description", result.Stdout, "multi-phase refactoring")

		// Direct dependencies (L3 tooling)
		v.Contains("shows flow-builder dependency", result.Stdout, "grove-flow-builder")
		v.Contains("shows cx-builder dependency", result.Stdout, "grove-cx-builder")

		// L2 domain protocol
		v.Contains("shows pool-developer dependency", result.Stdout, "genohype-pool-developer")

		// Nested dependencies (L3 operators under pool-developer)
		v.Contains("shows cluster-ops nested dependency", result.Stdout, "genohype-cluster-ops")
		v.Contains("shows pool-analyzer nested dependency", result.Stdout, "genohype-pool-analyzer")

		// Verify tree structure characters are present
		v.True("has tree branch characters", strings.Contains(result.Stdout, "├──") || strings.Contains(result.Stdout, "└──"))
	})
}

// treeCommandHandlesMissingSkills verifies that tree handles missing dependencies gracefully.
func treeCommandHandlesMissingSkills(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()
	projectDir := ctx.GetString("project_dir")

	// Test tree with a skill that has a missing dependency
	cmd := command.New(binary, "tree", "broken-skill").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("tree broken-skill", result.Stdout, result.Stderr)

	// The command should still succeed (exit 0) but show the missing dependency
	if result.ExitCode != 0 {
		return fmt.Errorf("tree command should handle missing dependencies gracefully, got exit code %d: %s", result.ExitCode, result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows broken-skill", result.Stdout, "broken-skill")
		v.Contains("shows missing dependency indicator", result.Stdout, "not found")
		v.Contains("shows missing skill name", result.Stdout, "non-existent-skill")
	})
}

// treeCommandDetectsCircular verifies that tree detects and handles circular dependencies.
func treeCommandDetectsCircular(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()
	projectDir := ctx.GetString("project_dir")

	// Test tree with a skill that has circular dependency
	cmd := command.New(binary, "tree", "circular-a").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("tree circular-a", result.Stdout, result.Stderr)

	// The command should still succeed but detect the cycle
	if result.ExitCode != 0 {
		return fmt.Errorf("tree command should handle circular dependencies gracefully, got exit code %d: %s", result.ExitCode, result.Stderr)
	}

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows circular-a", result.Stdout, "circular-a")
		v.Contains("shows circular-b", result.Stdout, "circular-b")
		v.Contains("detects circular dependency", result.Stdout, "circular dependency detected")
	})
}
