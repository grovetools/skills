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

	// UserPath overrides the default user skills directory (~/.config/grove/skills).
	// This is typically set in the global config to point to a shared skills repository.
	UserPath string `toml:"user_path" yaml:"user_path"`
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
	projectConfig, err := loadSkillsFromPath(node.Path)
	if err != nil {
		return nil, err
	}

	// Load ecosystem-level config if we're in an ecosystem
	var ecosystemConfig *SkillsConfig
	if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
		ecosystemConfig, err = loadSkillsFromPath(node.RootEcosystemPath)
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

	// The core config stores extra sections that can be accessed
	// We need to check if there's a skills section in the raw config
	// For now, use the config's GetSkillsUserPath if it has one
	// This integrates with core's config extension system

	// Try to get user_path from config extensions
	if cfg.Extensions != nil {
		if skillsExt, ok := cfg.Extensions["skills"]; ok {
			if skillsMap, ok := skillsExt.(map[string]interface{}); ok {
				result := &SkillsConfig{
					Dependencies: make(map[string]DependencyConfig),
				}

				if userPath, ok := skillsMap["user_path"].(string); ok {
					result.UserPath = expandPath(userPath)
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

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

// GetUserSkillsPath returns the effective user skills path, checking config first.
func GetUserSkillsPath(cfg *coreconfig.Config) string {
	// Check global config for user_path override
	globalCfg := LoadGlobalSkillsConfig(cfg)
	if globalCfg != nil && globalCfg.UserPath != "" {
		return globalCfg.UserPath
	}

	// Fall back to default
	path, _ := getDefaultUserSkillsPath()
	return path
}

// getDefaultUserSkillsPath returns the default user skills directory (~/.config/grove/skills).
func getDefaultUserSkillsPath() (string, error) {
	var configDir string

	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		configDir = xdgConfig
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, ".config")
	}

	return filepath.Join(configDir, "grove", "skills"), nil
}

// loadSkillsFromPath reads the [skills] block from grove.toml at the given path.
func loadSkillsFromPath(dir string) (*SkillsConfig, error) {
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

		// Project UserPath overrides ecosystem if specified
		UserPath: project.UserPath,
	}

	// If project didn't specify providers, use ecosystem's
	if len(merged.Providers) == 0 {
		merged.Providers = ecosystem.Providers
	}

	// If project didn't specify UserPath, use ecosystem's
	if merged.UserPath == "" {
		merged.UserPath = ecosystem.UserPath
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
		UserPath:     cfg.UserPath,
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
