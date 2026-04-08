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

// PlaybookPrecedenceScenario verifies the playbook resolver precedence
// between the ecosystem notebook tier and the user-scoped tier by
// materializing the same playbook name (`tier-pb`) at both locations
// with distinct skill content, running skills sync, and asserting
// which copy's skill body was synced. The current implementation
// exposes two overlapping tiers (see GetPlaybookSearchDirs): the
// project/ecosystem notebook playbooks dirs, and the user-scoped
// `~/.config/grove/playbooks` dir. Project-scoped `.grove/playbooks/`
// is not in the current search path.
func PlaybookPrecedenceScenario() *harness.Scenario {
	return &harness.Scenario{
		Name:        "playbook-precedence",
		Description: "Verify playbook resolver precedence (ecosystem-notebook > user)",
		Steps: []harness.Step{
			harness.NewStep("setup tiers", setupPlaybookPrecedenceEnvironment),
			harness.NewStep("ecosystem tier wins when both present", verifyEcosystemTierWinsInitial),
			harness.NewStep("user tier wins after ecosystem removed", verifyUserTierWins),
		},
	}
}

func setupPlaybookPrecedenceEnvironment(ctx *harness.Context) error {
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

	// Ecosystem that authorizes tier-pb. The ecosystem is what we
	// invoke sync from, and all three tier lookups orbit around this
	// workspace node.
	ecosystemDir := ctx.NewDir("prec-ecosystem")
	ecosystemTOML := `name = "prec-ecosystem"
workspaces = []

[playbooks]
use = ["tier-pb"]
`
	if err := fs.WriteString(filepath.Join(ecosystemDir, "grove.toml"), ecosystemTOML); err != nil {
		return err
	}
	repo, err := git.SetupTestRepo(ecosystemDir)
	if err != nil {
		return err
	}
	if err := repo.AddCommit("init"); err != nil {
		return err
	}

	// Helper that writes a full tier-pb playbook with a single skill
	// `tier-marker` whose description identifies the tier.
	writeTierPlaybook := func(root, tier string) error {
		if err := fs.CreateDir(root); err != nil {
			return err
		}
		manifest := fmt.Sprintf(`name = "tier-pb"
version = "1.0.0"
description = "%s tier"
`, tier)
		if err := fs.WriteString(filepath.Join(root, "playbook.toml"), manifest); err != nil {
			return err
		}
		skill := fmt.Sprintf(`---
name: tier-marker
description: %s tier marker skill
---

# Tier Marker (%s)
This body came from the %s tier.
`, tier, tier, tier)
		return fs.WriteString(filepath.Join(root, "skills", "tier-marker", "SKILL.md"), skill)
	}

	ecoPbDir := filepath.Join(notebookRoot, "workspaces", "prec-ecosystem", "playbooks", "tier-pb")
	if err := writeTierPlaybook(ecoPbDir, "ecosystem"); err != nil {
		return err
	}

	userPbDir := filepath.Join(ctx.HomeDir(), ".config", "grove", "playbooks", "tier-pb")
	if err := writeTierPlaybook(userPbDir, "user"); err != nil {
		return err
	}

	ctx.Set("ecosystem_dir", ecosystemDir)
	ctx.Set("eco_pb_dir", ecoPbDir)
	ctx.Set("user_pb_dir", userPbDir)
	return nil
}

func runSyncAndReadMarker(ctx *harness.Context) (string, error) {
	binary, err := FindBinary()
	if err != nil {
		return "", err
	}
	ecosystemDir := ctx.GetString("ecosystem_dir")

	cmd := command.New(binary, "sync").
		Dir(ecosystemDir).
		Env("HOME="+ctx.HomeDir(), "XDG_CONFIG_HOME="+ctx.ConfigDir())
	result := cmd.Run()
	if result.ExitCode != 0 {
		return "", fmt.Errorf("sync failed: %s", result.Stderr)
	}

	markerPath := filepath.Join(ecosystemDir, ".claude", "skills", "tier-marker", "SKILL.md")
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return "", fmt.Errorf("tier-marker not synced to %s: %w", markerPath, err)
	}
	return string(data), nil
}

func verifyEcosystemTierWinsInitial(ctx *harness.Context) error {
	content, err := runSyncAndReadMarker(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(content, "ecosystem tier") {
		return fmt.Errorf("expected ecosystem tier to win over user tier, got:\n%s", content)
	}
	return nil
}

func verifyUserTierWins(ctx *harness.Context) error {
	ecoPbDir := ctx.GetString("eco_pb_dir")
	if err := os.RemoveAll(ecoPbDir); err != nil {
		return err
	}
	content, err := runSyncAndReadMarker(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(content, "user tier") {
		return fmt.Errorf("expected user tier to win after ecosystem removal, got:\n%s", content)
	}
	return nil
}
