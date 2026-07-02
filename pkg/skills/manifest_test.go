package skills

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeSourceSkill creates an on-disk skill source dir with a SKILL.md.
func makeSourceSkill(t *testing.T, root, name string) ResolvedSkill {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name+"\n"), 0o644); err != nil { //nolint:gosec // G306: test
		t.Fatal(err)
	}
	return ResolvedSkill{
		Name:         name,
		SourceType:   SourceTypeEcosystem,
		PhysicalPath: dir,
		Providers:    []string{"claude"},
	}
}

// TestSyncConfiguredSkills_SecondSyncIsNoOp verifies the manifest round-trip:
// the first sync copies and records manifests; the second sync with unchanged
// sources reports zero changed skills and does not RemoveAll destinations
// (proven by a marker file surviving), in both the repo root and a worktree.
func TestSyncConfiguredSkills_SecondSyncIsNoOp(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	gitRoot := t.TempDir()
	srcRoot := t.TempDir()

	// A legacy-base worktree so the fan-out path is exercised too.
	wt := filepath.Join(gitRoot, ".grove-worktrees", "wt1")
	if err := os.MkdirAll(wt, 0o755); err != nil { //nolint:gosec // G301: test
		t.Fatal(err)
	}

	resolved := map[string]ResolvedSkill{
		"alpha": makeSourceSkill(t, srcRoot, "alpha"),
		"beta":  makeSourceSkill(t, srcRoot, "beta"),
	}

	changed, err := SyncConfiguredSkills(gitRoot, resolved, false, nil)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if len(changed) != 2 {
		t.Fatalf("first sync: expected 2 changed skills, got %v", changed)
	}

	rootDest := filepath.Join(GetSkillsDirectoryForWorktree(gitRoot, "claude"), "alpha")
	wtDest := filepath.Join(GetSkillsDirectoryForWorktree(wt, "claude"), "alpha")
	for _, dest := range []string{rootDest, wtDest} {
		if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
			t.Fatalf("expected synced skill at %s: %v", dest, err)
		}
		if _, err := os.Stat(filepath.Join(dest, syncManifestFileName)); err != nil {
			t.Fatalf("expected manifest at %s: %v", dest, err)
		}
		// Marker file: survives only if the second sync skips RemoveAll.
		if err := os.WriteFile(filepath.Join(dest, ".marker"), []byte("x"), 0o644); err != nil { //nolint:gosec // G306: test
			t.Fatal(err)
		}
	}

	changed, err = SyncConfiguredSkills(gitRoot, resolved, false, nil)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("second sync: expected no changed skills, got %v", changed)
	}
	for _, dest := range []string{rootDest, wtDest} {
		if _, err := os.Stat(filepath.Join(dest, ".marker")); err != nil {
			t.Fatalf("marker at %s removed — second sync was not a no-op: %v", dest, err)
		}
	}
}

// TestSyncConfiguredSkills_TouchedSourceResyncsOnlyThatSkill verifies that
// touching one source file re-syncs exactly that skill, leaving the other
// skill's destination untouched.
func TestSyncConfiguredSkills_TouchedSourceResyncsOnlyThatSkill(t *testing.T) {
	t.Setenv("GROVE_HOME", t.TempDir())

	gitRoot := t.TempDir()
	srcRoot := t.TempDir()

	alpha := makeSourceSkill(t, srcRoot, "alpha")
	beta := makeSourceSkill(t, srcRoot, "beta")
	resolved := map[string]ResolvedSkill{"alpha": alpha, "beta": beta}

	if _, err := SyncConfiguredSkills(gitRoot, resolved, false, nil); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	skillsDir := GetSkillsDirectoryForWorktree(gitRoot, "claude")
	for _, name := range []string{"alpha", "beta"} {
		if err := os.WriteFile(filepath.Join(skillsDir, name, ".marker"), []byte("x"), 0o644); err != nil { //nolint:gosec // G306: test
			t.Fatal(err)
		}
	}

	// Touch alpha's source (mtime bump is enough for the manifest to differ).
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filepath.Join(alpha.PhysicalPath, "SKILL.md"), future, future); err != nil {
		t.Fatal(err)
	}

	changed, err := SyncConfiguredSkills(gitRoot, resolved, false, nil)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(changed) != 1 || changed[0] != "alpha" {
		t.Fatalf("expected only alpha to change, got %v", changed)
	}

	if _, err := os.Stat(filepath.Join(skillsDir, "alpha", ".marker")); err == nil {
		t.Fatal("alpha dest was not re-copied (marker survived)")
	}
	if _, err := os.Stat(filepath.Join(skillsDir, "beta", ".marker")); err != nil {
		t.Fatalf("beta dest was touched but its source did not change: %v", err)
	}
}
