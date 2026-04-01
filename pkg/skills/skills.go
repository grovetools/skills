package skills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/skills/pkg/service"
	"gopkg.in/yaml.v3"
)

//go:embed data/skills
var embeddedSkillsFS embed.FS

// SkillMetadata represents the YAML frontmatter of a SKILL.md file
type SkillMetadata struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Requires    []string `yaml:"requires,omitempty"`
	Domain      string   `yaml:"domain,omitempty"`
}

// ValidationError represents a skill validation error
type ValidationError struct {
	SkillName string
	Errors    []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("skill '%s' validation failed: %v", e.SkillName, e.Errors)
}

// nameRegex validates skill names: lowercase alphanumeric with single hyphen separators
var nameRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateSkillContent validates the content of a SKILL.md file
func ValidateSkillContent(content []byte, expectedName string) error {
	metadata, err := ParseSkillFrontmatter(content)
	if err != nil {
		return fmt.Errorf("failed to parse SKILL.md frontmatter: %w", err)
	}

	var errors []string

	// Validate name
	if metadata.Name == "" {
		errors = append(errors, "missing required field 'name'")
	} else {
		if len(metadata.Name) > 64 {
			errors = append(errors, fmt.Sprintf("name exceeds 64 characters (got %d)", len(metadata.Name)))
		}
		if !nameRegex.MatchString(metadata.Name) {
			errors = append(errors, "name must be lowercase alphanumeric with single hyphen separators (e.g., 'my-skill-name')")
		}
		if expectedName != "" && metadata.Name != expectedName {
			errors = append(errors, fmt.Sprintf("name '%s' does not match directory name '%s'", metadata.Name, expectedName))
		}
	}

	// Validate description
	if metadata.Description == "" {
		errors = append(errors, "missing required field 'description'")
	} else if len(metadata.Description) > 1024 {
		errors = append(errors, fmt.Sprintf("description exceeds 1024 characters (got %d)", len(metadata.Description)))
	}

	if len(errors) > 0 {
		return &ValidationError{SkillName: expectedName, Errors: errors}
	}

	return nil
}

// ParseSkillFrontmatter extracts and parses YAML frontmatter from SKILL.md content
func ParseSkillFrontmatter(content []byte) (*SkillMetadata, error) {
	// Frontmatter must start with "---" on line 1
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("SKILL.md must start with '---' frontmatter delimiter")
	}

	// Find the closing "---"
	rest := content[3:]
	endIdx := bytes.Index(rest, []byte("\n---"))
	if endIdx == -1 {
		return nil, fmt.Errorf("missing closing '---' frontmatter delimiter")
	}

	frontmatter := rest[:endIdx]

	var metadata SkillMetadata
	if err := yaml.Unmarshal(frontmatter, &metadata); err != nil {
		return nil, fmt.Errorf("invalid YAML in frontmatter: %w", err)
	}

	return &metadata, nil
}

// getUserSkillsPath returns the path to the user-defined skills directory (~/.config/grove/skills).
func getUserSkillsPath() string {
	var configDir string

	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		configDir = xdgConfig
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		configDir = filepath.Join(home, ".config")
	}

	return filepath.Join(configDir, "grove", "skills")
}

// getUserSkillsPathWithConfig returns the user skills path.
// The service parameter is kept for API compatibility but is currently unused.
func getUserSkillsPathWithConfig(svc *service.Service) string {
	_ = svc // Unused, kept for API compatibility
	return getUserSkillsPath()
}

// ListBuiltinSkills returns a list of all built-in skill names.
func ListBuiltinSkills() []string {
	entries, err := fs.ReadDir(embeddedSkillsFS, "data/skills")
	if err != nil {
		return nil
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

// ListSkills returns a slice of available skill names and a map indicating their source.
// Precedence: notebook > user > builtin
// Skills with the same name as a skill from a lower-precedence source will take precedence.
func ListSkills() ([]string, map[string]string, error) {
	return ListSkillsWithService(nil)
}

// ListSkillsWithService returns a slice of available skill names and a map indicating their source.
// If a service is provided, notebook skills will also be discovered.
// Precedence: notebook > user > builtin
func ListSkillsWithService(svc *service.Service) ([]string, map[string]string, error) {
	skillMap := make(map[string]string)

	// 1. Load built-in skills
	entries, err := fs.ReadDir(embeddedSkillsFS, "data/skills")
	if err != nil {
		return nil, nil, fmt.Errorf("could not read embedded skills: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			skillMap[entry.Name()] = "builtin"
		}
	}

	// 2. Load user skills, overwriting built-in if names conflict
	userSkillsPath := getUserSkillsPathWithConfig(svc)
	if userSkillsPath != "" {
		if entries, err := os.ReadDir(userSkillsPath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					skillMap[entry.Name()] = "user"
				}
			}
		}
	}

	// 3. Load notebook skills (highest precedence)
	notebookSkills, err := findNotebookSkills(svc)
	if err == nil {
		for name := range notebookSkills {
			skillMap[name] = "notebook"
		}
	}

	var skillNames []string
	for name := range skillMap {
		skillNames = append(skillNames, name)
	}
	sort.Strings(skillNames)
	return skillNames, skillMap, nil
}

// GetSkillByWorkDir resolves a skill using full workspace-aware resolution from the
// given working directory. Unlike GetSkill/GetSkillWithService, this function does
// not depend on os.Getwd() or a *service.Service, making it suitable for callers
// that operate in a different directory than the process cwd (e.g., flow's daemon
// resolving skills for jobs running in worktrees).
//
// Precedence: project notebook > ecosystem notebook > user > builtin
func GetSkillByWorkDir(name string, workDir string) (map[string][]byte, error) {
	node, err := workspace.GetProjectByPath(workDir)
	if err == nil {
		coreCfg, cfgErr := config.LoadDefault()
		if cfgErr != nil {
			coreCfg = &config.Config{}
		}

		locator := workspace.NewNotebookLocator(coreCfg)

		// 1. Try project-level notebook skills
		if skillFiles, err := readSkillFromNotebook(locator, node, name); err == nil {
			return skillFiles, nil
		}

		// 2. Try ecosystem-level notebook skills
		if node.RootEcosystemPath != "" {
			ecoNode := &workspace.WorkspaceNode{
				Name:         filepath.Base(node.RootEcosystemPath),
				Path:         node.RootEcosystemPath,
				NotebookName: node.NotebookName,
			}
			if skillFiles, err := readSkillFromNotebook(locator, ecoNode, name); err == nil {
				return skillFiles, nil
			}
		}
	}

	// 3. User skills
	userSkillsPath := getUserSkillsPathWithConfig(nil)
	if userSkillsPath != "" {
		if skillFiles, err := readSkillFromDisk(filepath.Join(userSkillsPath, name)); err == nil {
			return skillFiles, nil
		}
	}

	// 4. Builtin/embedded skills
	return readSkillFromFS(embeddedSkillsFS, name)
}

// readSkillFromNotebook reads a skill from the notebook skills directory for a workspace node.
func readSkillFromNotebook(locator *workspace.NotebookLocator, node *workspace.WorkspaceNode, name string) (map[string][]byte, error) {
	skillsDir, err := locator.GetSkillsDir(node)
	if err != nil || skillsDir == "" {
		return nil, fmt.Errorf("no skills directory found")
	}
	return readSkillFromDisk(filepath.Join(skillsDir, name))
}

// GetSkill retrieves all files for a given skill, checking sources in order of precedence.
// Precedence: notebook > user > builtin
// It returns a map of relative file paths to their content.
func GetSkill(name string) (map[string][]byte, error) {
	return GetSkillWithService(nil, name)
}

// GetSkillWithService retrieves all files for a given skill, checking sources in order of precedence.
// If a service is provided, notebook skills will also be checked.
// Precedence: notebook > user > builtin
func GetSkillWithService(svc *service.Service, name string) (map[string][]byte, error) {
	// 1. Try notebook skills first (highest precedence)
	notebookSkills, err := findNotebookSkills(svc)
	if err == nil {
		if skillPath, ok := notebookSkills[name]; ok {
			skillFiles, err := readSkillFromDisk(skillPath)
			if err == nil {
				return skillFiles, nil // Found in notebook
			}
		}
	}

	// 2. Try user skills second
	userSkillsPath := getUserSkillsPathWithConfig(svc)
	if userSkillsPath != "" {
		skillFiles, err := readSkillFromDisk(filepath.Join(userSkillsPath, name))
		if err == nil {
			return skillFiles, nil // Found in user skills
		}
	}

	// 3. Fallback to embedded skills
	return readSkillFromFS(embeddedSkillsFS, name)
}

// readSkillFromDisk reads all files for a skill from a given directory path.
func readSkillFromDisk(skillRoot string) (map[string][]byte, error) {
	skillFiles := make(map[string][]byte)
	err := filepath.WalkDir(skillRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(skillRoot, path)
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		skillFiles[relPath] = content
		return nil
	})
	if err != nil || len(skillFiles) == 0 {
		return nil, fmt.Errorf("skill not found at %s", skillRoot)
	}
	return skillFiles, nil
}

// readSkillFromFS reads all files for a skill from an fs.FS.
func readSkillFromFS(srcFS fs.FS, name string) (map[string][]byte, error) {
	skillFiles := make(map[string][]byte)
	skillRoot := filepath.Join("data/skills", name)
	err := fs.WalkDir(srcFS, skillRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relPath, _ := filepath.Rel(skillRoot, path)
		content, err := fs.ReadFile(srcFS, path)
		if err != nil {
			return err
		}
		skillFiles[relPath] = content
		return nil
	})
	if err != nil || len(skillFiles) == 0 {
		return nil, fmt.Errorf("skill '%s' not found", name)
	}
	return skillFiles, nil
}

// findNotebookSkills discovers skills within the current workspace's notebook.
// It returns a map of skill names to their absolute paths on disk.
func findNotebookSkills(svc *service.Service) (map[string]string, error) {
	if svc == nil || svc.Provider == nil || svc.NotebookLocator == nil {
		return nil, fmt.Errorf("service not initialized for notebook skill discovery")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Get current workspace context using grove-core's workspace lookup
	node, err := workspace.GetProjectByPath(cwd)
	if err != nil {
		// Not in a workspace, no notebook skills to find
		return nil, nil
	}

	// Find the skills directory for this workspace using NotebookLocator
	skillsDir, err := svc.NotebookLocator.GetGroupDir(node, "skills")
	if err != nil || skillsDir == "" {
		return nil, nil
	}

	// Check if the skills directory exists
	if _, err := os.Stat(skillsDir); os.IsNotExist(err) {
		return nil, nil
	}

	// Scan for skill directories
	skillPaths := make(map[string]string)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil, nil // Directory may not exist or be readable
	}

	for _, entry := range entries {
		if entry.IsDir() {
			skillName := entry.Name()
			skillPaths[skillName] = filepath.Join(skillsDir, skillName)
		}
	}

	return skillPaths, nil
}
