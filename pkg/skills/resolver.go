package skills

import (
	"fmt"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/skills/pkg/service"
)

// ResolvedSkill represents a skill that has been resolved to a physical location.
type ResolvedSkill struct {
	// Name is the skill name.
	Name string

	// SourceType indicates where the skill was resolved from.
	SourceType SourceType

	// PhysicalPath is the path to the skill directory (or "(builtin)" for embedded).
	PhysicalPath string

	// Providers lists which agent providers should receive this skill.
	Providers []string
}

// ResolveConfiguredSkills resolves all skills declared in the configuration.
// It returns a map of skill names to their resolved information.
// Returns an error if any declared skill cannot be resolved.
// Supports workspace-qualified names like "grovetools:concept-maintainer".
func ResolveConfiguredSkills(svc *service.Service, node *workspace.WorkspaceNode, cfg *SkillsConfig) (map[string]ResolvedSkill, error) {
	if cfg == nil {
		return nil, nil
	}

	// Get all available skill sources (local precedence)
	availableSources := ListSkillSources(svc, node)

	// Determine default providers
	defaultProviders := cfg.Providers
	if len(defaultProviders) == 0 {
		defaultProviders = []string{"claude"}
	}

	resolved := make(map[string]ResolvedSkill)

	// Process all skills in the Use array
	for _, skillName := range cfg.Use {
		targetProviders := defaultProviders
		expectedSource := ""
		resolveName := skillName

		// Check for explicit dependency configuration
		if dep, exists := cfg.Dependencies[skillName]; exists {
			if len(dep.Providers) > 0 {
				targetProviders = dep.Providers
			}
			if dep.Source != "" {
				expectedSource = dep.Source
			}
			if dep.Name != "" {
				resolveName = dep.Name
			}
		}

		// Check if this is a workspace-qualified name (e.g., "grovetools:concept-maintainer")
		wsName, unqualifiedName := ResolveQualifiedSkillName(resolveName)

		var src SkillSource
		var found bool

		if wsName != "" {
			// Look up across all workspaces
			skill, err := FindSkillAcrossWorkspaces(svc, resolveName)
			if err != nil || skill == nil {
				return nil, fmt.Errorf("skill '%s' declared in config but not found in workspace '%s'", skillName, wsName)
			}
			src = SkillSource{
				Path: skill.Path,
				Type: SourceTypeEcosystem, // Workspace skills are treated as ecosystem type
			}
			found = true
			// Use unqualified name for the resolved skill
			resolveName = unqualifiedName
		} else {
			// Look up in local sources first
			src, found = availableSources[resolveName]
		}

		if !found {
			return nil, fmt.Errorf("skill '%s' declared in config but not found in any source", skillName)
		}

		// If a specific source was requested, verify it matches
		if expectedSource != "" && wsName == "" {
			expectedType := sourceStringToType(expectedSource)
			if expectedType != "" && src.Type != expectedType {
				// Try to find the skill specifically from the requested source
				sourceFound := false
				for name, s := range availableSources {
					if name == resolveName && s.Type == expectedType {
						src = s
						sourceFound = true
						break
					}
				}
				if !sourceFound {
					return nil, fmt.Errorf("skill '%s' requested from source '%s' but found in '%s'",
						skillName, expectedSource, src.Type)
				}
			}
		}

		// Use unqualified name as the key (the name that will be used in .claude/skills/)
		resolved[unqualifiedName] = ResolvedSkill{
			Name:         unqualifiedName,
			SourceType:   src.Type,
			PhysicalPath: src.Path,
			Providers:    targetProviders,
		}
	}

	// Also process skills that are only in dependencies (not in Use array)
	for skillName, dep := range cfg.Dependencies {
		// Skip if already processed via Use array
		_, wsName := ResolveQualifiedSkillName(skillName)
		if _, exists := resolved[wsName]; exists {
			continue
		}

		targetProviders := defaultProviders
		if len(dep.Providers) > 0 {
			targetProviders = dep.Providers
		}

		resolveName := skillName
		if dep.Name != "" {
			resolveName = dep.Name
		}

		// Check if this is a workspace-qualified name
		qualWs, unqualifiedName := ResolveQualifiedSkillName(resolveName)

		var src SkillSource
		var found bool

		if qualWs != "" {
			skill, err := FindSkillAcrossWorkspaces(svc, resolveName)
			if err != nil || skill == nil {
				return nil, fmt.Errorf("skill '%s' declared in dependencies but not found in workspace '%s'", skillName, qualWs)
			}
			src = SkillSource{
				Path: skill.Path,
				Type: SourceTypeEcosystem,
			}
			found = true
			resolveName = unqualifiedName
		} else {
			src, found = availableSources[resolveName]
		}

		if !found {
			return nil, fmt.Errorf("skill '%s' declared in dependencies but not found in any source", skillName)
		}

		// If a specific source was requested, verify it matches
		if dep.Source != "" && qualWs == "" {
			expectedType := sourceStringToType(dep.Source)
			if expectedType != "" && src.Type != expectedType {
				return nil, fmt.Errorf("skill '%s' requested from source '%s' but found in '%s'",
					skillName, dep.Source, src.Type)
			}
		}

		resolved[unqualifiedName] = ResolvedSkill{
			Name:         unqualifiedName,
			SourceType:   src.Type,
			PhysicalPath: src.Path,
			Providers:    targetProviders,
		}
	}

	return resolved, nil
}

// sourceStringToType converts a source string from config to SourceType.
func sourceStringToType(s string) SourceType {
	switch s {
	case "builtin":
		return SourceTypeBuiltin
	case "user":
		return SourceTypeUser
	case "ecosystem":
		return SourceTypeEcosystem
	case "project":
		return SourceTypeProject
	case "notebook":
		// "notebook" maps to either ecosystem or project, we'll accept either
		return ""
	default:
		return ""
	}
}

// GetAllDeclaredSkillNames returns all skill names declared in the config.
func GetAllDeclaredSkillNames(cfg *SkillsConfig) []string {
	if cfg == nil {
		return nil
	}

	seen := make(map[string]bool)
	var names []string

	for _, name := range cfg.Use {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	for name := range cfg.Dependencies {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	return names
}
