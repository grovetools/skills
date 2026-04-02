package skills

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// setupAuthTestWorkspace creates a minimal workspace with a git repo and grove.toml
// that declares specific skills in [skills] use. Returns the project directory.
func setupAuthTestWorkspace(t *testing.T, useSkills []string) string {
	t.Helper()

	// Create isolated HOME so we don't pick up the real user's config
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpHome, ".config"))

	// Create minimal global grove.toml (required for config.LoadDefault)
	globalConfigDir := filepath.Join(tmpHome, ".config", "grove")
	if err := os.MkdirAll(globalConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalConfigDir, "grove.toml"), []byte(`version = "1.0"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create project directory with git repo
	projectDir := filepath.Join(tmpHome, "test-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Build grove.toml with [skills] use
	toml := `name = "test-project"
version = "1.0"

[skills]
use = [`
	for i, s := range useSkills {
		if i > 0 {
			toml += ", "
		}
		toml += `"` + s + `"`
	}
	toml += "]\n"

	if err := os.WriteFile(filepath.Join(projectDir, "grove.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	// Initialize git repo (workspace discovery requires it)
	for _, args := range [][]string{
		{"git", "init"},
		{"git", "add", "."},
		{"git", "-c", "user.name=test", "-c", "user.email=test@test.com", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = projectDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %s\n%s", args, err, out)
		}
	}

	return projectDir
}

func TestLoadAuthorizedSkill_Authorized(t *testing.T) {
	// "explain-with-analogy" is a builtin skill — authorize it
	projectDir := setupAuthTestWorkspace(t, []string{"explain-with-analogy"})

	loaded, err := LoadAuthorizedSkill(projectDir, "explain-with-analogy")
	if err != nil {
		t.Fatalf("expected authorized skill to load, got error: %v", err)
	}
	if loaded.Name != "explain-with-analogy" {
		t.Errorf("expected name 'explain-with-analogy', got '%s'", loaded.Name)
	}
	if loaded.SourceType != SourceTypeBuiltin {
		t.Errorf("expected source type 'builtin', got '%s'", loaded.SourceType)
	}
	if _, ok := loaded.Files["SKILL.md"]; !ok {
		t.Error("expected SKILL.md in loaded files")
	}
}

func TestLoadAuthorizedSkill_Rejected(t *testing.T) {
	// Only authorize "explain-with-analogy", then try to load a different builtin
	projectDir := setupAuthTestWorkspace(t, []string{"explain-with-analogy"})

	_, err := LoadAuthorizedSkill(projectDir, "simplify")
	if err == nil {
		t.Fatal("expected ErrSkillNotAuthorized, got nil")
	}

	var authErr *ErrSkillNotAuthorized
	if !errors.As(err, &authErr) {
		t.Fatalf("expected ErrSkillNotAuthorized, got: %T: %v", err, err)
	}
	if authErr.SkillName != "simplify" {
		t.Errorf("expected skill name 'simplify', got '%s'", authErr.SkillName)
	}
}

func TestLoadAuthorizedSkill_EmptyUseRejectsAll(t *testing.T) {
	// No skills declared — everything should be rejected
	projectDir := setupAuthTestWorkspace(t, []string{})

	_, err := LoadAuthorizedSkill(projectDir, "explain-with-analogy")
	if err == nil {
		t.Fatal("expected ErrSkillNotAuthorized when use list is empty, got nil")
	}

	var authErr *ErrSkillNotAuthorized
	if !errors.As(err, &authErr) {
		t.Fatalf("expected ErrSkillNotAuthorized, got: %T: %v", err, err)
	}
}

func TestLoadSkillBypassingAccess_IgnoresAuthorization(t *testing.T) {
	// No skills declared, but bypassing access should still work
	projectDir := setupAuthTestWorkspace(t, []string{})

	loaded, err := LoadSkillBypassingAccess(projectDir, "explain-with-analogy")
	if err != nil {
		t.Fatalf("expected bypassing access to succeed, got error: %v", err)
	}
	if loaded.Name != "explain-with-analogy" {
		t.Errorf("expected name 'explain-with-analogy', got '%s'", loaded.Name)
	}
}
