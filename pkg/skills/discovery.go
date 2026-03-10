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
	// Name is the skill name (directory name).
	Name string

	// Workspace is the name of the workspace this skill belongs to.
	Workspace string

	// QualifiedName is "workspace:skill-name" for cross-workspace reference.
	QualifiedName string

	// Path is the absolute path to the skill directory.
	Path string

	// Description is extracted from SKILL.md frontmatter.
	Description string
}

// ListAllWorkspaceSkills discovers skills from all registered workspaces.
// This is similar to `nb concept list --all-workspaces`.
func ListAllWorkspaceSkills(svc *service.Service) ([]WorkspaceSkill, error) {
	if svc == nil || svc.Provider == nil {
		return nil, nil
	}

	var allSkills []WorkspaceSkill
	seenPaths := make(map[string]bool)

	allWorkspaces := svc.Provider.All()
	for _, ws := range allWorkspaces {
		skills, err := listSkillsForWorkspace(svc, ws, seenPaths)
		if err != nil {
			continue // Skip workspaces we can't read
		}
		allSkills = append(allSkills, skills...)
	}

	return allSkills, nil
}

// ListEcosystemSkills discovers skills from all workspaces in the current ecosystem.
// This is similar to `nb concept list --ecosystem`.
func ListEcosystemSkills(svc *service.Service, currentNode *workspace.WorkspaceNode) ([]WorkspaceSkill, error) {
	if svc == nil || svc.Provider == nil || currentNode == nil {
		return nil, nil
	}

	// Determine the ecosystem root path
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
		// Not in an ecosystem - return skills from current workspace only
		return listSkillsForWorkspace(svc, currentNode, nil)
	}

	if ecosystemRootPath == "" {
		return listSkillsForWorkspace(svc, currentNode, nil)
	}

	var allSkills []WorkspaceSkill
	seenPaths := make(map[string]bool)

	// Find all workspaces that belong to this ecosystem
	allWorkspaces := svc.Provider.All()
	for _, ws := range allWorkspaces {
		// Check if this workspace is part of the ecosystem
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
func listSkillsForWorkspace(svc *service.Service, ws *workspace.WorkspaceNode, seenPaths map[string]bool) ([]WorkspaceSkill, error) {
	if svc.NotebookLocator == nil {
		return nil, nil
	}

	// Get skills directory for this workspace
	skillsDir, err := svc.NotebookLocator.GetSkillsDir(ws)
	if err != nil || skillsDir == "" {
		return nil, nil
	}

	// Skip if we've already processed this directory
	if seenPaths != nil {
		if seenPaths[skillsDir] {
			return nil, nil
		}
		seenPaths[skillsDir] = true
	}

	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, err
	}

	var skills []WorkspaceSkill
	workspaceName := ws.Name

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillName := entry.Name()
		skillPath := filepath.Join(skillsDir, skillName)

		// Try to get description from SKILL.md
		description := ""
		skillMDPath := filepath.Join(skillPath, "SKILL.md")
		if content, err := os.ReadFile(skillMDPath); err == nil {
			if meta, err := ParseSkillFrontmatter(content); err == nil {
				description = meta.Description
			}
		}

		skills = append(skills, WorkspaceSkill{
			Name:          skillName,
			Workspace:     workspaceName,
			QualifiedName: workspaceName + ":" + skillName,
			Path:          skillPath,
			Description:   description,
		})
	}

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
		// If workspace is specified, match exactly
		if workspaceName != "" {
			if skill.Workspace == workspaceName && skill.Name == skillName {
				return &skill, nil
			}
		} else {
			// If no workspace specified, match just by name
			if skill.Name == skillName {
				return &skill, nil
			}
		}
	}

	return nil, nil
}
