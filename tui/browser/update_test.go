package browser

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

	// enter / edit must emit a rail EditRequestMsg, NOT a SplitEditorRequestMsg.
	for _, msgIn := range []tea.KeyMsg{keyRune('e'), {Type: tea.KeyEnter}} {
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
