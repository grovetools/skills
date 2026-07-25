package browser

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/grovetools/core/config"
	"github.com/grovetools/core/pkg/workspace"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
	skillskeymap "github.com/grovetools/skills/pkg/keymap"
	"github.com/grovetools/skills/pkg/service"
	"github.com/grovetools/skills/pkg/skills"
)

// DisplayNode represents a single row in the tree display.
// It can be either a group header or a skill leaf.
type DisplayNode struct {
	Name                    string
	IsGroup                 bool
	Prefix                  string // Tree prefix (├─, └─, etc.)
	Group                   string // Group name this skill belongs to
	Domain                  string
	Source                  skills.SourceType
	Description             string
	Path                    string
	Workspace               string // Workspace name for workspace-derived skills
	Depth                   int    // Nesting depth (0 for top-level skills, 1+ for sub-skills)
	ParentSkill             string // Name of parent skill if this is a sub-skill
	ConfiguredProject       bool   // Skill is configured in project grove.toml
	ConfiguredEcosystem     bool   // Skill is configured in ecosystem grove.toml
	ConfiguredGlobal        bool   // Skill is configured in global config
	ConfiguredUserProject   bool   // Skill is in user's global config scoped to this project
	ConfiguredUserEcosystem bool   // Skill is in user's global config scoped to this ecosystem
}

// Model represents the skills browser TUI state.
type Model struct {
	service     *service.Service
	config      *config.Config
	currentNode *workspace.WorkspaceNode
	keys        skillskeymap.BrowserKeyMap
	help        *help.Model
	theme       *theme.Theme

	// Display state
	nodes     []DisplayNode
	cursor    int
	width     int
	height    int
	ready     bool
	loading   bool
	statusMsg string
	errorMsg  string

	// View mode
	showAllSkills bool // false = show only configured skills, true = show all

	// Search state
	searching   bool
	filterInput textinput.Model
	filterText  string

	// Right pane viewport
	viewport viewport.Model

	// Pane focus state
	previewFocused bool

	// previewOpen tracks whether the hosted side-split preview (v) is open,
	// so a second v toggles it closed. Cursor movement deliberately does NOT
	// clear it — the preview follows the selection instead (stickyPreviewCmd).
	// Only the user closing it, or the host telling us the split is gone,
	// flips it back; if it desynchronises from the real pane, v inverts and
	// sticky navigation silently stops.
	previewOpen bool

	// previewPath is the SKILL.md the open preview split currently shows. It
	// lets sticky navigation skip re-emitting for a selection that already
	// owns the preview (a clamped page move, a re-render of the same row),
	// and it records which skill the split stays parked on when the cursor
	// lands on a row that has no previewable file.
	previewPath string

	// Cached skill details
	cachedSkillName string
	cachedTree      string
	cachedContent   string
	cachedMetadata  *skills.SkillMetadata

	// Sequence state for multi-key bindings (gg, etc.)
	sequence *keymap.SequenceState

	// hosted is true when running inside treemux; edit emits embed messages
	hosted bool

	// injectMode is the transient multi-select mode I toggles. Its only
	// purpose is composing a batch of "/skill" lines for the agent CLI in the
	// pane the user came from, so it is deliberately not persistent state:
	// dispatching or cancelling drops the whole batch.
	injectMode bool

	// injectSelected holds the picked skill names in the order the user picked
	// them, not in tree order. That order IS the order the host types the
	// lines at the agent's prompt, and a batch reads very differently
	// reversed ("/plan then /review" is not "/review then /plan"). Hence a
	// slice rather than a set: deselecting removes in place, so re-selecting
	// appends to the end and the user can rebuild the order by re-picking.
	injectSelected []string

	// injectPrompts maps a selected skill to the trailing instruction p
	// attached to it. Absent or blank means the line stays a bare "/<name>".
	injectPrompts map[string]string

	// injectPromptEditing/injectPromptTarget/injectPromptInput drive the
	// single-line prompt editor. A second textinput rather than a reuse of
	// filterInput: the two are independently live (the batch survives a
	// search, the search text survives a prompt edit) and sharing one would
	// make either open clobber the other's buffer.
	injectPromptEditing bool
	injectPromptTarget  string
	injectPromptInput   textinput.Model

	// injectPending gates AgentInjectResultMsg. The host broadcasts the result
	// to every panel it hosts, not just the emitter, so an ungated browser
	// would report another panel's delivery — or worse, another panel's
	// failure — in its own status line. Set on dispatch, cleared by the first
	// result that arrives afterwards.
	injectPending bool
}

// New creates a new skills browser model.
func New(svc *service.Service, cfg *config.Config, node *workspace.WorkspaceNode, hosted bool) Model {
	keys := skillskeymap.NewBrowserKeyMap(cfg)
	th := theme.DefaultTheme

	ti := textinput.New()
	ti.Placeholder = "Search skills..."
	ti.CharLimit = 64

	pi := textinput.New()
	pi.Placeholder = "trailing instruction..."
	pi.CharLimit = 256

	helpModel := help.New(keys)
	helpModel.Title = "Skills Browser - Help"
	helpModel.Theme = th

	return Model{
		service:       svc,
		config:        cfg,
		currentNode:   node,
		keys:          keys,
		help:          &helpModel,
		theme:         th,
		loading:       true,
		showAllSkills: false,
		filterInput:   ti,
		sequence:      keymap.NewSequenceState(),
		hosted:        hosted,

		injectPromptInput: pi,
		injectPrompts:     make(map[string]string),
	}
}

// Init initializes the model and starts loading skills.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		loadSkillsCmd(m.service, m.currentNode),
	)
}

// skillsLoadedMsg is sent when skills have been loaded.
type skillsLoadedMsg struct {
	nodes []DisplayNode
	err   error
}

// loadSkillsCmd loads skills asynchronously.
func loadSkillsCmd(svc *service.Service, node *workspace.WorkspaceNode) tea.Cmd {
	return func() tea.Msg {
		nodes, err := buildDisplayNodes(svc, node)
		return skillsLoadedMsg{nodes: nodes, err: err}
	}
}

// skillEntry represents a skill for grouping and display.
type skillEntry struct {
	name        string
	source      skills.SourceType
	desc        string
	path        string
	workspace   string
	domain      string
	group       string
	relPath     string // Relative path from skills dir (e.g., "sear/heat-pan")
	parentSkill string // Parent skill name if nested (e.g., "sear" for "sear/heat-pan")
}

// buildDisplayNodes creates the tree structure for display.
func buildDisplayNodes(svc *service.Service, node *workspace.WorkspaceNode) ([]DisplayNode, error) {
	// Collect all skills from various sources
	allSkills := make([]skillEntry, 0) //nolint:prealloc // size unknown until sources loaded

	// Determine configured sets at each level
	globalSet := make(map[string]bool)
	ecoSet := make(map[string]bool)
	projSet := make(map[string]bool)
	userProjSet := make(map[string]bool) // User-scoped project skills from global config
	userEcoSet := make(map[string]bool)  // User-scoped ecosystem skills from global config

	// Load global config directly from file (not cached svc.Config)
	// to ensure we see updates made by toggle operations
	globalConfigPath := skills.GetGlobalConfigPath()
	if globalConfigPath != "" {
		globalDir := filepath.Dir(globalConfigPath)
		globalCfg, _ := skills.LoadSkillsFromPath(globalDir)
		if globalCfg != nil {
			for _, u := range globalCfg.Use {
				globalSet[u] = true
			}
			for d := range globalCfg.Dependencies {
				globalSet[d] = true
			}

			// User-scoped ecosystem settings from [skills.ecosystems.<name>]
			if node != nil && globalCfg.Ecosystems != nil {
				var ecoName string
				if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
					ecoName = filepath.Base(node.RootEcosystemPath)
				} else if node.IsEcosystem() {
					ecoName = node.Name
				}
				if ecoName != "" {
					if ecoCfg, ok := globalCfg.Ecosystems[ecoName]; ok && ecoCfg != nil {
						for _, u := range ecoCfg.Use {
							userEcoSet[u] = true
						}
						for d := range ecoCfg.Dependencies {
							userEcoSet[d] = true
						}
					}
				}
			}

			// User-scoped project settings from [skills.projects.<name>]
			// Use repository name, not worktree name
			if node != nil && globalCfg.Projects != nil {
				projectName := node.Name
				if node.ParentProjectPath != "" {
					projectName = filepath.Base(node.ParentProjectPath)
				}
				if projCfg, ok := globalCfg.Projects[projectName]; ok && projCfg != nil {
					for _, u := range projCfg.Use {
						userProjSet[u] = true
					}
					for d := range projCfg.Dependencies {
						userProjSet[d] = true
					}
				}
			}
		}
	}

	// Load project and ecosystem configs if we have a workspace node
	if node != nil {
		projCfg, _ := skills.LoadSkillsFromPath(node.Path)
		if projCfg != nil {
			for _, u := range projCfg.Use {
				projSet[u] = true
			}
			for d := range projCfg.Dependencies {
				projSet[d] = true
			}
		}

		if node.RootEcosystemPath != "" && node.RootEcosystemPath != node.Path {
			ecoCfg, _ := skills.LoadSkillsFromPath(node.RootEcosystemPath)
			if ecoCfg != nil {
				for _, u := range ecoCfg.Use {
					ecoSet[u] = true
				}
				for d := range ecoCfg.Dependencies {
					ecoSet[d] = true
				}
			}
		} else if node.IsEcosystem() {
			// Project is the ecosystem, so ecosystem set equals project set
			ecoSet = projSet
		}
	}

	// 1. Get builtin and user skills (non-workspace sources)
	sources := skills.ListSkillSources(svc, nil)
	for name, src := range sources {
		// Skip ecosystem and project sources - we'll get workspace skills separately
		if src.Type == skills.SourceTypeEcosystem || src.Type == skills.SourceTypeProject {
			continue
		}

		domain := "uncategorized"
		desc := ""

		// Get skill content for metadata
		var content []byte
		if loadedSkill, err := skills.LoadSkillFromSource(name, src); err == nil {
			content = loadedSkill.Files["SKILL.md"]
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

		// Determine group based on nested RelPath, domain, or source type
		group := "uncategorized"
		relDir := filepath.Dir(src.RelPath)
		if relDir != "." && relDir != "" {
			group = relDir
		} else if domain != "" && domain != "uncategorized" {
			group = domain
		} else if src.Type == skills.SourceTypeUser {
			group = "User Skills"
		} else if src.Type == skills.SourceTypeBuiltin {
			group = "Built-in Skills"
		}

		allSkills = append(allSkills, skillEntry{
			name:   name,
			source: src.Type,
			desc:   desc,
			path:   src.Path,
			domain: domain,
			group:  group,
		})
	}

	// 2. Get all workspace skills
	workspaceSkills, _ := skills.ListAllWorkspaceSkills(svc)
	for _, ws := range workspaceSkills {
		domain := "uncategorized"
		desc := ws.Description

		// Try to get domain from SKILL.md
		skillMDPath := filepath.Join(ws.Path, "SKILL.md")
		if content, err := os.ReadFile(skillMDPath); err == nil { //nolint:gosec // G304: path from workspace
			if meta, err := skills.ParseSkillFrontmatter(content); err == nil {
				if meta.Domain != "" {
					domain = meta.Domain
				}
				if meta.Description != "" {
					desc = meta.Description
				}
			}
		}

		// Group workspace skills by workspace name; track parent for nested skills
		group := "Workspace Skills"
		if ws.Workspace != "" {
			group = ws.Workspace
		}

		// Determine if this is a sub-skill (nested under a parent skill directory)
		parentSkill := ""
		relDir := filepath.Dir(ws.RelPath)
		if relDir != "." && relDir != "" {
			// This skill is nested (e.g., relPath = "sear/heat-pan", parent = "sear")
			parentSkill = filepath.Base(relDir)
		}

		allSkills = append(allSkills, skillEntry{
			name:        ws.Name,
			source:      skills.SourceTypeProject,
			desc:        desc,
			path:        ws.Path,
			workspace:   ws.Workspace,
			domain:      domain,
			group:       group,
			relPath:     ws.RelPath,
			parentSkill: parentSkill,
		})
	}

	if len(allSkills) == 0 {
		return nil, nil
	}

	// Separate top-level skills from sub-skills
	topLevel := make(map[string][]skillEntry)  // group -> top-level skills
	subSkills := make(map[string][]skillEntry) // parentSkillName -> sub-skills
	for _, s := range allSkills {
		if s.parentSkill != "" {
			subSkills[s.parentSkill] = append(subSkills[s.parentSkill], s)
		} else {
			topLevel[s.group] = append(topLevel[s.group], s)
		}
	}

	// Sort groups
	groups := make([]string, 0, len(topLevel))
	for g := range topLevel {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	// Build flat node list with tree prefixes, nesting sub-skills under parents
	nodes := make([]DisplayNode, 0, len(allSkills)+len(groups))
	for _, groupName := range groups {
		skillList := topLevel[groupName]
		sort.Slice(skillList, func(i, j int) bool {
			return skillList[i].name < skillList[j].name
		})

		// Add group header
		nodes = append(nodes, DisplayNode{
			Name:    groupName,
			IsGroup: true,
		})

		// Add skills with tree prefixes, including nested sub-skills
		for i, s := range skillList {
			children := subSkills[s.name]
			isLastTopLevel := i == len(skillList)-1 && len(children) == 0

			prefix := "├─ "
			if isLastTopLevel || (i == len(skillList)-1 && len(children) > 0) {
				prefix = "└─ "
			}
			nodes = append(nodes, DisplayNode{
				Name:                    s.name,
				IsGroup:                 false,
				Prefix:                  prefix,
				Group:                   s.group,
				Domain:                  s.domain,
				Source:                  s.source,
				Description:             s.desc,
				Path:                    s.path,
				Workspace:               s.workspace,
				Depth:                   0,
				ConfiguredGlobal:        globalSet[s.name],
				ConfiguredEcosystem:     ecoSet[s.name],
				ConfiguredProject:       projSet[s.name],
				ConfiguredUserProject:   userProjSet[s.name],
				ConfiguredUserEcosystem: userEcoSet[s.name],
			})

			// Add sub-skills nested under this parent
			if len(children) > 0 {
				sort.Slice(children, func(a, b int) bool {
					return children[a].name < children[b].name
				})
				for j, child := range children {
					childPrefix := "   ├─ "
					if j == len(children)-1 {
						childPrefix = "   └─ "
					}
					// Adjust prefix based on whether parent was last in group
					if i < len(skillList)-1 {
						childPrefix = "│  " + childPrefix[3:]
					}
					nodes = append(nodes, DisplayNode{
						Name:                    child.name,
						IsGroup:                 false,
						Prefix:                  childPrefix,
						Group:                   s.group, // Same group as parent
						Domain:                  child.domain,
						Source:                  child.source,
						Description:             child.desc,
						Path:                    child.path,
						Workspace:               child.workspace,
						Depth:                   1,
						ParentSkill:             s.name,
						ConfiguredGlobal:        globalSet[child.name] || globalSet[s.name],
						ConfiguredEcosystem:     ecoSet[child.name] || ecoSet[s.name],
						ConfiguredProject:       projSet[child.name] || projSet[s.name],
						ConfiguredUserProject:   userProjSet[child.name] || userProjSet[s.name],
						ConfiguredUserEcosystem: userEcoSet[child.name] || userEcoSet[s.name],
					})
				}
			}
		}
	}

	return nodes, nil
}

// SelectedSkill returns the currently selected skill node, or nil if a group is selected.
func (m *Model) SelectedSkill() *DisplayNode {
	filtered := m.filteredNodes()
	if m.cursor < 0 || m.cursor >= len(filtered) {
		return nil
	}
	node := filtered[m.cursor]
	if node.IsGroup {
		return nil
	}
	return &node
}

// SelectedNode returns the currently selected node (skill or group).
func (m *Model) SelectedNode() *DisplayNode {
	filtered := m.filteredNodes()
	if m.cursor < 0 || m.cursor >= len(filtered) {
		return nil
	}
	node := filtered[m.cursor]
	return &node
}

// injectIndexOf returns the position of a skill in the inject batch, or -1 if
// it is not selected. Position, not a bare bool, because the batch is ordered
// and removals have to splice rather than delete.
func (m *Model) injectIndexOf(name string) int {
	for i, sel := range m.injectSelected {
		if sel == name {
			return i
		}
	}
	return -1
}

// injectLines renders the batch as the lines the agent will see typed at its
// prompt, in selection order. A skill with a trailing instruction becomes
// "/name instruction"; without one it stays a bare "/name", which is what the
// agent CLIs treat as "run this skill with no extra direction".
func (m *Model) injectLines() []string {
	lines := make([]string, 0, len(m.injectSelected))
	for _, name := range m.injectSelected {
		line := "/" + name
		if prompt := strings.TrimSpace(m.injectPrompts[name]); prompt != "" {
			line += " " + prompt
		}
		lines = append(lines, line)
	}
	return lines
}

// GetGroupSkills returns all skills belonging to a group.
func (m *Model) GetGroupSkills(groupName string) []DisplayNode {
	var skills []DisplayNode
	for _, n := range m.nodes {
		if !n.IsGroup && n.Group == groupName {
			skills = append(skills, n)
		}
	}
	return skills
}

// filteredNodes returns nodes that match the current filter and visibility settings.
func (m *Model) filteredNodes() []DisplayNode {
	// Helper to check if a node is visible based on configuration state
	isVisible := func(n DisplayNode) bool {
		return m.showAllSkills || n.ConfiguredProject || n.ConfiguredEcosystem || n.ConfiguredGlobal || n.ConfiguredUserProject || n.ConfiguredUserEcosystem
	}

	// If showing all skills with no filter, return all nodes
	if m.filterText == "" && m.showAllSkills {
		return m.nodes
	}

	var filtered []DisplayNode
	for _, node := range m.nodes {
		// Always include group headers if they have matching children
		if node.IsGroup {
			// Check if any children match
			hasMatch := false
			for _, n := range m.nodes {
				if !n.IsGroup && n.Group == node.Name {
					if matchesFilter(n, m.filterText) && isVisible(n) {
						hasMatch = true
						break
					}
				}
			}
			if hasMatch {
				filtered = append(filtered, node)
			}
		} else if matchesFilter(node, m.filterText) && isVisible(node) {
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

// getLeftPaneWidth dynamically calculates the ideal width for the left tree pane.
func (m *Model) getLeftPaneWidth() int {
	maxWidth := 25
	for _, n := range m.nodes {
		var w int
		if n.IsGroup {
			w = len(n.Name) + 4 // group name + padding
		} else {
			w = len(n.Prefix) + len(n.Name) + 4 // prefix + name + indicator
			// Only show workspace tag if different from group
			if n.Workspace != "" && n.Workspace != n.Group {
				w += len(" ["+n.Workspace+"]") + 2
			}
			// Buffer for configuration icons
			if n.ConfiguredProject || n.ConfiguredEcosystem || n.ConfiguredGlobal || n.ConfiguredUserProject || n.ConfiguredUserEcosystem {
				w += 12 // Space for icons
			}
		}
		if w > maxWidth {
			maxWidth = w
		}
	}

	// Inject mode prefixes every row with a 2-cell selection marker. Widen the
	// estimate to match, so entering the mode doesn't start truncating names
	// that fit a moment earlier — the tree visibly shrinking on I would read
	// as a bug rather than as a mode change.
	if m.injectMode {
		maxWidth += 2
	}

	maxWidth += 4 // extra padding

	// Desired width: content-driven, but never more than 40% of the panel.
	result := maxWidth
	if maxAllowed := m.width * 40 / 100; result > maxAllowed {
		result = maxAllowed
	}

	// Never crowd out the right pane. Reserve minRight columns (plus the
	// 1-col divider) for it so the right pane width can't go negative at
	// narrow total widths — the old hard 30-col floor pinned the left pane
	// wider than the whole panel.
	const (
		minLeft  = 10
		minRight = 20
	)
	if cap := m.width - minRight - 1; result > cap {
		result = cap
	}
	if result < minLeft {
		result = minLeft
	}
	// Final safety: never equal or exceed the panel width.
	if result > m.width-1 {
		result = m.width - 1
	}
	if result < 1 {
		result = 1
	}
	return result
}
