package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPlaybookFromDir(t *testing.T) {
	dir := t.TempDir()

	// Manifest
	manifest := `name = "minimal"
description = "A minimal test playbook"
version = "0.1.0"
default_recipe = "feature"
`
	if err := os.WriteFile(filepath.Join(dir, "playbook.toml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	// One skill
	skillDir := filepath.Join(dir, "skills", "hello")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillMd := `---
name: hello
description: A friendly greeting skill used to say hi to the user.
---

Hello.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0644); err != nil {
		t.Fatal(err)
	}

	// One prompt with purpose comment
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0755); err != nil {
		t.Fatal(err)
	}
	prompt := "<!-- purpose: Bootstrap a chat with strict rules -->\n\nSome body text.\n"
	if err := os.WriteFile(filepath.Join(dir, "prompts", "bootstrap.md"), []byte(prompt), 0644); err != nil {
		t.Fatal(err)
	}

	// One prompt without purpose comment — should fall back to first paragraph
	fallback := "The first real line of the prompt.\n\nSecond paragraph.\n"
	if err := os.WriteFile(filepath.Join(dir, "prompts", "fallback.md"), []byte(fallback), 0644); err != nil {
		t.Fatal(err)
	}

	// One recipe
	if err := os.MkdirAll(filepath.Join(dir, "recipes"), 0755); err != nil {
		t.Fatal(err)
	}
	recipe := "---\ndescription: Standard test recipe\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "recipes", "feature.md"), []byte(recipe), 0644); err != nil {
		t.Fatal(err)
	}

	pb, err := LoadPlaybookFromDir(dir)
	if err != nil {
		t.Fatalf("LoadPlaybookFromDir: %v", err)
	}
	if pb.Manifest.Name != "minimal" {
		t.Errorf("manifest name: got %q want %q", pb.Manifest.Name, "minimal")
	}
	if len(pb.Skills) != 1 || pb.Skills[0].Name != "hello" {
		t.Errorf("skills: got %+v", pb.Skills)
	}
	if len(pb.Prompts) != 2 {
		t.Fatalf("prompts: got %+v", pb.Prompts)
	}
	// Prompts are sorted by filename: bootstrap.md, fallback.md
	if pb.Prompts[0].File != "bootstrap.md" || pb.Prompts[0].Purpose != "Bootstrap a chat with strict rules" {
		t.Errorf("bootstrap prompt: got %+v", pb.Prompts[0])
	}
	if pb.Prompts[1].File != "fallback.md" || pb.Prompts[1].Purpose != "The first real line of the prompt." {
		t.Errorf("fallback prompt: got %+v", pb.Prompts[1])
	}
	if len(pb.Recipes) != 1 || pb.Recipes[0].Description != "Standard test recipe" {
		t.Errorf("recipes: got %+v", pb.Recipes)
	}
}

func TestLoadPlaybookViaSearchPath(t *testing.T) {
	ResetPlaybookSearchPaths()
	defer ResetPlaybookSearchPaths()

	root := t.TempDir()
	pbDir := filepath.Join(root, "tiny")
	if err := os.MkdirAll(pbDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `name = "tiny"
description = "Tiny"
version = "0.0.1"
`
	if err := os.WriteFile(filepath.Join(pbDir, "playbook.toml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	RegisterPlaybookSearchPath(root)

	pb, err := LoadPlaybook("tiny")
	if err != nil {
		t.Fatalf("LoadPlaybook: %v", err)
	}
	if pb.Manifest.Name != "tiny" {
		t.Errorf("got %q want %q", pb.Manifest.Name, "tiny")
	}
}
