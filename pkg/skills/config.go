package skills

import (
	"fmt"
	"os"
	"path/filepath"

	coreconfig "github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/pelletier/go-toml/v2"
)

// SeedScope controls which workspaces a single [skills] declaration is seeded
// into. It is a property of the declaring layer, not of the merged result:
// each layer of the cascade carries its own scope and is evaluated against the
// workspace being synced before its skills are merged in.
type SeedScope string

const (
	// SeedScopeAll seeds the declaration into every workspace that inherits
	// it — the ecosystem root and each of its member repositories. This is
	// the default and the historical behavior.
	SeedScopeAll SeedScope = "all"

	// SeedScopeEcosystemRoot seeds the declaration only into workspaces that
	// are an ecosystem root (or ecosystem worktree), plus standalone projects
	// that sit outside any ecosystem and are therefore their own root.
	// Member repositories of an ecosystem are skipped, so an agent working at
	// the top of an ecosystem sees the skill set once instead of once per
	// member. Skills declared for a member repository specifically — its own
	// grove.toml, or [skills.projects.<name>] in the global config — are
	// unaffected and still seeded there.
	SeedScopeEcosystemRoot SeedScope = "ecosystem-root"
)

// scopeAppliesTo reports whether a [skills] declaration carrying the given
// seed scope should contribute skills to node.
func scopeAppliesTo(scope SeedScope, node *workspace.WorkspaceNode) bool {
	if scope != SeedScopeEcosystemRoot {
		return true
	}
	if node == nil || node.IsEcosystem() {
		return true
	}
	// A workspace that belongs to no ecosystem is its own root: there is no
	// higher-level copy of these skills for an agent to fall back on, so
	// scoping them out would be pure loss rather than deduplication.
	return node.RootEcosystemPath == ""
}

// validateSeedScope rejects unrecognized scope values so a typo surfaces as a
// config error instead of silently reverting to the permissive default.
func validateSeedScope(cfg *SkillsConfig, source string) error {
	if cfg == nil {
		return nil
	}
	switch cfg.Scope {
	case "", SeedScopeAll, SeedScopeEcosystemRoot:
		return nil
	default:
		return fmt.Errorf("invalid [skills] scope %q in %s (expected %q or %q)",
			cfg.Scope, source, SeedScopeAll, SeedScopeEcosystemRoot)
	}
}

// DependencyConfig specifies how a particular skill should be resolved.
type DependencyConfig struct {
	// Source specifies where to resolve the skill from.
	// Valid values: "builtin", "user", "notebook", or empty for default precedence.
	Source string `toml:"source" yaml:"source"`

	// Name allows aliasing - use a different skill name for resolution.
	Name string `toml:"name" yaml:"name"`

	// Providers overrides the default providers for this skill.
	Providers []string `toml:"providers" yaml:"providers"`
}

// SkillsConfig represents the [skills] block in grove.toml.
type SkillsConfig struct {
	// Use lists the skills to be made available.
	Use []string `toml:"use" yaml:"use"`

	// Providers specifies the default agent providers to sync skills to.
	// Defaults to ["claude"] if not specified.
	Providers []string `toml:"providers" yaml:"providers"`

	// Dependencies provides explicit configuration for specific skills.
	Dependencies map[string]DependencyConfig `toml:"dependencies" yaml:"dependencies"`

	// Projects maps project names to user-scoped skill configurations.
	// Used in global config (~/.config/grove/grove.toml) to define
	// project-specific skills that live in dotfiles rather than repo config.
	Projects map[string]*SkillsConfig `toml:"projects" yaml:"projects"`

	// Ecosystems maps ecosystem names to user-scoped skill configurations.
	// Used in global config (~/.config/grove/grove.toml) to define
	// ecosystem-specific skills that live in dotfiles rather than repo config.
	Ecosystems map[string]*SkillsConfig `toml:"ecosystems" yaml:"ecosystems"`

	// Scope controls which workspaces THIS declaration is seeded into.
	// Defaults to SeedScopeAll. Set it to SeedScopeEcosystemRoot on a layer
	// whose skills should live only at the top of an ecosystem instead of
	// being copied into every member repository. Each layer of the cascade
	// carries its own scope; the merged result never does.
	Scope SeedScope `toml:"scope" yaml:"scope"`

	// ScopedOut is set by LoadSkillsConfig when at least one layer of the
	// cascade contributed nothing to this workspace because its Scope
	// excluded it. It is diagnostic only — never read from grove.toml — and
	// lets callers explain an empty skill set as "scoped elsewhere" rather
	// than "nothing configured".
	ScopedOut bool `toml:"-" yaml:"-"`
}

// groveTomlSkills is used to extract the skills block from grove.toml
type groveTomlSkills struct {
	Skills *SkillsConfig `toml:"skills"`
}

// LoadSkillsConfig extracts the skills configuration from grove.toml in the workspace.
// It handles inheritance by merging configurations in strict precedence order:
//
//  1. global.skills (base)
//  2. global.skills.ecosystems.<name> (user-scoped ecosystem overrides)
//  3. ecosystem grove.toml (team-shared ecosystem config)
//  4. global.skills.projects.<name> (user-scoped project overrides)
//  5. project grove.toml (team-shared project config, highest precedence)
//
// User config merges before actual project/ecosystem config, so team-configured
// skills take precedence but user preferences fill in the gaps.
//
// Each layer additionally carries its own seeding scope (SkillsConfig.Scope).
// A layer scoped to SeedScopeEcosystemRoot is skipped entirely for workspaces
// that are members of an ecosystem, so ecosystem-wide skills are seeded once at
// the ecosystem root instead of once per member repository. Layers that target
// a specific repository — its own grove.toml, or global [skills.projects.<name>]
// — are unaffected unless they opt in themselves.
func LoadSkillsConfig(cfg *coreconfig.Config, node *workspace.WorkspaceNode) (*SkillsConfig, error) {
	// Load global config first (contains both base skills and user-scoped overrides)
	globalConfig := loadSkillsFromGlobalConfig(cfg)
	if err := validateSeedScope(globalConfig, "global config [skills]"); err != nil {
		return nil, err
	}

	// If no node, just return base global config (without project/ecosystem scopes)
	if node == nil {
		return applySkillsDefaults(copySkillsConfig(globalConfig)), nil
	}

	// scopedOut records whether any layer was dropped for this workspace, so
	// callers can distinguish "scoped elsewhere" from "nothing configured".
	scopedOut := false
	apply := func(acc, layer *SkillsConfig) *SkillsConfig {
		if layer == nil {
			return acc
		}
		if !scopeAppliesTo(layer.Scope, node) {
			if len(layer.Use) > 0 || len(layer.Dependencies) > 0 {
				scopedOut = true
			}
			return acc
		}
		return mergeSkillsConfig(acc, layer)
	}

	// Start from the base global config
	var merged *SkillsConfig
	merged = apply(merged, globalConfig)

	// Determine ecosystem name for user-scoped lookups
	var ecoName string
	if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
		ecoName = filepath.Base(node.RootEcosystemPath)
	} else if node.IsEcosystem() {
		ecoName = node.Name
	}

	// 1. Apply global ecosystem overrides (user-scoped, from ~/.config/grove/grove.toml)
	if ecoName != "" && globalConfig != nil && globalConfig.Ecosystems != nil {
		if ecoCfg, ok := globalConfig.Ecosystems[ecoName]; ok {
			if err := validateSeedScope(ecoCfg, fmt.Sprintf("global config [skills.ecosystems.%s]", ecoName)); err != nil {
				return nil, err
			}
			merged = apply(merged, ecoCfg)
		}
	}

	// 2. Apply local ecosystem config (team-shared, from ecosystem grove.toml)
	if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
		ecosystemConfig, err := LoadSkillsFromPath(node.RootEcosystemPath)
		if err != nil {
			return nil, err
		}
		if err := validateSeedScope(ecosystemConfig, filepath.Join(node.RootEcosystemPath, "grove.toml")); err != nil {
			return nil, err
		}
		merged = apply(merged, ecosystemConfig)
	}

	// 3. Apply global project overrides (user-scoped, from ~/.config/grove/grove.toml)
	// Use repository name, not worktree name
	if globalConfig != nil && globalConfig.Projects != nil {
		projectName := node.Name
		if node.ParentProjectPath != "" {
			projectName = filepath.Base(node.ParentProjectPath)
		}
		if projCfg, ok := globalConfig.Projects[projectName]; ok {
			if err := validateSeedScope(projCfg, fmt.Sprintf("global config [skills.projects.%s]", projectName)); err != nil {
				return nil, err
			}
			merged = apply(merged, projCfg)
		}
	}

	// 4. Apply local project config (team-shared, from project grove.toml, highest precedence)
	projectConfig, err := LoadSkillsFromPath(node.Path)
	if err != nil {
		return nil, err
	}
	if err := validateSeedScope(projectConfig, filepath.Join(node.Path, "grove.toml")); err != nil {
		return nil, err
	}
	merged = apply(merged, projectConfig)

	// Every configured layer was scoped away from this workspace: return an
	// empty (not nil) config so callers can report why nothing was seeded.
	if merged == nil && scopedOut {
		merged = &SkillsConfig{}
	}
	if merged != nil {
		merged.ScopedOut = scopedOut
	}

	return applySkillsDefaults(merged), nil
}

// LoadGlobalSkillsConfig loads only the global skills configuration.
// This is useful when not in a workspace context.
func LoadGlobalSkillsConfig(cfg *coreconfig.Config) *SkillsConfig {
	return applySkillsDefaults(loadSkillsFromGlobalConfig(cfg))
}

// loadSkillsFromGlobalConfig extracts [skills] from the core config's raw data.
// Uses UnmarshalExtension to safely decode nested projects/ecosystems maps.
func loadSkillsFromGlobalConfig(cfg *coreconfig.Config) *SkillsConfig {
	if cfg == nil || cfg.Extensions == nil {
		return nil
	}

	var result SkillsConfig
	if err := cfg.UnmarshalExtension("skills", &result); err != nil {
		return nil
	}

	// Return nil if nothing was configured
	if len(result.Use) == 0 && len(result.Providers) == 0 &&
		len(result.Dependencies) == 0 && len(result.Projects) == 0 &&
		len(result.Ecosystems) == 0 {
		return nil
	}

	return &result
}

// applySkillsDefaults applies default values to a SkillsConfig.
func applySkillsDefaults(cfg *SkillsConfig) *SkillsConfig {
	if cfg == nil {
		return nil
	}

	if len(cfg.Providers) == 0 {
		cfg.Providers = []string{"claude"}
	}
	cfg.Use = deduplicateStrings(cfg.Use)

	return cfg
}

// LoadSkillsFromPath reads the [skills] block from grove.toml at the given path.
func LoadSkillsFromPath(dir string) (*SkillsConfig, error) {
	tomlPath := filepath.Join(dir, "grove.toml")
	data, err := os.ReadFile(tomlPath) //nolint:gosec // G304: path constructed from workspace directory
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var parsed groveTomlSkills
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	return parsed.Skills, nil
}

// mergeSkillsConfig merges ecosystem and project configs.
// Project config takes precedence for dependencies, but Use arrays are unioned.
// Scope is deliberately not carried into the result: it is a directive about
// where a single layer's skills are seeded, consumed by LoadSkillsConfig
// before the merge, so the effective config never has one.
func mergeSkillsConfig(ecosystem, project *SkillsConfig) *SkillsConfig {
	// If both are nil, return nil
	if ecosystem == nil && project == nil {
		return nil
	}

	// If only one exists, return a copy of it
	if ecosystem == nil {
		return copySkillsConfig(project)
	}
	if project == nil {
		return copySkillsConfig(ecosystem)
	}

	// Merge both configs
	merged := &SkillsConfig{
		// Union the Use arrays
		Use: unionStrings(ecosystem.Use, project.Use),

		// Project providers override ecosystem providers if specified
		Providers: project.Providers,

		// Deep merge dependencies (project overrides ecosystem)
		Dependencies: make(map[string]DependencyConfig),
	}

	// If project didn't specify providers, use ecosystem's
	if len(merged.Providers) == 0 {
		merged.Providers = ecosystem.Providers
	}

	// Copy ecosystem dependencies first
	for k, v := range ecosystem.Dependencies {
		merged.Dependencies[k] = v
	}
	// Project dependencies override
	for k, v := range project.Dependencies {
		merged.Dependencies[k] = v
	}

	return merged
}

// copySkillsConfig creates a deep copy of a SkillsConfig's effective fields.
// Projects/Ecosystems (looked up from the original global config) and Scope
// (consumed during the merge) are intentionally not carried over.
func copySkillsConfig(cfg *SkillsConfig) *SkillsConfig {
	if cfg == nil {
		return nil
	}

	copied := &SkillsConfig{
		Use:          make([]string, len(cfg.Use)),
		Providers:    make([]string, len(cfg.Providers)),
		Dependencies: make(map[string]DependencyConfig),
	}

	copy(copied.Use, cfg.Use)
	copy(copied.Providers, cfg.Providers)

	for k, v := range cfg.Dependencies {
		copied.Dependencies[k] = v
	}

	return copied
}

// unionStrings returns the union of two string slices, preserving order.
func unionStrings(a, b []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}

// deduplicateStrings removes duplicates from a string slice while preserving order.
func deduplicateStrings(input []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, s := range input {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	return result
}
