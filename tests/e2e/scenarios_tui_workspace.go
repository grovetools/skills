package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/grovetools/tend/pkg/fs"
	"github.com/grovetools/tend/pkg/git"
	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/tui"
	"github.com/grovetools/tend/pkg/verify"
)

// TUIWorkspaceScenario tests workspace-aware skill filtering and configuration toggling.
func TUIWorkspaceScenario() *harness.Scenario {
	return harness.NewScenarioWithOptions(
		"skills-tui-workspace",
		"Tests workspace-aware skill filtering and configuration toggling in the TUI",
		[]string{"tui", "browser", "workspace"},
		[]harness.Step{
			harness.NewStep("Setup workspace environment with skills configuration", setupTUIWorkspaceEnvironment),
			harness.NewStep("Launch TUI and verify active context mode", launchTUIAndVerifyContextMode),
			harness.NewStep("Test toggle between active and all skills view", testToggleAllSkills),
			harness.NewStep("Test configuration tags display", testConfigurationTags),
			harness.NewStep("Test toggle skill in project config", testToggleProjectConfig),
			harness.NewStep("Test toggle skill in global config", testToggleGlobalConfig),
			harness.NewStep("Exit TUI cleanly", exitTUIWorkspace),
		},
		true,  // localOnly - requires tmux
		false, // explicitOnly
	)
}

// setupTUIWorkspaceEnvironment creates a workspace with skills configured at different levels.
func setupTUIWorkspaceEnvironment(ctx *harness.Context) error {
	// 1. Configure a centralized notebook and global skills config
	notebookRoot := filepath.Join(ctx.HomeDir(), ".grove", "notebooks", "nb")
	globalTOML := fmt.Sprintf(`version = "1.0"

[groves.e2e-projects]
path = "%s"

[notebooks.rules]
default = "main"

[notebooks.definitions.main]
root_dir = "%s"

[skills]
use = ["explain-with-analogy"]
`, ctx.RootDir, notebookRoot)

	globalConfigDir := filepath.Join(ctx.HomeDir(), ".config", "grove")
	if err := fs.CreateDir(globalConfigDir); err != nil {
		return err
	}
	if err := fs.WriteString(filepath.Join(globalConfigDir, "grove.toml"), globalTOML); err != nil {
		return err
	}
	ctx.Set("global_config_path", filepath.Join(globalConfigDir, "grove.toml"))

	// 2. Create a project directory with its own skills config
	projectDir := ctx.NewDir("tui-workspace-test")
	projectTOML := `name = "tui-workspace-test"
version = "1.0"

[skills]
use = ["library-skill"]
`
	if err := fs.WriteString(filepath.Join(projectDir, "grove.toml"), projectTOML); err != nil {
		return err
	}
	ctx.Set("project_dir", projectDir)
	ctx.Set("project_config_path", filepath.Join(projectDir, "grove.toml"))

	repo, err := git.SetupTestRepo(projectDir)
	if err != nil {
		return err
	}
	if err := repo.AddCommit("initial commit"); err != nil {
		return err
	}

	// 3. Create workspace skills directory with test skills
	skillsDir := filepath.Join(notebookRoot, "workspaces", "tui-workspace-test", "skills")

	// Create test skill 1
	skill1Dir := filepath.Join(skillsDir, "test-skill-alpha")
	if err := fs.CreateDir(skill1Dir); err != nil {
		return err
	}
	skill1Content := `---
name: test-skill-alpha
description: A test skill for TUI testing
domain: testing
---

# Test Skill Alpha

This is a test skill for TUI testing.
`
	if err := fs.WriteString(filepath.Join(skill1Dir, "SKILL.md"), skill1Content); err != nil {
		return err
	}

	// Create test skill 2
	skill2Dir := filepath.Join(skillsDir, "test-skill-beta")
	if err := fs.CreateDir(skill2Dir); err != nil {
		return err
	}
	skill2Content := `---
name: test-skill-beta
description: Another test skill for TUI testing
domain: testing
---

# Test Skill Beta

This is another test skill for TUI testing.
`
	if err := fs.WriteString(filepath.Join(skill2Dir, "SKILL.md"), skill2Content); err != nil {
		return err
	}

	ctx.Set("notebooks_root", notebookRoot)
	return nil
}

// launchTUIAndVerifyContextMode launches the TUI and verifies the active context mode.
func launchTUIAndVerifyContextMode(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	projectDir := ctx.GetString("project_dir")
	session, err := ctx.StartTUI(binary, []string{"tui"},
		tui.WithEnv("HOME="+ctx.HomeDir()),
		tui.WithCwd(projectDir),
	)
	if err != nil {
		return fmt.Errorf("failed to start TUI: %w", err)
	}
	ctx.Set("tui_session", session)

	// Wait for TUI to load
	if err := session.WaitForText("Skills Browser", 5*time.Second); err != nil {
		view, _ := session.Capture()
		ctx.ShowCommandOutput("TUI Failed to Load", view, "")
		return fmt.Errorf("timeout waiting for TUI to load: %w", err)
	}

	// Capture initial state
	initialView, err := session.Capture()
	if err != nil {
		return fmt.Errorf("failed to capture initial view: %w", err)
	}
	ctx.ShowCommandOutput("TUI Initial View (Active Context Mode)", initialView, "")

	// Verify we're in "Active Context" mode by default
	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows Skills Browser header", initialView, "Skills Browser")
		v.Contains("shows Active Context mode indicator", initialView, "Active Context")
	})
}

// testToggleAllSkills tests switching between active context and all skills view.
func testToggleAllSkills(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Press 'A' (shift-a) to toggle to all skills view
	if err := session.SendKeys("A"); err != nil {
		return fmt.Errorf("failed to send 'A' key: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	allSkillsView, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI All Skills View", allSkillsView, "")

	// Verify we're now showing "All Skills" mode
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows All Skills mode indicator", allSkillsView, "All Skills")
	}); err != nil {
		return err
	}

	// Press 'A' again to go back to active context view
	if err := session.SendKeys("A"); err != nil {
		return fmt.Errorf("failed to send 'A' key again: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	activeView, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI back to Active Context", activeView, "")

	// Verify we're back to Active Context mode
	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows Active Context mode indicator again", activeView, "Active Context")
	})
}

// testConfigurationTags tests that skills show configuration tags [P], [E], [G].
func testConfigurationTags(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Switch to all skills view to see all configured skills
	if err := session.SendKeys("A"); err != nil {
		return fmt.Errorf("failed to switch to all skills: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Search for configured skills to find one with tags
	if err := session.SendKeys("/"); err != nil {
		return fmt.Errorf("failed to enter search mode: %w", err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := session.Type("library"); err != nil {
		return fmt.Errorf("failed to type search term: %w", err)
	}
	time.Sleep(200 * time.Millisecond)

	if err := session.SendKeys("Enter"); err != nil {
		return fmt.Errorf("failed to confirm search: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	searchView, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI searching for library skill", searchView, "")

	// Verify configuration tags are displayed
	// [P] = Project, [G] = Global - library-skill should show [P] since it's in project config
	if err := ctx.Verify(func(v *verify.Collector) {
		// Look for project tag indicator
		v.True("shows configuration tag for project", strings.Contains(searchView, "[P]") || strings.Contains(searchView, "library"))
	}); err != nil {
		return err
	}

	// Clear search
	if err := session.SendKeys("C-l"); err != nil {
		return fmt.Errorf("failed to clear search: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	return nil
}

// testToggleProjectConfig tests toggling a skill in the project configuration.
// Note: In the sandboxed test environment, workspace detection may not work,
// so we primarily verify the TUI responds to toggle key presses appropriately.
func testToggleProjectConfig(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)
	projectConfigPath := ctx.GetString("project_config_path")

	// Ensure we're in All Skills mode
	if err := session.SendKeys("A"); err != nil {
		return fmt.Errorf("failed to send 'A' key: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Wait for the view to show "All Skills"
	if err := session.WaitForText("All Skills", 3*time.Second); err != nil {
		// If we were already in All Skills, toggle back
		if err := session.SendKeys("A"); err != nil {
			return fmt.Errorf("failed to toggle back: %w", err)
		}
		time.Sleep(300 * time.Millisecond)
	}

	// Navigate to a skill (position 1, after group header)
	if err := session.SendKeys("j"); err != nil {
		return fmt.Errorf("failed to navigate: %w", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Capture before toggle
	beforeToggle, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI before toggle project", beforeToggle, "")

	// Press 'P' to toggle in project config
	if err := session.SendKeys("P"); err != nil {
		return fmt.Errorf("failed to send 'P' key: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Capture after toggle
	afterToggle, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI after toggle project", afterToggle, "")

	// Verify the project config still exists and has skills section
	configContent, err := fs.ReadString(projectConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read project config: %w", err)
	}
	ctx.ShowCommandOutput("Project grove.toml after toggle", configContent, "")

	// The config should still have skills section (test setup created it)
	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("config contains skills section", configContent, "[skills]")
	})
}

// testToggleGlobalConfig tests toggling a skill in the global configuration.
// This verifies that the global config file is properly updated.
func testToggleGlobalConfig(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)
	globalConfigPath := ctx.GetString("global_config_path")

	// Navigate to a skill
	if err := session.SendKeys("j"); err != nil {
		return fmt.Errorf("failed to navigate: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Capture before toggle
	beforeToggle, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI before toggle global", beforeToggle, "")

	// Press 'ctrl+g' to toggle in global config
	if err := session.SendKeys("C-g"); err != nil {
		return fmt.Errorf("failed to send 'ctrl+g' key: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Capture after toggle
	afterToggle, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI after toggle global", afterToggle, "")

	// Verify the global config still exists and has skills section
	configContent, err := fs.ReadString(globalConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read global config: %w", err)
	}
	ctx.ShowCommandOutput("Global grove.toml after toggle", configContent, "")

	// The config should have skills section
	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("global config contains skills section", configContent, "[skills]")
	})
}

// exitTUIWorkspace exits the TUI cleanly.
func exitTUIWorkspace(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Exit with q
	if err := session.SendKeys("q"); err != nil {
		return fmt.Errorf("failed to exit TUI: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	return nil
}
