package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/grovetools/core/config"
	corefs "github.com/grovetools/core/fs"
	"github.com/grovetools/core/git"
	"github.com/grovetools/core/logging"
	"github.com/grovetools/core/pkg/notespace"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/pkg/worktreeregistry"
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

// addNotebookSkillSources enumerates recorded primary notespaces and asks the
// core locator for each skills directory. It never infers notes-plane identity
// from a literal directory segment.
func addNotebookSkillSources(svc *service.Service, sources map[string]SkillSource) {
	if svc == nil || svc.Config == nil {
		return
	}
	machine, err := config.LoadMachineConfig()
	if err != nil || machine == nil {
		return
	}
	for _, skillsDir := range configuredNotespaceDirs(svc.Config, machine, func(locator *workspace.NotebookLocator, node *workspace.WorkspaceNode) (string, error) {
		return locator.GetSkillsDir(node)
	}) {
		addSkillSources(skillsDir, SourceTypeEcosystem, sources)
	}
}

// configuredNotespaceDirs enumerates stamped roots from configured notebooks,
// resolves each display name through core's primary routing contract, and then
// delegates content-path construction to NotebookLocator. Directory names are
// candidates only; only a matching resolver result is emitted.
func configuredNotespaceDirs(cfg *config.Config, machine *config.MachineConfig, resolve func(*workspace.NotebookLocator, *workspace.WorkspaceNode) (string, error)) []string {
	if cfg == nil || cfg.Notebooks == nil || machine == nil {
		return nil
	}
	locator := workspace.NewNotebookLocator(cfg)
	seen := make(map[string]bool)
	var dirs []string
	for notebookName, nb := range cfg.Notebooks.Definitions {
		if nb == nil || nb.RootDir == "" {
			continue
		}
		rootDir, err := pathutil.Expand(nb.RootDir)
		if err != nil || workspace.ValidateNotespaceLayout(rootDir) != nil {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(rootDir, workspace.NotespaceDirectory))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidateRoot := filepath.Join(rootDir, workspace.NotespaceDirectory, entry.Name())
			stamp, err := notespace.LoadNotespace(candidateRoot)
			if err != nil || stamp == nil {
				continue
			}
			resolution, err := workspace.ResolveNotespaceName(stamp.Name, cfg, machine)
			if err != nil || filepath.Clean(resolution.Root) != filepath.Clean(candidateRoot) {
				continue
			}
			node := &workspace.WorkspaceNode{Name: filepath.Base(resolution.Root), Path: resolution.Root, NotebookName: notebookName}
			dir, err := resolve(locator, node)
			if err != nil || dir == "" || seen[dir] {
				continue
			}
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	sort.Strings(dirs)
	return dirs
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
	case "pi":
		// pi consumes the Agent Skills standard (SKILL.md + name/description
		// frontmatter) natively from the project-local .pi/skills directory
		// (loadSkills in packages/coding-agent/src/core/skills.ts of the pi
		// source), so existing SKILL.md content syncs unchanged.
		return filepath.Join(worktreePath, ".pi", "skills")
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
	Workspace string
	// SyncedSkills lists the skills whose destination copies actually
	// CHANGED during this sync (source manifest differed from the stored
	// .grove-sync-manifest in at least one destination). Unchanged skills
	// are skipped entirely and not listed here. For dry runs no copying
	// happens, so SyncedSkills reports the full resolved set (what a real
	// run would ensure is installed).
	SyncedSkills []string
	// ResolvedSkills lists every skill resolved for this workspace,
	// regardless of whether its destinations needed updating.
	ResolvedSkills []string
	// MissingSkills lists configured skills that could not be resolved
	// from any source. They are skipped (with a warning) rather than
	// failing the sync — see ResolveConfiguredSkills.
	MissingSkills []string
	DestPaths     []string
	Error         string

	// ScopedOut is true when at least one configured layer was skipped for
	// this workspace because its [skills] scope excluded it (see SeedScope).
	// Paired with an empty SyncedSkills it means "seeded at the ecosystem
	// root instead", not "nothing configured".
	ScopedOut bool
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
	result.ScopedOut = skillsCfg.ScopedOut

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
			cleanupAllSkillDirs(gitRoot, providers)
		}
		return result, nil
	}

	resolved, missing, err := ResolveConfiguredSkills(svc, node, skillsCfg)
	if err != nil {
		return result, fmt.Errorf("failed to resolve skills: %w", err)
	}
	result.MissingSkills = missing
	if logger != nil {
		for _, name := range missing {
			logger.WarnPretty(fmt.Sprintf("Skipping '%s': declared in config but not found in any source", name))
		}
	}

	if len(resolved) == 0 {
		if opts.Prune && !opts.DryRun {
			cleanupAllSkillDirs(gitRoot, providers)
		}
		return result, nil
	}

	resolvedNames := make([]string, 0, len(resolved))
	destPathsMap := make(map[string]bool)
	for name, r := range resolved {
		resolvedNames = append(resolvedNames, name)
		for _, p := range r.Providers {
			destPathsMap[GetSkillsDirectoryForWorktree(gitRoot, p)] = true
		}
	}
	sort.Strings(resolvedNames)

	destPaths := make([]string, 0, len(destPathsMap))
	for p := range destPathsMap {
		destPaths = append(destPaths, p)
	}

	result.ResolvedSkills = resolvedNames
	result.DestPaths = destPaths

	if opts.DryRun {
		// No copying happens in a dry run, so report the full resolved set
		// as what a real sync would ensure.
		result.SyncedSkills = resolvedNames
		return result, nil
	}

	changed, err := SyncConfiguredSkills(gitRoot, resolved, opts.Prune, logger)
	result.SyncedSkills = changed
	return result, err
}

// cleanupAllSkillDirs removes every synced skill directory for the given
// providers, in the repository root and in each of its worktrees. It is the
// prune path for a workspace that resolves to no skills at all — nothing is
// configured, or every configured layer was scoped elsewhere (see SeedScope).
// Worktrees are included so that narrowing a declaration's scope actually
// clears the duplicated copies rather than leaving them behind everywhere but
// the main checkout.
func cleanupAllSkillDirs(gitRoot string, providers []string) {
	roots := append([]string{gitRoot}, collectWorktreePaths(gitRoot)...)
	for _, root := range roots {
		for _, provider := range providers {
			cleanupRemovedSkills(GetSkillsDirectoryForWorktree(root, provider), nil)
		}
	}
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
//
// Change detection: each destination skill dir carries a .grove-sync-manifest
// dot-file recording a hash of the source (relpath+size+mtime per file).
// When the freshly computed source manifest matches the stored one, the
// RemoveAll+copy is skipped entirely. Returns the sorted names of skills
// whose destination copies actually changed (in the repo root or any
// worktree).
func SyncConfiguredSkills(gitRoot string, resolved map[string]ResolvedSkill, prune bool, logger *logging.PrettyLogger) ([]string, error) {
	var lastErr error

	// Compute source manifests once, before any destination is touched.
	manifests := computeSkillManifests(resolved)
	changedSet := make(map[string]bool)

	// Track installed RelPaths per provider for pruning
	installedPerProvider := make(map[string]map[string]bool)

	for skillName, r := range resolved {
		manifest := manifests[skillName]
		for _, provider := range r.Providers {
			destBaseDir := GetSkillsDirectoryForWorktree(gitRoot, provider)
			destPath := filepath.Join(destBaseDir, skillName)

			if installedPerProvider[provider] == nil {
				installedPerProvider[provider] = make(map[string]bool)
			}
			installedPerProvider[provider][skillName] = true

			// Source unchanged since the last sync of this destination: no-op.
			if manifest != "" && readStoredManifest(destPath) == manifest {
				continue
			}

			if err := os.MkdirAll(destBaseDir, 0o755); err != nil { //nolint:gosec // G301: skills dir
				lastErr = fmt.Errorf("failed to create directory %s: %w", destBaseDir, err)
				continue
			}

			_ = os.RemoveAll(destPath)

			if err := installSkill(r, destPath); err != nil {
				lastErr = fmt.Errorf("failed to copy skill %s: %w", skillName, err)
				continue
			}
			writeStoredManifest(destPath, manifest)
			changedSet[skillName] = true
		}
	}

	if prune {
		pruneSkillsDir(gitRoot, installedPerProvider, logger)
	}

	syncSkillsToWorktrees(gitRoot, resolved, manifests, installedPerProvider, prune, logger, changedSet)

	changed := make([]string, 0, len(changedSet))
	for name := range changedSet {
		changed = append(changed, name)
	}
	sort.Strings(changed)
	return changed, lastErr
}

// installSkill copies a resolved skill's source into destPath (which must not
// already exist). Returns nil only if every file was written.
func installSkill(r ResolvedSkill, destPath string) error {
	if r.SourceType == SourceTypeBuiltin {
		files, err := readSkillFromFS(embeddedSkillsFS, r.RelPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(destPath, 0o755); err != nil { //nolint:gosec // G301: skills dir
			return err
		}
		for relPath, content := range files {
			filePath := filepath.Join(destPath, relPath)
			if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil { //nolint:gosec // G301: skill subdir
				return err
			}
			if err := os.WriteFile(filePath, content, 0o644); err != nil { //nolint:gosec // G306: skill files
				return err
			}
		}
		return nil
	}
	return corefs.CopyDir(r.PhysicalPath, destPath)
}

// syncSkillsToWorktrees syncs resolved skills to every worktree of the
// repository rooted at gitRoot. Worktree paths come from collectWorktreePaths,
// which unions the legacy workspace.WorktreeBases enumeration with the
// per-worktree registry so anchored/XDG worktrees living under a different
// sub-repo's base are reached.
//
// manifests carries the precomputed per-skill source manifests (see
// SyncConfiguredSkills); destinations whose stored manifest matches are
// skipped. changedSet, when non-nil, records the names of skills that were
// actually (re)copied into any worktree.
func syncSkillsToWorktrees(gitRoot string, resolved map[string]ResolvedSkill, manifests map[string]string, installedPerProvider map[string]map[string]bool, prune bool, logger *logging.PrettyLogger, changedSet map[string]bool) {
	for _, wtPath := range collectWorktreePaths(gitRoot) {
		syncSkillsToOneWorktree(wtPath, resolved, manifests, installedPerProvider, prune, logger, changedSet)
	}
}

// collectWorktreePaths returns the deduplicated absolute paths of every
// worktree that should receive synced skills for the repository rooted at
// gitRoot. It combines two sources:
//
//   - legacy enumeration of workspace.WorktreeBases(gitRoot) — a single-level
//     os.ReadDir of each base, preserving the original behavior (including its
//     limitation that nested branch-style names are not reached); and
//   - the per-worktree registry (worktreeregistry.ListAll), filtered to this
//     ecosystem. This captures anchored/XDG worktrees that live under a
//     different sub-repo's XDG base and are therefore invisible to
//     WorktreeBases.
//
// Paths are deduped by normalized absolute path so no worktree is synced twice.
func collectWorktreePaths(gitRoot string) []string {
	seen := make(map[string]struct{})
	var paths []string

	add := func(p string) {
		key, err := pathutil.NormalizeForLookup(p)
		if err != nil {
			key = p
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		paths = append(paths, p)
	}

	// Source 1: legacy worktree bases (single-level enumeration).
	for _, worktreesDir := range workspace.WorktreeBases(gitRoot) {
		entries, err := os.ReadDir(worktreesDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			add(filepath.Join(worktreesDir, entry.Name()))
		}
	}

	// Source 2: per-worktree registry, scoped to this ecosystem. Captures
	// anchored worktrees living under a sub-repo's XDG base, which Source 1
	// misses because they are not under WorktreeBases(gitRoot).
	regEntries, err := worktreeregistry.ListAll()
	if err == nil {
		normRoot, nErr := pathutil.NormalizeForLookup(gitRoot)
		if nErr != nil {
			normRoot = gitRoot
		}
		for _, e := range regEntries {
			if e == nil || e.AbsPath == "" {
				continue
			}
			if !ownerInEcosystem(e.Owner, normRoot) {
				continue
			}
			// Only sync worktrees that still exist on disk.
			if info, statErr := os.Stat(e.AbsPath); statErr != nil || !info.IsDir() {
				continue
			}
			add(e.AbsPath)
		}
	}

	return paths
}

// ownerInEcosystem reports whether a registry entry's Owner belongs to the
// ecosystem rooted at normRoot (an already-normalized gitRoot). True when the
// owner is gitRoot itself or a sub-repo nested under it.
func ownerInEcosystem(owner, normRoot string) bool {
	if owner == "" {
		return false
	}
	normOwner, err := pathutil.NormalizeForLookup(owner)
	if err != nil {
		normOwner = owner
	}
	if normOwner == normRoot {
		return true
	}
	return strings.HasPrefix(normOwner, normRoot+string(filepath.Separator))
}

// syncSkillsToOneWorktree writes the resolved skills into a single worktree at
// wtPath and optionally prunes unconfigured skills. Destinations whose stored
// .grove-sync-manifest matches the precomputed source manifest are skipped;
// skills actually (re)copied are recorded in changedSet when it is non-nil.
func syncSkillsToOneWorktree(wtPath string, resolved map[string]ResolvedSkill, manifests map[string]string, installedPerProvider map[string]map[string]bool, prune bool, logger *logging.PrettyLogger, changedSet map[string]bool) {
	for skillName, r := range resolved {
		manifest := manifests[skillName]
		for _, provider := range r.Providers {
			destBaseDir := GetSkillsDirectoryForWorktree(wtPath, provider)
			destPath := filepath.Join(destBaseDir, skillName)

			// Source unchanged since the last sync of this destination: no-op.
			if manifest != "" && readStoredManifest(destPath) == manifest {
				continue
			}

			if err := os.MkdirAll(destBaseDir, 0o755); err != nil { //nolint:gosec // G301: skills dir
				continue
			}

			_ = os.RemoveAll(destPath)

			if err := installSkill(r, destPath); err != nil {
				continue
			}
			writeStoredManifest(destPath, manifest)
			if changedSet != nil {
				changedSet[skillName] = true
			}
		}
	}

	if prune {
		pruneSkillsDir(wtPath, installedPerProvider, logger)
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
