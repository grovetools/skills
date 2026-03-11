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
}

// groveTomlSkills is used to extract the skills block from grove.toml
type groveTomlSkills struct {
	Skills *SkillsConfig `toml:"skills"`
}

// LoadSkillsConfig extracts the skills configuration from grove.toml in the workspace.
// It handles inheritance by merging global, ecosystem, and project configurations.
// Returns nil if no [skills] block is found.
func LoadSkillsConfig(cfg *coreconfig.Config, node *workspace.WorkspaceNode) (*SkillsConfig, error) {
	// Load global config first (lowest precedence for most settings)
	globalConfig := loadSkillsFromGlobalConfig(cfg)

	// If no node, just return global config
	if node == nil {
		return applySkillsDefaults(globalConfig), nil
	}

	// Load project-level config
	projectConfig, err := LoadSkillsFromPath(node.Path)
	if err != nil {
		return nil, err
	}

	// Load ecosystem-level config if we're in an ecosystem
	var ecosystemConfig *SkillsConfig
	if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
		ecosystemConfig, err = LoadSkillsFromPath(node.RootEcosystemPath)
		if err != nil {
			return nil, err
		}
	}

	// Merge configurations: global -> ecosystem -> project (later wins)
	merged := mergeSkillsConfig(globalConfig, ecosystemConfig)
	merged = mergeSkillsConfig(merged, projectConfig)

	return applySkillsDefaults(merged), nil
}

// LoadGlobalSkillsConfig loads only the global skills configuration.
// This is useful when not in a workspace context.
func LoadGlobalSkillsConfig(cfg *coreconfig.Config) *SkillsConfig {
	return applySkillsDefaults(loadSkillsFromGlobalConfig(cfg))
}

// loadSkillsFromGlobalConfig extracts [skills] from the core config's raw data.
func loadSkillsFromGlobalConfig(cfg *coreconfig.Config) *SkillsConfig {
	if cfg == nil {
		return nil
	}

	// Try to get skills config from extensions
	if cfg.Extensions != nil {
		if skillsExt, ok := cfg.Extensions["skills"]; ok {
			if skillsMap, ok := skillsExt.(map[string]interface{}); ok {
				result := &SkillsConfig{
					Dependencies: make(map[string]DependencyConfig),
				}

				if use, ok := skillsMap["use"].([]interface{}); ok {
					for _, u := range use {
						if s, ok := u.(string); ok {
							result.Use = append(result.Use, s)
						}
					}
				}

				if providers, ok := skillsMap["providers"].([]interface{}); ok {
					for _, p := range providers {
						if s, ok := p.(string); ok {
							result.Providers = append(result.Providers, s)
						}
					}
				}

				return result
			}
		}
	}

	return nil
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
	data, err := os.ReadFile(tomlPath)
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
