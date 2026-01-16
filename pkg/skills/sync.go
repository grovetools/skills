package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/fs"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/skills/pkg/service"
)

// SyncSkillsToDirectory copies all discoverable skills to a destination directory.
// Skills are collected from multiple sources with the following precedence (higher wins):
//   1. Built-in/embedded skills (lowest precedence)
//   2. User skills from ~/.config/grove/skills
//   3. Ecosystem skills from the notebook (if project is part of an ecosystem)
//   4. Project skills from the notebook (highest precedence)
//
// This is useful for syncing skills to worktrees or other isolated environments.
func SyncSkillsToDirectory(svc *service.Service, node *workspace.WorkspaceNode, destDir string) (int, error) {
	if node == nil {
		return 0, fmt.Errorf("workspace node is required")
	}

	// Collect skills from all sources (lower precedence first, higher overwrites)
	// Map: skillName -> sourcePath
	skillSources := make(map[string]string)

	// 1. User skills from ~/.config/grove/skills
	userSkillsPath, err := getUserSkillsPath()
	if err == nil && userSkillsPath != "" {
		collectSkillsFromDir(userSkillsPath, skillSources)
	}

	// 2. Ecosystem skills (if project is part of an ecosystem)
	if node.RootEcosystemPath != "" {
		ecoSkillsDir := getEcosystemSkillsDir(svc, node)
		if ecoSkillsDir != "" {
			collectSkillsFromDir(ecoSkillsDir, skillSources)
		}
	}

	// 3. Project skills (highest precedence)
	projectSkillsDir := getProjectSkillsDir(svc, node)
	if projectSkillsDir != "" {
		collectSkillsFromDir(projectSkillsDir, skillSources)
	}

	if len(skillSources) == 0 {
		return 0, nil
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Copy each skill directory
	var syncedCount int
	var lastErr error
	for skillName, srcPath := range skillSources {
		destPath := filepath.Join(destDir, skillName)

		if err := fs.CopyDir(srcPath, destPath); err != nil {
			lastErr = fmt.Errorf("failed to sync skill %s: %w", skillName, err)
		} else {
			syncedCount++
		}
	}

	return syncedCount, lastErr
}

// ListSkillSources returns a map of skill names to their source paths.
// This is useful for displaying where skills come from.
// Skills are listed in precedence order (later sources override earlier):
//   1. Built-in skills (embedded in binary)
//   2. User skills (~/.config/grove/skills)
//   3. Ecosystem skills (from notebook)
//   4. Project skills (from notebook)
func ListSkillSources(svc *service.Service, node *workspace.WorkspaceNode) map[string]SkillSource {
	sources := make(map[string]SkillSource)

	// 1. Built-in skills (lowest precedence)
	addBuiltinSkillSources(sources)

	// 2. User skills
	userSkillsPath, err := getUserSkillsPath()
	if err == nil && userSkillsPath != "" {
		addSkillSources(userSkillsPath, SourceTypeUser, sources)
	}

	// 3. Ecosystem skills
	if node != nil && node.RootEcosystemPath != "" {
		ecoSkillsDir := getEcosystemSkillsDir(svc, node)
		if ecoSkillsDir != "" {
			addSkillSources(ecoSkillsDir, SourceTypeEcosystem, sources)
		}
	}

	// 4. Project skills (highest precedence)
	if node != nil {
		projectSkillsDir := getProjectSkillsDir(svc, node)
		if projectSkillsDir != "" {
			addSkillSources(projectSkillsDir, SourceTypeProject, sources)
		}
	}

	return sources
}

// addBuiltinSkillSources adds embedded/built-in skills to the sources map
func addBuiltinSkillSources(sources map[string]SkillSource) {
	entries, err := embeddedSkillsFS.ReadDir("data/skills")
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		sources[skillName] = SkillSource{
			Path: "(builtin)",
			Type: SourceTypeBuiltin,
		}
	}
}

// SourceType indicates where a skill comes from
type SourceType string

const (
	SourceTypeBuiltin   SourceType = "builtin"
	SourceTypeUser      SourceType = "user"
	SourceTypeEcosystem SourceType = "ecosystem"
	SourceTypeProject   SourceType = "project"
)

// SkillSource represents a skill's origin
type SkillSource struct {
	Path string
	Type SourceType
}

// getEcosystemSkillsDir returns the skills directory for the ecosystem containing the node
func getEcosystemSkillsDir(svc *service.Service, node *workspace.WorkspaceNode) string {
	if svc == nil || svc.NotebookLocator == nil || node.RootEcosystemPath == "" {
		return ""
	}

	// Create a synthetic node for the ecosystem
	ecoNode := &workspace.WorkspaceNode{
		Name:         filepath.Base(node.RootEcosystemPath),
		Path:         node.RootEcosystemPath,
		NotebookName: node.NotebookName,
	}

	skillsDir, err := svc.NotebookLocator.GetSkillsDir(ecoNode)
	if err != nil {
		return ""
	}

	// Verify directory exists
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return ""
	}

	return skillsDir
}

// getProjectSkillsDir returns the skills directory for the project
func getProjectSkillsDir(svc *service.Service, node *workspace.WorkspaceNode) string {
	if svc == nil || svc.NotebookLocator == nil {
		return ""
	}

	skillsDir, err := svc.NotebookLocator.GetSkillsDir(node)
	if err != nil {
		return ""
	}

	// Verify directory exists
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return ""
	}

	return skillsDir
}

// collectSkillsFromDir scans a directory for skill subdirectories and adds them to the map
func collectSkillsFromDir(dir string, skillSources map[string]string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		skillSources[skillName] = filepath.Join(dir, skillName)
	}
}

// addSkillSources adds skills from a directory to the sources map
func addSkillSources(dir string, sourceType SourceType, sources map[string]SkillSource) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skillName := entry.Name()
		sources[skillName] = SkillSource{
			Path: filepath.Join(dir, skillName),
			Type: sourceType,
		}
	}
}

// GetSkillsDirectoryForWorktree returns the standard skills directory path for a worktree.
// This is the destination path where skills should be synced to.
func GetSkillsDirectoryForWorktree(worktreePath, provider string) string {
	switch provider {
	case "codex":
		return filepath.Join(worktreePath, ".codex", "skills")
	case "opencode":
		return filepath.Join(worktreePath, ".opencode", "skill")
	default: // claude
		return filepath.Join(worktreePath, ".claude", "skills")
	}
}

// NewServiceForNode creates a minimal service for skill operations on a specific node.
// This is useful when you don't have a full service but need skill discovery.
func NewServiceForNode(node *workspace.WorkspaceNode) (*service.Service, error) {
	cfg, err := config.LoadDefault()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	locator := workspace.NewNotebookLocator(cfg)

	return &service.Service{
		NotebookLocator: locator,
	}, nil
}
