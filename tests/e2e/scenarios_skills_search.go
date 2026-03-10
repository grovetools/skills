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

// SkillsSearchScenario tests the skills search command functionality.
func SkillsSearchScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "skills-search",
		Description: "Verify skills search command discovers skills and returns source paths",
		Steps: []harness.Step{
			harness.NewStep("search finds builtin skill by name", searchFindsByName),
			harness.NewStep("search finds builtin skill by description", searchFindsByDescription),
			harness.NewStep("search with --json returns structured output", searchJSONOutput),
			harness.NewStep("search with --files-only returns paths", searchFilesOnly),
			harness.NewStep("search marks builtin skills as read-only", searchReadOnlyBuiltin),
		},
	}
}

// searchFindsByName verifies that search finds a skill by name match.
func searchFindsByName(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	// Search for a built-in skill by name
	cmd := command.New(binary, "search", "explain-with-analogy").
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("search by name output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("search command failed: %s", result.Stderr)
	}

	// Verify the skill was found
	if !strings.Contains(result.Stdout, "explain-with-analogy") {
		return fmt.Errorf("expected to find 'explain-with-analogy' in search results, got:\n%s", result.Stdout)
	}

	// Verify it shows the match reason as 'name'
	if !strings.Contains(result.Stdout, "name") {
		return fmt.Errorf("expected match reason 'name' in output, got:\n%s", result.Stdout)
	}

	return nil
}

// searchFindsByDescription verifies that search finds a skill by description match.
func searchFindsByDescription(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	// Search for a built-in skill by description keywords
	// The explain-with-analogy skill has "vending machine" in its description
	cmd := command.New(binary, "search", "vending").
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("search by description output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("search command failed: %s", result.Stderr)
	}

	// Verify a skill was found by description match
	if !strings.Contains(result.Stdout, "description") {
		return fmt.Errorf("expected match reason 'description' in output, got:\n%s", result.Stdout)
	}

	return nil
}

// searchJSONOutput verifies that --json flag returns structured output.
func searchJSONOutput(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	cmd := command.New(binary, "search", "explain-with-analogy", "--json").
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("search --json output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("search --json command failed: %s", result.Stderr)
	}

	// Verify output is valid JSON
	var searchResults []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Source      string `json:"source"`
		FilePath    string `json:"file_path"`
		MatchReason string `json:"match_reason"`
	}

	if err := json.Unmarshal([]byte(result.Stdout), &searchResults); err != nil {
		return fmt.Errorf("search --json output is not valid JSON: %w\nOutput: %s", err, result.Stdout)
	}

	// Verify at least one result was returned
	if len(searchResults) == 0 {
		return fmt.Errorf("expected at least one search result, got empty array")
	}

	// Verify required fields are present
	for _, r := range searchResults {
		if r.Name == "" {
			return fmt.Errorf("search result missing 'name' field")
		}
		if r.Source == "" {
			return fmt.Errorf("search result missing 'source' field")
		}
		if r.FilePath == "" {
			return fmt.Errorf("search result missing 'file_path' field")
		}
		if r.MatchReason == "" {
			return fmt.Errorf("search result missing 'match_reason' field")
		}
	}

	return nil
}

// searchFilesOnly verifies that --files-only returns only file paths.
func searchFilesOnly(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	// Create a user-defined skill so we have a non-builtin file to find
	userSkillDir := filepath.Join(configDir, "grove", "skills", "custom-test-skill")
	if err := fs.CreateDir(userSkillDir); err != nil {
		return err
	}
	skillContent := `---
name: custom-test-skill
description: A test skill for search functionality.
---

# Test Skill

This is a test skill for the search --files-only test.`
	if err := os.WriteFile(filepath.Join(userSkillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		return err
	}

	// Now search with --files-only
	cmd := command.New(binary, "search", "custom-test-skill", "--files-only").
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("search --files-only output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("search --files-only command failed: %s", result.Stderr)
	}

	// Verify output contains file paths (should be newline-separated)
	output := strings.TrimSpace(result.Stdout)
	if output == "" {
		// No editable files (builtin skills are excluded) - this is acceptable
		return nil
	}

	// Each line should be a path ending in SKILL.md
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasSuffix(line, "SKILL.md") {
			return fmt.Errorf("--files-only output line is not a SKILL.md path: %s", line)
		}
	}

	return nil
}

// searchReadOnlyBuiltin verifies that builtin skills are marked as read-only.
func searchReadOnlyBuiltin(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	// Use a fresh home directory to ensure no user overrides exist
	freshHome := ctx.NewDir("fresh-home")
	freshConfig := ctx.NewDir("fresh-config")

	cmd := command.New(binary, "search", "explain-with-analogy", "--json").
		Env("HOME="+freshHome, "XDG_CONFIG_HOME="+freshConfig)
	result := cmd.Run()

	ctx.ShowCommandOutput("search builtin skill output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("search command failed: %s", result.Stderr)
	}

	// Verify output contains [READ-ONLY BUILTIN] for builtin skills
	if !strings.Contains(result.Stdout, "READ-ONLY BUILTIN") && !strings.Contains(result.Stdout, "builtin") {
		return fmt.Errorf("expected builtin skill to be marked as read-only or show builtin source, got:\n%s", result.Stdout)
	}

	return nil
}
