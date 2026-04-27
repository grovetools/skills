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

	"github.com/grovetools/skills/pkg/service"
	"gopkg.in/yaml.v3"
)

//go:embed data/skills
var embeddedSkillsFS embed.FS

// SkillMetadata represents the YAML frontmatter of a SKILL.md file
type SkillMetadata struct {
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	Requires      []string `yaml:"requires,omitempty"`
	Domain        string   `yaml:"domain,omitempty"`
	SkillSequence []string `yaml:"skill_sequence,omitempty"`
	Produces      []string `yaml:"produces,omitempty"`
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
	if !bytes.HasPrefix(content, []byte("---")) {
		return nil, fmt.Errorf("SKILL.md must start with '---' frontmatter delimiter")
	}

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
func getUserSkillsPathWithConfig(svc *service.Service) string {
	_ = svc
	return getUserSkillsPath()
}

// ListBuiltinSkills returns a list of all built-in skill names.
// Uses recursive WalkDir to discover nested builtin skills.
func ListBuiltinSkills() []string {
	var names []string
	_ = fs.WalkDir(embeddedSkillsFS, "data/skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		names = append(names, filepath.Base(filepath.Dir(path)))
		return nil
	})
	return names
}

// ListSkills returns a slice of available skill names and a map indicating their source.
func ListSkills() ([]string, map[string]string, error) {
	return ListSkillsWithService(nil)
}

// ListSkillsWithService returns a slice of available skill names and a map indicating their source.
func ListSkillsWithService(svc *service.Service) ([]string, map[string]string, error) {
	sources := ListSkillSources(svc, nil)

	skillMap := make(map[string]string)
	for name, src := range sources {
		skillMap[name] = string(src.Type)
	}

	skillNames := make([]string, 0, len(skillMap))
	for name := range skillMap {
		skillNames = append(skillNames, name)
	}
	sort.Strings(skillNames)
	return skillNames, skillMap, nil
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
		content, err := os.ReadFile(path) //nolint:gosec // G304: path from WalkDir
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
// relPath is the path relative to data/skills (e.g. "sear/heat-pan" or just "my-skill").
func readSkillFromFS(srcFS fs.FS, relPath string) (map[string][]byte, error) {
	skillFiles := make(map[string][]byte)
	skillRoot := filepath.Join("data/skills", relPath)
	err := fs.WalkDir(srcFS, skillRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rp, _ := filepath.Rel(skillRoot, path)
		content, err := fs.ReadFile(srcFS, path)
		if err != nil {
			return err
		}
		skillFiles[rp] = content
		return nil
	})
	if err != nil || len(skillFiles) == 0 {
		return nil, fmt.Errorf("skill '%s' not found", relPath)
	}
	return skillFiles, nil
}
