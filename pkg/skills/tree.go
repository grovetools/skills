package skills

import (
	"fmt"
	"strings"

	"github.com/grovetools/skills/pkg/service"
)

// BuildDependencyTreeString builds a tree representation of a skill's dependencies.
// It returns a string suitable for display in CLI or TUI.
func BuildDependencyTreeString(svc *service.Service, name string) (string, error) {
	var sb strings.Builder
	visited := make(map[string]bool)
	err := buildTreeNode(&sb, svc, name, "", true, true, visited)
	return sb.String(), err
}

// buildTreeNode recursively builds the tree string.
func buildTreeNode(sb *strings.Builder, svc *service.Service, name string, prefix string, isLast bool, isRoot bool, visited map[string]bool) error {
	if visited[name] {
		if isRoot {
			sb.WriteString(fmt.Sprintf("%s (circular dependency detected)\n", name))
		} else {
			marker := "├── "
			if isLast {
				marker = "└── "
			}
			sb.WriteString(fmt.Sprintf("%s%s%s (circular dependency detected)\n", prefix, marker, name))
		}
		return nil
	}

	// Mark current node as visited for this traversal path
	visited[name] = true
	defer func() { visited[name] = false }()

	skillFiles, err := GetSkillWithService(svc, name)
	if err != nil {
		if isRoot {
			sb.WriteString(fmt.Sprintf("%s (not found)\n", name))
		} else {
			marker := "├── "
			if isLast {
				marker = "└── "
			}
			sb.WriteString(fmt.Sprintf("%s%s%s (not found)\n", prefix, marker, name))
		}
		return nil
	}

	content, ok := skillFiles["SKILL.md"]
	if !ok {
		return fmt.Errorf("skill '%s' missing SKILL.md", name)
	}

	metadata, err := ParseSkillFrontmatter(content)
	if err != nil {
		return fmt.Errorf("skill '%s' has invalid frontmatter: %w", name, err)
	}

	// Format the root node differently from child nodes
	if isRoot {
		sb.WriteString(fmt.Sprintf("%s (%s)\n", name, metadata.Description))
	} else {
		marker := "├── "
		if isLast {
			marker = "└── "
		}
		sb.WriteString(fmt.Sprintf("%s%s%s (%s)\n", prefix, marker, name, metadata.Description))
	}

	// Calculate prefix for children
	childPrefix := prefix
	if !isRoot {
		if isLast {
			childPrefix += "    "
		} else {
			childPrefix += "│   "
		}
	}

	for i, req := range metadata.Requires {
		buildTreeNode(sb, svc, req, childPrefix, i == len(metadata.Requires)-1, false, visited)
	}

	return nil
}
