package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/notespace"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/pkg/worktreeregistry"
	"github.com/grovetools/core/util/pathutil"
)

func TestConfiguredNotespaceDirsUsesPrimaryResolverAndLocator(t *testing.T) {
	notebookRoot := t.TempDir()
	primaryRoot := filepath.Join(notebookRoot, workspace.NotespaceDirectory, "physical-primary")
	replicaRoot := filepath.Join(notebookRoot, workspace.NotespaceDirectory, "physical-replica")
	for _, dir := range []string{primaryRoot, replicaRoot, filepath.Join(notebookRoot, workspace.NotespaceDirectory, "unstamped")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	primary, err := notespace.MintNotespace(primaryRoot, notespace.NotespaceMutable{Name: "friendly-primary", Subject: "example.com/org/repo", Kind: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notespace.MintNotespace(replicaRoot, notespace.NotespaceMutable{Name: "friendly-replica", Subject: "example.com/org/repo", Kind: "repo"}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Notebooks: &config.NotebooksConfig{Definitions: map[string]*config.Notebook{
		"test": {RootDir: notebookRoot},
	}}}
	machine := &config.MachineConfig{Primaries: map[string]string{"example.com/org/repo": primary.ID}}

	got := configuredNotespaceDirs(cfg, machine, func(locator *workspace.NotebookLocator, node *workspace.WorkspaceNode) (string, error) {
		return locator.GetSkillsDir(node)
	})
	want := filepath.Join(primaryRoot, "skills")
	if len(got) != 1 || got[0] != want {
		t.Fatalf("configuredNotespaceDirs = %v, want only primary locator path %q", got, want)
	}
}

// normalizeForTest mirrors collectWorktreePaths' dedup key so assertions can
// compare paths through the same symlink/case normalization the code uses.
func normalizeForTest(t *testing.T, p string) string {
	t.Helper()
	n, err := pathutil.NormalizeForLookup(p)
	if err != nil {
		return p
	}
	return n
}

// containsNormalized reports whether want (after normalization) is present in
// the normalized form of got.
func containsNormalized(t *testing.T, got []string, want string) bool {
	t.Helper()
	wn := normalizeForTest(t, want)
	for _, g := range got {
		if normalizeForTest(t, g) == wn {
			return true
		}
	}
	return false
}

// TestCollectWorktreePaths_IncludesAnchoredRegistryWorktree verifies that an
// anchored worktree — owned by a sub-repo under the ecosystem and living
// OUTSIDE workspace.WorktreeBases(gitRoot) — is discovered via the registry
// and included in the skill-sync fan-out.
func TestCollectWorktreePaths_IncludesAnchoredRegistryWorktree(t *testing.T) {
	// Isolate GROVE_HOME so StateDir()/WorktreesDir() resolve under temp and
	// the registry we seed is the only one read back.
	t.Setenv("GROVE_HOME", t.TempDir())

	// Ecosystem root and a sub-repo nested under it.
	gitRoot := t.TempDir()
	subRepo := filepath.Join(gitRoot, "core")
	if err := os.MkdirAll(subRepo, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}

	// Anchored worktree lives under a different repo's XDG base — modeled here
	// as a standalone temp dir that is NOT under WorktreeBases(gitRoot).
	anchored := filepath.Join(t.TempDir(), "anchored-wt")
	if err := os.MkdirAll(anchored, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}

	// Sanity: the anchored path must not be reachable via the legacy bases,
	// otherwise the test would pass for the wrong reason. anchored sits in a
	// separate temp tree, so no base should be a parent of it.
	for _, base := range workspace.WorktreeBases(gitRoot) {
		if rel, err := filepath.Rel(base, anchored); err == nil && !filepathHasParentPrefix(rel) {
			t.Fatalf("test setup invalid: anchored %s is under WorktreeBases entry %s", anchored, base)
		}
	}

	if err := worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath: anchored,
		Owner:   subRepo, // owned by sub-repo under the ecosystem
		Plan:    "anchor-plan",
	}); err != nil {
		t.Fatal(err)
	}

	got := collectWorktreePaths(gitRoot)

	if !containsNormalized(t, got, anchored) {
		t.Fatalf("expected anchored worktree %s in fan-out, got %v", anchored, got)
	}
}

// filepathHasParentPrefix reports whether rel begins with a "../" component.
func filepathHasParentPrefix(rel string) bool {
	return len(rel) >= 2 && rel[0] == '.' && rel[1] == '.'
}

// TestCollectWorktreePaths_ExcludesForeignOwner verifies registry entries
// owned by a repo OUTSIDE the ecosystem are not synced into.
func TestCollectWorktreePaths_ExcludesForeignOwner(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	gitRoot := t.TempDir()
	foreignRoot := t.TempDir() // unrelated ecosystem

	foreignWt := filepath.Join(t.TempDir(), "foreign-wt")
	if err := os.MkdirAll(foreignWt, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}

	if err := worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath: foreignWt,
		Owner:   foreignRoot,
	}); err != nil {
		t.Fatal(err)
	}

	got := collectWorktreePaths(gitRoot)
	if containsNormalized(t, got, foreignWt) {
		t.Fatalf("foreign-owned worktree %s must not be in fan-out, got %v", foreignWt, got)
	}
}

// TestCollectWorktreePaths_SkipsMissingDir verifies a registry entry whose
// directory no longer exists is filtered out.
func TestCollectWorktreePaths_SkipsMissingDir(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	gitRoot := t.TempDir()
	missing := filepath.Join(t.TempDir(), "deleted-wt") // never created

	if err := worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath: missing,
		Owner:   gitRoot,
	}); err != nil {
		t.Fatal(err)
	}

	got := collectWorktreePaths(gitRoot)
	if containsNormalized(t, got, missing) {
		t.Fatalf("missing worktree %s must not be in fan-out, got %v", missing, got)
	}
}

// TestCollectWorktreePaths_DedupesAcrossSources verifies a worktree present in
// BOTH a legacy WorktreeBases enumeration and the registry appears only once.
func TestCollectWorktreePaths_DedupesAcrossSources(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	gitRoot := t.TempDir()

	// Create a worktree under the legacy in-repo base so Source 1 finds it.
	base := filepath.Join(gitRoot, ".grove-worktrees")
	wt := filepath.Join(base, "shared-wt")
	if err := os.MkdirAll(wt, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}

	// Also register the same path (owned by the ecosystem root).
	if err := worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath: wt,
		Owner:   gitRoot,
	}); err != nil {
		t.Fatal(err)
	}

	got := collectWorktreePaths(gitRoot)

	count := 0
	want := normalizeForTest(t, wt)
	for _, g := range got {
		if normalizeForTest(t, g) == want {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected worktree %s exactly once, got %d occurrences in %v", wt, count, got)
	}
}

// TestSyncSkillsToWorktrees_WritesIntoAnchoredWorktree exercises the full
// fan-out: an anchored, registry-only worktree must receive the synced skill
// files on disk.
func TestSyncSkillsToWorktrees_WritesIntoAnchoredWorktree(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	gitRoot := t.TempDir()
	subRepo := filepath.Join(gitRoot, "flow")
	if err := os.MkdirAll(subRepo, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}

	anchored := filepath.Join(t.TempDir(), "anchored-wt")
	if err := os.MkdirAll(anchored, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}

	if err := worktreeregistry.Save(&worktreeregistry.Entry{
		AbsPath: anchored,
		Owner:   subRepo,
	}); err != nil {
		t.Fatal(err)
	}

	// A resolved ecosystem skill with a real on-disk source directory.
	srcDir := filepath.Join(t.TempDir(), "my-skill")
	if err := os.MkdirAll(srcDir, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte("# my-skill\n"), 0o644); err != nil { //nolint:gosec // G306: test
		t.Fatal(err)
	}

	resolved := map[string]ResolvedSkill{
		"my-skill": {
			Name:         "my-skill",
			SourceType:   SourceTypeEcosystem,
			PhysicalPath: srcDir,
			Providers:    []string{"claude"},
		},
	}

	syncSkillsToWorktrees(gitRoot, resolved, nil, nil, false, nil, nil)

	want := filepath.Join(GetSkillsDirectoryForWorktree(anchored, "claude"), "my-skill", "SKILL.md")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected synced skill at %s: %v", want, err)
	}
}

func TestGetSkillsDirectoryForWorktree_ProviderMapping(t *testing.T) {
	cases := map[string]string{
		"claude":   filepath.Join("/wt", ".claude", "skills"),
		"codex":    filepath.Join("/wt", ".codex", "skills"),
		"opencode": filepath.Join("/wt", ".opencode", "skill"),
		// pi loads project skills from .pi/skills (Agent Skills standard,
		// skills.ts in the pi source).
		"pi": filepath.Join("/wt", ".pi", "skills"),
		// grove-agent is pi with configDir ".grove-agent" (agent/distro/
		// entry.ts). It must NOT fall through to the claude default: that
		// wrote skills to a directory its runtime never reads, which is how a
		// correctly authorized skill goes missing at launch.
		"grove-agent": filepath.Join("/wt", ".grove-agent", "skills"),
		// An unknown provider still lands somewhere writable rather than
		// returning "" and syncing into the worktree root.
		"something-else": filepath.Join("/wt", ".claude", "skills"),
	}
	for provider, want := range cases {
		if got := GetSkillsDirectoryForWorktree("/wt", provider); got != want {
			t.Errorf("GetSkillsDirectoryForWorktree(%q) = %q, want %q", provider, got, want)
		}
	}
}
