package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/tend/pkg/command"
	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/harness"
)

// SkillsIntegrateScenario tests the skills integrate command functionality.
func SkillsIntegrateScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "skills-integrate",
		Description: "Verify skills integrate command injects agent instructions into CLAUDE.md",
		Steps: []harness.Step{
			harness.NewStep("integrate creates new CLAUDE.md if none exists", integrateCreatesNew),
			harness.NewStep("integrate appends to existing CLAUDE.md", integrateAppendsExisting),
			harness.NewStep("integrate replaces existing block on re-run", integrateReplacesBlock),
			harness.NewStep("integrate preserves surrounding content", integratePreservesContent),
		},
	}
}

// integrateCreatesNew verifies that integrate creates CLAUDE.md if it doesn't exist.
func integrateCreatesNew(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	// Create a fresh project directory
	projectDir := ctx.NewDir("integrate-test-new")
	if err := fs.CreateDir(projectDir); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	cmd := command.New(binary, "integrate", "--scope", "project").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("integrate new output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("integrate command failed (exit %d): stdout=%s, stderr=%s", result.ExitCode, result.Stdout, result.Stderr)
	}

	// Verify CLAUDE.md was created
	claudePath := filepath.Join(projectDir, "CLAUDE.md")

	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		return fmt.Errorf("expected CLAUDE.md to be created at %s", claudePath)
	}

	// Verify content contains the markers and payload
	content, err := os.ReadFile(claudePath) //nolint:gosec // G304: test path
	if err != nil {
		return fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "<!-- GROVE:SKILLS:START -->") {
		return fmt.Errorf("expected start marker in CLAUDE.md")
	}
	if !strings.Contains(contentStr, "<!-- GROVE:SKILLS:END -->") {
		return fmt.Errorf("expected end marker in CLAUDE.md")
	}
	if !strings.Contains(contentStr, "grove-skills search") {
		return fmt.Errorf("expected grove-skills search instruction in CLAUDE.md")
	}
	if !strings.Contains(contentStr, "Delegation Principle") {
		return fmt.Errorf("expected Delegation Principle section in CLAUDE.md")
	}

	return nil
}

// integrateAppendsExisting verifies that integrate appends to existing CLAUDE.md.
func integrateAppendsExisting(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	// Create a project with existing CLAUDE.md
	projectDir := ctx.NewDir("integrate-test-append")
	existingContent := `# My Project Instructions

This is my existing CLAUDE.md content.
Do not remove this.
`
	if err := fs.WriteString(filepath.Join(projectDir, "CLAUDE.md"), existingContent); err != nil {
		return fmt.Errorf("failed to write existing CLAUDE.md: %w", err)
	}

	cmd := command.New(binary, "integrate", "--scope", "project").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("integrate append output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("integrate command failed: %s", result.Stderr)
	}

	// Verify CLAUDE.md was updated
	content, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md")) //nolint:gosec // G304: test
	if err != nil {
		return fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	contentStr := string(content)

	// Verify original content is preserved
	if !strings.Contains(contentStr, "My Project Instructions") {
		return fmt.Errorf("expected original content to be preserved")
	}
	if !strings.Contains(contentStr, "Do not remove this") {
		return fmt.Errorf("expected original content to be preserved")
	}

	// Verify new block was appended
	if !strings.Contains(contentStr, "<!-- GROVE:SKILLS:START -->") {
		return fmt.Errorf("expected start marker to be appended")
	}
	if !strings.Contains(contentStr, "grove-skills search") {
		return fmt.Errorf("expected grove-skills search instruction to be appended")
	}

	return nil
}

// integrateReplacesBlock verifies that running integrate twice replaces the block.
func integrateReplacesBlock(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	// Create a project and run integrate twice
	projectDir := ctx.NewDir("integrate-test-replace")
	if err := fs.CreateDir(projectDir); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}

	// First run
	cmd1 := command.New(binary, "integrate", "--scope", "project").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result1 := cmd1.Run()
	if result1.ExitCode != 0 {
		return fmt.Errorf("first integrate failed: %s", result1.Stderr)
	}

	// Get content after first run
	content1, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md")) //nolint:gosec // G304: test
	if err != nil {
		return fmt.Errorf("failed to read CLAUDE.md after first run: %w", err)
	}

	// Second run
	cmd2 := command.New(binary, "integrate", "--scope", "project").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result2 := cmd2.Run()

	ctx.ShowCommandOutput("integrate replace output", result2.Stdout, result2.Stderr)

	if result2.ExitCode != 0 {
		return fmt.Errorf("second integrate failed: %s", result2.Stderr)
	}

	// Get content after second run
	content2, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md")) //nolint:gosec // G304: test
	if err != nil {
		return fmt.Errorf("failed to read CLAUDE.md after second run: %w", err)
	}

	// Verify content is the same (block was replaced, not duplicated)
	if string(content1) != string(content2) {
		return fmt.Errorf("expected content to be identical after re-run (no duplication)")
	}

	// Verify only one start marker exists
	startCount := strings.Count(string(content2), "<!-- GROVE:SKILLS:START -->")
	if startCount != 1 {
		return fmt.Errorf("expected exactly 1 start marker, found %d", startCount)
	}

	return nil
}

// integratePreservesContent verifies that surrounding user content is preserved.
func integratePreservesContent(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	homeDir := ctx.HomeDir()
	configDir := ctx.ConfigDir()

	// Create a project with content before and after where the block will go
	projectDir := ctx.NewDir("integrate-test-preserve")
	existingContent := `# My Project

## Section Before

This content should be preserved.

## Section After

This content should also be preserved.
`
	if err := fs.WriteString(filepath.Join(projectDir, "CLAUDE.md"), existingContent); err != nil {
		return fmt.Errorf("failed to write existing CLAUDE.md: %w", err)
	}

	// Run integrate
	cmd := command.New(binary, "integrate", "--scope", "project").
		Dir(projectDir).
		Env("HOME="+homeDir, "XDG_CONFIG_HOME="+configDir)
	result := cmd.Run()

	ctx.ShowCommandOutput("integrate preserve output", result.Stdout, result.Stderr)

	if result.ExitCode != 0 {
		return fmt.Errorf("integrate command failed: %s", result.Stderr)
	}

	// Verify all sections are preserved
	content, err := os.ReadFile(filepath.Join(projectDir, "CLAUDE.md")) //nolint:gosec // G304: test
	if err != nil {
		return fmt.Errorf("failed to read CLAUDE.md: %w", err)
	}

	contentStr := string(content)

	if !strings.Contains(contentStr, "# My Project") {
		return fmt.Errorf("expected '# My Project' to be preserved")
	}
	if !strings.Contains(contentStr, "## Section Before") {
		return fmt.Errorf("expected '## Section Before' to be preserved")
	}
	if !strings.Contains(contentStr, "This content should be preserved") {
		return fmt.Errorf("expected 'This content should be preserved' to be preserved")
	}
	if !strings.Contains(contentStr, "## Section After") {
		return fmt.Errorf("expected '## Section After' to be preserved")
	}
	if !strings.Contains(contentStr, "This content should also be preserved") {
		return fmt.Errorf("expected 'This content should also be preserved' to be preserved")
	}

	// Verify grove-skills block was added
	if !strings.Contains(contentStr, "<!-- GROVE:SKILLS:START -->") {
		return fmt.Errorf("expected grove-skills block to be added")
	}

	return nil
}
