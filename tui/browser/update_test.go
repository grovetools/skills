package browser

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
	skillskeymap "github.com/grovetools/skills/pkg/keymap"
	"github.com/grovetools/skills/pkg/skills"
	tuimuxmsg "github.com/grovetools/tuimux/messages"
)

// newTestModel builds a minimal browser Model suitable for exercising the
// keybinding dispatch and rendering without a real service. It seeds one
// non-builtin (workspace) skill and one builtin skill.
func newTestModel(hosted bool) Model {
	keys := skillskeymap.NewBrowserKeyMap(nil)
	h := help.New(keys)
	return Model{
		keys:              keys,
		help:              &h,
		theme:             theme.DefaultTheme,
		sequence:          keymap.NewSequenceState(),
		hosted:            hosted,
		filterInput:       textinput.New(),
		injectPromptInput: textinput.New(),
		showAllSkills:     true, // so filteredNodes returns nodes verbatim
		nodes: []DisplayNode{
			{Name: "wsskill", Source: skills.SourceTypeProject, Path: "/tmp/wsskill"},
			{Name: "builtinskill", Source: skills.SourceTypeBuiltin},
			{Name: "othergroup", IsGroup: true},
			{Name: "wsskill2", Source: skills.SourceTypeProject, Path: "/tmp/wsskill2"},
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
	// An even split: the tree narrows itself to fit, the editor cannot.
	if open.Ratio != 0.5 {
		t.Errorf("expected Ratio 0.5, got %v", open.Ratio)
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

// openPreview presses v on the currently selected row and asserts the split
// actually opened, returning the resulting model.
func openPreview(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.handleKeyMsg(keyRune('v'))
	m = updated.(Model)
	if !m.previewOpen {
		t.Fatal("setup: expected v to open the preview")
	}
	if _, ok := msgFromCmd(cmd).(embed.SplitEditorRequestMsg); !ok {
		t.Fatalf("setup: expected SplitEditorRequestMsg from v, got %T", msgFromCmd(cmd))
	}
	return m
}

// TestStickyPreviewFollowsCursor is the core sticky-navigation guard: while the
// preview split is open, moving onto another previewable skill must re-emit a
// SplitEditorRequestMsg for THAT skill with Ratio 0, so the host retargets the
// existing split in place instead of respawning it at a fresh geometry.
func TestStickyPreviewFollowsCursor(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0 // wsskill
	m = openPreview(t, m)

	// Jump to the other previewable skill (index 3); index 1 is a builtin and
	// index 2 is a group header, both covered separately below.
	m.cursor = 2
	updated, cmd := m.handleKeyMsg(keyRune('j'))
	m = updated.(Model)
	if !m.previewOpen {
		t.Error("cursor movement must not clear previewOpen — the split is still up")
	}
	req, ok := msgFromCmd(cmd).(embed.SplitEditorRequestMsg)
	if !ok {
		t.Fatalf("expected a sticky SplitEditorRequestMsg on cursor move, got %T", msgFromCmd(cmd))
	}
	if req.Ratio != 0 {
		t.Errorf("sticky re-emit must send Ratio 0 (preserve geometry), got %v", req.Ratio)
	}
	if req.Focus {
		t.Error("sticky re-emit must not steal focus from the tree")
	}
	if !strings.HasPrefix(req.Path, "/tmp/wsskill2/") {
		t.Errorf("preview should have followed to wsskill2, got %q", req.Path)
	}
}

// TestStickyPreviewSilentWhenClosed guards the other half: with no preview
// open, cursor movement must stay silent rather than opening one.
func TestStickyPreviewSilentWhenClosed(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0
	_, cmd := m.handleKeyMsg(keyRune('j'))
	if msg := msgFromCmd(cmd); msg != nil {
		t.Fatalf("cursor movement with no preview open must emit nothing, got %T", msg)
	}
}

// TestStickyPreviewParksOnUnpreviewableRows pins the deliberate choice for rows
// with no SKILL.md behind them: the split stays on the last previewable skill
// rather than closing, matching what v itself does on a builtin.
func TestStickyPreviewParksOnUnpreviewableRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cursor int
	}{
		{"builtin skill", 1},
		{"group header", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(true)
			m.cursor = 0
			m = openPreview(t, m)

			m.cursor = tc.cursor - 1
			updated, cmd := m.handleKeyMsg(keyRune('j'))
			m = updated.(Model)
			if m.cursor != tc.cursor {
				t.Fatalf("setup: cursor = %d, want %d", m.cursor, tc.cursor)
			}
			if msg := msgFromCmd(cmd); msg != nil {
				t.Errorf("expected no message for an unpreviewable row, got %T", msg)
			}
			if !m.previewOpen {
				t.Error("preview must stay open (parked) on an unpreviewable row")
			}
			if !strings.HasPrefix(m.previewPath, "/tmp/wsskill/") {
				t.Errorf("preview should still be parked on wsskill, got %q", m.previewPath)
			}

			// v here refuses without touching the open split, so the toggle
			// must still read "open" and close on the next v over a skill.
			updated, cmd = m.handleKeyMsg(keyRune('v'))
			m = updated.(Model)
			if !m.previewOpen {
				t.Error("v on an unpreviewable row must not clear previewOpen")
			}
			if msg := msgFromCmd(cmd); msg != nil {
				t.Errorf("v on an unpreviewable row must emit nothing, got %T", msg)
			}
		})
	}
}

// TestStickyPreviewSkipsRedundantReemit ensures a cursor move that lands back
// on the skill already in the split does not churn the host with a request.
func TestStickyPreviewSkipsRedundantReemit(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0
	m = openPreview(t, m)

	// k at the top of the list is a no-op move; so is re-selecting the same row.
	_, cmd := m.handleKeyMsg(keyRune('k'))
	if msg := msgFromCmd(cmd); msg != nil {
		t.Errorf("no-op cursor move must not re-emit, got %T", msg)
	}

	// A selection re-render that lands on the skill already in the split
	// (a list reload that didn't move the cursor) is a no-op for the host.
	if cmd := m.selectionChanged(); cmd != nil {
		t.Errorf("re-selecting the skill already previewed must not re-emit, got %T", msgFromCmd(cmd))
	}
}

// TestHostClosedSplitResyncsToggle covers the state-honesty case: when the user
// closes the preview from the host side (leader-x, or the editor exiting),
// treemux notifies the origin panel with DetailPaneClosedMsg. If skills ignored
// it, previewOpen would stay true and the next v would emit a close for a pane
// that no longer exists instead of reopening.
func TestHostClosedSplitResyncsToggle(t *testing.T) {
	for _, closeMsg := range []tea.Msg{
		tuimuxmsg.DetailPaneClosedMsg{DetailPanelID: "split-editor-1-SKILL.md", OriginPanelID: "skills-1"},
		embed.SplitEditorClosedMsg{Path: "/tmp/wsskill/SKILL.md"},
	} {
		m := newTestModel(true)
		m.cursor = 0
		m = openPreview(t, m)

		updated, _ := m.Update(closeMsg)
		m = updated.(Model)
		if m.previewOpen {
			t.Fatalf("%T: previewOpen must be false once the host closed the split", closeMsg)
		}
		if m.previewPath != "" {
			t.Errorf("%T: previewPath must be cleared, got %q", closeMsg, m.previewPath)
		}

		// Sticky navigation must go quiet, and v must OPEN rather than close.
		_, cmd := m.handleKeyMsg(keyRune('j'))
		if msg := msgFromCmd(cmd); msg != nil {
			t.Errorf("%T: sticky nav must stop after the split is gone, got %T", closeMsg, msg)
		}
		updated, cmd = m.handleKeyMsg(keyRune('v'))
		m = updated.(Model)
		if !m.previewOpen {
			t.Errorf("%T: v must reopen after a host-side close", closeMsg)
		}
		if _, ok := msgFromCmd(cmd).(embed.SplitEditorRequestMsg); !ok {
			t.Errorf("%T: v must emit an open request, got %T", closeMsg, msgFromCmd(cmd))
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

// enterInject presses I and asserts the browser actually entered inject mode,
// returning the resulting model.
func enterInject(t *testing.T, m Model) Model {
	t.Helper()
	updated, _ := m.handleKeyMsg(keyRune('I'))
	m = updated.(Model)
	if !m.injectMode {
		t.Fatal("setup: expected I to enter inject mode")
	}
	return m
}

// pressInject sends a key through handleKeyMsg and returns the new model plus
// whatever message the resulting cmd produced.
func pressInject(m Model, msg tea.KeyMsg) (Model, tea.Msg) {
	updated, cmd := m.handleKeyMsg(msg)
	return updated.(Model), msgFromCmd(cmd)
}

// TestInjectModeRequiresHost pins the two answers to I: hosted it opens the
// mode, standalone it explains why it can't rather than stranding the user in
// a mode whose enter key could never reach an agent.
func TestInjectModeRequiresHost(t *testing.T) {
	hostedModel := newTestModel(true)
	updated, cmd := hostedModel.handleKeyMsg(keyRune('I'))
	hostedModel = updated.(Model)
	if !hostedModel.injectMode {
		t.Error("hosted: I must enter inject mode")
	}
	if msg := msgFromCmd(cmd); msg != nil {
		t.Errorf("hosted: entering inject mode must emit nothing, got %T", msg)
	}

	standalone := newTestModel(false)
	updated, _ = standalone.handleKeyMsg(keyRune('I'))
	standalone = updated.(Model)
	if standalone.injectMode {
		t.Error("standalone: I must not enter inject mode")
	}
	if !strings.Contains(standalone.statusMsg, "treemux-hosted") {
		t.Errorf("standalone: expected an explanatory status, got %q", standalone.statusMsg)
	}
}

// TestInjectSelectionOrderPreserved is the core selection guard: the batch is
// an ordered list, not a set, because its order is the order the host types
// the lines at the agent's prompt.
func TestInjectSelectionOrderPreserved(t *testing.T) {
	m := enterInject(t, newTestModel(true))

	// Select wsskill2 (index 3) first, then wsskill (index 0).
	m.cursor = 3
	m, _ = pressInject(m, keyRune(' '))
	m.cursor = 0
	m, _ = pressInject(m, keyRune(' '))
	if got, want := m.injectSelected, []string{"wsskill2", "wsskill"}; !equalStrings(got, want) {
		t.Fatalf("selection = %v, want %v", got, want)
	}

	// Deselecting the first pick and re-selecting it must move it to the END:
	// re-picking is how a user rebuilds the send order.
	m.cursor = 3
	m, _ = pressInject(m, keyRune(' '))
	if got, want := m.injectSelected, []string{"wsskill"}; !equalStrings(got, want) {
		t.Fatalf("after deselect: selection = %v, want %v", got, want)
	}
	m, _ = pressInject(m, keyRune(' '))
	if got, want := m.injectSelected, []string{"wsskill", "wsskill2"}; !equalStrings(got, want) {
		t.Fatalf("after reselect: selection = %v, want %v", got, want)
	}
}

// TestInjectSelectionRefusesUnsendableRows covers the two rows that have no
// "/name" behind them: builtins (embedded, never installed for the agent) and
// group headers (display scaffolding).
func TestInjectSelectionRefusesUnsendableRows(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cursor     int
		wantStatus string
	}{
		{"builtin skill", 1, "builtin"},
		{"group header", 2, "Groups"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := enterInject(t, newTestModel(true))
			m.cursor = tc.cursor
			m, _ = pressInject(m, keyRune(' '))
			if len(m.injectSelected) != 0 {
				t.Errorf("selection = %v, want empty", m.injectSelected)
			}
			if !strings.Contains(m.statusMsg, tc.wantStatus) {
				t.Errorf("status = %q, want it to mention %q", m.statusMsg, tc.wantStatus)
			}
		})
	}
}

// TestSpaceOutsideInjectModeStillOpensPane is the regression guard for the
// overload: space only means "select" inside the mode. Everywhere else it must
// still pin SKILL.md to its own treemux pane.
func TestSpaceOutsideInjectModeStillOpensPane(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0
	_, out := pressInject(m, keyRune(' '))
	req, ok := out.(embed.EditRequestMsg)
	if !ok {
		t.Fatalf("expected EditRequestMsg outside inject mode, got %T", out)
	}
	if !req.Dedicated {
		t.Error("space outside inject mode must still request a dedicated pane")
	}
}

// TestInjectNavigationStillWorks pins the overlay contract: keys the mode does
// not claim fall through to the normal keymap, so the tree still navigates and
// filters while a batch is being assembled.
func TestInjectNavigationStillWorks(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m, _ = pressInject(m, keyRune('j'))
	if m.cursor != 1 {
		t.Errorf("j must still move the cursor in inject mode, cursor = %d", m.cursor)
	}
	m, _ = pressInject(m, keyRune('/'))
	if !m.searching {
		t.Error("/ must still open search in inject mode")
	}
	if !m.injectMode {
		t.Error("navigation keys must not drop out of inject mode")
	}
}

// TestInjectPromptAttachesInstruction walks the p flow end to end: open the
// editor, type, commit, dispatch — and assert the line the host receives is
// "/name instruction".
func TestInjectPromptAttachesInstruction(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m.cursor = 0

	// p selects implicitly and opens the editor.
	m, _ = pressInject(m, keyRune('p'))
	if !m.injectPromptEditing {
		t.Fatal("p must open the prompt editor")
	}
	if m.injectPromptTarget != "wsskill" {
		t.Errorf("prompt target = %q, want wsskill", m.injectPromptTarget)
	}
	if got, want := m.injectSelected, []string{"wsskill"}; !equalStrings(got, want) {
		t.Errorf("p must select implicitly: selection = %v, want %v", got, want)
	}

	// The editor owns every key, so typing routes through Update.
	for _, r := range "be brief" {
		updated, _ := m.Update(keyRune(r))
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.injectPromptEditing {
		t.Fatal("Enter must close the prompt editor")
	}
	if !m.injectMode {
		t.Fatal("committing a prompt must not leave inject mode")
	}
	if got := m.injectPrompts["wsskill"]; got != "be brief" {
		t.Fatalf("stored prompt = %q, want %q", got, "be brief")
	}

	_, out := pressInject(m, tea.KeyMsg{Type: tea.KeyEnter})
	req, ok := out.(embed.AgentInjectRequestMsg)
	if !ok {
		t.Fatalf("expected AgentInjectRequestMsg, got %T", out)
	}
	if got, want := req.Lines, []string{"/wsskill be brief"}; !equalStrings(got, want) {
		t.Errorf("lines = %v, want %v", got, want)
	}
}

// TestInjectPromptEscKeepsBatch pins the narrow scope of Esc inside the prompt
// editor: it abandons the edit, not the mode and not the selection.
func TestInjectPromptEscKeepsBatch(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m.cursor = 0
	m, _ = pressInject(m, keyRune('p'))
	for _, r := range "oops" {
		updated, _ := m.Update(keyRune(r))
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(Model)

	if m.injectPromptEditing {
		t.Error("Esc must close the prompt editor")
	}
	if !m.injectMode {
		t.Error("Esc in the prompt editor must not leave inject mode")
	}
	if got, want := m.injectSelected, []string{"wsskill"}; !equalStrings(got, want) {
		t.Errorf("selection = %v, want %v", got, want)
	}
	if got, ok := m.injectPrompts["wsskill"]; ok {
		t.Errorf("cancelled edit must store nothing, got %q", got)
	}
}

// TestInjectDispatch is the payload guard: enter sends one line per selected
// skill, in selection order, asks for focus, and leaves the mode clean.
func TestInjectDispatch(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m.cursor = 3
	m, _ = pressInject(m, keyRune(' '))
	m.cursor = 0
	m, _ = pressInject(m, keyRune(' '))

	m, out := pressInject(m, tea.KeyMsg{Type: tea.KeyEnter})
	req, ok := out.(embed.AgentInjectRequestMsg)
	if !ok {
		t.Fatalf("expected AgentInjectRequestMsg, got %T", out)
	}
	if got, want := req.Lines, []string{"/wsskill2", "/wsskill"}; !equalStrings(got, want) {
		t.Errorf("lines = %v, want %v (selection order)", got, want)
	}
	if !req.Focus {
		t.Error("dispatch must ask the host to focus the agent pane")
	}
	if m.injectMode {
		t.Error("dispatch must leave inject mode")
	}
	if len(m.injectSelected) != 0 || len(m.injectPrompts) != 0 {
		t.Errorf("dispatch must clear the batch, got %v / %v", m.injectSelected, m.injectPrompts)
	}
	if !m.injectPending {
		t.Error("dispatch must arm injectPending so the result is accepted")
	}
}

// TestInjectDispatchWithEmptySelectionStays guards the impatient-enter case:
// nothing is sent and the mode survives, so the user keeps the prompt edits
// they were about to make.
func TestInjectDispatchWithEmptySelectionStays(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m, out := pressInject(m, tea.KeyMsg{Type: tea.KeyEnter})
	if out != nil {
		t.Fatalf("empty selection must emit nothing, got %T", out)
	}
	if !m.injectMode {
		t.Error("empty enter must stay in inject mode")
	}
	if m.injectPending {
		t.Error("empty enter must not arm injectPending")
	}
	if m.statusMsg == "" {
		t.Error("empty enter should explain that nothing is selected")
	}
}

// TestInjectCancelClearsBatch covers both cancel keys: esc and a second I.
func TestInjectCancelClearsBatch(t *testing.T) {
	for _, cancel := range []tea.KeyMsg{{Type: tea.KeyEscape}, keyRune('I')} {
		m := enterInject(t, newTestModel(true))
		m.cursor = 0
		m, _ = pressInject(m, keyRune('p'))
		updated, _ := m.Update(keyRune('x'))
		m = updated.(Model)
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(Model)

		m, out := pressInject(m, cancel)
		if out != nil {
			t.Errorf("%v: cancel must emit nothing, got %T", cancel, out)
		}
		if m.injectMode {
			t.Errorf("%v: cancel must leave inject mode", cancel)
		}
		if len(m.injectSelected) != 0 {
			t.Errorf("%v: cancel must clear the selection, got %v", cancel, m.injectSelected)
		}
		if len(m.injectPrompts) != 0 {
			t.Errorf("%v: cancel must clear the prompts, got %v", cancel, m.injectPrompts)
		}
	}
}

// TestInjectResultIsGatedByPending is the broadcast guard: the host fans the
// result out to every panel, so a browser that never dispatched must ignore it
// rather than reporting a neighbour's failure as its own.
func TestInjectResultIsGatedByPending(t *testing.T) {
	failure := embed.AgentInjectResultMsg{Err: "no agent pane in scope"}

	idle := newTestModel(true)
	updated, _ := idle.Update(failure)
	idle = updated.(Model)
	if idle.errorMsg != "" {
		t.Errorf("a browser with no pending request must ignore the result, got errorMsg %q", idle.errorMsg)
	}

	pending := newTestModel(true)
	pending.injectPending = true
	updated, _ = pending.Update(failure)
	pending = updated.(Model)
	if !strings.Contains(pending.errorMsg, "no agent pane in scope") {
		t.Errorf("errorMsg = %q, want it to carry the host's error", pending.errorMsg)
	}
	if pending.injectPending {
		t.Error("the first result must clear injectPending")
	}

	// A second, unrelated broadcast must no longer be adopted.
	pending.errorMsg = ""
	updated, _ = pending.Update(embed.AgentInjectResultMsg{Err: "someone else's problem"})
	pending = updated.(Model)
	if pending.errorMsg != "" {
		t.Errorf("a post-result broadcast must be ignored, got %q", pending.errorMsg)
	}
}

// TestInjectResultSuccessStatus pins the success wording, including the
// pluralisation and the target suffix.
func TestInjectResultSuccessStatus(t *testing.T) {
	for _, tc := range []struct {
		result embed.AgentInjectResultMsg
		want   string
	}{
		{embed.AgentInjectResultMsg{Delivered: 1, Target: "claude"}, "Injected 1 skill → claude"},
		{embed.AgentInjectResultMsg{Delivered: 2, Target: "claude"}, "Injected 2 skills → claude"},
		{embed.AgentInjectResultMsg{Delivered: 2}, "Injected 2 skills"},
	} {
		m := newTestModel(true)
		m.injectPending = true
		updated, _ := m.Update(tc.result)
		m = updated.(Model)
		if m.statusMsg != tc.want {
			t.Errorf("statusMsg = %q, want %q", m.statusMsg, tc.want)
		}
		if m.errorMsg != "" {
			t.Errorf("a successful result must not set errorMsg, got %q", m.errorMsg)
		}
	}
}

// TestInjectModeRenderNoOverflow re-runs the narrow-width layout guard with the
// mode's extra markers in play, since the selection marker and the prompt
// editor both add width the base tests never see.
func TestInjectModeRenderNoOverflow(t *testing.T) {
	for _, w := range []int{20, 40, 80} {
		m := newTestModel(true)
		m.loading = false
		updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		m = updated.(Model)
		m = enterInject(t, m)
		m.cursor = 0
		m, _ = pressInject(m, keyRune(' '))
		m, _ = pressInject(m, keyRune('p'))

		if got := maxLineWidth(m.View()); got > w {
			t.Errorf("width %d: inject View() has a line of width %d (must be <= %d)", w, got, w)
		}
		if got := maxLineWidth(m.FooterView()); got > w {
			t.Errorf("width %d: inject FooterView() has a line of width %d (must be <= %d)", w, got, w)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
