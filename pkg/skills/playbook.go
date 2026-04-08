package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/pathutil"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// PlaybookManifest mirrors the contents of a playbook's `playbook.toml` file.
type PlaybookManifest struct {
	Name          string   `toml:"name"`
	Description   string   `toml:"description"`
	Version       string   `toml:"version"`
	Authors       []string `toml:"authors"`
	DefaultRecipe string   `toml:"default_recipe"`
}

// PlaybookSkill is a summary entry for a skill shipped in a playbook.
type PlaybookSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// RelPath is the path of the skill directory relative to the playbook's
	// skills/ root (useful for nested skill layouts).
	RelPath string `json:"rel_path"`
}

// PlaybookPrompt is a summary entry for a prompt file shipped in a playbook.
type PlaybookPrompt struct {
	File    string `json:"file"`
	Purpose string `json:"purpose"`
}

// PlaybookRecipe is a summary entry for a recipe file shipped in a playbook.
type PlaybookRecipe struct {
	File        string `json:"file"`
	Description string `json:"description"`
}

// Playbook represents a loaded playbook — a versioned bundle of skills,
// prompts, recipes, and references that together define a coherent
// methodology (e.g. "gdv2").
type Playbook struct {
	Manifest PlaybookManifest `json:"manifest"`
	// Path is the absolute filesystem path to the playbook root directory.
	Path    string           `json:"path"`
	Skills  []PlaybookSkill  `json:"skills"`
	Prompts []PlaybookPrompt `json:"prompts"`
	Recipes []PlaybookRecipe `json:"recipes"`
}

// playbookCache memoizes loaded playbooks by absolute path for the lifetime
// of the process. Playbooks are small and loaded once per CLI invocation;
// filesystem walks are cheap enough that per-path caching is sufficient.
var (
	playbookCache   sync.Map // key: absolute path, value: *Playbook
	playbookSearch  []string
	playbookSearchM sync.RWMutex
)

// RegisterPlaybookSearchPath appends a directory that LoadPlaybook should
// consult when resolving a playbook by name. Paths are searched in the order
// they are registered, from highest to lowest precedence.
func RegisterPlaybookSearchPath(dir string) {
	if dir == "" {
		return
	}
	playbookSearchM.Lock()
	defer playbookSearchM.Unlock()
	for _, existing := range playbookSearch {
		if existing == dir {
			return
		}
	}
	playbookSearch = append(playbookSearch, dir)
}

// ResetPlaybookSearchPaths clears the registered search paths. Intended for
// tests.
func ResetPlaybookSearchPaths() {
	playbookSearchM.Lock()
	defer playbookSearchM.Unlock()
	playbookSearch = nil
	playbookCache.Range(func(k, _ any) bool {
		playbookCache.Delete(k)
		return true
	})
}

// GetPlaybookSearchDirs returns the directories that ResolvePlaybookPath
// should consult for a given working directory, in strict 4-tier
// precedence order:
//
//  1. Project — the notebook workspace playbooks/ directory for the
//     project that contains workDir (e.g.
//     ~/notebooks/<nb>/workspaces/<project>/playbooks/).
//  2. Ecosystem — the playbooks/ directory for the root ecosystem that
//     contains the project (e.g.
//     ~/notebooks/<nb>/workspaces/<ecosystem>/playbooks/).
//  3. User — the global user-scoped playbooks directory
//     (~/.config/grove/playbooks, or $XDG_CONFIG_HOME/grove/playbooks).
//  4. Builtin — no builtin playbooks ship today; reserved for future use.
//
// Any directories that tests register via RegisterPlaybookSearchPath are
// appended after the tiered locations so test fixtures continue to work.
// Duplicates are removed while preserving precedence order. An empty
// workDir skips tiers 1 and 2 and returns only the user and
// globally-registered directories.
//
// This mirrors the skill resolver's 4-tier discovery
// (skills/pkg/skills/discovery.go) and uses the same NotebookLocator
// helper (core/pkg/workspace/notebook_locator.go) to compute playbook
// paths so the two resolvers stay in lockstep.
func GetPlaybookSearchDirs(workDir string) []string {
	var dirs []string

	// Tiers 1 & 2: project and ecosystem notebook playbooks dirs.
	if workDir != "" {
		if node, _ := workspace.GetProjectByPath(workDir); node != nil {
			if svc, err := NewServiceForNode(node); err == nil && svc != nil && svc.NotebookLocator != nil {
				// Tier 1 — project
				if pbDir, err := svc.NotebookLocator.GetPlaybooksDir(node); err == nil && pbDir != "" {
					dirs = append(dirs, pbDir)
				}
				// Tier 2 — ecosystem (if the project is inside one)
				if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
					ecoNode := &workspace.WorkspaceNode{
						Name:         filepath.Base(node.RootEcosystemPath),
						Path:         node.RootEcosystemPath,
						NotebookName: node.NotebookName,
					}
					if ecoDir, err := svc.NotebookLocator.GetPlaybooksDir(ecoNode); err == nil && ecoDir != "" {
						dirs = append(dirs, ecoDir)
					}
				}
			}
		}
	}

	// Tier 2b — all notebook workspaces known to the user's config.
	// This ensures `flow playbook show <name>` works even when the
	// current directory isn't inside a grove-managed project: a
	// playbook that lives in any configured notebook workspace's
	// playbooks/ directory is discoverable. Mirrors
	// sync.go's addNotebookSkillSources for skill discovery.
	dirs = append(dirs, notebookWorkspacePlaybookDirs()...)

	// Tier 3 — user.
	if userDir := userPlaybooksDir(); userDir != "" {
		dirs = append(dirs, userDir)
	}

	// Tier 4 — builtin (none today).

	// Append globally-registered search paths (tests, ad-hoc callers).
	playbookSearchM.RLock()
	extra := append([]string(nil), playbookSearch...)
	playbookSearchM.RUnlock()
	dirs = append(dirs, extra...)

	// De-duplicate while preserving precedence order.
	seen := make(map[string]bool, len(dirs))
	unique := dirs[:0]
	for _, dir := range dirs {
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		unique = append(unique, dir)
	}
	return unique
}

// notebookWorkspacePlaybookDirs returns a `playbooks/` directory for
// every workspace under every notebook defined in the user's grove
// config. This is the "global" fallback discovery path used when the
// CLI is invoked without a grove-managed workspace context, so e.g.
// `flow playbook show gdv2` can find a playbook that lives inside
// ~/notebooks/grovetools/workspaces/grovetools/playbooks/gdv2/ without
// requiring any sync to have run first. Mirrors the skill discovery
// logic in sync.go's addNotebookSkillSources.
func notebookWorkspacePlaybookDirs() []string {
	cfg, err := config.LoadDefault()
	if err != nil || cfg == nil || cfg.Notebooks == nil {
		return nil
	}
	var dirs []string
	for _, nb := range cfg.Notebooks.Definitions {
		if nb == nil || nb.RootDir == "" {
			continue
		}
		rootDir, err := pathutil.Expand(nb.RootDir)
		if err != nil {
			continue
		}
		workspacesDir := filepath.Join(rootDir, "workspaces")
		entries, err := os.ReadDir(workspacesDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			dirs = append(dirs, filepath.Join(workspacesDir, entry.Name(), "playbooks"))
		}
	}
	return dirs
}

// userPlaybooksDir returns the global user-scoped playbooks directory,
// respecting XDG_CONFIG_HOME.
func userPlaybooksDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "grove", "playbooks")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "grove", "playbooks")
}

// ResolvePlaybookPath returns the absolute path of the named playbook,
// consulting the 4-tier search paths (project, ecosystem, user, builtin)
// plus any globally-registered directories for the given workDir and
// returning the first directory that contains a `playbook.toml` manifest.
// An empty workDir restricts the search to tiers 3 & 4 plus
// globally-registered dirs, which is the correct fallback when no
// workspace context is available.
func ResolvePlaybookPath(workDir, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("playbook name is required")
	}
	for _, dir := range GetPlaybookSearchDirs(workDir) {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(filepath.Join(candidate, "playbook.toml")); err == nil {
			abs, err := filepath.Abs(candidate)
			if err != nil {
				return candidate, nil
			}
			return abs, nil
		}
	}
	return "", fmt.Errorf("playbook %q not found in any registered search path", name)
}

// LoadPlaybook resolves the named playbook via the 4-tier precedence
// search rooted at workDir and parses its manifest, skills, prompts, and
// recipes into a Playbook struct. Results are cached per absolute path
// for the process lifetime.
func LoadPlaybook(workDir, name string) (*Playbook, error) {
	path, err := ResolvePlaybookPath(workDir, name)
	if err != nil {
		return nil, err
	}
	return LoadPlaybookFromDir(path)
}

// LoadPlaybookFromDir loads a playbook from an explicit directory. The
// directory must contain a `playbook.toml` manifest.
func LoadPlaybookFromDir(dir string) (*Playbook, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	if cached, ok := playbookCache.Load(abs); ok {
		return cached.(*Playbook), nil
	}

	manifestPath := filepath.Join(abs, "playbook.toml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading playbook manifest: %w", err)
	}
	var manifest PlaybookManifest
	if err := toml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing playbook manifest: %w", err)
	}

	pb := &Playbook{
		Manifest: manifest,
		Path:     abs,
		Skills:   loadPlaybookSkills(filepath.Join(abs, "skills")),
		Prompts:  loadPlaybookPrompts(filepath.Join(abs, "prompts")),
		Recipes:  loadPlaybookRecipes(filepath.Join(abs, "recipes")),
	}

	playbookCache.Store(abs, pb)
	return pb, nil
}

// loadPlaybookSkills walks the skills/ directory, parses SKILL.md frontmatter
// for each found skill, and returns summary entries sorted by name.
func loadPlaybookSkills(skillsDir string) []PlaybookSkill {
	if _, err := os.Stat(skillsDir); err != nil {
		return nil
	}
	var out []PlaybookSkill
	_ = filepath.WalkDir(skillsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		meta, err := ParseSkillFrontmatter(content)
		if err != nil || meta == nil {
			return nil
		}
		relDir, _ := filepath.Rel(skillsDir, filepath.Dir(path))
		name := meta.Name
		if name == "" {
			name = filepath.Base(filepath.Dir(path))
		}
		out = append(out, PlaybookSkill{
			Name:        name,
			Description: strings.TrimSpace(meta.Description),
			RelPath:     filepath.ToSlash(relDir),
		})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// purposeCommentRegex matches an HTML comment of the form:
//
//	<!-- purpose: some description -->
var purposeCommentRegex = regexp.MustCompile(`(?i)<!--\s*purpose:\s*(.+?)\s*-->`)

// loadPlaybookPrompts walks the prompts/ directory and returns summary
// entries. The purpose string is pulled from a `<!-- purpose: ... -->`
// comment if present, otherwise the first non-empty paragraph of the file.
func loadPlaybookPrompts(promptsDir string) []PlaybookPrompt {
	if _, err := os.Stat(promptsDir); err != nil {
		return nil
	}
	var out []PlaybookPrompt
	entries, err := os.ReadDir(promptsDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(promptsDir, entry.Name()))
		if err != nil {
			continue
		}
		out = append(out, PlaybookPrompt{
			File:    entry.Name(),
			Purpose: extractPromptPurpose(content),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out
}

// extractPromptPurpose returns the first `<!-- purpose: X -->` comment in
// the prompt content, or the first non-empty paragraph as a fallback.
func extractPromptPurpose(content []byte) string {
	if m := purposeCommentRegex.FindSubmatch(content); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	// Strip a leading YAML frontmatter block if present.
	text := string(content)
	if strings.HasPrefix(text, "---") {
		if end := strings.Index(text[3:], "\n---"); end >= 0 {
			text = text[3+end+4:]
		}
	}
	for _, para := range strings.Split(text, "\n\n") {
		trimmed := strings.TrimSpace(para)
		if trimmed == "" {
			continue
		}
		// Use the first line of the paragraph as the summary.
		line := strings.SplitN(trimmed, "\n", 2)[0]
		return strings.TrimSpace(line)
	}
	return ""
}

// loadPlaybookRecipes walks the recipes/ directory and extracts the
// `description:` field from each recipe's YAML frontmatter.
func loadPlaybookRecipes(recipesDir string) []PlaybookRecipe {
	if _, err := os.Stat(recipesDir); err != nil {
		return nil
	}
	var out []PlaybookRecipe
	entries, err := os.ReadDir(recipesDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(recipesDir, entry.Name()))
		if err != nil {
			continue
		}
		out = append(out, PlaybookRecipe{
			File:        entry.Name(),
			Description: extractRecipeDescription(content),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out
}

// extractRecipeDescription returns the `description:` field from a recipe
// file's YAML frontmatter, or an empty string if none is present.
func extractRecipeDescription(content []byte) string {
	text := string(content)
	if !strings.HasPrefix(text, "---") {
		return ""
	}
	end := strings.Index(text[3:], "\n---")
	if end < 0 {
		return ""
	}
	frontmatter := text[3 : 3+end]
	var fm struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return ""
	}
	return strings.TrimSpace(fm.Description)
}
