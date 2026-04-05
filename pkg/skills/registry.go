package skills

import (
	"fmt"

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
			for _, u := range cfg.Use {
				if u == skillName {
					authorized = true
					break
				}
			}
			if !authorized {
				if _, exists := cfg.Dependencies[skillName]; exists {
					authorized = true
				}
			}
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
