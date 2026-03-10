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

func SkillsScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "skill-management",
		Description: "Verify declarative skill sync and validation",
		Steps: []harness.Step{
			harness.NewStep("setup git repo and grove.toml", func(ctx *harness.Context) error {
				// Initialize git repo in root dir
				_, err := git.SetupTestRepo(ctx.RootDir)
				if err != nil {
					return fmt.Errorf("failed to setup git repo: %w", err)
				}

				// Create grove.toml with skills configuration
				tomlContent := `name = "test-project"

[skills]
use = ["explain-with-analogy"]
providers = ["claude"]
`
				if err := fs.WriteString(filepath.Join(ctx.RootDir, "grove.toml"), tomlContent); err != nil {
					return err
				}

				return nil
			}),
			harness.NewStep("declarative sync writes skills to provider directories", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				cmd := command.New(binary, "sync").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("sync output", result.Stdout, result.Stderr)

				if result.ExitCode != 0 {
					return fmt.Errorf("sync failed: %s", result.Stderr)
				}

				// Verify skill was synced to .claude/skills/
				skillPath := filepath.Join(ctx.RootDir, ".claude", "skills", "explain-with-analogy", "SKILL.md")
				if _, err := os.Stat(skillPath); os.IsNotExist(err) {
					return fmt.Errorf("skill not synced to %s", skillPath)
				}

				return nil
			}),
			harness.NewStep("validate command succeeds for configured skills", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				cmd := command.New(binary, "validate").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("validate output", result.Stdout, result.Stderr)

				if result.ExitCode != 0 {
					return fmt.Errorf("validate failed: %s", result.Stderr)
				}

				// Check output contains success message
				if !strings.Contains(result.Stdout, "All declared skills resolved successfully") {
					return fmt.Errorf("expected success message in output, got: %s", result.Stdout)
				}

				return nil
			}),
			harness.NewStep("validate command fails for unknown skills", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// Update grove.toml to include a non-existent skill
				tomlContent := `name = "test-project"

[skills]
use = ["non-existent-skill-xyz"]
`
				if err := fs.WriteString(filepath.Join(ctx.RootDir, "grove.toml"), tomlContent); err != nil {
					return err
				}

				cmd := command.New(binary, "validate").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("validate output for unknown skill", result.Stdout, result.Stderr)

				if result.ExitCode == 0 {
					return fmt.Errorf("validate should have failed for unknown skill")
				}

				combined := result.Stdout + result.Stderr
				if !strings.Contains(combined, "not found") {
					return fmt.Errorf("expected 'not found' error, got: %s", combined)
				}

				return nil
			}),
			harness.NewStep("sync with multiple providers", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// Update grove.toml to sync to multiple providers
				tomlContent := `name = "test-project"

[skills]
use = ["explain-with-analogy"]
providers = ["claude", "codex"]
`
				if err := fs.WriteString(filepath.Join(ctx.RootDir, "grove.toml"), tomlContent); err != nil {
					return err
				}

				cmd := command.New(binary, "sync").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("sync to multiple providers", result.Stdout, result.Stderr)

				if result.ExitCode != 0 {
					return fmt.Errorf("sync failed: %s", result.Stderr)
				}

				// Verify skill was synced to both providers
				claudePath := filepath.Join(ctx.RootDir, ".claude", "skills", "explain-with-analogy", "SKILL.md")
				if _, err := os.Stat(claudePath); os.IsNotExist(err) {
					return fmt.Errorf("skill not synced to .claude")
				}

				codexPath := filepath.Join(ctx.RootDir, ".codex", "skills", "explain-with-analogy", "SKILL.md")
				if _, err := os.Stat(codexPath); os.IsNotExist(err) {
					return fmt.Errorf("skill not synced to .codex")
				}

				return nil
			}),
			harness.NewStep("sync with prune removes unconfigured skills", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// Change config to use a different skill
				tomlContent := `name = "test-project"

[skills]
use = ["grove-skill-guide"]
providers = ["claude"]
`
				if err := fs.WriteString(filepath.Join(ctx.RootDir, "grove.toml"), tomlContent); err != nil {
					return err
				}

				cmd := command.New(binary, "sync", "--prune").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("sync with prune", result.Stdout, result.Stderr)

				if result.ExitCode != 0 {
					return fmt.Errorf("sync prune failed: %s", result.Stderr)
				}

				// Verify explain-with-analogy was pruned from claude
				oldSkillPath := filepath.Join(ctx.RootDir, ".claude", "skills", "explain-with-analogy")
				if _, err := os.Stat(oldSkillPath); !os.IsNotExist(err) {
					return fmt.Errorf("old skill was not pruned from .claude")
				}

				// Verify grove-skill-guide was added
				newSkillPath := filepath.Join(ctx.RootDir, ".claude", "skills", "grove-skill-guide", "SKILL.md")
				if _, err := os.Stat(newSkillPath); os.IsNotExist(err) {
					return fmt.Errorf("new skill not synced")
				}

				return nil
			}),
			harness.NewStep("list command shows CONFIGURED column", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				cmd := command.New(binary, "list").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("list output", result.Stdout, result.Stderr)

				if result.ExitCode != 0 {
					return fmt.Errorf("list failed: %s", result.Stderr)
				}

				// Check header includes CONFIGURED column
				if !strings.Contains(result.Stdout, "CONFIGURED") {
					return fmt.Errorf("expected CONFIGURED column in list output, got: %s", result.Stdout)
				}

				// Check grove-skill-guide shows as configured
				lines := strings.Split(result.Stdout, "\n")
				foundConfigured := false
				for _, line := range lines {
					if strings.Contains(line, "grove-skill-guide") && strings.Contains(line, "Yes") {
						foundConfigured = true
						break
					}
				}
				if !foundConfigured {
					return fmt.Errorf("expected grove-skill-guide to show as 'Yes' configured")
				}

				return nil
			}),
			harness.NewStep("dry-run shows what would be synced", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// Add another skill to config
				tomlContent := `name = "test-project"

[skills]
use = ["grove-skill-guide", "explain-with-analogy"]
providers = ["claude"]
`
				if err := fs.WriteString(filepath.Join(ctx.RootDir, "grove.toml"), tomlContent); err != nil {
					return err
				}

				cmd := command.New(binary, "sync", "--dry-run").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("dry-run output", result.Stdout, result.Stderr)

				if result.ExitCode != 0 {
					return fmt.Errorf("dry-run failed: %s", result.Stderr)
				}

				combined := result.Stdout + result.Stderr
				if !strings.Contains(combined, "DRY RUN") {
					return fmt.Errorf("expected DRY RUN message in output")
				}

				// Verify explain-with-analogy wasn't actually synced (it's a dry run)
				skillPath := filepath.Join(ctx.RootDir, ".claude", "skills", "explain-with-analogy", "SKILL.md")
				if _, err := os.Stat(skillPath); err == nil {
					return fmt.Errorf("dry-run should not have synced the skill")
				}

				return nil
			}),
			harness.NewStep("remove command removes installed skills", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// First sync to ensure skills are present
				syncCmd := command.New(binary, "sync").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				syncResult := syncCmd.Run()
				if syncResult.ExitCode != 0 {
					return fmt.Errorf("sync failed: %s", syncResult.Stderr)
				}

				// Verify skill exists before removal
				skillPath := filepath.Join(ctx.RootDir, ".claude", "skills", "grove-skill-guide")
				if _, err := os.Stat(skillPath); os.IsNotExist(err) {
					return fmt.Errorf("skill should exist before removal")
				}

				// Remove the skill
				cmd := command.New(binary, "remove", "grove-skill-guide", "--scope", "project", "--provider", "claude").
					Dir(ctx.RootDir).
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("remove output", result.Stdout, result.Stderr)

				if result.ExitCode != 0 {
					return fmt.Errorf("remove failed: %s", result.Stderr)
				}

				// Verify skill was removed
				if _, err := os.Stat(skillPath); !os.IsNotExist(err) {
					return fmt.Errorf("skill should have been removed")
				}

				return nil
			}),
			harness.NewStep("user_path config overrides default user skills location", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()

				// Create a custom skills directory
				customSkillsDir := ctx.NewDir("custom-skills")
				customSkillDir := filepath.Join(customSkillsDir, "custom-user-skill")
				if err := fs.CreateDir(customSkillDir); err != nil {
					return err
				}

				skillContent := `---
name: custom-user-skill
description: A custom skill from user_path config.
---

# Custom User Skill

This skill was loaded from a custom user_path.`
				if err := fs.WriteString(filepath.Join(customSkillDir, "SKILL.md"), skillContent); err != nil {
					return err
				}

				// Create global config with user_path pointing to custom directory
				globalConfigDir := filepath.Join(configDir, "grove")
				if err := fs.CreateDir(globalConfigDir); err != nil {
					return err
				}
				globalConfig := fmt.Sprintf(`[skills]
user_path = "%s"
`, customSkillsDir)
				if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.toml"), globalConfig); err != nil {
					return err
				}

				// Run list command - should show custom-user-skill as "user" source
				cmd := command.New(binary, "list").
					Dir("/tmp"). // Run from outside any workspace
					Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
				result := cmd.Run()

				ctx.ShowCommandOutput("list with user_path output", result.Stdout, result.Stderr)

				if result.ExitCode != 0 {
					return fmt.Errorf("list failed: %s", result.Stderr)
				}

				// Verify custom-user-skill is listed as "user" source
				if !strings.Contains(result.Stdout, "custom-user-skill") {
					return fmt.Errorf("expected custom-user-skill in list output")
				}

				// Check it's marked as user source
				for _, line := range strings.Split(result.Stdout, "\n") {
					if strings.Contains(line, "custom-user-skill") {
						if !strings.Contains(line, "user") {
							return fmt.Errorf("expected custom-user-skill to have 'user' source, got: %s", line)
						}
						break
					}
				}

				return nil
			}),
		},
	}
}
