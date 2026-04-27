package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/harness"
)

// SkillsShowScenario tests the skills show command functionality.
func SkillsShowScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "skills-show",
		Description: "Verify skills show command displays skill content and metadata",
		Steps: []harness.Step{
			harness.NewStep("show displays builtin skill content", showBuiltinSkill),
			harness.NewStep("show with --json returns structured output", showJSONOutput),
			harness.NewStep("show displays user skill content", showUserSkill),
			harness.NewStep("show returns error for non-existent skill", showNonExistentSkill),
		},
	}
}

// showBuiltinSkill verifies that show displays a builtin skill's content.
func showBuiltinSkill(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	cmd := command.New(binary, "show", "explain-with-analogy").
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("show builtin skill output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("show command failed: %s", result.Stderr)
	}

	// Verify metadata is displayed
	if !strings.Contains(result.Stdout, "explain-with-analogy") {
		return fmt.Errorf("expected skill name 'explain-with-analogy' in output, got:\n%s", result.Stdout)
	}

	// Verify source is shown as builtin
	if !strings.Contains(result.Stdout, "builtin") {
		return fmt.Errorf("expected 'builtin' source in output, got:\n%s", result.Stdout)
	}

	// Verify content section exists
	if !strings.Contains(result.Stdout, "Content") || !strings.Contains(result.Stdout, "---") {
		return fmt.Errorf("expected content section with frontmatter in output, got:\n%s", result.Stdout)
	}

	return nil
}

// showJSONOutput verifies that --json flag returns structured output.
func showJSONOutput(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	cmd := command.New(binary, "show", "explain-with-analogy", "--json").
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("show --json output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("show --json command failed: %s", result.Stderr)
	}

	// Verify output is valid JSON
	var showResult struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Domain      string   `json:"domain"`
		Requires    []string `json:"requires"`
		Source      string   `json:"source"`
		FilePath    string   `json:"file_path"`
		Content     string   `json:"content"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &showResult); err != nil {
		return fmt.Errorf("show --json output is not valid JSON: %w\nOutput: %s", err, result.Stdout)
	}

	// Verify required fields are present
	if showResult.Name == "" {
		return fmt.Errorf("show result missing 'name' field")
	}
	if showResult.Name != "explain-with-analogy" {
		return fmt.Errorf("expected name 'explain-with-analogy', got '%s'", showResult.Name)
	}
	if showResult.Source == "" {
		return fmt.Errorf("show result missing 'source' field")
	}
	if showResult.Source != "builtin" {
		return fmt.Errorf("expected source 'builtin', got '%s'", showResult.Source)
	}
	if showResult.FilePath == "" {
		return fmt.Errorf("show result missing 'file_path' field")
	}
	if showResult.Content == "" {
		return fmt.Errorf("show result missing 'content' field")
	}
	// Content should include frontmatter
	if !strings.HasPrefix(showResult.Content, "---") {
		return fmt.Errorf("content should start with frontmatter '---', got: %s", showResult.Content[:50])
	}

	return nil
}

// showUserSkill verifies that show displays a user-defined skill.
func showUserSkill(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	// Create a user-defined skill
	userSkillDir := filepath.Join(configDir, "grove", "skills", "test-show-skill")
	if err := fs.CreateDir(userSkillDir); err != nil {
		return err
	}
	skillContent := `---
name: test-show-skill
description: A test skill for the show command.
domain: testing
requires:
  - other-skill
---

# Test Show Skill

This is a test skill created for the show command E2E test.

## Usage

Use this skill to verify the show command works correctly.`

	if err := os.WriteFile(filepath.Join(userSkillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil { //nolint:gosec // G306: test
		return err
	}

	// Now show the skill
	cmd := command.New(binary, "show", "test-show-skill", "--json").
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("show user skill output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("show command failed: %s", result.Stderr)
	}

	// Verify JSON output
	var showResult struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Domain      string   `json:"domain"`
		Requires    []string `json:"requires"`
		Source      string   `json:"source"`
		FilePath    string   `json:"file_path"`
		Content     string   `json:"content"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &showResult); err != nil {
		return fmt.Errorf("show --json output is not valid JSON: %w\nOutput: %s", err, result.Stdout)
	}

	// Verify metadata
	if showResult.Name != "test-show-skill" {
		return fmt.Errorf("expected name 'test-show-skill', got '%s'", showResult.Name)
	}
	if showResult.Description != "A test skill for the show command." {
		return fmt.Errorf("expected description 'A test skill for the show command.', got '%s'", showResult.Description)
	}
	if showResult.Domain != "testing" {
		return fmt.Errorf("expected domain 'testing', got '%s'", showResult.Domain)
	}
	if len(showResult.Requires) != 1 || showResult.Requires[0] != "other-skill" {
		return fmt.Errorf("expected requires ['other-skill'], got %v", showResult.Requires)
	}
	if showResult.Source != "user" {
		return fmt.Errorf("expected source 'user', got '%s'", showResult.Source)
	}
	// FilePath should point to the user skill directory
	if !strings.Contains(showResult.FilePath, "test-show-skill") {
		return fmt.Errorf("expected file_path to contain 'test-show-skill', got '%s'", showResult.FilePath)
	}

	return nil
}

// showNonExistentSkill verifies that show returns an error for non-existent skills.
func showNonExistentSkill(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	cmd := command.New(binary, "show", "non-existent-skill-12345").
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("show non-existent skill output", result.Stdout, result.Stderr)

	// Should fail with non-zero exit code
	if result.ExitCode == 0 {
		return fmt.Errorf("expected show command to fail for non-existent skill, but it succeeded")
	}

	// Error message should indicate skill not found
	combinedOutput := result.Stdout + result.Stderr
	if !strings.Contains(combinedOutput, "not found") {
		return fmt.Errorf("expected 'not found' in error message, got:\n%s", combinedOutput)
	}

	return nil
}
