package skills

import (
	"fmt"
	"slices"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/skills/pkg/service"
)

// LoadedSkill represents a skill whose files have been loaded into memory,
// along with its provenance (where it came from).
type LoadedSkill struct {
	Name         string
	SourceType   SourceType
	PhysicalPath string
	Files        map[string][]byte
}

// ErrSkillNotAuthorized is returned when a skill exists but is not declared in grove.toml.
type ErrSkillNotAuthorized struct {
	SkillName string
	WorkDir   string
}

func (e *ErrSkillNotAuthorized) Error() string {
	return fmt.Sprintf("skill '%s' is not authorized in workspace '%s' (add to grove.toml [skills] use)", e.SkillName, e.WorkDir)
}

// LoadAuthorizedSkill resolves a skill and ensures the workspace has explicitly declared it
// in grove.toml (via [skills] use or [skills.dependencies]).
func LoadAuthorizedSkill(workDir, skillName string) (*LoadedSkill, error) {
	node, _ := workspace.GetProjectByPath(workDir)
	var svc *service.Service
	if node != nil {
		var err error
		svc, err = NewServiceForNode(node)
		if err != nil {
			return nil, fmt.Errorf("failed to create service for node: %w", err)
		}
	}

	// Enforce access control when we have a workspace context
	if svc != nil {
		cfg, err := LoadSkillsConfig(svc.Config, node)
		if err != nil {
			return nil, fmt.Errorf("failed to load skills config: %w", err)
		}

		authorized := false
		if cfg != nil {
			// Expand [skills] use with skills owned by authorized
			// playbooks ([playbooks] use). A playbook-owned skill is
			// implicitly authorized without requiring a redundant
			// entry in [skills] use. This mirrors the sync-path
			// behavior in ResolveConfiguredSkills.
			useWithPlaybooks := ExpandUseWithPlaybookSkills(node, cfg.Use)
			if slices.Contains(useWithPlaybooks, skillName) {
				authorized = true
			}
			if !authorized {
				if _, exists := cfg.Dependencies[skillName]; exists {
					authorized = true
				}
			}
		}

		// If not directly authorized, check if any authorized skill transitively
		// includes this skill via its skill_sequence (implicit authorization).
		if !authorized {
			authorized = isTransitivelyAuthorized(svc, node, skillName, cfg)
		}

		if !authorized {
			return nil, &ErrSkillNotAuthorized{SkillName: skillName, WorkDir: workDir}
		}
	}

	return loadSkillInternal(svc, node, skillName)
}

// LoadSkillBypassingAccess resolves a skill without checking grove.toml authorization.
// Useful for CLI inspection commands (show, tree, search).
func LoadSkillBypassingAccess(workDir, skillName string) (*LoadedSkill, error) {
	node, _ := workspace.GetProjectByPath(workDir)
	var svc *service.Service
	if node != nil {
		svc, _ = NewServiceForNode(node)
	}
	return loadSkillInternal(svc, node, skillName)
}

// LoadSkillBypassingAccessWithService is a helper for CLI commands that already have a service.
func LoadSkillBypassingAccessWithService(svc *service.Service, node *workspace.WorkspaceNode, skillName string) (*LoadedSkill, error) {
	return loadSkillInternal(svc, node, skillName)
}

// LoadSkillFromSource loads the files for a skill given its resolved SkillSource.
// Uses RelPath for builtin skills to support nested paths.
func LoadSkillFromSource(skillName string, src SkillSource) (*LoadedSkill, error) {
	_, unqualifiedName := ResolveQualifiedSkillName(skillName)

	var files map[string][]byte
	var err error

	if src.Type == SourceTypeBuiltin {
		files, err = readSkillFromFS(embeddedSkillsFS, src.RelPath)
	} else {
		files, err = readSkillFromDisk(src.Path)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to read skill files: %w", err)
	}

	return &LoadedSkill{
		Name:         unqualifiedName,
		SourceType:   src.Type,
		PhysicalPath: src.Path,
		Files:        files,
	}, nil
}

// isTransitivelyAuthorized checks if a skill is implicitly authorized via the
// skill_sequence of any directly authorized skill. This allows sub-skills
// declared in a parent's SKILL.md frontmatter to be loaded without explicit
// grove.toml authorization. The check is recursive: a sub-skill of a sub-skill
// is also considered authorized.
func isTransitivelyAuthorized(svc *service.Service, node *workspace.WorkspaceNode, targetSkill string, cfg *SkillsConfig) bool {
	if cfg == nil {
		return false
	}

	// Collect all directly authorized skill names, including skills
	// owned by authorized playbooks (see ExpandUseWithPlaybookSkills).
	authorizedNames := ExpandUseWithPlaybookSkills(node, cfg.Use)
	for name := range cfg.Dependencies {
		authorizedNames = append(authorizedNames, name)
	}

	visited := make(map[string]bool)
	for _, name := range authorizedNames {
		if checkSkillSequenceContains(svc, node, name, targetSkill, visited) {
			return true
		}
	}
	return false
}

// checkSkillSequenceContains recursively checks if a skill's skill_sequence
// (or any transitive sub-skill's sequence) contains the target skill name.
func checkSkillSequenceContains(svc *service.Service, node *workspace.WorkspaceNode, skillName, targetSkill string, visited map[string]bool) bool {
	if visited[skillName] {
		return false
	}
	visited[skillName] = true

	loaded, err := loadSkillInternal(svc, node, skillName)
	if err != nil {
		return false
	}

	content, ok := loaded.Files["SKILL.md"]
	if !ok {
		return false
	}

	meta, err := ParseSkillFrontmatter(content)
	if err != nil {
		return false
	}

	for _, seqSkill := range meta.SkillSequence {
		if seqSkill == targetSkill {
			return true
		}
		if checkSkillSequenceContains(svc, node, seqSkill, targetSkill, visited) {
			return true
		}
	}
	return false
}

// loadSkillInternal handles the actual resolution and file loading.
func loadSkillInternal(svc *service.Service, node *workspace.WorkspaceNode, skillName string) (*LoadedSkill, error) {
	wsName, unqualifiedName := ResolveQualifiedSkillName(skillName)

	var src SkillSource
	var found bool

	if wsName != "" {
		skill, err := FindSkillAcrossWorkspaces(svc, skillName)
		if err != nil {
			return nil, fmt.Errorf("failed to search workspaces: %w", err)
		}
		if skill == nil {
			return nil, fmt.Errorf("skill '%s' not found in workspace '%s'", unqualifiedName, wsName)
		}
		src = SkillSource{Path: skill.Path, RelPath: skill.RelPath, Type: SourceTypeEcosystem}
		found = true
	} else {
		sources := ListSkillSources(svc, node)
		src, found = sources[unqualifiedName]
	}

	if !found {
		return nil, fmt.Errorf("skill '%s' not found", unqualifiedName)
	}

	return LoadSkillFromSource(unqualifiedName, src)
}
