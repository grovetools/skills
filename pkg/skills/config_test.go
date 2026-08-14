package skills

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	coreconfig "github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
)

// globalCfg builds a core config whose [skills] extension is the given map,
// mirroring how ~/.config/grove/grove.toml reaches LoadSkillsConfig.
func globalCfg(skillsBlock map[string]interface{}) *coreconfig.Config {
	return &coreconfig.Config{Extensions: map[string]interface{}{"skills": skillsBlock}}
}

// writeGroveToml writes a grove.toml carrying the given body into dir.
func writeGroveToml(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "grove.toml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSeedScopeEcosystemRootSkipsMembers pins the core of the seeding-scope
// knob: a global declaration scoped to the ecosystem root reaches the root but
// not its member repositories, which is what stops the same skill set being
// loaded once per module by an ecosystem-top agent.
func TestSeedScopeEcosystemRootSkipsMembers(t *testing.T) {
	ecoPath := t.TempDir()
	cfg := globalCfg(map[string]interface{}{
		"use":   []interface{}{"alpha", "beta"},
		"scope": "ecosystem-root",
	})

	root := &workspace.WorkspaceNode{
		Name: "eco", Path: ecoPath,
		Kind:              workspace.KindEcosystemRoot,
		RootEcosystemPath: ecoPath,
	}
	got, err := LoadSkillsConfig(cfg, root)
	if err != nil {
		t.Fatalf("LoadSkillsConfig(root) error: %v", err)
	}
	if got == nil || !slices.Equal(got.Use, []string{"alpha", "beta"}) {
		t.Fatalf("ecosystem root should receive the scoped skills, got %+v", got)
	}
	if got.ScopedOut {
		t.Errorf("ecosystem root should not be reported as scoped out")
	}

	memberPath := filepath.Join(ecoPath, "member")
	member := &workspace.WorkspaceNode{
		Name: "member", Path: memberPath,
		Kind:              workspace.KindEcosystemSubProject,
		RootEcosystemPath: ecoPath,
	}
	got, err = LoadSkillsConfig(cfg, member)
	if err != nil {
		t.Fatalf("LoadSkillsConfig(member) error: %v", err)
	}
	if got == nil {
		t.Fatal("member config should be non-nil so the sync can report why it is empty")
	}
	if len(got.Use) != 0 {
		t.Errorf("member should receive no ecosystem-root-scoped skills, got %v", got.Use)
	}
	if !got.ScopedOut {
		t.Errorf("member should be flagged ScopedOut so an empty sync explains itself")
	}
}

// TestSeedScopeDefaultsToAll guards backward compatibility: a declaration
// without an explicit scope still reaches every member workspace.
func TestSeedScopeDefaultsToAll(t *testing.T) {
	ecoPath := t.TempDir()
	cfg := globalCfg(map[string]interface{}{"use": []interface{}{"alpha"}})

	member := &workspace.WorkspaceNode{
		Name: "member", Path: filepath.Join(ecoPath, "member"),
		Kind:              workspace.KindEcosystemSubProject,
		RootEcosystemPath: ecoPath,
	}
	got, err := LoadSkillsConfig(cfg, member)
	if err != nil {
		t.Fatalf("LoadSkillsConfig error: %v", err)
	}
	if got == nil || !slices.Equal(got.Use, []string{"alpha"}) {
		t.Fatalf("unscoped declaration should still reach members, got %+v", got)
	}
	if got.ScopedOut {
		t.Errorf("nothing was scoped out, ScopedOut should be false")
	}
}

// TestSeedScopeKeepsPerRepoSkills is the "without breaking per-repo scoped
// skills" half of the contract: narrowing the shared layers must not touch a
// member's own grove.toml or its [skills.projects.<name>] entry.
func TestSeedScopeKeepsPerRepoSkills(t *testing.T) {
	ecoPath := t.TempDir()
	memberPath := filepath.Join(ecoPath, "member")
	writeGroveToml(t, memberPath, "[skills]\nuse = [\"local-only\"]\n")

	cfg := globalCfg(map[string]interface{}{
		"use":   []interface{}{"shared"},
		"scope": "ecosystem-root",
		"projects": map[string]interface{}{
			"member": map[string]interface{}{"use": []interface{}{"user-scoped"}},
		},
	})

	member := &workspace.WorkspaceNode{
		Name: "member", Path: memberPath,
		Kind:              workspace.KindEcosystemSubProject,
		RootEcosystemPath: ecoPath,
	}
	got, err := LoadSkillsConfig(cfg, member)
	if err != nil {
		t.Fatalf("LoadSkillsConfig error: %v", err)
	}
	if got == nil {
		t.Fatal("expected a config")
	}
	for _, want := range []string{"user-scoped", "local-only"} {
		if !slices.Contains(got.Use, want) {
			t.Errorf("per-repo skill %q should survive an ecosystem-root scope, got %v", want, got.Use)
		}
	}
	if slices.Contains(got.Use, "shared") {
		t.Errorf("ecosystem-root-scoped skill leaked into a member: %v", got.Use)
	}
	if !got.ScopedOut {
		t.Errorf("the shared layer was dropped, ScopedOut should be true")
	}
}

// TestSeedScopeAppliesToStandaloneProject checks that a project outside any
// ecosystem keeps its skills: it is its own root, so there is no higher-level
// copy to dedupe against and dropping them would be pure loss.
func TestSeedScopeAppliesToStandaloneProject(t *testing.T) {
	cfg := globalCfg(map[string]interface{}{
		"use":   []interface{}{"alpha"},
		"scope": "ecosystem-root",
	})

	standalone := &workspace.WorkspaceNode{
		Name: "solo", Path: t.TempDir(),
		Kind: workspace.KindStandaloneProject,
	}
	got, err := LoadSkillsConfig(cfg, standalone)
	if err != nil {
		t.Fatalf("LoadSkillsConfig error: %v", err)
	}
	if got == nil || !slices.Contains(got.Use, "alpha") {
		t.Fatalf("standalone project should still receive the skills, got %+v", got)
	}
}

// TestSeedScopeOnEcosystemLayer exercises the per-layer nature of the knob:
// the ecosystem-scoped layer can be narrowed while the global base stays wide.
func TestSeedScopeOnEcosystemLayer(t *testing.T) {
	ecoRoot := t.TempDir()
	ecoName := filepath.Base(ecoRoot)
	cfg := globalCfg(map[string]interface{}{
		"use": []interface{}{"base"},
		"ecosystems": map[string]interface{}{
			ecoName: map[string]interface{}{
				"use":   []interface{}{"eco-wide"},
				"scope": "ecosystem-root",
			},
		},
	})

	member := &workspace.WorkspaceNode{
		Name: "member", Path: filepath.Join(ecoRoot, "member"),
		Kind:              workspace.KindEcosystemSubProject,
		RootEcosystemPath: ecoRoot,
	}
	got, err := LoadSkillsConfig(cfg, member)
	if err != nil {
		t.Fatalf("LoadSkillsConfig error: %v", err)
	}
	if got == nil || !slices.Contains(got.Use, "base") {
		t.Fatalf("unscoped global base should still reach the member, got %+v", got)
	}
	if slices.Contains(got.Use, "eco-wide") {
		t.Errorf("ecosystem layer was scoped to the root but leaked into the member: %v", got.Use)
	}
}

// TestSeedScopeRejectsUnknownValue keeps a typo from silently degrading to the
// permissive default.
func TestSeedScopeRejectsUnknownValue(t *testing.T) {
	cfg := globalCfg(map[string]interface{}{
		"use":   []interface{}{"alpha"},
		"scope": "ecosystem_root",
	})
	node := &workspace.WorkspaceNode{Name: "solo", Path: t.TempDir(), Kind: workspace.KindStandaloneProject}
	if _, err := LoadSkillsConfig(cfg, node); err == nil {
		t.Fatal("expected an error for an unrecognized [skills] scope")
	}
}

// TestSeedScopeFromEcosystemGroveToml covers the team-shared path: the
// ecosystem's own grove.toml narrows its declaration for every member.
func TestSeedScopeFromEcosystemGroveToml(t *testing.T) {
	ecoRoot := t.TempDir()
	writeGroveToml(t, ecoRoot, "[skills]\nuse = [\"team-wide\"]\nscope = \"ecosystem-root\"\n")

	memberPath := filepath.Join(ecoRoot, "member")
	if err := os.MkdirAll(memberPath, 0o755); err != nil {
		t.Fatal(err)
	}
	member := &workspace.WorkspaceNode{
		Name: "member", Path: memberPath,
		Kind:              workspace.KindEcosystemSubProject,
		RootEcosystemPath: ecoRoot,
	}
	got, err := LoadSkillsConfig(nil, member)
	if err != nil {
		t.Fatalf("LoadSkillsConfig(member) error: %v", err)
	}
	if got != nil && slices.Contains(got.Use, "team-wide") {
		t.Errorf("scoped ecosystem grove.toml leaked into a member: %v", got.Use)
	}

	root := &workspace.WorkspaceNode{
		Name: filepath.Base(ecoRoot), Path: ecoRoot,
		Kind:              workspace.KindEcosystemRoot,
		RootEcosystemPath: ecoRoot,
	}
	got, err = LoadSkillsConfig(nil, root)
	if err != nil {
		t.Fatalf("LoadSkillsConfig(root) error: %v", err)
	}
	if got == nil || !slices.Contains(got.Use, "team-wide") {
		t.Fatalf("ecosystem root should receive its own declaration, got %+v", got)
	}
}
