package skills

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/fs"
	"github.com/grovetools/core/logging"
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

	// 1. User skills
	userSkillsPath := getUserSkillsPathWithConfig(svc)
	if userSkillsPath != "" {
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
	userSkillsPath := getUserSkillsPathWithConfig(svc)
	if userSkillsPath != "" {
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
		Config:          cfg,
	}, nil
}

// SyncConfiguredSkills syncs resolved skills to their target provider directories.
// It writes skills to the provider-specific directory within the git root.
// If prune is true, skills not in the resolved map are removed from the destination.
func SyncConfiguredSkills(gitRoot string, resolved map[string]ResolvedSkill, prune bool, logger *logging.PrettyLogger) (int, error) {
	syncedCount := 0
	var lastErr error

	// Track what we installed per provider for pruning
	installedPerProvider := make(map[string]map[string]bool)

	// Sync each skill to its target providers
	for skillName, r := range resolved {
		for _, provider := range r.Providers {
			destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
			destPath := filepath.Join(destBaseDir, skillName)

			if installedPerProvider[provider] == nil {
				installedPerProvider[provider] = make(map[string]bool)
			}
			installedPerProvider[provider][skillName] = true

			// Ensure base directory exists
			if err := os.MkdirAll(destBaseDir, 0755); err != nil {
				lastErr = fmt.Errorf("failed to create directory %s: %w", destBaseDir, err)
				continue
			}

			// Remove existing skill directory before writing
			os.RemoveAll(destPath)

			// Handle builtin vs local skills
			if r.SourceType == SourceTypeBuiltin {
				// Extract from embedded FS
				files, err := readSkillFromFS(embeddedSkillsFS, skillName)
				if err != nil {
					lastErr = err
					continue
				}

				if err := os.MkdirAll(destPath, 0755); err != nil {
					lastErr = err
					continue
				}

				for relPath, content := range files {
					filePath := filepath.Join(destPath, relPath)
					if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
						lastErr = err
						continue
					}
					if err := os.WriteFile(filePath, content, 0644); err != nil {
						lastErr = err
						continue
					}
				}
				syncedCount++
			} else {
				// Copy from local filesystem
				if err := fs.CopyDir(r.PhysicalPath, destPath); err != nil {
					lastErr = fmt.Errorf("failed to copy skill %s: %w", skillName, err)
				} else {
					syncedCount++
				}
			}
		}
	}

	// Prune skills not in config if requested
	if prune {
		for provider, validSkills := range installedPerProvider {
			destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
			entries, err := os.ReadDir(destBaseDir)
			if err != nil {
				continue
			}

			for _, entry := range entries {
				if entry.IsDir() && !validSkills[entry.Name()] {
					pathToRemove := filepath.Join(destBaseDir, entry.Name())
					if err := os.RemoveAll(pathToRemove); err != nil {
						if logger != nil {
							logger.WarnPretty(fmt.Sprintf("Failed to prune skill '%s': %v", entry.Name(), err))
						}
					} else {
						if logger != nil {
							logger.InfoPretty(fmt.Sprintf("Pruned unconfigured skill: %s (provider: %s)", entry.Name(), provider))
						}
					}
				}
			}
		}
	}

	return syncedCount, lastErr
}
