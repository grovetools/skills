package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/grovetools/core/config"
	corefs "github.com/grovetools/core/fs"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/util/pathutil"
	"github.com/grovetools/skills/pkg/service"
)

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
	Path    string
	RelPath string // Path relative to the root of the skills directory (e.g. "sear/heat-pan")
	Type    SourceType
}

// addSkillSourceSafely adds a skill source, handling duplicates by preferring the shallowest path
// within the same source type. Across different source types, later calls overwrite earlier ones
// (callers are responsible for calling in precedence order).
func addSkillSourceSafely(sources map[string]SkillSource, name string, newSource SkillSource) {
	existing, ok := sources[name]
	if !ok {
		sources[name] = newSource
		return
	}
	if newSource.Type == existing.Type {
		newDepth := strings.Count(filepath.ToSlash(newSource.RelPath), "/")
		existingDepth := strings.Count(filepath.ToSlash(existing.RelPath), "/")
		if newDepth < existingDepth {
			sources[name] = newSource
		}
	} else {
		// Later source types overwrite earlier ones (called in precedence order)
		sources[name] = newSource
	}
}

// SyncSkillsToDirectory copies all discoverable skills to a destination directory.
// Skills are collected from multiple sources with the following precedence (higher wins):
//  1. User skills from ~/.config/grove/skills
//  2. Ecosystem skills from the notebook (if project is part of an ecosystem)
//  3. Project skills from the notebook (highest precedence)
//
// Supports nested skill directories: skills/kitchen/prep/SKILL.md resolves as skill "prep"
// and is synced flattened to destDir/prep/.
func SyncSkillsToDirectory(svc *service.Service, node *workspace.WorkspaceNode, destDir string) (int, error) {
	if node == nil {
		return 0, fmt.Errorf("workspace node is required")
	}

	// Map: skillName -> sourcePath (flattened to leaf directory name)
	skillSources := make(map[string]string)

	userSkillsPath := getUserSkillsPathWithConfig(svc)
	if userSkillsPath != "" {
		collectSkillsFromDir(userSkillsPath, skillSources)
	}

	if node.RootEcosystemPath != "" {
		if ecoDir := getEcosystemSkillsDir(svc, node); ecoDir != "" {
			collectSkillsFromDir(ecoDir, skillSources)
		}
	}

	if projDir := getProjectSkillsDir(svc, node); projDir != "" {
		collectSkillsFromDir(projDir, skillSources)
	}

	if len(skillSources) == 0 {
		return 0, nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil { //nolint:gosec // G301: skills dir needs traversal
		return 0, fmt.Errorf("failed to create destination directory: %w", err)
	}

	var syncedCount int
	var lastErr error
	for skillName, srcPath := range skillSources {
		destPath := filepath.Join(destDir, skillName)
		if err := corefs.CopyDir(srcPath, destPath); err != nil {
			lastErr = fmt.Errorf("failed to sync skill %s: %w", skillName, err)
		} else {
			syncedCount++
		}
	}

	return syncedCount, lastErr
}

// collectSkillsFromDir recursively scans a directory for SKILL.md files and adds them to the map.
// The map key is the leaf directory name (skill name), flattening any nesting.
// Directories without SKILL.md are treated as organizational folders and skipped.
func collectSkillsFromDir(dir string, skillSources map[string]string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}

		skillPath := filepath.Dir(path)
		relDir, _ := filepath.Rel(dir, skillPath)
		// Prevent infinite scanning of deeply nested directories
		if strings.Count(filepath.ToSlash(relDir), "/") > 4 {
			return nil
		}

		skillName := filepath.Base(skillPath)
		skillSources[skillName] = skillPath
		return nil
	})
}

// addSkillSources recursively discovers skills from a directory and adds them to the sources map.
// Skill name is always the leaf directory containing SKILL.md.
// Directories without SKILL.md are organizational folders — they are recursed into but not added.
func addSkillSources(dir string, sourceType SourceType, sources map[string]SkillSource) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}

		skillPath := filepath.Dir(path)
		relDir, _ := filepath.Rel(dir, skillPath)
		if strings.Count(filepath.ToSlash(relDir), "/") > 4 {
			return nil
		}

		// Skill name is the leaf directory containing SKILL.md
		skillName := filepath.Base(skillPath)

		addSkillSourceSafely(sources, skillName, SkillSource{
			Path:    skillPath,
			RelPath: relDir,
			Type:    sourceType,
		})
		return nil
	})
}

// addBuiltinSkillSources adds embedded/built-in skills to the sources map.
// Supports nested builtin skills by walking the embedded FS recursively.
func addBuiltinSkillSources(sources map[string]SkillSource) {
	_ = fs.WalkDir(embeddedSkillsFS, "data/skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}

		skillPath := filepath.Dir(path)
		relDir, _ := filepath.Rel("data/skills", skillPath)
		if strings.Count(filepath.ToSlash(relDir), "/") > 4 {
			return nil
		}

		skillName := filepath.Base(skillPath)

		addSkillSourceSafely(sources, skillName, SkillSource{
			Path:    "(builtin)",
			RelPath: relDir,
			Type:    SourceTypeBuiltin,
		})
		return nil
	})
}

// ListSkillSources returns a map of skill names to their source paths.
// Skills are listed in precedence order (later sources override earlier):
//  1. Built-in skills (embedded in binary)
//  2. User skills (~/.config/grove/skills)
//  3. Notebook skills (from all configured notebook workspaces)
//  4. Ecosystem skills (from notebook)
//  5. Project skills (from notebook)
func ListSkillSources(svc *service.Service, node *workspace.WorkspaceNode) map[string]SkillSource {
	sources := make(map[string]SkillSource)

	addBuiltinSkillSources(sources)

	if userPath := getUserSkillsPathWithConfig(svc); userPath != "" {
		addSkillSources(userPath, SourceTypeUser, sources)
	}

	addNotebookSkillSources(svc, sources)

	if node != nil && node.RootEcosystemPath != "" {
		if ecoDir := getEcosystemSkillsDir(svc, node); ecoDir != "" {
			addSkillSources(ecoDir, SourceTypeEcosystem, sources)
		}
	}

	if node != nil {
		if projDir := getProjectSkillsDir(svc, node); projDir != "" {
			addSkillSources(projDir, SourceTypeProject, sources)
		}
	}

	// Playbook-owned skills: walk playbooks/<name>/skills for each playbook
	// bundle in the workspace's playbooks directory. These skills sync
	// identically to standalone skills.
	addPlaybookSkillSources(svc, node, sources)

	return sources
}

// addPlaybookSkillSources discovers skills shipped inside playbook bundles
// and registers them as standard skill sources. It walks the full 4-tier
// playbook search path (project > ecosystem > user > builtin) so sync
// honors the same precedence LoadPlaybook uses. Higher-precedence tiers
// overwrite lower ones in the sources map unconditionally.
func addPlaybookSkillSources(svc *service.Service, node *workspace.WorkspaceNode, sources map[string]SkillSource) {
	if node == nil {
		return
	}

	// GetPlaybookSearchDirs returns dirs in precedence order
	// (project first). Walk in reverse so later overwrites win.
	dirs := GetPlaybookSearchDirs(node.Path)
	for i := len(dirs) - 1; i >= 0; i-- {
		playbooksDir := dirs[i]
		entries, err := os.ReadDir(playbooksDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pbRoot := filepath.Join(playbooksDir, entry.Name())
			if _, err := os.Stat(filepath.Join(pbRoot, "playbook.toml")); err != nil {
				continue
			}
			pbSkills := filepath.Join(pbRoot, "skills")

			// Collect skills from this tier into a temporary map
			// and forcibly overwrite the main sources map, rather
			// than using addSkillSourceSafely (which picks shallowest
			// path and keeps the first-seen entry when types match).
			tierSources := make(map[string]SkillSource)
			addSkillSources(pbSkills, SourceTypeProject, tierSources)
			for name, src := range tierSources {
				sources[name] = src
			}

			// Register the playbook's parent directory as a search
			// path so LoadPlaybook can resolve this playbook by name
			// from other call sites.
			RegisterPlaybookSearchPath(playbooksDir)
		}
	}
}

// addNotebookSkillSources scans all configured notebook definitions for skill directories.
func addNotebookSkillSources(svc *service.Service, sources map[string]SkillSource) {
	if svc == nil || svc.Config == nil || svc.Config.Notebooks == nil {
		return
	}

	for _, nb := range svc.Config.Notebooks.Definitions {
		if nb == nil || nb.RootDir == "" {
			continue
		}

		rootDir, err := pathutil.Expand(nb.RootDir)
		if err != nil {
			continue
		}

		workspacesDir := filepath.Join(rootDir, "workspaces")
		wsEntries, err := os.ReadDir(workspacesDir)
		if err != nil {
			continue
		}

		for _, wsEntry := range wsEntries {
			if !wsEntry.IsDir() {
				continue
			}
			skillsDir := filepath.Join(workspacesDir, wsEntry.Name(), "skills")
			addSkillSources(skillsDir, SourceTypeEcosystem, sources)
		}
	}
}

// getEcosystemSkillsDir returns the skills directory for the ecosystem containing the node
func getEcosystemSkillsDir(svc *service.Service, node *workspace.WorkspaceNode) string {
	if svc == nil || svc.NotebookLocator == nil || node.RootEcosystemPath == "" {
		return ""
	}

	ecoNode := &workspace.WorkspaceNode{
		Name:         filepath.Base(node.RootEcosystemPath),
		Path:         node.RootEcosystemPath,
		NotebookName: node.NotebookName,
	}

	skillsDir, err := svc.NotebookLocator.GetSkillsDir(ecoNode)
	if err != nil {
		return ""
	}
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
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return ""
	}
	return skillsDir
}

// GetSkillsDirectoryForWorktree returns the standard skills directory path for a worktree.
func GetSkillsDirectoryForWorktree(worktreePath, provider string) string {
	switch provider {
	case "codex":
		return filepath.Join(worktreePath, ".codex", "skills")
	case "opencode":
		return filepath.Join(worktreePath, ".opencode", "skill")
	default:
		return filepath.Join(worktreePath, ".claude", "skills")
	}
}

// NewServiceForNode creates a minimal service for skill operations on a specific node.
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

// SyncOptions configures the behavior of a workspace skill synchronization.
type SyncOptions struct {
	Prune  bool
	DryRun bool
}

// SyncResult holds the results of a SyncWorkspace operation.
type SyncResult struct {
	Workspace    string
	SyncedSkills []string
	DestPaths    []string
	Error        string
}

// SyncWorkspace resolves and installs skills for a single workspace node.
func SyncWorkspace(svc *service.Service, node *workspace.WorkspaceNode, opts SyncOptions, logger *logging.PrettyLogger) (*SyncResult, error) {
	result := &SyncResult{
		Workspace: "global",
	}
	if node != nil {
		result.Workspace = node.Name
	}

	if node == nil {
		return result, fmt.Errorf("workspace node is required")
	}

	gitRoot, err := git.GetGitRoot(node.Path)
	if err != nil {
		gitRoot = node.Path
	}

	skillsCfg, err := LoadSkillsConfig(svc.Config, node)
	if err != nil {
		return result, fmt.Errorf("failed to load skills config: %w", err)
	}

	// Synthesize a skills config if none exists so playbook-authorized
	// skills still get resolved. A grove.toml with only [playbooks] must
	// still sync those playbook-owned skills.
	if skillsCfg == nil {
		skillsCfg = &SkillsConfig{}
	}

	providers := []string{"claude"}
	if len(skillsCfg.Providers) > 0 {
		providers = skillsCfg.Providers
	}

	hasPlaybookSkills := false
	if node != nil {
		if pbCfg, _ := LoadPlaybooksFromPath(node.Path); pbCfg != nil && len(pbCfg.Use) > 0 {
			hasPlaybookSkills = true
		}
	}

	if len(skillsCfg.Use) == 0 && len(skillsCfg.Dependencies) == 0 && !hasPlaybookSkills {
		if opts.Prune && !opts.DryRun {
			for _, provider := range providers {
				destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
				cleanupRemovedSkills(destBaseDir, nil)
			}
		}
		return result, nil
	}

	resolved, err := ResolveConfiguredSkills(svc, node, skillsCfg)
	if err != nil {
		return result, fmt.Errorf("failed to resolve skills: %w", err)
	}

	if len(resolved) == 0 {
		if opts.Prune && !opts.DryRun {
			for _, provider := range providers {
				destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
				cleanupRemovedSkills(destBaseDir, nil)
			}
		}
		return result, nil
	}

	synced := make([]string, 0, len(resolved))
	destPathsMap := make(map[string]bool)
	for name, r := range resolved {
		synced = append(synced, name)
		for _, p := range r.Providers {
			destPathsMap[GetSkillsDirectoryForWorktree(gitRoot, p)] = true
		}
	}

	destPaths := make([]string, 0, len(destPathsMap))
	for p := range destPathsMap {
		destPaths = append(destPaths, p)
	}

	result.SyncedSkills = synced
	result.DestPaths = destPaths

	if opts.DryRun {
		return result, nil
	}

	_, err = SyncConfiguredSkills(gitRoot, resolved, opts.Prune, logger)
	return result, err
}

// cleanupRemovedSkills removes skill directories that are no longer in the configured set.
// If configuredSkills is nil, removes ALL skill directories.
func cleanupRemovedSkills(skillsDir string, configuredSkills map[string]bool) {
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if configuredSkills == nil || !configuredSkills[entry.Name()] {
			_ = os.RemoveAll(filepath.Join(skillsDir, entry.Name()))
		}
	}
}

// SyncConfiguredSkills syncs resolved skills to their target provider directories.
// Skills are always flattened to a single level: .claude/skills/<skillName>/.
func SyncConfiguredSkills(gitRoot string, resolved map[string]ResolvedSkill, prune bool, logger *logging.PrettyLogger) (int, error) {
	syncedCount := 0
	var lastErr error

	// Track installed RelPaths per provider for pruning
	installedPerProvider := make(map[string]map[string]bool)

	for skillName, r := range resolved {
		for _, provider := range r.Providers {
			destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
			destPath := filepath.Join(destBaseDir, skillName)

			if installedPerProvider[provider] == nil {
				installedPerProvider[provider] = make(map[string]bool)
			}
			installedPerProvider[provider][skillName] = true

			if err := os.MkdirAll(destBaseDir, 0o755); err != nil { //nolint:gosec // G301: skills dir
				lastErr = fmt.Errorf("failed to create directory %s: %w", destBaseDir, err)
				continue
			}

			_ = os.RemoveAll(destPath)

			if r.SourceType == SourceTypeBuiltin {
				files, err := readSkillFromFS(embeddedSkillsFS, r.RelPath)
				if err != nil {
					lastErr = err
					continue
				}

				if err := os.MkdirAll(destPath, 0o755); err != nil { //nolint:gosec // G301: skills dir
					lastErr = err
					continue
				}

				for relPath, content := range files {
					filePath := filepath.Join(destPath, relPath)
					if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil { //nolint:gosec // G301: skill subdir
						lastErr = err
						continue
					}
					if err := os.WriteFile(filePath, content, 0o644); err != nil { //nolint:gosec // G306: skill files
						lastErr = err
						continue
					}
				}
				syncedCount++
			} else {
				if err := corefs.CopyDir(r.PhysicalPath, destPath); err != nil {
					lastErr = fmt.Errorf("failed to copy skill %s: %w", skillName, err)
				} else {
					syncedCount++
				}
			}
		}
	}

	if prune {
		pruneSkillsDir(gitRoot, installedPerProvider, logger)
	}

	syncSkillsToWorktrees(gitRoot, resolved, installedPerProvider, prune, logger)
	return syncedCount, lastErr
}

// syncSkillsToWorktrees syncs resolved skills to all worktrees under .grove-worktrees/.
func syncSkillsToWorktrees(gitRoot string, resolved map[string]ResolvedSkill, installedPerProvider map[string]map[string]bool, prune bool, logger *logging.PrettyLogger) {
	worktreesDir := filepath.Join(gitRoot, ".grove-worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		wtPath := filepath.Join(worktreesDir, entry.Name())

		for skillName, r := range resolved {
			for _, provider := range r.Providers {
				destBaseDir := GetSkillsDirectoryForWorktree(wtPath, provider)
				destPath := filepath.Join(destBaseDir, skillName)

				if err := os.MkdirAll(destBaseDir, 0o755); err != nil { //nolint:gosec // G301: skills dir
					continue
				}

				_ = os.RemoveAll(destPath)

				if r.SourceType == SourceTypeBuiltin {
					files, err := readSkillFromFS(embeddedSkillsFS, r.RelPath)
					if err != nil {
						continue
					}
					if err := os.MkdirAll(destPath, 0o755); err != nil { //nolint:gosec // G301
						continue
					}
					for relPath, content := range files {
						filePath := filepath.Join(destPath, relPath)
						_ = os.MkdirAll(filepath.Dir(filePath), 0o755) //nolint:gosec // G301
						_ = os.WriteFile(filePath, content, 0o644)     //nolint:gosec // G306
					}
				} else {
					_ = corefs.CopyDir(r.PhysicalPath, destPath)
				}
			}
		}

		if prune {
			pruneSkillsDir(wtPath, installedPerProvider, logger)
		}
	}
}

// pruneSkillsDir removes skills not in the installed map from a directory.
// Skills are always one level deep (flat structure) under the provider skills dir.
func pruneSkillsDir(root string, installedPerProvider map[string]map[string]bool, logger *logging.PrettyLogger) {
	for provider, validNames := range installedPerProvider {
		destBaseDir := GetSkillsDirectoryForWorktree(root, provider)

		entries, err := os.ReadDir(destBaseDir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if !validNames[entry.Name()] {
				path := filepath.Join(destBaseDir, entry.Name())
				_ = os.RemoveAll(path)
				if logger != nil {
					logger.InfoPretty(fmt.Sprintf("Pruned unconfigured skill at: %s", path))
				}
			}
		}
	}
}
