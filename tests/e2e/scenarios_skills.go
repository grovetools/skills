package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattsolo1/grove-tend/pkg/command"
	"github.com/mattsolo1/grove-tend/pkg/harness"
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
			harness.NewStep("create and install user-defined skill", func(ctx *harness.Context) error {
				binary, err := FindBinary()
				if err != nil {
					return err
				}

				homeDir := ctx.HomeDir()
				configDir := ctx.ConfigDir()
				// Create a custom skill in the XDG config path within the test's home directory
				userSkillDir := filepath.Join(configDir, "grove", "skills", "my-custom-skill")
				if err := os.MkdirAll(userSkillDir, 0755); err != nil {
					return err
				}
				skillContent := "---\nname: my-custom-skill\ndescription: A custom skill.\n---\nHello from custom skill!"
				userSkillPath := filepath.Join(userSkillDir, "SKILL.md")
				if err := os.WriteFile(userSkillPath, []byte(skillContent), 0644); err != nil {
					return err
				}

				// Verify the user-defined skill SKILL.md was created
				if _, err := os.Stat(userSkillPath); os.IsNotExist(err) {
					return fmt.Errorf("user-defined SKILL.md not found at %s", userSkillPath)
				}

				// Install it to the project scope for the 'codex' provider
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

				// Verify both skills are present
				basePath := filepath.Join(homeDir, ".opencode", "skill")
				if _, err := os.Stat(filepath.Join(basePath, "explain-with-analogy")); err != nil {
					return err
				}
				if _, err := os.Stat(filepath.Join(basePath, "my-custom-skill")); err != nil {
					return err
				}

				// Now, remove the custom user skill from the source
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

				// Verify the custom skill was pruned
				if _, err := os.Stat(filepath.Join(basePath, "my-custom-skill")); !os.IsNotExist(err) {
					return fmt.Errorf("custom skill was not pruned as expected")
				}
				// Verify the built-in skill still exists
				if _, err := os.Stat(filepath.Join(basePath, "explain-with-analogy")); err != nil {
					return fmt.Errorf("built-in skill was incorrectly pruned")
				}
				return nil
			}),
		},
	}
}
