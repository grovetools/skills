package browser

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
	skillskeymap "github.com/grovetools/skills/pkg/keymap"
	"github.com/grovetools/skills/pkg/skills"
)

// newTestModel builds a minimal browser Model suitable for exercising the
// keybinding dispatch and rendering without a real service. It seeds one
// non-builtin (workspace) skill and one builtin skill.
func newTestModel(hosted bool) Model {
	keys := skillskeymap.NewBrowserKeyMap(nil)
	h := help.New(keys)
	return Model{
		keys:          keys,
		help:          &h,
		theme:         theme.DefaultTheme,
		sequence:      keymap.NewSequenceState(),
		hosted:        hosted,
		showAllSkills: true, // so filteredNodes returns nodes verbatim
		nodes: []DisplayNode{
			{Name: "wsskill", Source: skills.SourceTypeProject, Path: "/tmp/wsskill"},
			{Name: "builtinskill", Source: skills.SourceTypeBuiltin},
		},
	}
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// msgFromCmd executes a tea.Cmd and returns the produced message, or nil.
func msgFromCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestHostedPreviewToggle(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0 // wsskill (non-builtin)

	// First v opens a non-focused side split.
	updated, cmd := m.handleKeyMsg(keyRune('v'))
	m = updated.(Model)
	if !m.previewOpen {
		t.Fatal("expected previewOpen to be true after first v")
	}
	msg := msgFromCmd(cmd)
	open, ok := msg.(embed.SplitEditorRequestMsg)
	if !ok {
		t.Fatalf("expected SplitEditorRequestMsg, got %T", msg)
	}
	if open.Focus {
		t.Error("expected Focus to be false so the tree keeps focus")
	}
	if open.Ratio != 0.35 {
		t.Errorf("expected Ratio 0.35, got %v", open.Ratio)
	}
	if !strings.HasSuffix(open.Path, "/SKILL.md") {
		t.Errorf("expected path ending in SKILL.md, got %q", open.Path)
	}

	// Second v on the same skill closes the split.
	updated, cmd = m.handleKeyMsg(keyRune('v'))
	m = updated.(Model)
	if m.previewOpen {
		t.Fatal("expected previewOpen to be false after second v")
	}
	if _, ok := msgFromCmd(cmd).(embed.SplitEditorCloseRequestMsg); !ok {
		t.Fatalf("expected SplitEditorCloseRequestMsg on second v, got %T", msgFromCmd(cmd))
	}
}

func TestHostedEditOpensRailEditor(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0 // wsskill (non-builtin)

	// enter / space / edit must emit a rail EditRequestMsg, NOT a
	// SplitEditorRequestMsg (that is v's job).
	for _, msgIn := range []tea.KeyMsg{keyRune('e'), keyRune(' '), {Type: tea.KeyEnter}} {
		_, cmd := m.handleKeyMsg(msgIn)
		out := msgFromCmd(cmd)
		if _, ok := out.(embed.EditRequestMsg); !ok {
			t.Fatalf("key %v: expected EditRequestMsg, got %T", msgIn, out)
		}
		if _, ok := out.(embed.SplitEditorRequestMsg); ok {
			t.Fatalf("key %v: edit should not emit SplitEditorRequestMsg", msgIn)
		}
	}
}

// TestHostedOpenTargets pins the three-way split the skills browser shares
// with nb and flow: space/enter pin SKILL.md to its own treemux pane
// (Dedicated), e replaces the buffer in the host's singleton Editor pane.
func TestHostedOpenTargets(t *testing.T) {
	cases := []struct {
		name          string
		key           tea.KeyMsg
		wantDedicated bool
	}{
		{"space opens a dedicated pane", keyRune(' '), true},
		{"enter opens a dedicated pane", tea.KeyMsg{Type: tea.KeyEnter}, true},
		{"e quick-opens in the singleton editor", keyRune('e'), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(true)
			m.cursor = 0 // wsskill (non-builtin)
			_, cmd := m.handleKeyMsg(tc.key)
			out := msgFromCmd(cmd)
			req, ok := out.(embed.EditRequestMsg)
			if !ok {
				t.Fatalf("expected EditRequestMsg, got %T", out)
			}
			if req.Dedicated != tc.wantDedicated {
				t.Errorf("Dedicated = %v, want %v", req.Dedicated, tc.wantDedicated)
			}
			if !strings.HasSuffix(req.Path, "/SKILL.md") {
				t.Errorf("expected path ending in SKILL.md, got %q", req.Path)
			}
		})
	}
}

func TestBuiltinSkillCannotEditOrPreview(t *testing.T) {
	for _, key := range []tea.KeyMsg{keyRune('v'), keyRune('e'), {Type: tea.KeyEnter}} {
		m := newTestModel(true)
		m.cursor = 1 // builtinskill
		updated, cmd := m.handleKeyMsg(key)
		m = updated.(Model)
		if msg := msgFromCmd(cmd); msg != nil {
			t.Fatalf("key %v: builtin skill should emit no editor message, got %T", key, msg)
		}
		if !strings.Contains(strings.ToLower(m.statusMsg), "cannot") {
			t.Fatalf("key %v: expected a 'cannot' status, got %q", key, m.statusMsg)
		}
		if m.previewOpen {
			t.Fatalf("key %v: builtin skill must not open preview", key)
		}
	}
}

func TestStandalonePreviewFallsBackToEdit(t *testing.T) {
	m := newTestModel(false) // standalone
	m.cursor = 0
	_, cmd := m.handleKeyMsg(keyRune('v'))
	if cmd == nil {
		t.Fatal("expected a non-nil cmd (editSkillCmd fallback) in standalone mode")
	}
	if m.previewOpen {
		t.Error("standalone mode should not set previewOpen")
	}
}

// TestNarrowWidthRenderNoPanic ensures the two-pane layout survives very
// narrow widths without panicking on negative lipgloss widths.
func TestNarrowWidthRenderNoPanic(t *testing.T) {
	for _, w := range []int{20, 40, 80} {
		m := newTestModel(false)
		m.loading = false
		updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		m = updated.(Model)
		out := m.View()
		if out == "" {
			t.Errorf("width %d: View() produced empty output", w)
		}
	}
}

// maxLineWidth returns the widest visible (ANSI-aware) line in a rendered
// block, ignoring trailing whitespace which does not affect wrapping.
func maxLineWidth(s string) int {
	maxW := 0
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(strings.TrimRight(line, " ")); w > maxW {
			maxW = w
		}
	}
	return maxW
}

func lineCount(s string) int { return len(strings.Split(s, "\n")) }

// TestDetailPaneTruncatesInsteadOfWrapping is the core regression guard for the
// ticket: at narrow widths the detail pane must TRUNCATE long lines (with an
// ellipsis) rather than soft-wrap them into many reflowed lines. We render the
// pane once with a short preview line and once with a pathologically long
// single line; truncation adds at most one output line, whereas the old
// width-wrapping reflow ballooned a single line into ~a dozen.
func TestDetailPaneTruncatesInsteadOfWrapping(t *testing.T) {
	for _, w := range []int{40, 60} {
		base := newTestModel(false)
		base.loading = false
		updated, _ := base.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		base = updated.(Model)
		base.cursor = 0
		contentWidth := base.viewport.Width - 2

		short := base
		short.cachedContent = "short preview line"
		shortOut := short.renderSkillDetails(&short.nodes[0])

		long := base
		long.cachedContent = strings.Repeat("x", contentWidth*12) // would wrap to ~12 lines
		longOut := long.renderSkillDetails(&long.nodes[0])

		if delta := lineCount(longOut) - lineCount(shortOut); delta > 1 {
			t.Errorf("width %d: a single long preview line added %d rendered lines (want <=1); it is wrapping, not truncating", w, delta)
		}
		if !strings.Contains(longOut, "…") {
			t.Errorf("width %d: expected a truncation ellipsis in the detail pane", w)
		}
		if got := maxLineWidth(longOut); got > contentWidth {
			t.Errorf("width %d: detail line width %d exceeds content width %d", w, got, contentWidth)
		}
	}
}

// TestNarrowWidthNoLineExceedsPanel renders the full browser (header + two
// panes) and the footer at narrow widths with long content and asserts no
// rendered line exceeds the panel width, so nothing overflows the terminal and
// forces a hard wrap. This guards the header/footer/left-pane paths, which are
// joined with JoinVertical and are not otherwise width-bounded.
func TestNarrowWidthNoLineExceedsPanel(t *testing.T) {
	longDesc := strings.Repeat("verylongtoken ", 60)
	longPath := "/very/long/absolute/path/" + strings.Repeat("segment/", 40) + "SKILL.md"

	for _, w := range []int{20, 40, 60} {
		m := newTestModel(false)
		m.loading = false
		m.nodes[0].Description = longDesc
		m.nodes[0].Path = longPath
		m.nodes[0].Workspace = strings.Repeat("ws", 40)

		updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		m = updated.(Model)
		m.cursor = 0
		// Exercise the search footer path (long filter text) and a long preview.
		m.searching = true
		m.filterInput.SetValue(strings.Repeat("query", 20))
		m.cachedContent = strings.Repeat("x", 500) + "\n" + longDesc
		m.viewport.SetContent(m.renderSkillDetails(&m.nodes[0]))

		if got := maxLineWidth(m.View()); got > w {
			t.Errorf("width %d: View() has a line of width %d (must be <= %d)", w, got, w)
		}
		if got := maxLineWidth(m.FooterView()); got > w {
			t.Errorf("width %d: FooterView() has a line of width %d (must be <= %d)", w, got, w)
		}

		// Group details path.
		m.nodes = append([]DisplayNode{{Name: strings.Repeat("group", 20), IsGroup: true}}, m.nodes...)
		m.viewport.SetContent(m.renderGroupDetails(&m.nodes[0]))
		if got := maxLineWidth(m.View()); got > w {
			t.Errorf("width %d: group View() has a line of width %d (must be <= %d)", w, got, w)
		}
	}
}

func TestGetLeftPaneWidthNeverExceedsPanel(t *testing.T) {
	for _, w := range []int{10, 20, 30, 40, 80, 120} {
		m := newTestModel(false)
		m.width = w
		lw := m.getLeftPaneWidth()
		if lw >= w {
			t.Errorf("width %d: left pane %d must be < total width", w, lw)
		}
		if lw < 1 {
			t.Errorf("width %d: left pane %d must be >= 1", w, lw)
		}
	}
}
