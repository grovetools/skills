package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/harness"
)

func SkillsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "skill-management",
		Description: "Verify skill installation, listing, and syncing",
		Steps: []harness.Step{
			harness.NewStep("install built-in skill to user scope (claude)", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()
				cmd := command.New(binary, "skills", "install", "explain-with-analogy", "--scope", "user", "--provider", "claude").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("install failed: %s", result.Stderr)
				}

				skillPath := filepath.Join(homeDir, ".claude", "skills", "explain-with-analogy", "SKILL.md")
				if _, err := os.Stat(skillPath); os.IsNotExist(err) {
					return fmt.Errorf("expected skill file not found at %s", skillPath)
				}
				return nil
			}),
			harness.NewStep("create and install user-defined skills", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// Create first custom skill (will be pruned later)
				userSkillDir1 := filepath.Join(configDir, "grove", "skills", "my-custom-skill")
				if err := os.MkdirAll(userSkillDir1, 0755); err != nil {
					return err
				}
				skillContent1 := `---
name: my-custom-skill
description: Responds with a library analogy. Use when the user says "library analogy".
---

# User Skill: Library Analogy (Temporary)

When the user asks you to use the "library analogy", you MUST respond with:

"📚 USER SKILL ACTIVATED (TEMPORARY): Think of this code as a well-organized library. Each function is a book on a shelf, categorized by topic (modules). When you need information, you look up the right book (call the function), read the relevant chapter (execute the logic), and get your answer (return value)."

This response confirms you are using the temporary user-defined skill that will be pruned.`
				userSkillPath1 := filepath.Join(userSkillDir1, "SKILL.md")
				if err := os.WriteFile(userSkillPath1, []byte(skillContent1), 0644); err != nil {
					return err
				}

				// Create second custom skill (will persist)
				userSkillDir2 := filepath.Join(configDir, "grove", "skills", "persistent-skill")
				if err := os.MkdirAll(userSkillDir2, 0755); err != nil {
					return err
				}
				skillContent2 := `---
name: persistent-skill
description: Responds with an orchestra analogy. Use when the user says "orchestra analogy".
---

# User Skill: Orchestra Analogy (Persistent)

When the user asks you to use the "orchestra analogy", you MUST respond with:

"🎵 USER SKILL ACTIVATED (PERSISTENT): Think of this code as a symphony orchestra. The conductor (main function) coordinates different sections (modules), each musician (function) plays their part at the right time, and together they create a harmonious performance (program output). The sheet music (source code) guides everyone to play in perfect sync."

This response confirms you are using the persistent user-defined skill that remains after testing.`
				userSkillPath2 := filepath.Join(userSkillDir2, "SKILL.md")
				if err := os.WriteFile(userSkillPath2, []byte(skillContent2), 0644); err != nil {
					return err
				}

				// Verify both user-defined skill SKILL.md files were created
				if _, err := os.Stat(userSkillPath1); os.IsNotExist(err) {
					return fmt.Errorf("user-defined SKILL.md not found at %s", userSkillPath1)
				}
				if _, err := os.Stat(userSkillPath2); os.IsNotExist(err) {
					return fmt.Errorf("user-defined SKILL.md not found at %s", userSkillPath2)
				}

				// Install first skill to the project scope for the 'codex' provider
				cmd := command.New(binary, "skills", "install", "my-custom-skill", "--scope", "project", "--provider", "codex").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("install failed: %s", result.Stderr)
				}

				projectSkillPath := filepath.Join(ctx.RootDir, ".codex", "skills", "my-custom-skill", "SKILL.md")
				if _, err := os.Stat(projectSkillPath); os.IsNotExist(err) {
					return fmt.Errorf("expected project skill file not found at %s", projectSkillPath)
				}
				return nil
			}),
			harness.NewStep("list all skills and verify sources", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()
				cmd := command.New(binary, "skills", "list").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("list command failed: %s", result.Stderr)
				}

				if !strings.Contains(result.Stdout, "explain-with-analogy") || !strings.Contains(result.Stdout, "builtin") {
					return fmt.Errorf("expected to find 'explain-with-analogy' from builtin source, got:\n%s", result.Stdout)
				}
				if !strings.Contains(result.Stdout, "my-custom-skill") || !strings.Contains(result.Stdout, "user") {
					return fmt.Errorf("expected to find 'my-custom-skill' from user source, got:\n%s", result.Stdout)
				}
				if !strings.Contains(result.Stdout, "persistent-skill") || !strings.Contains(result.Stdout, "user") {
					return fmt.Errorf("expected to find 'persistent-skill' from user source, got:\n%s", result.Stdout)
				}
				return nil
			}),
			harness.NewStep("sync and prune skills", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()
				// First, sync all skills to a new provider dir
				cmdSync := command.New(binary, "skills", "sync", "--scope", "user", "--provider", "opencode").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				if res := cmdSync.Run(); res.ExitCode != 0 {
					return res.Error
				}

				// Verify all three skills are present
				basePath := filepath.Join(homeDir, ".opencode", "skill")
				if _, err := os.Stat(filepath.Join(basePath, "explain-with-analogy")); err != nil {
					return err
				}
				if _, err := os.Stat(filepath.Join(basePath, "my-custom-skill")); err != nil {
					return err
				}
				if _, err := os.Stat(filepath.Join(basePath, "persistent-skill")); err != nil {
					return err
				}

				// Now, remove only my-custom-skill from the source (persistent-skill stays)
				userSkillDir := filepath.Join(configDir, "grove", "skills", "my-custom-skill")
				if err := os.RemoveAll(userSkillDir); err != nil {
					return err
				}

				// Sync again with --prune
				cmdPrune := command.New(binary, "skills", "sync", "--scope", "user", "--provider", "opencode", "--prune").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				if res := cmdPrune.Run(); res.ExitCode != 0 {
					return res.Error
				}

				// Verify my-custom-skill was pruned
				if _, err := os.Stat(filepath.Join(basePath, "my-custom-skill")); !os.IsNotExist(err) {
					return fmt.Errorf("my-custom-skill was not pruned as expected")
				}
				// Verify the built-in skill still exists
				if _, err := os.Stat(filepath.Join(basePath, "explain-with-analogy")); err != nil {
					return fmt.Errorf("built-in skill was incorrectly pruned")
				}
				// Verify persistent-skill still exists (not pruned)
				if _, err := os.Stat(filepath.Join(basePath, "persistent-skill")); err != nil {
					return fmt.Errorf("persistent-skill was incorrectly pruned")
				}
				// Verify persistent-skill SKILL.md still exists in config directory
				persistentSkillPath := filepath.Join(configDir, "grove", "skills", "persistent-skill", "SKILL.md")
				if _, err := os.Stat(persistentSkillPath); os.IsNotExist(err) {
					return fmt.Errorf("persistent-skill SKILL.md should still exist in config dir at %s", persistentSkillPath)
				}
				return nil
			}),
			harness.NewStep("remove installed skill", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// First verify skill exists
				skillPath := filepath.Join(homeDir, ".opencode", "skill", "explain-with-analogy")
				if _, err := os.Stat(skillPath); os.IsNotExist(err) {
					return fmt.Errorf("skill should exist before removal at %s", skillPath)
				}

				// Remove the skill
				cmd := command.New(binary, "skills", "remove", "explain-with-analogy", "--scope", "user", "--provider", "opencode").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("remove failed: %s", result.Stderr)
				}

				// Verify skill was removed
				if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
					return fmt.Errorf("skill should have been removed from %s", skillPath)
				}

				// Try to remove non-existent skill (should fail)
				cmdFail := command.New(binary, "skills", "remove", "non-existent-skill", "--scope", "user", "--provider", "opencode").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				resultFail := cmdFail.Run()
				if resultFail.ExitCode == 0 {
					return fmt.Errorf("removing non-existent skill should have failed")
				}

				return nil
			}),
			harness.NewStep("validation rejects invalid skills", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// Create skill with missing description
				invalidSkillDir := filepath.Join(configDir, "grove", "skills", "invalid-skill")
				if err := os.MkdirAll(invalidSkillDir, 0755); err != nil {
					return err
				}
				invalidContent := `---
name: invalid-skill
---

Missing description field.`
				if err := os.WriteFile(filepath.Join(invalidSkillDir, "SKILL.md"), []byte(invalidContent), 0644); err != nil {
					return err
				}

				// Try to install - should fail validation
				cmd := command.New(binary, "skills", "install", "invalid-skill", "--scope", "project").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()
				if result.ExitCode == 0 {
					return fmt.Errorf("install should have failed due to validation")
				}
				if !strings.Contains(result.Stderr, "missing required field 'description'") {
					return fmt.Errorf("expected validation error about missing description, got: %s", result.Stderr)
				}

				// Install with --skip-validation should succeed
				cmdSkip := command.New(binary, "skills", "install", "invalid-skill", "--scope", "project", "--skip-validation").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				resultSkip := cmdSkip.Run()
				if resultSkip.ExitCode != 0 {
					return fmt.Errorf("install with --skip-validation should succeed: %s", resultSkip.Stderr)
				}

				return nil
			}),
			harness.NewStep("force flag required for overwrite", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// Install a skill first
				skillPath := filepath.Join(homeDir, ".claude", "skills", "persistent-skill")
				cmd := command.New(binary, "skills", "install", "persistent-skill", "--scope", "user", "--provider", "claude").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()
				if result.ExitCode != 0 {
					return fmt.Errorf("initial install failed: %s", result.Stderr)
				}

				// Try to install again without --force (should fail)
				cmdNoForce := command.New(binary, "skills", "install", "persistent-skill", "--scope", "user", "--provider", "claude").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				resultNoForce := cmdNoForce.Run()
				if resultNoForce.ExitCode == 0 {
					return fmt.Errorf("install without --force should fail when skill exists")
				}
				if !strings.Contains(resultNoForce.Stderr, "already exists") {
					return fmt.Errorf("expected 'already exists' error, got: %s", resultNoForce.Stderr)
				}

				// Install with --force should succeed
				cmdForce := command.New(binary, "skills", "install", "persistent-skill", "--scope", "user", "--provider", "claude", "--force").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				resultForce := cmdForce.Run()
				if resultForce.ExitCode != 0 {
					return fmt.Errorf("install with --force should succeed: %s", resultForce.Stderr)
				}

				// Verify skill still exists
				if _, err := os.Stat(skillPath); os.IsNotExist(err) {
					return fmt.Errorf("skill should exist after force install at %s", skillPath)
				}

				return nil
			}),
		},
	}
}
