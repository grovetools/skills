package skills

import (
	"os"
	"path/filepath"

	coreconfig "github.com/grovetools/core/config"
	"github.com/pelletier/go-toml/v2"
)

// PlaybooksConfig represents the [playbooks] block in grove.toml.
// A playbook is a versioned bundle of skills, prompts, recipes, and references
// that together define a coherent methodology (e.g., "gdv2").
type PlaybooksConfig struct {
	// Use lists the playbooks authorized for this workspace. Playbooks listed here
	// contribute their skills to the skill resolver and their recipes to the flow
	// recipe resolver.
	Use []string `toml:"use" yaml:"use"`
}

// groveTomlPlaybooks is used to extract the [playbooks] block from grove.toml
type groveTomlPlaybooks struct {
	Playbooks *PlaybooksConfig `toml:"playbooks"`
}

// LoadPlaybooksConfig extracts the playbooks configuration from grove.toml in the
// workspace. Missing [playbooks] section yields an empty (non-nil) config.
func LoadPlaybooksConfig(cfg *coreconfig.Config) *PlaybooksConfig {
	if cfg == nil || cfg.Extensions == nil {
		return &PlaybooksConfig{}
	}
	var result PlaybooksConfig
	if err := cfg.UnmarshalExtension("playbooks", &result); err != nil {
		return &PlaybooksConfig{}
	}
	return &result
}

// LoadPlaybooksFromPath reads the [playbooks] block from grove.toml at the given path.
// Returns a non-nil empty config if grove.toml doesn't exist or has no [playbooks] block.
func LoadPlaybooksFromPath(dir string) (*PlaybooksConfig, error) {
	tomlPath := filepath.Join(dir, "grove.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &PlaybooksConfig{}, nil
		}
		return nil, err
	}

	var parsed groveTomlPlaybooks
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	if parsed.Playbooks == nil {
		return &PlaybooksConfig{}, nil
	}
	return parsed.Playbooks, nil
}
