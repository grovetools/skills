package browser

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
	skillskeymap "github.com/grovetools/skills/pkg/keymap"
	"github.com/grovetools/skills/pkg/service"
	"github.com/grovetools/skills/pkg/skills"
)

// DisplayNode represents a single row in the tree display.
// It can be either a domain header or a skill leaf.
type DisplayNode struct {
	Name        string
	IsDomain    bool
	Prefix      string // Tree prefix (├─, └─, etc.)
	Domain      string
	Source      skills.SourceType
	Description string
	Path        string
}

// Model represents the skills browser TUI state.
type Model struct {
	service *service.Service
	config  *config.Config
	keys    skillskeymap.BrowserKeyMap
	help    *help.Model
	theme   *theme.Theme

	// Display state
	nodes       []DisplayNode
	cursor      int
	width       int
	height      int
	ready       bool
	loading     bool
	statusMsg   string
	errorMsg    string

	// Search state
	searching   bool
	filterInput textinput.Model
	filterText  string

	// Right pane viewport
	viewport viewport.Model

	// Cached skill details
	cachedSkillName string
	cachedTree      string
	cachedContent   string

	// Sequence state for multi-key bindings (gg, etc.)
	sequence *keymap.SequenceState
}

// New creates a new skills browser model.
func New(svc *service.Service, cfg *config.Config) Model {
	keys := skillskeymap.NewBrowserKeyMap(cfg)
	th := theme.DefaultTheme

	ti := textinput.New()
	ti.Placeholder = "Search skills..."
	ti.CharLimit = 64

	helpModel := help.New(keys)
	helpModel.Title = "Skills Browser - Help"
	helpModel.Theme = th

	return Model{
		service:     svc,
		config:      cfg,
		keys:        keys,
		help:        &helpModel,
		theme:       th,
		loading:     true,
		filterInput: ti,
		sequence:    keymap.NewSequenceState(),
	}
}

// Init initializes the model and starts loading skills.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadSkillsCmd(m.service),
	)
}

// skillsLoadedMsg is sent when skills have been loaded.
type skillsLoadedMsg struct {
	nodes []DisplayNode
	err   error
}

// loadSkillsCmd loads skills asynchronously.
func loadSkillsCmd(svc *service.Service) tea.Cmd {
	return func() tea.Msg {
		nodes, err := buildDisplayNodes(svc)
		return skillsLoadedMsg{nodes: nodes, err: err}
	}
}

// buildDisplayNodes creates the tree structure for display.
func buildDisplayNodes(svc *service.Service) ([]DisplayNode, error) {
	sources := skills.ListSkillSources(svc, nil)
	if len(sources) == 0 {
		return nil, nil
	}

	// Group skills by domain
	domainSkills := make(map[string][]struct {
		name   string
		source skills.SkillSource
		desc   string
	})

	for name, src := range sources {
		domain := "uncategorized"
		desc := ""

		// Get skill content for metadata
		var content []byte
		if src.Type == skills.SourceTypeBuiltin {
			files, err := skills.GetSkill(name)
			if err == nil {
				content = files["SKILL.md"]
			}
		} else {
			data, err := os.ReadFile(filepath.Join(src.Path, "SKILL.md"))
			if err == nil {
				content = data
			}
		}

		if content != nil {
			meta, err := skills.ParseSkillFrontmatter(content)
			if err == nil {
				if meta.Domain != "" {
					domain = meta.Domain
				}
				desc = meta.Description
			}
		}

		domainSkills[domain] = append(domainSkills[domain], struct {
			name   string
			source skills.SkillSource
			desc   string
		}{name: name, source: src, desc: desc})
	}

	// Sort domains
	var domains []string
	for d := range domainSkills {
		domains = append(domains, d)
	}
	sort.Strings(domains)

	// Build flat node list with tree prefixes
	var nodes []DisplayNode
	for _, domain := range domains {
		skillList := domainSkills[domain]
		sort.Slice(skillList, func(i, j int) bool {
			return skillList[i].name < skillList[j].name
		})

		// Add domain header
		nodes = append(nodes, DisplayNode{
			Name:     domain,
			IsDomain: true,
		})

		// Add skills with tree prefixes
		for i, s := range skillList {
			prefix := "├─ "
			if i == len(skillList)-1 {
				prefix = "└─ "
			}
			nodes = append(nodes, DisplayNode{
				Name:        s.name,
				IsDomain:    false,
				Prefix:      prefix,
				Domain:      domain,
				Source:      s.source.Type,
				Description: s.desc,
				Path:        s.source.Path,
			})
		}
	}

	return nodes, nil
}

// SelectedSkill returns the currently selected skill node, or nil if a domain is selected.
func (m *Model) SelectedSkill() *DisplayNode {
	if m.cursor < 0 || m.cursor >= len(m.nodes) {
		return nil
	}
	node := &m.nodes[m.cursor]
	if node.IsDomain {
		return nil
	}
	return node
}

// filteredNodes returns nodes that match the current filter.
func (m *Model) filteredNodes() []DisplayNode {
	if m.filterText == "" {
		return m.nodes
	}

	var filtered []DisplayNode
	for _, node := range m.nodes {
		// Always include domain headers if they have matching children
		if node.IsDomain {
			// Check if any children match
			hasMatch := false
			for _, n := range m.nodes {
				if !n.IsDomain && n.Domain == node.Name {
					if matchesFilter(n, m.filterText) {
						hasMatch = true
						break
					}
				}
			}
			if hasMatch {
				filtered = append(filtered, node)
			}
		} else if matchesFilter(node, m.filterText) {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

// matchesFilter checks if a node matches the search filter.
func matchesFilter(node DisplayNode, filter string) bool {
	if filter == "" {
		return true
	}
	// Case-insensitive search in name, domain, and description
	lowerFilter := filter
	return contains(node.Name, lowerFilter) ||
		contains(node.Domain, lowerFilter) ||
		contains(node.Description, lowerFilter)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && containsLower(s, substr)))
}

func containsLower(s, substr string) bool {
	// Simple case-insensitive contains
	sl := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			sl[i] = c + 32
		} else {
			sl[i] = c
		}
	}
	subl := make([]byte, len(substr))
	for i := 0; i < len(substr); i++ {
		c := substr[i]
		if c >= 'A' && c <= 'Z' {
			subl[i] = c + 32
		} else {
			subl[i] = c
		}
	}
	return bytesContains(sl, subl)
}

func bytesContains(s, substr []byte) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
