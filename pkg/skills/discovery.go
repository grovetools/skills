package skills

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/skills/pkg/service"
)

// WorkspaceSkill represents a skill discovered from a workspace's notebook.
type WorkspaceSkill struct {
	// Name is the skill name (leaf directory name containing SKILL.md).
	Name string

	// Workspace is the name of the workspace this skill belongs to.
	Workspace string

	// QualifiedName is "workspace:skill-name" for cross-workspace reference.
	QualifiedName string

	// Path is the absolute path to the skill directory.
	Path string

	// RelPath is the nested path relative to the skills directory root (e.g. "kitchen/prep").
	RelPath string

	// Description is extracted from SKILL.md frontmatter.
	Description string
}

// ListAllWorkspaceSkills discovers skills from all registered workspaces.
func ListAllWorkspaceSkills(svc *service.Service) ([]WorkspaceSkill, error) {
	if svc == nil || svc.Provider == nil {
		return nil, nil
	}

	var allSkills []WorkspaceSkill
	seenPaths := make(map[string]bool)

	for _, ws := range svc.Provider.All() {
		skills, err := listSkillsForWorkspace(svc, ws, seenPaths)
		if err != nil {
			continue
		}
		allSkills = append(allSkills, skills...)
	}

	return allSkills, nil
}

// ListEcosystemSkills discovers skills from all workspaces in the current ecosystem.
func ListEcosystemSkills(svc *service.Service, currentNode *workspace.WorkspaceNode) ([]WorkspaceSkill, error) {
	if svc == nil || svc.Provider == nil || currentNode == nil {
		return nil, nil
	}

	var ecosystemRootPath string

	switch currentNode.Kind {
	case workspace.KindEcosystemRoot:
		ecosystemRootPath = currentNode.Path
	case workspace.KindEcosystemWorktree,
		workspace.KindEcosystemSubProject,
		workspace.KindEcosystemSubProjectWorktree,
		workspace.KindEcosystemWorktreeSubProject,
		workspace.KindEcosystemWorktreeSubProjectWorktree:
		if currentNode.RootEcosystemPath != "" {
			ecosystemRootPath = currentNode.RootEcosystemPath
		} else if currentNode.ParentEcosystemPath != "" {
			ecosystemRootPath = currentNode.ParentEcosystemPath
		}
	default:
		return listSkillsForWorkspace(svc, currentNode, nil)
	}

	if ecosystemRootPath == "" {
		return listSkillsForWorkspace(svc, currentNode, nil)
	}

	var allSkills []WorkspaceSkill
	seenPaths := make(map[string]bool)

	for _, ws := range svc.Provider.All() {
		isInEcosystem := ws.Path == ecosystemRootPath ||
			ws.RootEcosystemPath == ecosystemRootPath ||
			ws.ParentEcosystemPath == ecosystemRootPath

		if !isInEcosystem {
			continue
		}

		skills, err := listSkillsForWorkspace(svc, ws, seenPaths)
		if err != nil {
			continue
		}
		allSkills = append(allSkills, skills...)
	}

	return allSkills, nil
}

// listSkillsForWorkspace lists skills from a single workspace's notebook.
// Uses recursive WalkDir to discover nested skills. Skill name is the leaf directory
// containing SKILL.md. Directories without SKILL.md are organizational folders.
func listSkillsForWorkspace(svc *service.Service, ws *workspace.WorkspaceNode, seenPaths map[string]bool) ([]WorkspaceSkill, error) {
	if svc.NotebookLocator == nil {
		return nil, nil
	}

	skillsDir, err := svc.NotebookLocator.GetSkillsDir(ws)
	if err != nil || skillsDir == "" {
		return nil, nil
	}

	if seenPaths != nil {
		if seenPaths[skillsDir] {
			return nil, nil
		}
		seenPaths[skillsDir] = true
	}

	var skills []WorkspaceSkill
	workspaceName := ws.Name

	_ = filepath.WalkDir(skillsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}

		skillPath := filepath.Dir(path)
		relPath, _ := filepath.Rel(skillsDir, skillPath)
		if strings.Count(filepath.ToSlash(relPath), "/") > 4 {
			return nil
		}

		// Skill name is the leaf directory
		skillName := filepath.Base(skillPath)

		description := ""
		if content, err := os.ReadFile(path); err == nil { //nolint:gosec // G304: path from WalkDir
			if meta, err := ParseSkillFrontmatter(content); err == nil {
				description = meta.Description
			}
		}

		skills = append(skills, WorkspaceSkill{
			Name:          skillName,
			Workspace:     workspaceName,
			QualifiedName: workspaceName + ":" + skillName,
			Path:          skillPath,
			RelPath:       relPath,
			Description:   description,
		})

		return nil
	})

	return skills, nil
}

// ResolveQualifiedSkillName resolves a workspace:skill-name reference.
// Returns the workspace name and skill name. If no workspace is specified,
// returns empty workspace (meaning use default precedence).
func ResolveQualifiedSkillName(name string) (workspace string, skillName string) {
	parts := strings.SplitN(name, ":", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", name
}

// FindSkillAcrossWorkspaces looks for a skill by qualified name across all workspaces.
func FindSkillAcrossWorkspaces(svc *service.Service, qualifiedName string) (*WorkspaceSkill, error) {
	workspaceName, skillName := ResolveQualifiedSkillName(qualifiedName)

	allSkills, err := ListAllWorkspaceSkills(svc)
	if err != nil {
		return nil, err
	}

	for _, skill := range allSkills {
		if workspaceName != "" {
			if skill.Workspace == workspaceName && skill.Name == skillName {
				return &skill, nil
			}
		} else {
			if skill.Name == skillName {
				return &skill, nil
			}
		}
	}

	return nil, nil
}
