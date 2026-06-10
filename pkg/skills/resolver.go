package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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

	// RelPath is the nested path relative to skills dir (e.g. "sear/heat-pan").
	RelPath string

	// Providers lists which agent providers should receive this skill.
	Providers []string
}

// ExpandUseWithPlaybookSkills returns the input skill-use list with any
// skills owned by authorized playbooks ([playbooks] use in grove.toml)
// appended. This is how a playbook-owned skill becomes "configured" for
// the workspace without requiring a redundant [skills] use entry.
func ExpandUseWithPlaybookSkills(node *workspace.WorkspaceNode, use []string) []string {
	result := append([]string(nil), use...)
	if node == nil {
		return result
	}
	pbCfg, err := LoadPlaybooksFromPath(node.Path)
	if err != nil || pbCfg == nil || len(pbCfg.Use) == 0 {
		return result
	}
	seen := make(map[string]bool, len(result))
	for _, name := range result {
		seen[name] = true
	}
	for _, pbName := range pbCfg.Use {
		pb, err := LoadPlaybook(node.Path, pbName)
		if err != nil || pb == nil {
			continue
		}
		for _, s := range pb.Skills {
			if s.Name == "" || seen[s.Name] {
				continue
			}
			seen[s.Name] = true
			result = append(result, s.Name)
		}
	}
	return result
}

// ResolveConfiguredSkills resolves all skills declared in the configuration.
// It also recursively traverses SKILL.md dependencies to implicitly resolve
// nested sub-skills (via skill_sequence and requires).
//
// Skills that cannot be found in any source are returned in the missing
// slice rather than aborting resolution — config layers (global, ecosystem,
// project) accumulate stale entries over time, and one dead entry must not
// hold every other skill hostage. Genuine config contradictions (circular
// dependencies, source pin mismatches) still return an error.
func ResolveConfiguredSkills(svc *service.Service, node *workspace.WorkspaceNode, cfg *SkillsConfig) (resolved map[string]ResolvedSkill, missing []string, err error) {
	if cfg == nil {
		return nil, nil, nil
	}

	availableSources := ListSkillSources(svc, node)
	defaultProviders := cfg.Providers
	if len(defaultProviders) == 0 {
		defaultProviders = []string{"claude"}
	}

	// Implicitly authorize skills owned by authorized playbooks. A
	// playbook listed in [playbooks] use contributes all of its
	// playbook-owned skills to the resolver's "use" set so they sync
	// without requiring redundant per-skill entries in [skills] use.
	useWithPlaybooks := ExpandUseWithPlaybookSkills(node, cfg.Use)

	resolved = make(map[string]ResolvedSkill)
	inProgress := make(map[string]bool)
	missingSeen := make(map[string]bool)
	recordMissing := func(name string) {
		if !missingSeen[name] {
			missingSeen[name] = true
			missing = append(missing, name)
		}
	}

	var resolveTransitive func(skillName string, targetProviders []string, expectedSource string) error
	resolveTransitive = func(skillName string, targetProviders []string, expectedSource string) error {
		// Detect circular dependencies
		if inProgress[skillName] {
			return fmt.Errorf("circular skill sequence dependency detected: %s", skillName)
		}
		if _, exists := resolved[skillName]; exists {
			return nil // Already resolved
		}

		inProgress[skillName] = true
		defer func() { delete(inProgress, skillName) }()

		resolveName := skillName
		depProviders := targetProviders
		depSource := expectedSource

		if dep, exists := cfg.Dependencies[skillName]; exists {
			if len(dep.Providers) > 0 {
				depProviders = dep.Providers
			}
			if dep.Source != "" {
				depSource = dep.Source
			}
			if dep.Name != "" {
				resolveName = dep.Name
			}
		}

		wsName, unqualifiedName := ResolveQualifiedSkillName(resolveName)
		var src SkillSource
		var found bool

		if wsName != "" {
			skill, err := FindSkillAcrossWorkspaces(svc, resolveName)
			if err != nil || skill == nil {
				recordMissing(skillName)
				return nil
			}
			src = SkillSource{
				Path:    skill.Path,
				RelPath: skill.RelPath,
				Type:    SourceTypeEcosystem,
			}
			found = true
			resolveName = unqualifiedName
		} else {
			src, found = availableSources[resolveName]
		}

		if !found {
			recordMissing(skillName)
			return nil
		}

		if depSource != "" && wsName == "" {
			expectedType := sourceStringToType(depSource)
			if expectedType != "" && src.Type != expectedType {
				sourceFound := false
				for name, s := range availableSources {
					if name == resolveName && s.Type == expectedType {
						src = s
						sourceFound = true
						break
					}
				}
				if !sourceFound {
					return fmt.Errorf("skill '%s' requested from source '%s' but found in '%s'",
						skillName, depSource, src.Type)
				}
			}
		}

		resolved[unqualifiedName] = ResolvedSkill{
			Name:         unqualifiedName,
			SourceType:   src.Type,
			PhysicalPath: src.Path,
			RelPath:      src.RelPath,
			Providers:    depProviders,
		}

		// Read SKILL.md to recursively resolve implicit dependencies (skill_sequence, requires)
		var content []byte
		var err error
		if src.Type == SourceTypeBuiltin {
			content, err = fs.ReadFile(embeddedSkillsFS, filepath.Join("data/skills", src.RelPath, "SKILL.md"))
		} else {
			content, err = os.ReadFile(filepath.Join(src.Path, "SKILL.md"))
		}

		if err == nil {
			if meta, err := ParseSkillFrontmatter(content); err == nil {
				for _, req := range meta.Requires {
					if err := resolveTransitive(req, depProviders, ""); err != nil {
						return err
					}
				}
				for _, seq := range meta.SkillSequence {
					if err := resolveTransitive(seq, depProviders, ""); err != nil {
						return err
					}
				}
			}
		}

		return nil
	}

	for _, skillName := range useWithPlaybooks {
		if err := resolveTransitive(skillName, defaultProviders, ""); err != nil {
			return nil, missing, err
		}
	}

	for skillName := range cfg.Dependencies {
		_, unqualifiedName := ResolveQualifiedSkillName(skillName)
		if _, exists := resolved[unqualifiedName]; !exists {
			if err := resolveTransitive(skillName, defaultProviders, ""); err != nil {
				return nil, missing, err
			}
		}
	}

	return resolved, missing, nil
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
