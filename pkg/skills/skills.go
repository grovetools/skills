package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

//go:embed data/skills
var embeddedSkillsFS embed.FS

// getUserSkillsPath returns the path to the user-defined skills directory (~/.config/grove/skills).
// It respects XDG_CONFIG_HOME if set, otherwise falls back to $HOME/.config
func getUserSkillsPath() (string, error) {
	var configDir string

	// Check XDG_CONFIG_HOME first
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		configDir = xdgConfig
	} else {
		// Fall back to $HOME/.config
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("could not get user home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config")
	}

	return filepath.Join(configDir, "grove", "skills"), nil
}

// ListSkills returns a slice of available skill names and a map indicating their source ("builtin" or "user").
// User skills with the same name as a built-in skill will take precedence.
func ListSkills() ([]string, map[string]string, error) {
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
	userSkillsPath, err := getUserSkillsPath()
	if err == nil {
		if entries, err := os.ReadDir(userSkillsPath); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					skillMap[entry.Name()] = "user"
				}
			}
		}
	}

	var skillNames []string
	for name := range skillMap {
		skillNames = append(skillNames, name)
	}
	sort.Strings(skillNames)
	return skillNames, skillMap, nil
}

// GetSkill retrieves all files for a given skill, checking user-defined skills first.
// It returns a map of relative file paths to their content.
func GetSkill(name string) (map[string][]byte, error) {
	// 1. Try user skills first
	userSkillsPath, err := getUserSkillsPath()
	if err == nil {
		skillFiles, err := readSkillFromDisk(filepath.Join(userSkillsPath, name))
		if err == nil {
			return skillFiles, nil // Found in user skills
		}
	}

	// 2. Fallback to embedded skills
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
		return nil, fmt.Errorf("embedded skill '%s' not found", name)
	}
	return skillFiles, nil
}
