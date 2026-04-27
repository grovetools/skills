package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
)

// pollCondition polls a check function every 200ms until it returns true or timeout expires.
func pollCondition(timeout time.Duration, check func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("condition not met within %s", timeout)
}

// pollFileContains polls until a file exists and contains the expected substring.
func pollFileContains(path, expected string, timeout time.Duration) error {
	return pollCondition(timeout, func() bool {
		content, err := os.ReadFile(path) //nolint:gosec // G304: test helper
		if err != nil {
			return false
		}
		return strings.Contains(string(content), expected)
	})
}

// pollFileNotExists polls until a path no longer exists.
func pollFileNotExists(path string, timeout time.Duration) error {
	return pollCondition(timeout, func() bool {
		_, err := os.Stat(path)
		return os.IsNotExist(err)
	})
}

// pollFileExists polls until a path exists.
func pollFileExists(path string, timeout time.Duration) error {
	return pollCondition(timeout, func() bool {
		_, err := os.Stat(path)
		return err == nil
	})
}

// GrovedSkillSyncScenario tests that the groved daemon watches skill directories
// and syncs skills to workspaces when files change, config changes, or skills are removed.
func GrovedSkillSyncScenario() *harness.Scenario {
	return harness.NewScenarioWithOptions(
		"groved-skill-sync",
		"Verify groved watches skill directories and syncs on changes",
		[]string{"groved", "daemon"},
		[]harness.Step{
			harness.NewStep("setup sandboxed environment", setupGrovedEnvironment),
			harness.NewStep("start groved daemon", startGrovedDaemon),
			harness.NewStep("wait for initial skill sync", waitForInitialSync),
			harness.NewStep("skill file edit triggers sync", verifySkillFileEditSync),
			harness.NewStep("config change adds new skill", verifyConfigChangeAddsSkill),
			harness.NewStep("config change prunes removed skill", verifyConfigChangePrunesSkill),
		},
		false, // localOnly
		true,  // explicitOnly - requires groved binary from daemon repo
	).WithTeardown(
		harness.NewStep("stop groved daemon", stopGrovedDaemon),
	)
}

// setupGrovedEnvironment creates a fully sandboxed environment for groved:
// - GROVE_HOME with config, run, state directories
// - A grove.yml pointing to our test ecosystem
// - An ecosystem with grove.toml configuring skills
// - Notebook skills directory with test skills
func setupGrovedEnvironment(ctx *harness.Context) error {
	// Use a short GROVE_HOME path to avoid Unix socket path length limits
	groveHome := filepath.Join(os.TempDir(), fmt.Sprintf("grove-e2e-%d", os.Getpid()))

	// Create all required GROVE_HOME subdirectories
	for _, sub := range []string{"config/grove", "data", "state", "cache", "run"} {
		if err := fs.CreateDir(filepath.Join(groveHome, sub)); err != nil {
			return fmt.Errorf("creating %s: %w", sub, err)
		}
	}
	ctx.Set("grove_home", groveHome)

	// Create the ecosystem directory (must be a git repo for workspace discovery)
	ecosystemDir := ctx.NewDir("test-ecosystem")
	ctx.Set("ecosystem_dir", ecosystemDir)

	// Create ecosystem grove.toml with skills config
	ecosystemTOML := `name = "test-ecosystem"

[skills]
use = ["test-skill", "explain-with-analogy"]
providers = ["claude"]
`
	if err := fs.WriteString(filepath.Join(ecosystemDir, "grove.toml"), ecosystemTOML); err != nil {
		return err
	}

	// Initialize git repo (required for workspace discovery)
	repo, err := git.SetupTestRepo(ecosystemDir)
	if err != nil {
		return fmt.Errorf("setting up git repo: %w", err)
	}
	if err := repo.AddCommit("initial ecosystem setup"); err != nil {
		return fmt.Errorf("initial commit: %w", err)
	}
	ctx.Set("ecosystem_repo", repo)

	// Create notebook root with skills for this ecosystem
	notebookRoot := filepath.Join(groveHome, "notebooks", "main")
	ecosystemSkillsDir := filepath.Join(notebookRoot, "workspaces", "test-ecosystem", "skills")

	// Create test-skill in notebook
	testSkillDir := filepath.Join(ecosystemSkillsDir, "test-skill")
	if err := fs.CreateDir(testSkillDir); err != nil {
		return err
	}
	testSkillContent := `---
name: test-skill
description: A test skill for groved E2E testing.
---

# Test Skill

This is the original test skill content.
`
	if err := fs.WriteString(filepath.Join(testSkillDir, "SKILL.md"), testSkillContent); err != nil {
		return err
	}
	ctx.Set("test_skill_path", filepath.Join(testSkillDir, "SKILL.md"))

	// Create global grove.yml config
	groveYML := fmt.Sprintf(`version: "1.0"

groves:
  e2e-test:
    path: "%s"

notebooks:
  rules:
    default: "main"
  definitions:
    main:
      root_dir: "%s"

daemon:
  workspace_interval: "2s"
  skill_sync_debounce_ms: 500
`, filepath.Dir(ecosystemDir), notebookRoot)

	if err := fs.WriteString(filepath.Join(groveHome, "config", "grove", "grove.yml"), groveYML); err != nil {
		return err
	}

	return nil
}

// startGrovedDaemon locates and starts the groved binary as a background process.
func startGrovedDaemon(ctx *harness.Context) error {
	binary, err := FindDaemonBinary()
	if err != nil {
		return err
	}

	groveHome := ctx.GetString("grove_home")

	cmd := exec.Command(binary, "start", "--collectors", "workspace") //nolint:gosec // G204: binary from FindDaemonBinary
	cmd.Env = append(os.Environ(),
		"GROVE_HOME="+groveHome,
		"HOME="+ctx.HomeDir(),
	)
	// Run from GROVE_HOME to avoid picking up the skills repo's grove.toml
	// via config.LoadDefault()'s CWD-based project config discovery.
	cmd.Dir = groveHome
	// Redirect stdout/stderr so the daemon doesn't hold onto test output
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting groved: %w", err)
	}

	ctx.Set("groved_cmd", cmd)
	ctx.Set("groved_process", cmd.Process)

	// Wait for the daemon socket to appear (indicates it's ready)
	sockPath := filepath.Join(groveHome, "run", "groved.sock")
	if err := pollFileExists(sockPath, 10*time.Second); err != nil {
		return fmt.Errorf("groved did not start (socket not found at %s): %w", sockPath, err)
	}

	return nil
}

// waitForInitialSync waits for the daemon to perform its initial workspace discovery
// and skill sync. The daemon discovers workspaces, then the skill handler syncs on
// the first UpdateWorkspaces event.
func waitForInitialSync(ctx *harness.Context) error {
	ecosystemDir := ctx.GetString("ecosystem_dir")

	// Wait for the initial sync to write explain-with-analogy (a builtin skill)
	syncedSkill := filepath.Join(ecosystemDir, ".claude", "skills", "explain-with-analogy", "SKILL.md")
	if err := pollFileExists(syncedSkill, 15*time.Second); err != nil {
		return fmt.Errorf("initial sync did not complete - explain-with-analogy not found at %s: %w", syncedSkill, err)
	}

	// Also verify the notebook skill was synced
	testSkillSynced := filepath.Join(ecosystemDir, ".claude", "skills", "test-skill", "SKILL.md")
	if err := pollFileContains(testSkillSynced, "original test skill content", 5*time.Second); err != nil {
		return fmt.Errorf("initial sync did not sync test-skill: %w", err)
	}

	return nil
}

// verifySkillFileEditSync modifies a notebook skill file and verifies
// the daemon detects the change and re-syncs it to the workspace.
func verifySkillFileEditSync(ctx *harness.Context) error {
	testSkillPath := ctx.GetString("test_skill_path")
	ecosystemDir := ctx.GetString("ecosystem_dir")

	// Overwrite the skill file with updated content
	updatedContent := `---
name: test-skill
description: A test skill for groved E2E testing (UPDATED).
---

# Test Skill (Updated)

This skill has been MODIFIED by the E2E test.
`
	if err := fs.WriteString(testSkillPath, updatedContent); err != nil {
		return fmt.Errorf("writing updated skill: %w", err)
	}

	// Poll until the synced copy reflects the update
	syncedSkill := filepath.Join(ecosystemDir, ".claude", "skills", "test-skill", "SKILL.md")
	if err := pollFileContains(syncedSkill, "MODIFIED by the E2E test", 10*time.Second); err != nil {
		return fmt.Errorf("skill file edit was not synced: %w", err)
	}

	return nil
}

// verifyConfigChangeAddsSkill updates grove.toml to add a new skill and verifies
// the daemon picks up the config change and syncs the new skill.
func verifyConfigChangeAddsSkill(ctx *harness.Context) error {
	ecosystemDir := ctx.GetString("ecosystem_dir")
	groveHome := ctx.GetString("grove_home")

	// Create a second notebook skill
	notebookRoot := filepath.Join(groveHome, "notebooks", "main")
	secondSkillDir := filepath.Join(notebookRoot, "workspaces", "test-ecosystem", "skills", "second-skill")
	if err := fs.CreateDir(secondSkillDir); err != nil {
		return err
	}
	secondSkillContent := `---
name: second-skill
description: Second test skill added via config change.
---

# Second Skill

This is the second skill added during the E2E test.
`
	if err := fs.WriteString(filepath.Join(secondSkillDir, "SKILL.md"), secondSkillContent); err != nil {
		return err
	}

	// Update grove.toml to include the new skill
	updatedTOML := `name = "test-ecosystem"

[skills]
use = ["test-skill", "explain-with-analogy", "second-skill"]
providers = ["claude"]
`
	if err := fs.WriteString(filepath.Join(ecosystemDir, "grove.toml"), updatedTOML); err != nil {
		return err
	}

	// Poll until the new skill appears in the synced directory
	syncedSecondSkill := filepath.Join(ecosystemDir, ".claude", "skills", "second-skill", "SKILL.md")
	if err := pollFileContains(syncedSecondSkill, "second skill added during the E2E test", 10*time.Second); err != nil {
		return fmt.Errorf("config change did not sync second-skill: %w", err)
	}

	return nil
}

// verifyConfigChangePrunesSkill updates grove.toml to remove a skill and verifies
// the daemon prunes it from the synced directory.
func verifyConfigChangePrunesSkill(ctx *harness.Context) error {
	ecosystemDir := ctx.GetString("ecosystem_dir")

	// Update grove.toml to remove test-skill (keep second-skill and explain-with-analogy)
	prunedTOML := `name = "test-ecosystem"

[skills]
use = ["explain-with-analogy", "second-skill"]
providers = ["claude"]
`
	if err := fs.WriteString(filepath.Join(ecosystemDir, "grove.toml"), prunedTOML); err != nil {
		return err
	}

	// Poll until test-skill is removed
	removedSkillDir := filepath.Join(ecosystemDir, ".claude", "skills", "test-skill")
	if err := pollFileNotExists(removedSkillDir, 10*time.Second); err != nil {
		return fmt.Errorf("removed skill was not pruned: %w", err)
	}

	// Verify the remaining skills still exist
	for _, skill := range []string{"explain-with-analogy", "second-skill"} {
		skillPath := filepath.Join(ecosystemDir, ".claude", "skills", skill, "SKILL.md")
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			return fmt.Errorf("skill %q should still exist after pruning test-skill", skill)
		}
	}

	return nil
}

// stopGrovedDaemon kills the groved process and cleans up GROVE_HOME.
func stopGrovedDaemon(ctx *harness.Context) error {
	// Kill the daemon process
	if proc, ok := ctx.Get("groved_process").(*os.Process); ok && proc != nil {
		_ = proc.Kill()
		_, _ = proc.Wait()
	}

	// Clean up GROVE_HOME
	if groveHome := ctx.GetString("grove_home"); groveHome != "" {
		_ = os.RemoveAll(groveHome)
	}

	return nil
}
