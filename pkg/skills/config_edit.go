package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GetGlobalConfigPath returns the path to the global grove.toml config file.
func GetGlobalConfigPath() string {
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

	return filepath.Join(configDir, "grove", "grove.toml")
}

// ToggleSkillInConfig surgically toggles a skill in the `use` array of a grove.toml file.
// If the file or [skills] block does not exist, it creates them.
func ToggleSkillInConfig(tomlPath, skillName string) error {
	return toggleSkillInSection(tomlPath, skillName, "[skills]")
}

// ToggleUserProjectSkillInConfig toggles a skill in the user's global config,
// scoped to a specific project. This allows users to configure project-specific
// skills that live in their dotfiles rather than the project's grove.toml.
func ToggleUserProjectSkillInConfig(tomlPath, skillName, projectName string) error {
	sectionHeader := fmt.Sprintf("[skills.projects.%s]", projectName)
	return toggleSkillInSection(tomlPath, skillName, sectionHeader)
}

// toggleSkillInSection toggles a skill in a specific TOML section's use array.
// The sectionHeader should be the full section name including brackets (e.g., "[skills]").
func toggleSkillInSection(tomlPath, skillName, sectionHeader string) error {
	if err := os.MkdirAll(filepath.Dir(tomlPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	content, err := os.ReadFile(tomlPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read config: %w", err)
	}

	text := string(content)

	// Ensure section exists
	if !strings.Contains(text, sectionHeader) {
		if len(text) > 0 && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += fmt.Sprintf("\n%s\nuse = [\"%s\"]\n", sectionHeader, skillName)
		return os.WriteFile(tomlPath, []byte(text), 0644)
	}

	// Find the section
	sectionIdx := strings.Index(text, sectionHeader)
	if sectionIdx == -1 {
		return fmt.Errorf("internal error: could not find %s section", sectionHeader)
	}

	// Look for `use = [...]` in the section
	// We need to find it after the section header but potentially before the next section
	subText := text[sectionIdx:]

	// Find the next section (if any)
	nextSectionRegex := regexp.MustCompile(`\n\[[^\]]+\]`)
	nextSectionMatch := nextSectionRegex.FindStringIndex(subText)
	var sectionEnd int
	if nextSectionMatch != nil {
		sectionEnd = sectionIdx + nextSectionMatch[0]
	} else {
		sectionEnd = len(text)
	}

	sectionText := text[sectionIdx:sectionEnd]

	// Look for use = [...] pattern - handle multiline arrays
	useRegex := regexp.MustCompile(`(?s)use\s*=\s*\[(.*?)\]`)
	match := useRegex.FindStringSubmatchIndex(sectionText)

	if match == nil {
		// Insert `use = [...]` directly after section header
		insertion := fmt.Sprintf("\nuse = [\"%s\"]", skillName)
		endOfHeader := sectionIdx + len(sectionHeader)
		newText := text[:endOfHeader] + insertion + text[endOfHeader:]
		return os.WriteFile(tomlPath, []byte(newText), 0644)
	}

	// Extract existing skills from the array
	arrayStart := sectionIdx + match[2]
	arrayEnd := sectionIdx + match[3]
	arrayContent := text[arrayStart:arrayEnd]

	// Parse array elements
	var currentSkills []string
	if strings.TrimSpace(arrayContent) != "" {
		// Handle both single-line and multi-line arrays
		parts := strings.Split(arrayContent, ",")
		for _, p := range parts {
			s := strings.Trim(strings.TrimSpace(p), `"'`)
			// Remove any newlines/tabs from the skill name
			s = strings.TrimSpace(s)
			if s != "" {
				currentSkills = append(currentSkills, s)
			}
		}
	}

	// Toggle logic: remove if present, add if not
	found := false
	var newSkills []string
	for _, s := range currentSkills {
		if s == skillName {
			found = true
		} else {
			newSkills = append(newSkills, s)
		}
	}
	if !found {
		newSkills = append(newSkills, skillName)
	}

	// Build new array line
	var newArrayLine string
	if len(newSkills) == 0 {
		newArrayLine = "use = []"
	} else {
		quoted := make([]string, len(newSkills))
		for i, s := range newSkills {
			quoted[i] = fmt.Sprintf(`"%s"`, s)
		}
		newArrayLine = "use = [" + strings.Join(quoted, ", ") + "]"
	}

	// Replace the entire use = [...] match
	useMatchStart := sectionIdx + match[0]
	useMatchEnd := sectionIdx + match[1]
	newText := text[:useMatchStart] + newArrayLine + text[useMatchEnd:]

	return os.WriteFile(tomlPath, []byte(newText), 0644)
}
