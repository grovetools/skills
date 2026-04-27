package skills

import (
	"os"
	"path/filepath"

	coreconfig "github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/pelletier/go-toml/v2"
)

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
func LoadSkillsConfig(cfg *coreconfig.Config, node *workspace.WorkspaceNode) (*SkillsConfig, error) {
	// Load global config first (contains both base skills and user-scoped overrides)
	globalConfig := loadSkillsFromGlobalConfig(cfg)

	// If no node, just return base global config (without project/ecosystem scopes)
	if node == nil {
		return applySkillsDefaults(copySkillsConfig(globalConfig)), nil
	}

	// Start with a copy of the base global config
	merged := copySkillsConfig(globalConfig)

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
			merged = mergeSkillsConfig(merged, ecoCfg)
		}
	}

	// 2. Apply local ecosystem config (team-shared, from ecosystem grove.toml)
	if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
		ecosystemConfig, err := LoadSkillsFromPath(node.RootEcosystemPath)
		if err != nil {
			return nil, err
		}
		merged = mergeSkillsConfig(merged, ecosystemConfig)
	}

	// 3. Apply global project overrides (user-scoped, from ~/.config/grove/grove.toml)
	// Use repository name, not worktree name
	if globalConfig != nil && globalConfig.Projects != nil {
		projectName := node.Name
		if node.ParentProjectPath != "" {
			projectName = filepath.Base(node.ParentProjectPath)
		}
		if projCfg, ok := globalConfig.Projects[projectName]; ok {
			merged = mergeSkillsConfig(merged, projCfg)
		}
	}

	// 4. Apply local project config (team-shared, from project grove.toml, highest precedence)
	projectConfig, err := LoadSkillsFromPath(node.Path)
	if err != nil {
		return nil, err
	}
	merged = mergeSkillsConfig(merged, projectConfig)

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

// copySkillsConfig creates a deep copy of a SkillsConfig.
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
